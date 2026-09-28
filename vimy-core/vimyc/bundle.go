package vimyc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Bundle freezes the source files used for a compilation or replay. Keys are
// basenames, so a recording is independent of its original checkout path.
type Bundle map[string]string

func ReadBundle(paths []string) (Bundle, error) {
	b := Bundle{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		b[filepath.Base(p)] = string(raw)
	}
	return b, nil
}

func (b Bundle) Materialize() ([]string, func(), error) {
	dir, err := os.MkdirTemp("", "vimyc-sources-")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	var paths []string
	for name, source := range b {
		if filepath.Base(name) != name || filepath.Ext(name) != ".vy" || name == "seed.vy" {
			cleanup()
			return nil, func() {}, fmt.Errorf("invalid doctrine source name %q", name)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			cleanup()
			return nil, func() {}, err
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		cleanup()
		return nil, func() {}, fmt.Errorf("empty source bundle")
	}
	sort.Strings(paths)
	return paths, cleanup, nil
}
