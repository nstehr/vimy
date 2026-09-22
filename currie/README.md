# Currie

Named for Arthur Currie, who planned Vimy Ridge.

Relies on game state archives and the vimyc compiler to provide insights into
previous games. It can make suggestions, and provide insight on whether it's the
core doctrines that impacted the game or the rules themselves.

## What a game report shows

Four things, in the order a reader needs them:

- **What the engine recorded** — the trade in credits, buildings destroyed,
  peak army against the enemy's, and where the losses happened. The only
  numbers in the archive that are not Vimy's own opinion of events: where
  Vimy's counting disagreed with these, Vimy was wrong.
- **What the money bought** — each rule's act count times the engine's own
  price for what it builds. An estimate, and labelled as one. Blame analysis
  cannot see any of this, because nothing was blocking these rules.
- **What the compiler said** — vimyc's warnings, counted per doctrine window.
  A rule that never enters an exclusive contest has no blocking clause to
  report and so is invisible in the blame; only the compiler finds it.
- **What stopped each rule** — the blame, window by window, against the rule
  set each doctrine actually compiled to.

Prices come from `-engine-rules` (default `../engine/mods/ra/rules`), parsed
rather than transcribed so they track the mod. Which rule buys what lives in
`rule-items.json`.

## The counted half

The four analyses above are computed from the state export, which records one
evaluation in fifteen. Every row carries the world it was judged against —
that is what makes the blame possible — and none of it can be counted.
`build-war-factory` fires exactly once per game, in 56 of 69 archived games,
and appears in the export zero times.

So the report also reads the streamed tables, which are unsampled: one row per
evaluation, written to the sidecar's write-ahead log while the game ran and
shipped by `currie -ship`. Three sections come from there, and they are kept
apart from the blame on the page rather than merged into it — adding a counted
number to a sampled one produces a figure that is neither.

- **What fired, and what never did.** Two tables, not one ranking: a rule that
  never fired and a rule that fired constantly are both answers to "what did
  this game do" and they do not rank against each other. Ordering them
  together buried every firing rule under fifty silent ones.
- **Does the stream agree with the counters.** `rule_firings` in SQLite counts
  the same firings by incrementing an integer in the rule loop. Two routes to
  one measurement, so the stream has an oracle, and the check is a badge on the
  page rather than a `make` target someone remembers to run. Scoped to the
  first rule-set swap onward, which is the only window the counters can see —
  the opening is reported separately.
- **Has anything moved.** Every game grouped by `rules_digest`, the fingerprint
  of the compiled rule set, with the standard error of each group's mean.
  Squad spread sat near 20 cells against a required 8 through four separate
  fixes, each judged by eye against the next game or two, against a metric
  whose game-to-game noise is larger than the effect being chased.

All of it is optional. No ClickHouse, a server that is down, a game played
before `-stream` — each costs these sections and nothing else. The blame
analysis is the product and it needs only the archive.

`ch/` is the transport: HTTP, server-side parameters, a generic decode, and
`readonly=2`. `clickhouse.go` holds the queries. They are strings, so the
compiler checks none of them —

```sh
make test-ch     # runs every query against a real server
```

is what catches a typo, and it is worth running after touching any SQL. Of the
four bugs found writing this, three were invisible to `go test` and to
reading: an alias that shadowed the column it was computed from, a monotonic
function ClickHouse moved *inside* an aggregate so a condition stopped being a
condition, and a `WITH` alias the analyzer would not resolve.

## While the game runs

```sh
currie                           # or: make run
open http://localhost:8090/live
```

One process. The server ships the sidecar's write-ahead log into ClickHouse on
a background goroutine for as long as it runs, because the two halves share
nothing — the shipper reads sealed files and POSTs them, and touches neither
the archive nor the replay cache.

It was two processes, and forgetting the second one did not look like an
error: the pages still rendered and the live panel simply showed the last
session it had, which is indistinguishable from a quiet game.

`-no-ship` turns it off, for a machine where something else is already
shipping. `currie -ship` is still the shipper on its own — a headless box that
only moves rows, or `make -C clickhouse ship` for a one-off catch-up of
segments written while nothing was running. Running both is safe: the
deduplication token is the segment's path, so a segment shipped twice lands
once.

Nothing is lost if ClickHouse is down. The log stays on disk and the next
shipper to run moves every segment it missed.

Vimy's own dashboard shows what the strategist *intends* — the directive, the
doctrine compiled right now, the weights over time. `/live` is what the squads
actually did: rallies, strikes that did not happen, and the squad's distance
from its target as a fraction of the map diagonal, plotted rather than reduced
to three bucket counts. It polls on the shipper's interval; asking faster only
re-renders the same rows.

It is keyed on the **session**, not the game. A game in progress has no id —
SQLite hands that out when the retrospective archives — so anything that joined
through `game_id` would show an empty page for every running game.

It lives here rather than in `vimy-core/server`, which already serves the
doctrine dashboard. The sidecar must not grow a reason to talk to a database:
that constraint is the whole point of the write-ahead log, and a panel is not
worth relaxing it for.

## Without a browser

The same four analyses print to stdout, for the moment after a game ends:

```
currie -game latest        the newest game that recorded an export
currie -game 127           a particular one
currie -game 127 -top 40   more of the blame than the default eight
```

One code path, two renderings: `-game` replays through the same `replayGame`
and prices through the same `computeSpend` as the page, so the two cannot
disagree. This replaced a Python script that took the same shape and got the
blame wrong — it blamed every sampled state against **one** of the game's
doctrines, and game 127 ran 121 of them. Pairing each state to the doctrine
that was actually running is the point of the replay, so a second tool that
skipped it was not a convenience; it was a wrong answer that looked right.
