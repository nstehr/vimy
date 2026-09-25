// Package wal is the contract between the sidecar and whatever ships its
// telemetry onward.
//
// The sidecar cannot afford to talk to a database from the rule loop: a rule
// evaluates in ~17us, and one bad network write would stall the game. So it
// appends rows to local files and something else moves them -- Currie's
// `stream` package, today. This package holds the half both sides must agree
// on, in vimy-core because the writer lives here and a wire format duplicated
// across two modules drifts.
//
// # Layout
//
//	<stream-dir>/<session>/session.json            written at game start
//	<stream-dir>/<session>/evals-000001.jsonl      sealed, ready to ship
//	<stream-dir>/<session>/events-000001.jsonl     a second stream, same rules
//	<stream-dir>/<session>/evals-000002.jsonl.partial   still being written
//
// Two streams, not one table with a kind column, because they have different
// shapes and wildly different rates: evaluations arrive ~136/s and events a
// few hundred a game.
//
// A segment is sealed by renaming away the .partial suffix, which is atomic on
// a local filesystem. The shipper never opens a .partial file, so a writer and
// a shipper need no lock between them and a crash costs at most the open
// segment.
//
// # Why JSONEachRow
//
// Fatter than TSV, and chosen anyway: it is self-describing, so a field added
// to the sidecar's rows loads into an older table as a default instead of
// silently shifting every column. The whole point of moving this data to
// ClickHouse was to stop paying a migration for each new question.
package wal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SegmentSuffix marks a segment that is still being appended to. The sidecar
// writes <name>.jsonl.partial and renames to <name>.jsonl to seal it.
const (
	PartialSuffix = ".partial"
	SegmentExt    = ".jsonl"
	SessionFile   = "session.json"

	// EvalsPrefix carries one row per rule evaluation; EventsPrefix carries
	// the sparse, structured things worth a row of their own -- a rally, a
	// strike that did not happen. A reader keys off the prefix to know which
	// table a segment belongs in.
	EvalsPrefix  = "evals-"
	EventsPrefix = "events-"

	// ThreatPrefix carries the AI's own danger map: the field that decides
	// whether an approach detours or drives straight in. Sparse - only zones
	// carrying threat - because most of the map is empty and an empty one is
	// the interesting case, not a gap.
	ThreatPrefix = "threat-"

	// UnitsPrefix carries one row per unit per sampled state: where everything
	// on the field was, ours and theirs. Every diagnosis this telemetry has
	// supported so far has been made from scalars - spread, members, a distance
	// ratio - and read wrong more than once. Positions are what those scalars
	// are summaries of.
	UnitsPrefix = "units-"

	// CompressedExt marks a sealed segment that has already been shipped and
	// then compressed in place. These files are still segments and still
	// shippable -- the archive stays re-playable if the database is ever lost
	// -- so they keep the SAME logical name and therefore the same
	// deduplication token. Compressing a segment must not make ClickHouse
	// think it is new.
	CompressedExt = ".gz"
)

// Session is what the sidecar knows at game start, plus what it learns at the
// end. GameID is zero until the retrospective archives the game and can say
// which row in SQLite this session became -- the id is an autoincrement handed
// out at archive time, so nothing during the game can know it.
type Session struct {
	ID        string    `json:"session_id"`
	StartedAt time.Time `json:"started_at"`
	GameID    int64     `json:"game_id,omitempty"`

	// What ran. RulesDigest fingerprints the compiled rule set for a fixed
	// reference doctrine, so it moves when the `.vy` sources or vimyc's
	// codegen move and stays put when the LLM merely picks different weights
	// -- which is the distinction "did my fix work" depends on. Revision and
	// Modified say which sidecar build, and whether it had uncommitted edits.
	RulesDigest string `json:"rules_digest,omitempty"`
	Revision    string `json:"revision,omitempty"`
	Modified    bool   `json:"modified,omitempty"`

	// The coarse terrain grid, one character per zone, row-major: '.' land,
	// '~' water, '#' cliff, '=' bridge. The sidecar has had this since it
	// gained terrain awareness and nothing has ever drawn it, so every map of
	// a game has been units floating on a blank square.
	//
	// It goes on the session rather than into a stream because it is static for
	// the whole game: 1024 characters once, not 1024 rows per sample.
	TerrainCols  int    `json:"terrain_cols,omitempty"`
	TerrainRows  int    `json:"terrain_rows,omitempty"`
	TerrainCellW int    `json:"terrain_cell_w,omitempty"`
	TerrainCellH int    `json:"terrain_cell_h,omitempty"`
	Terrain      string `json:"terrain,omitempty"`

	// Filled in by Finish. A session whose RowsDropped is non-zero has holes,
	// and any count taken from it is a floor rather than a number.
	RowsWritten uint64 `json:"rows_written,omitempty"`
	RowsDropped uint64 `json:"rows_dropped,omitempty"`
}

