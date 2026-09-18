# Brag Plan: Vimy

## What is this app?
Vimy is an AI that plays Command & Conquer: Red Alert (OpenRA) by having an LLM write a
high-level *doctrine* — a set of weights — which a compiler turns into deterministic rules
that execute at game speed. The LLM never issues a single unit order.

## The angle
Everyone builds agents by letting the model drive. Vimy does the opposite and wins with it:
the model writes the *strategy*, a compiler writes the *rules*, and only the rules touch the
game. The video follows one sentence of plain English all the way down to a firing rule —
directive → weights → compiled rule → game speed. The brag is the architecture, shown as a
pipeline, using the project's real dashboard and its real rule language.

The strongest line the project already owns, verbatim from the docs:

> The LLM never directly controls the game. It produces a doctrine, a set of weights.

## Hook (first 2-3 seconds)
The real Directive box from the Vimy dashboard, on the near-black dashboard background,
with a caret blinking. A human sentence types itself into a war machine:
**"Naval superiority. Build up ships that can attack land first, then get sea dominance."**
Overline, small and quiet: `You write one sentence.` The hook is the unsettling gap between
casual English and what it is about to become.

## Key moments (the middle)
- **The doctrine panel resolves.** `Naval First, Mechanized Followthrough` lands in emerald,
  and the real weight bars fill one at a time: Economy 0.80, Infantry 0.15, Vehicle 0.55,
  Naval 0.65. The model's entire output is numbers between 0 and 1.
- **`0.55` becomes a rule.** The vehicle weight lifts out of its bar and drops into a real
  `.vy` rule block from `vimy-core/rules/vy/production.vy`, syntax-highlighted in the
  dashboard's own scheme. `68 rules` badge snaps in. A weight became a hard condition.
- **The rules fire.** The compiled-rules panel with real rule names and `expr` conditions
  (`produce-heavy-vehicle`, `produce-siege-vehicle`) and a fired counter ticking up — the
  deterministic half doing the work, at game speed, with no model in the loop.

## Outro / punchline
The pipeline collapses into one line, then the mark:

```
VIMY
LLM-written doctrine. Deterministic execution.
```

## User flow worth showing
Yes — the dashboard is the working app and the flow is exactly three beats:
1. **Entry** — operator types a plain-English directive into the Directive box and hits Update.
2. **Key action** — the LLM returns a named doctrine + weights; the compiler compiles them into a rule set.
3. **Result** — compiled rules listed with their real conditions, firing counts climbing.

The centerpiece scenes (1, 2, 3) are this flow, recreated from
`vimy-core/server/views/{layout,components}.templ` and the docs screenshots. No marketing slide
substitutes for it.

## Tone
- Preset: `polished`
- Creative direction: *a systems film — the machine explained by showing it work, not by claiming anything*
- Interpretation: Restraint is the flex. Few scenes, long enough holds to read real code, no
  ALL CAPS, no hype adjectives. Motion is precise and mechanical (bars filling, a value
  dropping into place, a counter ticking) rather than decorative. The project is genuinely
  impressive, so the video never has to say so.

## Format: landscape — 1920x1080
## Duration: 24.9 seconds (as built)

## Visual identity (from the project)
Pulled from `vimy-core/server/views/layout.templ` (`body { background: #030712; color: #f3f4f6 }`)
and the Tailwind classes used throughout `components.templ`.

- Background: `#030712` (gray-950), panels `#111827`/`#0f172a` with `#1f2937` borders
- Accent: `#34d399` (emerald-400) for headings, values, live state; `#10b981` (emerald-500) for filled bars and the Update button
- Secondary accent: `#fbbf24` (amber-400) — the `excl` badge, warnings
- Text: `#f3f4f6` primary, `#9ca3af` (gray-400) labels, `#6b7280` (gray-500) meta
- Display font: system UI sans (Tailwind default stack) — the dashboard's own
- Body/data font: monospace (`font-mono`) for every number, tick, rule name and condition — this is the dashboard's signature
- Strongest visual element: the Current Doctrine panel — emerald weight bars with mono values right-aligned at 0.80 / 0.15 / 0.55 / 0.65

