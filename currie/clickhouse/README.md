# ClickHouse for Currie

A derived view of `~/.vimy`. Nothing in this directory is a source of truth:
the SQLite archive and the `.json.gz` exports are mounted **read-only**, and
`make reset && make load` rebuilds everything from them.

## Why

`vimy-core/store/migrations/` is fifteen migrations, and most of them say the
same thing in different words: a question came up, the archive could not answer
it, so a column was added and every game played before that point answers NULL.
`0006` could not say what the losses bought. `0007` could say how many died and
never where. `0012` split a counter because one value was hiding two causes with
opposite fixes — and games 1–131 can never be re-asked.

The tables here hold no aggregates. `rule_evals` is one row per rule per
evaluation; `game_states` is the sampled world those rules were evaluated
against, as maps rather than columns. Every number Currie reports is a SELECT
over those rows. A new question is a new query, and it runs over every game
already loaded.

## Two ways in

```sh
make local     # clickhouse-local: no server, no ingest, no state.
               # Reads ~/.vimy/exports/*.json.gz and vimy.db in place.

make up        # the server, at http://localhost:8123/play (currie / currie)
make load      # ingest. Re-runnable; truncates and reloads. ~4s for 70 games.
make currie    # the Currie-shaped queries
make query Q="SELECT ..."
make shell     # interactive clickhouse-client
make reset     # drop the data volume. The archive is untouched.
```

Start with `make local`. It needs nothing but Docker, and if those queries do
not feel better than `replayGame`, the server is not worth setting up.

## What loads

| table | rows (70 exports) | from |
|---|---|---|
| `rule_evals` | 1,232,433 | `cases[]` in each export |
| `game_states` | 42,469 | `states[]` in each export |
| `doctrines` | 8,938 | `archived_doctrines` in SQLite |
| `games` | 69 | `games` in SQLite, where `export_path` is set |

**These rows are a sample, not a census.** `-export-states` exists to feed
vimyc's differential corpus: it records 1 evaluation in 15 and stops at 20,000
cases, and 37 of the 69 exports sit at that cap, covering a mean 85% of their
game's ticks. Checked against `rule_firings` in SQLite over the same games the
undercount is nowhere near uniform -- `repair-buildings` 27x, `produce-vehicle`
37x, `build-power` 25x -- so neither counts nor rates off `rule_evals` mean
anything absolute. `build-war-factory` fires exactly once per game, in 56 of
these 69 games, and appears in the export 0 times.

Use `rule_evals` for state-conditioned questions, where every row carries the
world it was judged against. Use SQLite's `rule_firings` to count. If ClickHouse
is to be the analytics store rather than a view of a test corpus, the engine
needs an unsampled event sink -- that is the next piece of work, not a query.

There is no ETL script. ClickHouse reads the gzipped JSON with `file()` and the
SQLite file with `sqlite()`, so the whole pipeline is four `INSERT … SELECT`s in
`sql/02_load.sql`.

`games` is 69 against 70 export files: one export on disk has no `games` row
pointing at it, so the `INNER JOIN` drops it. That is the join doing its job,
not a loss — but it is worth knowing which one, some time.

## The bit that matters

Currie's core job is *"pairing each state to the doctrine that was actually
running"* — the thing the Python script skipped, which is why it produced a
wrong answer that looked right. That is an `ASOF JOIN`:

```sql
FROM currie.rule_evals AS e
ASOF INNER JOIN currie.doctrines AS d
    ON e.game_id = d.game_id AND e.tick >= d.tick
```

Game 147 ran 25 rule sets across 24 doctrines; `replayGame` exists to walk them
in order. Here the pairing is two lines and runs across all 69 games at once.
Note the equality on `game_id`: `ASOF` requires at least one equi-join column,
and it is also what keeps the pairing from leaking between games.

## Gotchas already paid for

- **The entrypoint chowns `user_files` recursively** and dies on a read-only
  bind mount. `CLICKHOUSE_DO_NOT_CHOWN=1` skips it. `CLICKHOUSE_RUN_AS_ROOT=1`
  also skips it and then fails differently, because the image's data directory
  belongs to the `clickhouse` user and the server refuses the mismatch.