// Stream is which stream a segment belongs to, taken from its name.
func (s Segment) Stream() string { return StreamOf(s.Name) }

// Row is one rule's evaluation. The shipper never unmarshals these -- it hands
// the bytes to ClickHouse -- but the type is the written contract for what a
// line contains, and the tests round-trip through it.
//
// StateIdx is -1 when the evaluation was not sampled for projection. Projecting
// costs roughly 60x evaluating, so the sidecar streams every evaluation and
// projects only some of them: counting is exact, and the state-conditioned
// questions stay honest about being a sample.
type Row struct {
	Tick     int    `json:"tick"`
	Rule     string `json:"rule"`
	RuleSet  string `json:"rule_set"`
	StateIdx int    `json:"state"`
	Fired    bool   `json:"fired"`
	Skipped  bool   `json:"skipped"`
}

// Unit is one actor at one sampled tick.
//
// Deliberately flat and deliberately repeated per tick rather than diffed: the
// rows compress to roughly two bytes each once ClickHouse sorts them by tick,
// because a unit that has not moved costs nothing to store twice, and a diff
// format would trade that for the ability to lose a base state.
//
// Side rather than an owner name so the table is useful without knowing which
// faction was which that game.
type Unit struct {
	Tick int    `json:"tick"`
	ID   int    `json:"unit_id"`
	Type string `json:"type"`
	Side string `json:"side"` // "ours" or "enemy"
	X    int    `json:"x"`
	Y    int    `json:"y"`
	HP   int    `json:"hp"`
	Idle bool   `json:"idle"`
	// Building separates what holds ground from what moves over it. Both are
	// actors on the field and a map without bases on it cannot be read.
	Building bool `json:"is_building"`
	// Remembered marks intel rather than sight: where the AI believes something
	// is, not where it can currently see one. GameState.Enemies carries only
	// what is visible this instant, which is a small minority of samples, while
	// targeting and the threat field run off memory. Drawing only the former
	// gives an empty enemy half of the map during a battle.
	Remembered bool `json:"remembered"`
}

// Threat is one zone of the danger map at one sampled tick.
//
// This is the field BestApproachAxis scores corridors against, so it is the
// difference between "the front door was genuinely the best way in" and "we
// had seen nothing, so every corridor scored zero and the detour switched
// itself off". The latter cost an evening and a squad walked into flame towers
// it had never sighted. Drawn, it is obvious at a glance.
type Threat struct {
	Tick  int     `json:"tick"`
	Col   int     `json:"col"`
	Row   int     `json:"row"`
	Value float64 `json:"value"`
}

// Segment is a sealed file waiting to be shipped.
type Segment struct {
	Session string // session id, i.e. the directory name
	// Name is the LOGICAL name, always without .gz: evals-000001.jsonl. It is
	// what Token is built from, so a segment keeps its identity when it is
	// compressed after shipping.
	Name string
	// Path is the file as it exists, which may carry .gz.
	Path string
	Size int64
	// Compressed says whether Path needs decompressing to read.
	Compressed bool
}

// Token is the stable identity of a segment, used as ClickHouse's
// insert_deduplication_token. A retry after a crash between the INSERT landing
// and the ledger being written carries the same token, and ClickHouse drops
// the duplicate block rather than doubling the rows.
func (s Segment) Token() string { return s.Session + "/" + s.Name }

// SegmentName builds the file name for the nth segment of a stream.
func SegmentName(prefix string, seq int) string {
	return fmt.Sprintf("%s%06d%s", prefix, seq, SegmentExt)
}

