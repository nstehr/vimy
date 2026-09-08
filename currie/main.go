// Currie is a web app over a Vimy archive.
//
// Point it at a state directory and it lists the games recorded there. Pick one
// and it replays it against the rule sets that ran, window by window, and shows
// what stopped each rule from firing. See README.md.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

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
	bin := flag.String("vimyc", "vimyc", "the vimyc binary")
	addr := flag.String("addr", ":8090", "listen address")
	flag.Parse()

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

	srv, err := newServer(expand(*dir), *rulesDir, *bin, st, ins)
	if err != nil {
		return err
	}
	defer srv.Close()

	slog.Info("currie listening", "addr", *addr, "dir", expand(*dir))
	return http.ListenAndServe(*addr, srv.routes())
}
