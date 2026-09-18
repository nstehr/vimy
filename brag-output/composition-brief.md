# Hyperframes Composition Brief: Vimy

## Objective
Create a short launch-style brag video for Vimy — an AI that plays Command & Conquer: Red Alert
(OpenRA) where an LLM writes a doctrine of weights and a compiler turns those weights into
deterministic rules that execute at game speed.

## Output
- Composition directory: `brag-output/composition/`
- Rendered video: `brag-output/brag.mp4`
- Format: landscape — 1920x1080
- Duration: 24.9 seconds (as built)

## Source Material
- Project root: `/Users/nstehr/code/vimy`
- Primary files read:
  - `README.md` — architecture, one-line pitch
  - `docs/index.md` — the long-form writeup; source of the key claim and the doctrine JSON
  - `vimy-core/server/views/layout.templ` — dashboard chrome, exact background/text colors
  - `vimy-core/server/views/components.templ` — doctrine panel, weight bars, compiled-rules panel, badges
  - `vimy-core/rules/vy/production.vy` — the real `.vy` rule language and a real rule block
  - `docs/assets/Screenshot 2026-03-26 at *.png` — the actual rendered dashboard (directive box, doctrine panel, compiled rules)
- Product name: **Vimy**
- Tagline / strongest claim (verbatim from `docs/index.md`):
  > The LLM never directly controls the game. It produces a doctrine, a set of weights.
- Key UI moments to recreate (in order): the **Directive** input card, the **Current Doctrine**
  weight-bar panel, a real `.vy` **rule block**, the **Compiled Rules** list.
- Copy that must appear verbatim:
  - `VIMY` / `Doctrine Dashboard` (nav)
  - `Directive`
  - `Naval superiority. Build up ships that can attack land first, then get sea dominance.`
  - `Update`
  - `Current Doctrine`
  - `Naval First, Mechanized Followthrough`
  - `Tick 19070`
  - `Enemy Located`
  - `Economy` `0.80` / `Infantry` `0.15` / `Vehicle` `0.55` / `Naval` `0.65`
  - `produce-heavy-vehicle`, `priority 475`, `category produce-vehicle exclusive`,
    `require has-role(war-factory)`, `require heavy-vehicle-count < army-cap(1.0, 8.0, 0.55)`
  - `68 rules`, `excl`
  - `produce-vehicle` / `produce-siege-vehicle` with their real `expr` conditions
  - `LLM-written doctrine. Deterministic execution.`

## Creative Direction
- Tone preset: `polished`
- Creative direction: *a systems film — the machine explained by showing it work, not by claiming anything*
- Interpretation: Restraint is the flex. Five scenes, holds long enough to read real code, no
  ALL CAPS, no hype adjectives. Motion is precise and mechanical (bars filling, a value dropping
  into a slot, a counter ticking), never decorative.
- Angle: Everyone builds agents by letting the model drive; Vimy does the opposite. The video
  follows one sentence of plain English all the way down to a firing rule — directive → weights →
  compiled rule → game speed — using the project's real dashboard and its real rule language.
  The brag is the architecture.
- Hook: the real Directive textarea, caret blinking, a human sentence typing itself into a war
  machine, under a quiet overline `You write one sentence.`
- Outro / punchline: `VIMY` / `LLM-written doctrine. Deterministic execution.`
- Avoid:
  - Generic SaaS language
  - Abstract filler visuals
  - Unrelated visual redesign — this must look like the Vimy dashboard, not a new brand
  - Game footage pastiche, explosions, camo textures, military clip art

## Visual Identity
From `layout.templ` (`body { background: #030712; color: #f3f4f6; }`) and the Tailwind classes in
`components.templ`.

