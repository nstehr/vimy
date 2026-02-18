package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/nstehr/vimy/vimy-core/agent"
	"github.com/nstehr/vimy/vimy-core/ipc"
)

const banner = `
██╗   ██╗██╗███╗   ███╗██╗   ██╗
██║   ██║██║████╗ ████║╚██╗ ██╔╝
██║   ██║██║██╔████╔██║ ╚████╔╝
╚██╗ ██╔╝██║██║╚██╔╝██║  ╚██╔╝
 ╚████╔╝ ██║██║ ╚═╝ ██║   ██║
  ╚═══╝  ╚═╝╚═╝     ╚═╝   ╚═╝

Doctrine-Driven RTS Intelligence`

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	fmt.Println(banner)

	slog.Info("starting vimy")

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
			go handleConn(conn)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
}

func handleConn(conn net.Conn) {
	c := ipc.NewConnection(conn, nil)
	a := agent.New(c)
	c.RegisterHandler(ipc.TypeHello, a.HandleHello())
	c.RegisterHandler(ipc.TypeGameState, a.HandleGameState())
	c.ReadLoop()
}