// StreamOf names the stream a segment belongs to, or "" if the file is not a
// segment at all.
func StreamOf(name string) string {
	name = strings.TrimSuffix(name, CompressedExt)
	for _, p := range []string{EvalsPrefix, EventsPrefix, UnitsPrefix, ThreatPrefix} {
		if strings.HasPrefix(name, p) && strings.HasSuffix(name, SegmentExt) {
			return p
		}
	}
	return ""
}

// ReadSession loads a session's metadata. A session directory without a
// session.json is not an error -- the sidecar may have died before writing one
// -- and yields a Session with only the id filled in, so its rows still ship.
func ReadSession(dir string) (Session, error) {
	s := Session{ID: filepath.Base(dir)}
	b, err := os.ReadFile(filepath.Join(dir, SessionFile))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("%s: %w", filepath.Join(dir, SessionFile), err)
	}
	if s.ID == "" {
		s.ID = filepath.Base(dir)
	}
	return s, nil
}

// WriteSession records the session metadata, replacing what is there. Used by
// the seeder, and by whatever eventually stamps GameID onto a finished game.
func WriteSession(dir string, s Session) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, SessionFile), append(b, '\n'), 0o644)
}

// Sessions lists the session directories under a stream directory. A missing
// stream directory yields none rather than an error: the shipper may well
// start before the first game does.
func Sessions(streamDir string) ([]string, error) {
	entries, err := os.ReadDir(streamDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join(streamDir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

// SealedSegments lists a session's shippable segments in write order.
// Anything still carrying .partial is skipped: it is the file the sidecar is
// appending to right now.
func SealedSegments(sessionDir string) ([]Segment, error) {
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return nil, err
	}
	session := filepath.Base(sessionDir)
	var out []Segment
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasSuffix(name, PartialSuffix) {
			continue
		}
		if StreamOf(name) == "" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		gz := strings.HasSuffix(name, CompressedExt)
		out = append(out, Segment{
			Session:    session,
			Name:       strings.TrimSuffix(name, CompressedExt),
			Path:       filepath.Join(sessionDir, name),
			Size:       info.Size(),
			Compressed: gz,
		})
	}
	// Lexical order is write order within a stream: the sequence is
	// zero-padded. Across streams the interleaving does not matter -- they
	// land in different tables.
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Event is one sparse, structured thing worth its own row: a rally, a strike
// that did not happen. Counters for these already exist in the archive -- and
// that is the problem. `strike_blocked_no_target` had to be split in two
// because one number was hiding two causes with opposite fixes, and the split
// could not be applied backwards. A row per occurrence cannot hide anything,
// and Attrs means the next thing worth recording is not a migration.
type Event struct {
	Tick   int    `json:"tick"`
	Kind   string `json:"kind"`             // "strike-blocked", "rally"
	Squad  string `json:"squad"`            // the squad it happened to
	Reason string `json:"reason,omitempty"` // strike-blocked: which blocker

	// Squad shape. Members and Spread mean the same thing for every kind that
	// sets them: the whole squad, and the distance from the centroid to the
	// furthest member.
	//
	// Idle and Near do NOT. Idle is the subset an ORDER CAN REACH (rally) --
	// the roster minus whoever is retreating or held, which is who the rally
	// order is actually sent to. It is NOT a count of idle units and never was:
	// squadAssaultActorIDs passes onlyIdle=false. The name is historical and
	// kept only because renaming the field would orphan every row already
	// shipped and every segment still on disk.
	// Near is the subset WITHIN squadRallyRadius OF THE CENTRE, written by
	// transit and, since game 154, by rally too — where it is the clump gate's
	// own numerator and the only field that says whether the gate can pass.
	// Both are "a subset of members" and they are different measurements, so
	// they get different fields -- one column meaning two things is what
	// forced strike_blocked_no_target to be split, and that split could not be
	// applied backwards.
	//
	// Each kind fills a subset of these. Reading Idle on a transit row gives
	// 0, which is why every query here filters on kind first.
	Members int `json:"members"`
	Idle    int `json:"idle"`
	Near    int `json:"near"`
	Spread  int `json:"spread"`

	// Attrs carries what does not deserve a column. For transit that is
	// target_fraction: the squad's distance from its target as a fraction of
	// the map diagonal, which the in-memory sampler throws away by bucketing
	// into three bands and summing. Keeping the number means the bands become
	// a choice made at query time -- which is the only way to ask whether
	// moving a threshold moved anything.
	Attrs map[string]float64 `json:"attrs,omitempty"`
}