- **`ReplacingMergeTree` does not deduplicate on read.** It collapses duplicates
  when parts merge, which is whenever the server feels like it, or never. An
  earlier version of `02_load.sql` truncated only the event tables; `games`
  quietly held two rows per game, and because the export loads `INNER JOIN`
  against it, every evaluation landed twice — a clean 2x with nothing in the
  output to suggest it. Read Replacing tables with `FINAL` or dedupe explicitly.
- **`ASOF JOIN` needs an equi-join column.** `ON e.tick >= d.tick` alone is a
  syntax error.

## Streaming

Everything above is a view of files written after a game ends. The streamed
half is unsampled and arrives while the game runs.

The sidecar cannot talk to a database from the rule loop -- a rule evaluates in
~17us and one slow write stalls the game -- so it appends rows to a local
write-ahead log and Currie ships them. `stream/wal.go` is the contract between
the halves: a segment is sealed by renaming away a `.partial` suffix, which is
atomic, so writer and shipper need no lock and a crash costs at most the open
segment.

```sh
make stream    # create the stream tables
make ship      # one pass
make ship-loop # keep shipping every 5s, which is what you want during a game
```

**Exactly-once, without a distributed transaction.** The data INSERT and the
ledger INSERT cannot be atomic together, so a crash between them leaves a
segment in the table and not in the ledger, and the next pass resends it. The
resend carries the same `insert_deduplication_token` -- the segment's path --
and ClickHouse drops the repeated block. Verified: truncating `stream_segments`
mid-flight and re-shipping leaves 18,130 rows, not 36,260.

**`game_id` does not exist during a game.** It is SQLite's autoincrement, handed
out when the retrospective archives. So the sidecar streams under a session id
and `stream_sessions` carries the mapping, rewritten on every pass because the
id arrives late. Every cross-game query joins through it.

**The sidecar has a sink.** `vimy-core -stream` opens a `wal.Log` per game and
writes two streams: `evals-*.jsonl` (one row per rule evaluation) and
`events-*.jsonl` (one row per rally, one per blocked strike, one per transit
sample). The rule loop
never waits on it -- rows go to a buffered channel and are *dropped* when it is
full, because a stalled game is worse than an incomplete log. Drops are counted
into `session.json` and land in `stream_sessions.rows_dropped`, so an analysis
can see it is reading a floor rather than a number.

Both streams are live. The evaluation stream is **unsampled**: every rule, every
evaluation, including the ones an exclusive winner skipped. That is what makes
`stream_rule_evals` countable where `rule_evals` is not -- `build-war-factory`
fires exactly once per game, in 56 of 69 games, and the 1-in-15 export catches
it in none of them.

`state_idx` is `-1` when no projection happened, which is most rows. Projecting
costs ~60x evaluating, so it stays sampled: counting is exact while the
state-conditioned questions stay honestly a sample, and `state_idx >= 0` keeps
the two apart.

Cost, measured (`go test ./rules/ -bench Evaluate`): 41.5us per evaluation of
115 rules without streaming, 62.0us with the real log attached -- about 0.18us
per row. At the archive's measured rate of ~1.8 evaluation cycles per second
that is ~36us of extra CPU per second of game.

**`transit` is the one with most to gain.** The in-memory sampler computes the
squad's distance from its target as a fraction of the map diagonal and then
throws the number away, bucketing into three bands and summing: game 148's
118,920 ticks reduce to 76 far / 5 mid / 0 near. That cannot say *when* the
squad stopped closing, and it bakes the band boundaries into storage, so a
threshold change cannot be evaluated against anything played before it. The
event keeps `attrs['target_fraction']`, and `sql/05_tuning.sql` queries 5 and 6
re-band at query time.

One trap worth knowing: `members` and `spread` mean the same thing for every
event kind, but `idle` and `near` do not. `idle` is the subset an order can
reach (rally); `near` is the subset within `squadRallyRadius` of the centre
(transit). Different measurements, so different columns -- sharing one is what
forced `strike_blocked_no_target` to be split, and that split could not be
applied backwards. Every query filters on `kind` first.

