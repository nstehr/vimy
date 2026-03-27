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
	directive string
	addr      string
)

func main() {
	flag.StringVar(&directive, "doctrine", "", "initial doctrine directive (e.g. \"Blitzkrieg\", \"guerrilla warfare\")")
	flag.StringVar(&addr, "addr", ":8080", "HTTP dashboard listen address")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	fmt.Println(banner)

	slog.Info("starting vimy", "doctrine", directive)

	// Create engine and strategist at top level so the dashboard can access them
	// before a game connection arrives.
	engine, err := rules.NewEngine(rules.DefaultRules())
	if err != nil {
		slog.Error("failed to create rule engine", "error", err)
		os.Exit(1)
	}
	slog.Info("rule engine initialized", "rules", len(rules.DefaultRules()))

	var strategist *agent.Strategist
	if directive != "" {
		strategist = agent.NewStrategist(engine, directive, 500)
	}

	// Start the HTTP dashboard.
	srv := server.New(strategist)
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
			go handleConn(ctx, conn, engine, strategist)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
}

func handleConn(ctx context.Context, conn net.Conn, engine *rules.Engine, strategist *agent.Strategist) {
	c := ipc.NewConnection(conn, nil)
	a := agent.New(c, engine, strategist, ctx)
	c.RegisterHandler(ipc.TypeHello, a.HandleHello)
	c.RegisterHandler(ipc.TypeGameState, a.HandleGameState)
	c.ReadLoop()
}
