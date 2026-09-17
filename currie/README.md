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
