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
	"time"

	"github.com/nstehr/vimy/currie/ch"
	"github.com/nstehr/vimy/vimy-core/store"
)

//go:embed index.html.tmpl report.html.tmpl insight.html.tmpl sweep.html.tmpl live.html.tmpl field.html.tmpl investigation.html.tmpl
var pages embed.FS

// server adapts HTTP requests to the archive and analysis service.
type server struct {
	dir string
	// The engine's own rules, for pricing what a game spent. Read rather than
	// transcribed, so the numbers track the mod.
	engineRules string
	// The streamed half, unsampled. Nil when no server is reachable: every
	// section it feeds is then a section the page skips, the same way a missing
	// model costs the prose and nothing else.
	ch      *ch.Client
	store   *store.Store
	tmpl    *template.Template
	insight insighter

	analysis *analysisService
}

// insighter turns a replay into prose. Nil when no model is configured; the
// report stands on its own.
type insighter interface {
	Read(ctx context.Context, r *Replay) (*Insight, error)
}

// parseTemplates builds the page set.
//
// Separate from newServer so a test can render every template without an
// archive behind it: a mistyped field in a template is a runtime error on a
// page nobody looks at until it is being looked at.
func parseTemplates() (*template.Template, error) {
	t, err := template.New("").Funcs(template.FuncMap{
		"pct":   func(f float64) string { return fmt.Sprintf("%.4f", f) },
		"pct1":  func(f float64) string { return fmt.Sprintf("%.0f", f*100) },
		"sub":   func(a, b float64) float64 { return a - b },
		"ticks": func(n int) string { return fmt.Sprintf("%d", n) },
		"date":  func(t time.Time) string { return t.Format("2 Jan 2006 15:04") },
		"clock": func(t time.Time) string { return t.Format("15:04") },
	}).ParseFS(pages, "*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("templates: %w", err)
	}
	return t, nil
}

func newServer(dir, rulesDir, engineRules, bin string, st *store.Store, ins insighter, chc *ch.Client) (*server, error) {
	// A reading survives a restart; a replay is cheap enough to redo.
	stored, err := newCache(dir, rulesDir)
	if err != nil {
		return nil, err
	}
	t, terr := parseTemplates()
	if terr != nil {
		stored.Close()
		return nil, terr
	}
	return &server{
		dir: dir, engineRules: engineRules, store: st,
		tmpl: t, insight: ins, ch: chc,
		analysis: &analysisService{rulesDir: rulesDir, engineRules: engineRules, bin: bin, store: st, ch: chc, insight: ins, cached: stored, runner: newJobRunner(2, 64)},
	}, nil
}

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /game/{id}", s.handleGame)
	mux.HandleFunc("GET /game/{id}/insight", s.handleInsight)
	mux.HandleFunc("GET /game/{id}/sweep", s.handleSweep)
	mux.HandleFunc("POST /game/{id}/investigate", s.handleInvestigate)
	mux.HandleFunc("GET /game/{id}/investigation", s.handleInvestigation)
	mux.HandleFunc("POST /link/{id}", s.handleLink)
	// The game as it is played. A page of its own rather than a section of the
	// report: the report is about a game that finished, and this one has no id
	// to hang off yet.
	mux.HandleFunc("GET /live", s.handleLive)
	mux.HandleFunc("GET /live/panel", s.handleLivePanel)
	mux.HandleFunc("GET /field/{session}", s.handleField)
	mux.HandleFunc("GET /field/{session}/frame", s.handleFieldFrame)
	return mux
}