- Background: `#030712` (gray-950); panels `#111827`, inset/code surfaces `#0f172a`/`#030712`; borders `#1f2937`
- Text: `#f3f4f6` primary, `#9ca3af` labels, `#6b7280` meta/mono-small
- Accent: `#34d399` (emerald-400) for headings, values, live state; `#10b981` (emerald-500) for filled bars and the Update button
- Secondary accent: `#fbbf24` (amber-400) — `excl` badges, numeric literals in code
- Display font: the dashboard's own system UI sans stack, bold + tight tracking for `VIMY`
- Body/data font: monospace for every number, tick, rule name, and condition string — this is the dashboard's signature; do not replace it with a sans face
- Visual references from the project:
  - Nav: `VIMY` emerald bold + `Doctrine Dashboard` gray, thin `#1f2937` bottom border
  - Weight row: gray label, full-width dark track with emerald-500 fill, mono value right-aligned
  - Badge pills: emerald text on `emerald-400/10`; amber text on `amber-400/10`
  - Rule row: mono priority in gray-500 (right-aligned, narrow), rule name in gray-100, `excl` pill, condition string beneath in small mono gray-500

## Storyboard
Use the storyboard in `brag-output/brag-plan.md` as the creative contract.

Scene summary:
1. **Directive** — 5.8s (0.0 → 5.8) — Dashboard nav + Directive card. Overline `You write one sentence.` The real directive types in character-by-character with a caret, finishes and settles, then a cursor clicks the emerald `Update` button.
2. **Doctrine** — 5.6s (5.8 → 11.4) — `Current Doctrine` panel. `Naval First, Mechanized Followthrough` lands in emerald with `Tick 19070` and an `Enemy Located` pill. Label `The model answers with weights. Not moves.` Four weight rows arrive one by one (Economy 0.80, Infantry 0.15, Vehicle 0.55, Naval 0.65) — bar fills + value counts up. `Vehicle 0.55` stays highlighted at the end.
3. **Compilation** — 4.9s (11.4 → 16.3) — The `Vehicle 0.55` row becomes a header; the `0.55` detaches and travels into a syntax-highlighted `.vy` rule block, seating into `army-cap(1.0, 8.0, 0.55)` with a one-shot emerald flash. `excl` and `68 rules` badges snap in. Label `A compiler turns weights into rules.`
4. **Execution** — 5.0s (16.3 → 21.3) — `Compiled Rules` panel; 3 real rule rows arrive one by one with their `expr` conditions; a mono stat row ticks `68 rules available · 41 fired · 1,284 matched` with `41` counting up. Label `Rules execute at game speed. The model never sees a unit.`
5. **Mark** — 3.6s (21.3 → 24.9) — Flat `#030712`. `VIMY` lands large in emerald-400; beneath it `LLM-written doctrine. Deterministic execution.` Held still; music fades under for the last ~1.2s.

## Audio
- Audio role: warm, confident bed with sparse professional accents — the sound of a system working
- Audio arc: quiet and human under the typed opening → steady and mechanical as weights resolve and compile → momentum on the firing rules → a single bell on the mark as the bed fades to near-silence
- Music: `happy-beats-business-moves-vol-11-by-ende-dot-app.mp3` (114.84 BPM)
- Music treatment: starts at 0.0 at a low posture under the hook, holds steady through the pipeline, fades out across the final ~1.2s so the closing line lands quiet.
- Music cue guidance: preset available at
  `<brag-assets>/music/cues/happy-beats-business-moves-vol-11-by-ende-dot-app.music-cues.{md,json}`
  (brag assets root for this install: `/Users/nstehr/.claude/plugins/cache/brag/brag/0.2.2/skills/brag/assets/`).
  Beat spacing ~0.525s.
  - Strong cues locked (3): **5.80s** (Update click → cut to doctrine), **12.65s** (`0.55` seats into the rule — the thesis beat), **22.65s** (VIMY wordmark lands). **17.91s** carries the first compiled-rule row.
  - Beat-grid windows for sequential reveals: doctrine weight bars at **6.86 / 7.91 / 8.96 / 10.01**; compiled-rule rows at **17.91 / 18.96 / 20.02**. Both use *every other* beat (~1.05s) because each item carries a readable label — do not reveal readable text on every beat.
