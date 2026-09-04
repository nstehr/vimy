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
	vimycRules   bool
	rulesFile    string
	exportStates bool
	exportDir    string
	exportEvery  int
	exportMax    int
)

func main() {
	flag.StringVar(&directive, "doctrine", "", "initial doctrine directive (e.g. \"Blitzkrieg\", \"guerrilla warfare\")")
	flag.StringVar(&addr, "addr", ":8080", "HTTP dashboard listen address")
	flag.BoolVar(&traceRules, "trace-rules", false, "record per-rule firing counters per doctrine window; archives rule_firings rows on game end and exposes live counters to the dashboard")
	flag.BoolVar(&vimycRules, "vimyc-rules", false, "start from the vimyc-compiled rule set (rules/seed_rules.json) rather than DefaultRules; the same rules by a different route. A -doctrine swaps them out once the first doctrine lands, so use it without one to play a whole game on them")
	flag.StringVar(&rulesFile, "rules-file", "", "load a rule set compiled by vimyc from this file, instead of the built-in rules. Pair with no -doctrine: the strategist replaces the rule set as soon as its first doctrine lands")
	flag.BoolVar(&exportStates, "export-states", false, "record sampled rule evaluations for vimyc's differential corpus, one file per game under -export-dir")
	flag.StringVar(&exportDir, "export-dir", "", "where -export-states writes; defaults to ~/.vimy/exports, alongside the database")
	flag.IntVar(&exportEvery, "export-every", 5, "with -export-states, record one evaluation in this many")
	flag.IntVar(&exportMax, "export-max", 20000, "with -export-states, stop after this many recorded cases")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	fmt.Println(banner)

	slog.Info("starting vimy", "doctrine", directive)

	// Create engine and strategist at top level so the dashboard can access them
	// before a game connection arrives.
	startingRules := rules.DefaultRules()
	ruleSource := "go"
	if rulesFile != "" {
		data, err := os.ReadFile(rulesFile)
		if err != nil {
			slog.Error("cannot read the rule set", "path", rulesFile, "error", err)
			os.Exit(1)
		}
		compiled, err := rules.LoadArtifact(data)
		if err != nil {
			slog.Error("cannot load the rule set", "path", rulesFile, "error", err)
			os.Exit(1)
		}
		startingRules, ruleSource = compiled, rulesFile
	} else if vimycRules {
		compiled, err := rules.SeedRules()
		if err != nil {
			slog.Error("cannot load the vimyc rule set", "error", err)
			os.Exit(1)
		}
		startingRules, ruleSource = compiled, "vimyc"
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

	// Off by default: projecting a state costs roughly 60x what evaluating the
	// rules does, so this samples and is opt-in. See rules/export.go.
	if exportStates {
		exporter, err := rules.NewStateExporter(exportDir, exportEvery, exportMax)
		if err != nil {
			slog.Error("cannot set up the state exporter", "error", err)
			os.Exit(1)
		}
		// Checked now rather than at game end: a bad location discovered after
		// the game loses the recording it was meant to save.
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

	dataStore, err := store.New("")
	if err != nil {
		slog.Error("failed to create store", "error", err)
		os.Exit(1)
	}
	slog.Info("record store loaded", "wins", dataStore.Wins(), "losses", dataStore.Losses())

	if strategist != nil {
		strategist.SetStore(dataStore)
	}

	// Start the HTTP dashboard.
	srv := server.New(strategist, dataStore)
	go func() {
		slog.Info("starting dashboard", "addr", addr)
		if err := srv.Start(addr); err != nil {
			slog.Error("dashboard server failed", "error", err)
		}
	}()

	const socketPath = "/tmp/vimy.sock"

	// Unix sockets leave behind a file on unclean shutdown; remove it so we can rebind.
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
