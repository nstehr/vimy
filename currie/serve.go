package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nstehr/vimy/vimy-core/store"
)

//go:embed index.html.tmpl report.html.tmpl insight.html.tmpl sweep.html.tmpl
var pages embed.FS

// The web app.
//
// Point it at a Vimy state directory and it lists what is in there. Replaying a
// game costs a vimyc subprocess per doctrine window — a few seconds for a long
// game — so a result is cached until the process exits. Nothing about an
// archived game changes, so there is nothing to invalidate.
type server struct {
	dir      string
	rulesDir string
	bin      string
	store    *store.Store
	tmpl     *template.Template
	insight  insighter
	cached   *cache

	mu       sync.Mutex
	cache    map[int64]*Replay
	insights map[int64]*insightJob
	sweeps   map[int64]*sweepJob
}

// sweepJob is one game's parameter sweep, in flight or finished. Like the
// model's reading it takes longer than a page load — a few hundred vimyc runs —
// so it is started with the page and fetched when it is done.
type sweepJob struct {
	done   chan struct{}
	sweeps []Sweep
	err    error
}

// insightJob is one game's reading, in flight or finished.
//
// The replay takes half a second and the model takes half a minute, so they
// cannot share a request. The job starts when the page is asked for and the
// page polls for it; a finished one is kept, because it costs a call to make
// and nothing about an archived game changes.
type insightJob struct {
	done    chan struct{}
	insight *Insight
	err     error
}

// insighter turns a replay into prose. Nil when no model is configured, which
// is not an error — the report stands on its own.
type insighter interface {
	Read(ctx context.Context, r *Replay) (*Insight, error)
}

func newServer(dir, rulesDir, bin string, st *store.Store, ins insighter) (*server, error) {
	// A reading survives a restart; a replay is cheap enough to redo.
	stored, err := newCache(dir, rulesDir)
	if err != nil {
		return nil, err
	}
	t, terr := template.New("").Funcs(template.FuncMap{
		"pct":   func(f float64) string { return fmt.Sprintf("%.4f", f) },
		"pct1":  func(f float64) string { return fmt.Sprintf("%.0f", f*100) },
		"sub":   func(a, b float64) float64 { return a - b },
		"ticks": func(n int) string { return fmt.Sprintf("%d", n) },
		"date":  func(t time.Time) string { return t.Format("2 Jan 2006 15:04") },
	}).ParseFS(pages, "*.tmpl")
	if terr != nil {
		return nil, fmt.Errorf("templates: %w", terr)
	}
	return &server{
		dir: dir, rulesDir: rulesDir, bin: bin, store: st,
		tmpl: t, insight: ins, cached: stored,
		cache:    map[int64]*Replay{},
		insights: map[int64]*insightJob{},
		sweeps:   map[int64]*sweepJob{},
	}, nil
}

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /game/{id}", s.handleGame)
	mux.HandleFunc("GET /game/{id}/insight", s.handleInsight)
	mux.HandleFunc("GET /game/{id}/sweep", s.handleSweep)
	mux.HandleFunc("POST /link/{id}", s.handleLink)
	return mux
}

type indexView struct {
	Dir        string
	Games      []gameRow
	Unlinked   int
	HasInsight bool
	// Export files in the state directory that no game claims, offered for
	// attaching to a game recorded before the archive noted where its states
	// went.
	Loose []looseExport
	Note  string
}

type looseExport struct {
	Path  string
	Name  string
	Size  string
	Taken bool
}

type gameRow struct {
	store.ReplayableGame
	Replayable bool
	Outcome    string
	Loose      []looseExport
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	games, err := s.store.AllGames(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	claimed := map[string]bool{}
	for _, g := range games {
		if g.ExportPath != "" {
			claimed[g.ExportPath] = true
		}
	}
	loose := s.exports(claimed)

	v := indexView{Dir: s.dir, HasInsight: s.insight != nil, Loose: loose, Note: r.URL.Query().Get("note")}
	for _, g := range games {
		row := gameRow{ReplayableGame: g, Replayable: g.ExportPath != "", Outcome: "loss"}
		if g.Won {
			row.Outcome = "win"
		}
		if !row.Replayable {
			v.Unlinked++
			row.Loose = loose
		}
		v.Games = append(v.Games, row)
	}
	s.render(w, "index.html.tmpl", v)
}

// exports lists the state exports sitting in the directory, newest first,
// marking the ones a game already claims.
//
// Offered rather than guessed: an export's filename is the minute it was
// written, which is close to a game's end but not derivable from it, and a
// wrong pairing produces a confident report about the wrong game.
func (s *server) exports(claimed map[string]bool) []looseExport {
	entries, err := os.ReadDir(filepath.Join(s.dir, "exports"))
	if err != nil {
		return nil
	}
	var out []looseExport
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || (!strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".json.gz")) {
			continue
		}
		path := filepath.Join(s.dir, "exports", name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, looseExport{
			Path:  path,
			Name:  name,
			Size:  fmt.Sprintf("%.1f MB", float64(info.Size())/(1<<20)),
			Taken: claimed[path],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out
}

// handleLink attaches an export to a game recorded before the archive noted
// where its states went.
func (s *server) handleLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "not a game id", http.StatusBadRequest)
		return
	}
	path := r.FormValue("export")
	if path == "" {
		http.Redirect(w, r, "/?note=pick+an+export+first", http.StatusSeeOther)
		return
	}
	if _, err := os.Stat(path); err != nil {
		http.Redirect(w, r, "/?note=that+file+is+not+readable", http.StatusSeeOther)
		return
	}
	if err := s.store.LinkExport(r.Context(), id, path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// A replay of this game is now possible and any cached one is stale.
	s.mu.Lock()
	delete(s.cache, id)
	s.mu.Unlock()
	http.Redirect(w, r, fmt.Sprintf("/game/%d", id), http.StatusSeeOther)
}