type indexView struct {
	Dir string
	// Whether to offer the live page. Hidden rather than shown-and-broken when
	// there is no server to read.
	HasStream  bool
	Games      []gameRow
	Unlinked   int
	HasInsight bool
	// Unclaimed exports, offered for attaching to games recorded before the
	// archive tracked export paths.
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

	v := indexView{Dir: s.dir, HasInsight: s.insight != nil, HasStream: s.ch != nil, Loose: loose, Note: r.URL.Query().Get("note")}
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

// exports lists the state exports in the directory, newest first, marking those
// a game already claims.
//
// Offered rather than guessed: the filename is the minute it was written, close
// to a game's end but not derivable from it, and a wrong pairing yields a
// confident report about the wrong game.
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
	// Analysis keys include the linked export contents; no stale entry is reused.
	http.Redirect(w, r, fmt.Sprintf("/game/%d", id), http.StatusSeeOther)
}

func (s *server) handleGame(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "not a game id", http.StatusBadRequest)
		return
	}

	rep, err := s.analysis.replay(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	v := s.analysis.gameView(r.Context(), rep)
	v.Home = "/"

	// The report is what the reader came for and it is ready now; the prose
	// arrives when it arrives.
	if s.insight != nil {
		s.analysis.startInsight(id, rep)
		v.InsightURL = fmt.Sprintf("/game/%d/insight", id)
		v.InvestigateID = id
	}
	if knobs := sweepKnobs(v.Sensitivity, firstParams(rep)); len(knobs) > 0 {
		s.analysis.startSweep(id, rep, knobs)
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

func (s *server) handleSweep(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "not a game id", http.StatusBadRequest)
		return
	}
	job := s.analysis.sweeps.get(id)

	v := sweepView{URL: fmt.Sprintf("/game/%d/sweep", id)}
	if job == nil {
		v.Error = "nothing running for this game"
	} else {
		select {
		case <-job.done:
			v.Sweeps, v.Error = job.value, errText(job.err)
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

type investigationView struct {
	URL     string
	GameID  int64
	Started bool
	Pending bool
	Insight *Insight
	Trace   []TraceStep
	Error   string
}

// handleInvestigate starts one. A POST because it spends money.
func (s *server) handleInvestigate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "not a game id", http.StatusBadRequest)
		return
	}
	if s.insight == nil {
		s.render(w, "investigation", investigationView{GameID: id, Error: "no model configured"})
		return
	}
	rep, err := s.analysis.replay(r.Context(), id)
	if err != nil {
		s.render(w, "investigation", investigationView{GameID: id, Error: errText(err)})
		return
	}
	s.analysis.startInvestigation(id, rep)
	s.renderInvestigation(w, id)
}

// handleInvestigation serves the fragment while it runs and when it lands.
func (s *server) handleInvestigation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "not a game id", http.StatusBadRequest)
		return
	}
	s.renderInvestigation(w, id)
}

func (s *server) renderInvestigation(w http.ResponseWriter, id int64) {
	job := s.analysis.investigations.get(id)

	v := investigationView{URL: fmt.Sprintf("/game/%d/investigation", id), GameID: id}
	if job == nil {
		s.render(w, "investigation", v)
		return
	}
	v.Started = true
	select {
	case <-job.done:
		v.Insight, v.Trace, v.Error = job.value.Insight, job.value.Trace, errText(job.err)
	default:
		v.Pending = true
	}
	s.render(w, "investigation", v)
}

// handleInsight serves the prose when it is ready, and a placeholder that asks
// again when it is not.
func (s *server) handleInsight(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "not a game id", http.StatusBadRequest)
		return
	}
	job := s.analysis.insights.get(id)

	v := insightView{URL: fmt.Sprintf("/game/%d/insight", id)}
	switch {
	case job == nil:
		v.Error = "nothing running for this game"
	default:
		select {
		case <-job.done:
			v.Insight, v.Error = job.value, errText(job.err)
		default:
			v.Pending = true
		}
	}
	s.render(w, "insight.html.tmpl", v)
}

// insightView carries the poll URL so the placeholder knows where to ask again.
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

// Close cancels and joins analysis jobs before closing their cache. The archive belongs to the caller.
func (s *server) Close() error { return s.analysis.Close() }

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