`make verify` cross-checks the stream against SQLite's `rule_firings` for a
game. Both measure the same thing by different routes, and keeping both means
the stream has an oracle. Exact agreement is expected; drift means dropped rows
or two definitions that have come apart.

**Every session records what played the game.** `rules_digest` fingerprints the
compiled rule set for a *fixed* doctrine, so it moves when the `.vy` sources or
vimyc's codegen move and stays put when the LLM merely picks different weights.
`revision` is the sidecar build, and `modified` flags a dirty tree -- a
before/after that spans one of those is not an answer. `make tuning` is the
query this exists for.

**Nothing sampled can reach these tables, by construction.** The only writer is
the shipper, the only producer of segments is `wal.Log`, and the sidecar feeds
it one row per occurrence. There is no `sampled` flag to remember to filter on,
because there is nothing to filter -- a WAL seeder that could have replayed the
1-in-15 exports into this table was deleted rather than made safe. Historical
exports load into `rule_evals` via `make load`, where they are labelled a sample
and belong. If a sampled producer is ever added -- thinning the stream on a
marathon game, say -- that is when the flag goes in, with something to test it
against.

## Reading back out

For a while nothing did. The shipper moved rows in and every question these
tables answered was answered by a human running a `make` target, while the
report page computed its blame from the sampled export — the table this README
says cannot be counted.

`../ch` is the read path: an HTTP client, server-side `{name:Type}` parameters
and a generic decode, about a hundred lines. `../clickhouse.go` holds the
queries the report asks, `../live.go` the ones the live page asks. The `make`
targets here are unchanged and still the right tool for exploring; what is new
is that three sections of the report are now SELECTs over these rows.

Set `readonly=2` on every read. It permits SELECT and per-query settings and
refuses everything that writes, which costs nothing today and has to already be
in place before a model is choosing the SQL.

### Gotchas paid for writing it

- **Never alias a result column to the name of a source column.** ClickHouse
  resolves the alias ahead of the column it shadows, *everywhere* in the query,
  including inside other aggregates. `countIf(fired) AS fired` makes the next
  `countIf(fired)` count a UInt64 and the query fails with `Illegal type Int64
  of last argument for aggregate function with If suffix` — which does not
  name the alias. `count() AS fired` in a query that also says `WHERE fired`
  fails with `Aggregate function count() is found in WHERE`.
- **A monotonic function gets moved inside min/max.** `toInt64(minIf(tick,
  fired))` becomes `minIf(toInt64(tick), toInt64(fired))` and the condition
  stops being a condition. Cast the column, not the aggregate — or do not
  cast: a UInt32 needs none.
- **The analyzer will not resolve a `WITH` alias from the SELECT list** of the
  same query. Two round trips read better anyway.
- **`countIf` returns UInt64, and UInt64 subtraction wraps.** The drift between
  the stream and SQLite's counters is a signed quantity; without
  `toInt64(...) - toInt64(...)` a counter ahead by one reports
  18446744073709551615 and reads as catastrophe.

None of these is visible to the Go compiler, and none was caught by review.
`make -C .. test-ch` runs every query against a real server and is what found
all four.

## Files

```
docker-compose.yml     server + clickhouse-local, both read-only on ~/.vimy
sql/01_schema.sql      runs once on first boot (docker-entrypoint-initdb.d)
sql/02_load.sql        ingest; re-runnable
sql/03_currie.sql      the ASOF pairing, conjunct-level blame, per-opponent
sql/explore.sql        the same ideas with no server at all
sql/04_stream.sql      the streamed tables, fed by ../stream
sql/05_tuning.sql      did that change move the number
../ch/                 the read path: HTTP, parameters, decode
../clickhouse.go       the report's queries
../live.go             the live page's queries
../stream/ship.go      the shipper
../../vimy-core/wal/   the contract, and the sidecar's writer
```
- **A successful `SELECT 1` does not mean the server is up.** The entrypoint
  boots a temporary server to run `docker-entrypoint-initdb.d`, then stops it
  and execs the real one. `make up` waits for three consecutive successes.