- Audio-reactive treatment: subtle. Use music RMS/bass to breathe the emerald panel glow and the doctrine card's presence only. No waveform bars, equalizers, musical notes, strobing, or anything that scales text.
- Audio-coupled moments:
  - Scene 1 directive typing — randomized per-character keypress ticks, low volume
  - Scene 1 Update button — one sharp interface click at the press (~5.80s)
  - Scene 2 weight bars — one soft drop per bar at its settle, on the beat grid
  - Scene 3 `0.55` seating into `army-cap(...)` — one deeper accent, beat-locked ~12.65s
  - Scene 4 rule rows — one restrained tick per row; no sound on the counter
  - Scene 5 `VIMY` mark — a single soft bell/impact, allowed to ring over the music fade
- SFX selection guidance: sparse and motion-matched, roughly 5-7 cues across 21.5s, never
  stacked. Keyboard set for typing; a clean interface click for the button; soft drops for bars;
  a deeper interface or soft impact for the compile beat; a restrained bell for the mark.
  Nothing comedic, aggressive, or whoosh-like. If a moment is already legible, it gets no cue.
- SFX analysis guidance: read
  `/Users/nstehr/.claude/plugins/cache/brag/brag/0.2.2/skills/brag/assets/sfx/sfx-analysis.md`
  before choosing files; prefer low/medium high-frequency-risk picks — this is a polished tone
  with repeated ticks.
- Exact SFX choice: Hyperframes chooses filenames, timestamps, density, and volume once the
  animation exists.
- Audio files: copy the chosen music and every selected SFX into
  `brag-output/composition/assets/` (relative paths only — never absolute).

## Hyperframes Instructions
Load the composition-building Hyperframes domain skills — `hyperframes-core` (composition contract
+ `data-*` timing), `hyperframes-animation` (motion), `hyperframes-creative` (design spec, beats,
audio-reactive), `hyperframes-keyframes` (seek-safe keyframes), and `hyperframes-cli`
(lint/check/render). `/brag` is its own workflow: do not enter the `hyperframes` entry-point intent
interview and do not route into its generic promo / launch-video workflow. Prefer native
Hyperframes conventions over anything in `/brag`.

Requirements:
- Show real UI, copy, and code from the source project — Scenes 1-4 are all recreated Vimy dashboard.
- Keep all text readable in the final render. The `.vy` rule block in Scene 3 and the `expr`
  conditions in Scene 4 must be legible at 1080p; scale type up and trim the conditions rather
  than shrinking them below readability.
- Reading-time floor: short labels ~0.8s settled; sentence lines ~0.3s/word, min ~1.2s. The
  overline in Scene 1 and the label lines in Scenes 2-4 hold for their full scene.
- Keep the video within 15-25 seconds (built at 24.9s).
- Include the planned music and SFX layer.
- Treat `/brag` audio notes as guidance, not a fixed cue sheet. Choose SFX after the visual
  animation exists.
- Treat music cue metadata as optional timing hints. Major reveals may move toward nearby strong
  cues within ~0.15s; smaller entrances may align to nearby beats within ~0.10s. Use only 2-3
  strong cue locks. Ignore any cue that hurts readability or scene pacing.
- Mark beat work in the source: `// beat-locked: 12.65s`, `// beat-grid: bar 1 at 6.86s, ...`.
- When music is present, use the Hyperframes audio-reactive workflow (owned by
  `hyperframes-creative` — let that skill locate its own extraction helper) to wire at least one
  subtle visual to RMS/bass. If extraction is unavailable, note it and skip — do not block the render.
- Use local assets only; no absolute paths.
- Run `npx hyperframes check` before render — it is brag's single gate. Fix every error,
  including WCAG contrast failures (gray-500 `#6b7280` on `#030712` is the likely offender in the
  mono meta text — lift toward gray-400 `#9ca3af` within the palette family rather than disabling
  the contrast pass).
