package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/nstehr/vimy/vimy-core/agent"
	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/server"
	"github.com/nstehr/vimy/vimy-core/store"
)

const banner = `
██╗   ██╗██╗███╗   ███╗██╗   ██╗
██║   ██║██║████╗ ████║╚██╗ ██╔╝
██║   ██║██║██╔████╔██║ ╚████╔╝
╚██╗ ██╔╝██║██║╚██╔╝██║  ╚██╔╝
 ╚████╔╝ ██║██║ ╚═╝ ██║   ██║
  ╚═══╝  ╚═╝╚═╝     ╚═╝   ╚═╝

Doctrine-Driven RTS Intelligence`

var (
	directive    string
	addr         string
	traceRules   bool
	rulesFile    string
	vimycBin     string
	exportStates bool
	exportDir    string
	exportEvery  int
	exportMax    int
)

// Rule tracing and state export default on. They were opt-in while nothing read
// them; Currie reads both, and a game played without them can never be replayed
// — there is no going back to record a game already lost, which is exactly the
// game worth looking at.
func main() {
	flag.StringVar(&directive, "doctrine", "", "initial doctrine directive (e.g. \"Blitzkrieg\", \"guerrilla warfare\")")
	flag.StringVar(&addr, "addr", ":8080", "HTTP dashboard listen address")
	flag.BoolVar(&traceRules, "trace-rules", true, "record per-rule firing counters per doctrine window; archives rule_firings rows on game end and exposes live counters to the dashboard. -trace-rules=false to turn it off")
	flag.StringVar(&rulesFile, "rules-file", "", "load a rule set compiled by vimyc from this file, instead of the built-in rules. Pair with no -doctrine: the strategist replaces the rule set as soon as its first doctrine lands")
	flag.StringVar(&vimycBin, "vimyc-bin", "vimyc", "the vimyc binary that compiles doctrines; found on PATH by default")
	flag.BoolVar(&exportStates, "export-states", true, "record sampled game states and rule evaluations, one file per game under -export-dir. What Currie replays and what vimyc's differential corpus is built from. -export-states=false to turn it off")
	flag.StringVar(&exportDir, "export-dir", "", "where -export-states writes; defaults to ~/.vimy/exports, alongside the database")
	flag.IntVar(&exportEvery, "export-every", 15, "with -export-states, record one evaluation in this many")
	flag.IntVar(&exportMax, "export-max", 20000, "with -export-states, stop after this many recorded cases")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	fmt.Println(banner)

	slog.Info("starting vimy", "doctrine", directive)

	// The seed rules, compiled by vimyc rather than built in Go. Identical to
	// DefaultRules down to the action function pointers, but they carry their
	// `.vy` source, so the dashboard shows the language from the first tick.
	startingRules, err := rules.SeedRules()
	if err != nil {
		slog.Error("cannot load the seed rule set", "error", err)
		os.Exit(1)
	}
	ruleSource := "seed.vy"
	if rulesFile != "" {
		data, readErr := os.ReadFile(rulesFile)
		if readErr != nil {
			err = readErr
		}
		if err != nil {
			slog.Error("cannot read the rule set", "path", rulesFile, "error", err)
			os.Exit(1)
		}
		compiled, loadErr := rules.LoadArtifact(data)
		if loadErr != nil {
			slog.Error("cannot load the rule set", "path", rulesFile, "error", loadErr)
			os.Exit(1)
		}
		startingRules, ruleSource = compiled, rulesFile
	}

	engine, err := rules.NewEngine(startingRules)
	if err != nil {
		slog.Error("failed to create rule engine", "error", err)
		os.Exit(1)
	}
	slog.Info("rule engine initialized", "rules", len(startingRules), "source", ruleSource)

	if traceRules {
		engine.SetTraceFirings(true)
	}

	// Sampled, not exhaustive: projecting a state costs roughly 60x evaluating
	// the rules. See rules/export.go.
	if exportStates {
		exporter, err := rules.NewStateExporter(exportDir, exportEvery, exportMax)
		if err != nil {
			slog.Error("cannot set up the state exporter", "error", err)
			os.Exit(1)
		}
		// A bad location found at game end loses the recording it was meant to
		// save.
		if err := exporter.Writable(); err != nil {
			slog.Error("cannot write to the export directory", "error", err)
			os.Exit(1)
		}
		engine.SetExporter(exporter)
		slog.Info("recording rule evaluations",
			"dir", exporter.Dir(), "every", exportEvery, "max", exportMax)
	}

	var strategist *agent.Strategist
	if directive != "" {
		strategist = agent.NewStrategist(engine, directive, 500)
	}

	// The only compiler there is, probed now rather than at the first doctrine
	// twenty minutes in.
	if strategist != nil {
		compiler, err := rules.NewVimycCompiler(vimycBin)
		if err != nil {
			slog.Error("cannot use vimyc", "error", err)
			os.Exit(1)
		}
		strategist.UseVimyc(compiler)
		slog.Info("compiling doctrines through vimyc", "bin", compiler.Bin)
	}

	dataStore, err := store.New("")
	if err != nil {
		slog.Error("failed to create store", "error", err)
		os.Exit(1)
	}
	slog.Info("record store loaded", "wins", dataStore.Wins(), "losses", dataStore.Losses())

	if strategist != nil {
		strategist.SetStore(dataStore)
	}

	srv := server.New(strategist, dataStore)
	go func() {
		slog.Info("starting dashboard", "addr", addr)
		if err := srv.Start(addr); err != nil {
			slog.Error("dashboard server failed", "error", err)
		}
	}()

	const socketPath = "/tmp/vimy.sock"

	// An unclean shutdown leaves the socket file behind; clear it to rebind.
	if err := os.RemoveAll(socketPath); err != nil {
		slog.Error("failed to clean up socket", "path", socketPath, "error", err)
		os.Exit(1)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		slog.Error("failed to listen on socket", "path", socketPath, "error", err)
		os.Exit(1)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	slog.Info("listening on domain socket", "path", socketPath)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					slog.Error("failed to accept connection", "error", err)
					continue
				}
			}
			slog.Info("new connection accepted")
			go handleConn(ctx, conn, engine, strategist, dataStore)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
}

func handleConn(ctx context.Context, conn net.Conn, engine *rules.Engine, strategist *agent.Strategist, dataStore *store.Store) {
	c := ipc.NewConnection(conn, nil)
	a := agent.New(c, engine, strategist, dataStore, ctx)
	c.RegisterHandler(ipc.TypeHello, a.HandleHello)
	c.RegisterHandler(ipc.TypeGameState, a.HandleGameState)
	c.RegisterHandler(ipc.TypeGameEnd, a.HandleGameEnd)
	c.ReadLoop()
}
