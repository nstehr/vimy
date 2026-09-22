// Currie is a web app over a Vimy archive.
//
// Point it at a state directory and it lists the games recorded there. Pick one
// and it replays it against the rule sets that ran, window by window, and shows
// what stopped each rule from firing. See README.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/nstehr/vimy/currie/ch"
	"github.com/nstehr/vimy/currie/stream"
	"github.com/nstehr/vimy/vimy-core/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "currie:", err)
		os.Exit(1)
	}
}

func run() error {
	dir := flag.String("dir", "~/.vimy", "the Vimy state directory")
	rulesDir := flag.String("rules", "../vimy-core/rules/vy", "directory of .vy rule sources")
	engineRules := flag.String("engine-rules", "../engine/mods/ra/rules",
		"the engine's rule yaml, for pricing what a game spent")
	bin := flag.String("vimyc", "vimyc", "the vimyc binary")
	addr := flag.String("addr", ":8090", "listen address")
	game := flag.String("game", "", "print one game's post mortem to stdout and exit: an id, or \"latest\"")
	top := flag.Int("top", 8, "how many blamed rules to list with -game")

	ship := flag.Bool("ship", false,
		"ship the write-ahead log into ClickHouse and nothing else: no server, no archive. The server ships on its own, so this is for a headless shipper or a one-off catch-up with -ship-every 0")
	shipEvery := flag.Duration("ship-every", 5*time.Second,
		"how often to ship. Zero with -ship makes a single pass and exits")
	noShip := flag.Bool("no-ship", false,
		"do not ship in the background. The live page then shows only what some other shipper has moved")
	streamDir := flag.String("stream-dir", "", "the write-ahead log directory; defaults to <dir>/stream")
	chURL := flag.String("clickhouse", "http://localhost:8123", "ClickHouse HTTP endpoint")
	chDB := flag.String("clickhouse-db", "currie", "ClickHouse database")
	chUser := flag.String("clickhouse-user", "currie", "ClickHouse user")
	chPass := flag.String("clickhouse-password", "currie", "ClickHouse password")
	flag.Parse()

	walDir := *streamDir
	if walDir == "" {
		walDir = filepath.Join(expand(*dir), "stream")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Shipping alone: no server, no archive opened, because it needs neither.
	// Still its own mode -- a headless box that only moves rows, and the
	// one-off catch-up `make ship` runs -- but no longer the only way to get
	// rows into ClickHouse, which is what made running a game a two-terminal
	// affair.
	if *ship {
		sh := stream.New(walDir, *chURL, *chDB, *chUser, *chPass)
		if err := sh.Ping(ctx); err != nil {
			return fmt.Errorf("clickhouse: %w (is the stream schema loaded? see clickhouse/sql/04_stream.sql)", err)
		}
		slog.Info("shipping", "wal", walDir, "clickhouse", *chURL, "every", *shipEvery)
		return sh.Run(ctx, *shipEvery)
	}

	// Optional, like the model and for the same reason: the report's product is
	// the blame analysis, which needs nothing but the archive. A ClickHouse
	// that is absent, unreachable or missing 04_stream.sql costs the counted
	// sections and the live page, and is a warning rather than a failure.
	chc := ch.New(*chURL, *chDB, *chUser, *chPass)
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	if err := chc.Ping(pingCtx); err != nil {
		// Also turns off background shipping, since there is nowhere to ship
		// to. Nothing is lost by that: the write-ahead log stays on disk and a
		// later `currie -ship -ship-every 0` moves every segment it missed.
		slog.Warn("no ClickHouse: the counted sections, the live page and background shipping are all off",
			"clickhouse", *chURL, "error", err,
			"fix", "make -C clickhouse up && make -C clickhouse stream, then restart currie")
		chc = nil
	}
	cancelPing()

	st, err := store.New(expand(*dir))
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer st.Close()

	// A model is optional. Without one the report is unchanged minus the prose,
	// which is the right way round: the numbers are the product.
	ins, err := newInsighter()
	if err != nil {
		slog.Warn("insight disabled", "error", err)
	}
	if ins == nil {
		slog.Info("insight disabled: no OPENAI_API_KEY or ANTHROPIC_API_KEY")
	}

	srv, err := newServer(expand(*dir), *rulesDir, expand(*engineRules), *bin, st, ins, chc)
	if err != nil {
		return err
	}
	defer srv.Close()

	// One game to a terminal, no server. The same replay and the same prices as
	// the page; only the rendering differs.
	if *game != "" {
		ctx := context.Background()
		id, err := resolveGame(ctx, st, *game)
		if err != nil {
			return err
		}
		return srv.postmortem(ctx, os.Stdout, id, *top)
	}

	// The shipper, in the background, for the life of the server.
	//
	// It used to be a second process someone had to remember, and forgetting it
	// did not look like an error: the pages rendered, the live panel just said
	// the last session it had, which is indistinguishable from a quiet game.
	// A goroutine because the two halves share nothing -- the shipper reads
	// sealed files and POSTs them, and touches neither the archive nor the
	// replay cache.
	if chc != nil && !*noShip {
		sh := stream.New(walDir, *chURL, *chDB, *chUser, *chPass)
		go func() {
			slog.Info("shipping in the background", "wal", walDir, "every", *shipEvery)
			// Run only returns here when the context is cancelled: with an
			// interval no pass is fatal, so a ClickHouse that restarts costs
			// a log line and not the rest of the game.
			if err := sh.Run(ctx, *shipEvery); err != nil {
				slog.Error("shipper stopped", "error", err)
			}
		}()
	} else if chc != nil {
		slog.Info("not shipping", "reason", "-no-ship")
	}

	// A real server rather than ListenAndServe, so ^C stops the shipper too and
	// the open segment is not left half-moved.
	httpSrv := &http.Server{Addr: *addr, Handler: srv.routes()}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdown); err != nil {
			slog.Warn("shutdown", "error", err)
		}
	}()

	slog.Info("currie listening", "addr", *addr, "dir", expand(*dir))
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// resolveGame turns the -game value into an id.
//
// "latest" is the newest game that recorded an export, not the newest game:
// one without an export cannot be replayed at all, and failing on it would be
// a worse answer than reporting the most recent game there is something to say
// about.
func resolveGame(ctx context.Context, st *store.Store, arg string) (int64, error) {
	if arg != "latest" {
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("-game: %q is not an id or \"latest\"", arg)
		}
		return id, nil
	}
	games, err := st.ReplayableGames(ctx)
	if err != nil {
		return 0, err
	}
	if len(games) == 0 {
		return 0, fmt.Errorf("-game latest: no game has recorded an export")
	}
	return games[0].ID, nil
}
