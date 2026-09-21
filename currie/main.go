// Currie is a web app over a Vimy archive.
//
// Point it at a state directory and it lists the games recorded there. Pick one
// and it replays it against the rule sets that ran, window by window, and shows
// what stopped each rule from firing. See README.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

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
		"ship the sidecar's write-ahead evaluation log into ClickHouse and exit when done, or keep shipping with -ship-every")
	shipEvery := flag.Duration("ship-every", 0, "with -ship, keep shipping on this interval instead of making one pass")
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

	// A whole job, not a mode of the server: it runs and exits before the
	// archive is opened, because it does not need it.
	if *ship {
		sh := stream.New(walDir, *chURL, *chDB, *chUser, *chPass)
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := sh.Ping(ctx); err != nil {
			return fmt.Errorf("clickhouse: %w (is the stream schema loaded? see clickhouse/sql/04_stream.sql)", err)
		}
		slog.Info("shipping", "wal", walDir, "clickhouse", *chURL, "every", *shipEvery)
		return sh.Run(ctx, *shipEvery)
	}

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

	srv, err := newServer(expand(*dir), *rulesDir, expand(*engineRules), *bin, st, ins)
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

	slog.Info("currie listening", "addr", *addr, "dir", expand(*dir))
	return http.ListenAndServe(*addr, srv.routes())
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