## Share copy (draft)
Built Vimy: an LLM plays Red Alert without ever touching a unit. It writes a doctrine — just
weights — and a compiler turns those weights into deterministic rules that run at game speed.

## Audio direction
- Role: warm, confident bed with sparse professional accents — the sound of a system working
- Music: `happy-beats-business-moves-vol-11-by-ende-dot-app.mp3` (114.84 BPM, clean pulse, strong cues well spaced across the whole 0-25s window)
- Music treatment: start at 0.0 under the hook at a low posture, hold steady through the pipeline, and fade the last ~1.2s under the VIMY mark so the final line lands in near-silence.
- Music cue guidance: preset read from `assets/music/cues/happy-beats-business-moves-vol-11-by-ende-dot-app.music-cues.md`. Tempo 114.84 BPM, beat spacing ~0.525s.
  - Strong cues locked (3): **5.80s** (Update clicked → cut to doctrine), **12.65s** (the `0.55` lands in the compiled rule), **22.65s** (the VIMY wordmark lands). **17.91s** also carries the first compiled-rule row.
  - Beat-grid windows for sequential reveals: doctrine weight bars across **6.86 / 7.91 / 8.96 / 10.01** — every *other* beat (~1.05s apart) because each bar carries a readable label and value. Compiled-rule rows across **17.91 / 18.96 / 20.02**, same every-other-beat spacing.
  - Restraint note: polished tone — 2-3 strong locks maximum, and readability outranks every cue. Drop any snap that rushes a line.
- Audio-reactive treatment: subtle. Let music RMS breathe the emerald panel glow and the doctrine card's presence slightly. No waveform bars, no equalizers, no strobing, nothing that moves text.
- SFX posture: sparse and motion-matched. Roughly 5-7 cues in 21.5s, never stacked.
- Audio-coupled moments:
  - Directive typing — per-character key ticks, randomized across the keypress set, quiet.
  - Update button click — one sharp interface click at the cut.
  - Weight bars — one soft drop per bar as it settles, on the beat grid.
  - `0.55` landing in the rule block — one deeper accent; this is the thesis of the video.
  - VIMY mark — a single restrained bell/impact, allowed to ring as the music fades.
- Restraint rule: no sound on every animation. Nothing comedic, nothing aggressive, no
  rising whooshes. If a moment is already legible, it gets no cue.

## Storyboard

### Scene 1 — Directive — 5.8s (0.0 → 5.8)
The Vimy dashboard chrome: thin top nav with `VIMY` in emerald-400 bold and `Doctrine Dashboard`
in gray beside it, on `#030712`. Below, the real Directive panel — rounded `#111827` card, border
`#1f2937`, heading **Directive**, and a large dark textarea.

Small gray overline above the card, held for the full scene: `You write one sentence.`

The textarea types out, character by character with a caret, the project's real directive:
`Naval superiority. Build up ships that can attack land first, then get sea dominance.`
(~14 words — needs ~4.2s of typing; it must finish typing and sit fully settled before the click.)
Then the emerald **Update** button at bottom-right gets a cursor click and a brief pressed state.

Sequential/interaction: yes — per-character typing with a caret, then a simulated cursor click on Update.
Audio intent: quiet, focused, human. The only sound is a person typing.
Audio-coupled idea: randomized keypress ticks per character, low volume; one interface click on the button press at ~5.80s.
Music: low, steady bed already running.
Transition mood: clean → Scene 2

### Scene 2 — Doctrine — 5.6s (5.8 → 11.4)
Hard-ish clean cut to the **Current Doctrine** panel. The doctrine name lands first in emerald-400
at large size: `Naval First, Mechanized Followthrough`, with `Tick 19070` in mono gray at top-right
and a small emerald `Enemy Located` pill beneath it.

Gray label line: `The model answers with weights. Not moves.`

Four weight rows then fill one at a time — label left in gray-300, emerald-500 bar filling from
left, mono value right-aligned:
- `Economy` → 0.80
- `Infantry` → 0.15
- `Vehicle` → 0.55
- `Naval` → 0.65

