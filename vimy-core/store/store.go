package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// GameRecord is the persisted form of a single game result.
type GameRecord struct {
	Faction string `json:"faction"`
	Won     bool   `json:"won"`
}

// Data is the top-level structure persisted to disk.
type Data struct {
	Games []GameRecord `json:"games"`
}

// Store handles loading and saving vimy state to a JSON file.
type Store struct {
	path string
	data Data
}

// New creates a store that reads/writes to dir/record.json.
// If dir is empty, defaults to ~/.vimy.
func New(dir string) (*Store, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".vimy")
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	s := &Store{path: filepath.Join(dir, "record.json")}
	s.load()
	return s, nil
}

// load reads the data file. Missing file is not an error — we start fresh.
func (s *Store) load() {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("failed to read store file", "path", s.path, "error", err)
		}
		return
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		slog.Warn("failed to parse store file", "path", s.path, "error", err)
	}
}

// save writes the current data to disk.
func (s *Store) save() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o644)
}

// RecordGame appends a game result and persists to disk.
func (s *Store) RecordGame(r GameRecord) error {
	s.data.Games = append(s.data.Games, r)
	if err := s.save(); err != nil {
		slog.Error("failed to save record", "error", err)
		return err
	}
	slog.Info("game record persisted", "path", s.path, "total_games", len(s.data.Games))
	return nil
}

// Games returns a copy of all recorded games.
func (s *Store) Games() []GameRecord {
	out := make([]GameRecord, len(s.data.Games))
	copy(out, s.data.Games)
	return out
}

// Wins returns the total number of wins.
func (s *Store) Wins() int {
	n := 0
	for _, g := range s.data.Games {
		if g.Won {
			n++
		}
	}
	return n
}

// Losses returns the total number of losses.
func (s *Store) Losses() int {
	n := 0
	for _, g := range s.data.Games {
		if !g.Won {
			n++
		}
	}
	return n
}
