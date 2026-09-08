package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nstehr/vimy/currie/store/db"
)

func rulesDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "core.vy"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCacheReturnsWhatItStored(t *testing.T) {
	c, err := newCache(t.TempDir(), rulesDir(t, "rule r {}"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.write(77, &Insight{Summary: "the economy starved", Suggestion: "lower the reserve"})

	got := c.read(77)
	if got == nil {
		t.Fatal("nothing came back")
	}
	if got.Summary != "the economy starved" {
		t.Errorf("summary = %q", got.Summary)
	}
}

// The reading is about numbers produced by a particular rule set. Change the
// sources and those numbers change, so the old prose no longer describes
// anything and must not be served.
func TestCacheMissesAfterTheRulesChange(t *testing.T) {
	state := t.TempDir()
	dir := rulesDir(t, "rule r {}")

	before, err := newCache(state, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer before.Close()
	before.write(77, &Insight{Summary: "written against the old rules"})

	if err := os.WriteFile(filepath.Join(dir, "core.vy"), []byte("rule r { require cash >= 1 }"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := newCache(state, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Close()
	if got := after.read(77); got != nil {
		t.Errorf("served a reading written against different rules: %q", got.Summary)
	}
	// And the old entry is still there for the old rules, not clobbered.
	if got := before.read(77); got == nil {
		t.Error("the entry for the previous rules was lost")
	}
}

func TestCacheMissesForAnotherGame(t *testing.T) {
	c, err := newCache(t.TempDir(), rulesDir(t, "rule r {}"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.write(77, &Insight{Summary: "game 77"})
	if got := c.read(76); got != nil {
		t.Errorf("game 76 got game 77's reading: %q", got.Summary)
	}
}

// A fingerprint that ignored file names would collide when content moved
// between files without changing in total.
func TestFingerprintNoticesContentMovingBetweenFiles(t *testing.T) {
	a := t.TempDir()
	os.WriteFile(filepath.Join(a, "core.vy"), []byte("rule x {}"), 0o600)
	os.WriteFile(filepath.Join(a, "economy.vy"), []byte("rule y {}"), 0o600)

	b := t.TempDir()
	os.WriteFile(filepath.Join(b, "core.vy"), []byte("rule y {}"), 0o600)
	os.WriteFile(filepath.Join(b, "economy.vy"), []byte("rule x {}"), 0o600)

	fa, err := fingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := fingerprint(b)
	if err != nil {
		t.Fatal(err)
	}
	if fa == fb {
		t.Error("same fingerprint for the same rules in different files")
	}
}

func TestCacheDiscardsACorruptEntry(t *testing.T) {
	c, err := newCache(t.TempDir(), rulesDir(t, "rule r {}"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.queries.PutInsight(context.Background(), db.PutInsightParams{
		GameID: 77, Rules: c.rules, InsightJson: "{not json",
	}); err != nil {
		t.Fatal(err)
	}

	if got := c.read(77); got != nil {
		t.Error("returned something from a corrupt row")
	}
	if got, _ := c.Readings(context.Background()); len(got) != 0 {
		t.Error("the corrupt row was left behind to fail again next time")
	}
}

// The reading is queryable, which is the point of a table rather than a
// directory of files: the cross-game work reads it this way.
func TestCacheListsReadingsForTheCurrentRules(t *testing.T) {
	state := t.TempDir()
	dir := rulesDir(t, "rule r {}")
	c, err := newCache(state, dir)
	if err != nil {
		t.Fatal(err)
	}
	c.write(76, &Insight{Summary: "seventy-six"})
	c.write(77, &Insight{Summary: "seventy-seven"})

	got, err := c.Readings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[77] != "seventy-seven" {
		t.Errorf("readings = %v", got)
	}
	c.Close()

	// Under different rules the same store is empty, not wrong.
	if err := os.WriteFile(filepath.Join(dir, "core.vy"), []byte("rule r { require cash >= 1 }"), 0o600); err != nil {
		t.Fatal(err)
	}
	other, err := newCache(state, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if got, _ := other.Readings(context.Background()); len(got) != 0 {
		t.Errorf("readings under changed rules = %v, want none", got)
	}
}