Each bar animates its width to the value and the number counts up to it. Each row holds ~0.8s+
fully settled. The `Vehicle 0.55` row stays highlighted at the end of the scene — it is the one
that carries into Scene 3.

Sequential/interaction: yes — 4 weight rows arrive one by one on every other beat (6.86 / 7.91 / 8.96 / 10.01), each with a bar fill + counting value.
Audio intent: mechanical certainty; a system resolving.
Audio-coupled idea: one soft drop per bar settling, exactly on the visual landing.
Transition mood: clean → Scene 3

### Scene 3 — Compilation — 4.9s (11.4 → 16.3)
The `Vehicle 0.55` row is the only thing left on screen; it slides up to act as a header. The
`0.55` value detaches and travels down into a code panel that fades in below — a real `.vy` rule
block in the dashboard's syntax-highlight scheme (emerald keywords, amber numbers, gray idents):

```
rule produce-heavy-vehicle {
  priority 475
  category produce-vehicle exclusive
  do produce-heavy-vehicle
  require vehicle-weight > 0.1
  require has-role(war-factory)
  require heavy-vehicle-count < army-cap(1.0, 8.0, 0.55)
}
```

The `0.55` arrives into the `army-cap(...)` slot on a strong cue and the line flashes emerald once.
An amber `excl` badge and an emerald `68 rules` badge snap in at the panel's top-right.

Gray label line, held: `A compiler turns weights into rules.`

Sequential/interaction: yes — the `0.55` value physically travels from the weight bar into the rule's argument slot; badges snap in after.
Audio intent: the click of a mechanism engaging — this is the thesis beat.
Audio-coupled idea: one deeper interface/impact accent at the moment `0.55` seats into the rule (beat-lock 12.65s); a small tick per badge.
Transition mood: clean → Scene 4

### Scene 4 — Execution — 5.0s (16.3 → 21.3)
Cut to the **Compiled Rules** panel as seen in the real dashboard: category header
`produce_vehicle` with a count pill, then rule rows arriving one by one — mono priority number
in gray on the left, rule name in gray-100, amber `excl` badge, and the real condition string
beneath in small mono gray:

- `480  produce-vehicle  excl` — `HasRole("war_factory") && !QueueBusy("Vehicle") && CombatVehicleCount() < 5 && Cash() >= 800`
- `475  produce-heavy-vehicle  excl` — `HasRole("tech_center") && CanBuildRole("heavy_tank") && Cash() >= 1200`
- `460  produce-siege-vehicle  excl` — `HasRole("radar") && CanBuildRole("artillery") && Cash() >= 900`

A small mono stat row underneath ticks live: `68 rules available · 41 fired · 1,284 matched`,
with `41` counting up in emerald.

Gray label line: `Rules execute at game speed. The model never sees a unit.`

Sequential/interaction: yes — 3 rule rows arrive on every other beat (18.44 / 19.49 / 20.54); the fired counter ticks up continuously.
Audio intent: momentum without hype — the deterministic half doing the work.
Audio-coupled idea: one restrained tick per rule row; no sound on the counter.
Transition mood: soft → Scene 5

### Scene 5 — Mark — 3.6s (21.3 → 24.9; last ~1.4s fading to near-silence)
Everything clears to flat `#030712`. `VIMY` lands large in emerald-400 with the dashboard's
bold tight tracking. Beneath it, in gray-400 at small size:
`LLM-written doctrine. Deterministic execution.`
Held still. Music fades under. Nothing else moves.

Sequential/interaction: none.
Audio intent: settle and land. One clean accent, then let the room go quiet.
Audio-coupled idea: a single soft bell/impact on the VIMY landing, allowed to ring out over the music fade.
Transition mood: hold to end

**Total: 5.8 + 5.6 + 4.9 + 5.0 + 3.6 = 24.9s**

**Music mood for this video:** clean, confident, forward-moving — a working system, not a hype reel.
**Audio summary:** A quiet typed opening over a low bed, the bed steadying as weights resolve and compile with sparse mechanical accents, then a single bell on the VIMY mark as the music fades to near-silence.