func (s *server) handleGame(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "not a game id", http.StatusBadRequest)
		return
	}

	rep, err := s.replay(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	v := buildWith(fmt.Sprintf("game %d · %s vs %s · %s", rep.Game.ID, rep.Game.OurFaction,
		rep.Game.OpponentFaction, outcome(rep.Game.Won)), rep.Report, rep.Windows_, rep.Firings, rep.Game.DurationTicks)
	v.Windows = rep.Windows
	v.Orphaned = rep.Orphaned
	v.Approximate = rep.Approximate
	v.Home = "/"

	// Started here, rendered later. The report is what the reader came for and
	// it is ready now; the prose arrives when it arrives.
	if s.insight != nil {
		s.startInsight(id, rep)
		v.InsightURL = fmt.Sprintf("/game/%d/insight", id)
	}
	if knobs := sweepKnobs(v.Sensitivity, firstParams(rep)); len(knobs) > 0 {
		s.startSweep(id, rep, knobs)
		v.SweepURL = fmt.Sprintf("/game/%d/sweep", id)
	}
	s.render(w, "report.html.tmpl", v)
}

func firstParams(rep *Replay) map[string]float64 {
	if len(rep.Windows_) == 0 {
		return nil
	}
	return rep.Windows_[0].Params
}

// startSweep replays the game with each implicated input overridden, once.
func (s *server) startSweep(id int64, rep *Replay, knobs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sweeps[id]; ok {
		return
	}
	job := &sweepJob{done: make(chan struct{})}
	s.sweeps[id] = job
	go func() {
		defer close(job.done)
		started := time.Now()
		run := func(params map[string]float64, i int) (report, error) {
			return blameStates(params, rep.Windows_[i].Raw, s.rulesDir, s.bin)
		}
		job.sweeps, job.err = sweep(knobs, rep.Windows_, run)
		if job.err != nil {
			slog.Warn("sweep failed", "game", id, "error", job.err)
			return
		}
		slog.Info("sweep ready", "game", id, "knobs", knobs, "took", time.Since(started))
	}()
}

func (s *server) handleSweep(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "not a game id", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	job := s.sweeps[id]
	s.mu.Unlock()

	v := sweepView{URL: fmt.Sprintf("/game/%d/sweep", id)}
	if job == nil {
		v.Error = "nothing running for this game"
	} else {
		select {
		case <-job.done:
			v.Sweeps, v.Error = job.sweeps, errText(job.err)
		default:
			v.Pending = true
		}
	}
	s.render(w, "sweep.html.tmpl", v)
}

type sweepView struct {
	URL     string
	Pending bool
	Sweeps  []Sweep
	Error   string
}

// startInsight kicks off the model in the background, once per game.
func (s *server) startInsight(id int64, rep *Replay) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.insights[id]; ok {
		return
	}
	job := &insightJob{done: make(chan struct{})}
	s.insights[id] = job

	// Already read, under these same rules: nothing to call for.
	if prior := s.cached.read(id); prior != nil {
		job.insight = prior
		close(job.done)
		slog.Info("insight from cache", "game", id)
		return
	}

	go func() {
		defer close(job.done)
		// Its own context: the request that started this is long gone by the
		// time the model answers, and cancelling then would waste the call.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		started := time.Now()
		job.insight, job.err = s.insight.Read(ctx, rep)
		if job.err != nil {
			slog.Warn("insight failed", "game", id, "error", job.err, "took", time.Since(started))
			return
		}
		s.cached.write(id, job.insight)
		slog.Info("insight ready", "game", id, "took", time.Since(started))
	}()
}

// handleInsight serves the prose when it is ready, and a placeholder that asks
// again when it is not.
func (s *server) handleInsight(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "not a game id", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	job := s.insights[id]
	s.mu.Unlock()

	v := insightView{URL: fmt.Sprintf("/game/%d/insight", id)}
	switch {
	case job == nil:
		v.Error = "nothing running for this game"
	default:
		select {
		case <-job.done:
			v.Insight, v.Error = job.insight, errText(job.err)
		default:
			v.Pending = true
		}
	}
	s.render(w, "insight.html.tmpl", v)
}

// insightView is the fragment's own model, so the placeholder knows where to
// ask again.
type insightView struct {
	URL     string
	Pending bool
	Insight *Insight
	Error   string
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *server) replay(ctx context.Context, id int64) (*Replay, error) {
	s.mu.Lock()
	if r, ok := s.cache[id]; ok {
		s.mu.Unlock()
		return r, nil
	}
	s.mu.Unlock()

	games, err := s.store.ReplayableGames(ctx)
	if err != nil {
		return nil, err
	}
	var game *store.ReplayableGame
	for i := range games {
		if games[i].ID == id {
			game = &games[i]
			break
		}
	}
	if game == nil {
		return nil, fmt.Errorf("game %d has no export recorded; link one with `currie link --game %d --export <file>`", id, id)
	}

	started := time.Now()
	rep, err := replayGame(ctx, s.store, *game, s.rulesDir, s.bin)
	if err != nil {
		return nil, err
	}
	slog.Info("replayed", "game", id, "windows", rep.Windows,
		"states", rep.Matched, "orphaned", rep.Orphaned, "took", time.Since(started))

	s.mu.Lock()
	s.cache[id] = rep
	s.mu.Unlock()
	return rep, nil
}

// Close releases the reading cache. The archive belongs to the caller.
func (s *server) Close() error { return s.cached.Close() }

func (s *server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("render", "template", name, "error", err)
	}
}

func outcome(won bool) string {
	if won {
		return "win"
	}
	return "loss"
}
