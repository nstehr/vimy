// Post-game retrospective. Runs once at game end, reads the doctrine
// trajectory + events + outcome, and produces a structured review that
// feeds the persistent doctrine library.

class DoctrineRating {
  name string @description("The Doctrine.name field from the reviewed doctrine")
  rating string @description("'strong' | 'adequate' | 'weak'")
  reasoning string @description("One-sentence justification")
}

class TurningPoint {
  tick int
  what_happened string
  what_should_have_happened string
}

class RetroLesson {
  trigger_text string @description("Free-text description of the condition under which this lesson applies, e.g. 'when enemy shows 3+ tanks by mid game and we have no AA'")
  guidance_text string @description("Free-text advice for that situation, e.g. 'prioritize AA structures and tech to mammoth tanks'")
  confidence float @description("0.0-1.0 — how confident the reviewer is that this lesson generalizes beyond this single game")
}

class GameReview {
  summary string @description("2-3 sentence summary of the game arc — how the strategy evolved and why it won or lost")
  quality_tag string @description("MUST be one of: 'exemplary' | 'mixed' | 'cautionary'. Rules (strict): 'exemplary' requires won=true AND doctrine choices were principled (not just lucky). 'cautionary' requires won=false AND the loss is attributable to a specific mistake. 'mixed' is EVERYTHING else — won messily, lost to bad luck, outcome ambiguous, or the game simply doesn't teach much. When in doubt, use 'mixed' — the library should be curated, not exhaustive.")
  turning_points TurningPoint[]
  doctrine_ratings DoctrineRating[]
  lessons RetroLesson[]
}

class DoctrineHistoryEntry {
  tick int
  doctrine Doctrine
}

function ReviewGame(
    our_faction: string,
    opponent_faction: string,
    won: bool,
    duration_ticks: int,
    doctrine_history: DoctrineHistoryEntry[],
    events: GameEvent[],
    final_combat_stats: CombatStats
) -> GameReview {
  client CustomGPT5Mini
  prompt #"
    You are a military analyst reviewing a completed Red Alert game played by an AI commander.
    Your job is to extract durable lessons — patterns that will help the commander play better NEXT game,
    not blow-by-blow narration of this one.

    GAME OUTCOME
    Faction: {{ our_faction }} vs {{ opponent_faction }}
    Result: {% if won %}WIN{% else %}LOSS{% endif %}
    Duration: {{ duration_ticks }} ticks
    Final losses: {{ final_combat_stats.infantry_lost }} infantry, {{ final_combat_stats.vehicles_lost }} vehicles, {{ final_combat_stats.aircraft_lost }} aircraft, {{ final_combat_stats.naval_lost }} naval

    DOCTRINE TRAJECTORY (chronological — each entry is the doctrine in effect starting at that tick)
    {% for d in doctrine_history %}
    [tick {{ d.tick }}] {{ d.doctrine.name }}: {{ d.doctrine.rationale }}
      aggression={{ d.doctrine.aggression }}, economy={{ d.doctrine.economy_priority }}, ground_def={{ d.doctrine.ground_defense_priority }}, air_def={{ d.doctrine.air_defense_priority }}, tech={{ d.doctrine.tech_priority }}
      units: inf={{ d.doctrine.infantry_weight }}, vehicle={{ d.doctrine.vehicle_weight }}, air={{ d.doctrine.air_weight }}, naval={{ d.doctrine.naval_weight }}
    {% endfor %}

    EVENTS (adverse conditions the engine detected during play)
    {% if events %}
    {% for e in events %}
    - [tick {{ e.tick }}] {{ e.kind }}: {{ e.detail }}
    {% endfor %}
    {% else %}
    (no adverse events detected)
    {% endif %}

    YOUR TASK — produce a GameReview with:

    1. summary: 2-3 sentences capturing the arc. What did the commander try? What worked or failed?

    2. quality_tag — STRICT rules, check the result field above:
       - 'exemplary' REQUIRES result=WIN AND the doctrine choices were principled. A lucky or sloppy
         win is NOT exemplary — it's mixed.
       - 'cautionary' REQUIRES result=LOSS AND the loss is attributable to a specific identifiable
         mistake in the doctrine trajectory. A loss to overwhelming force or bad luck is mixed.
       - 'mixed' is the default when the above strict criteria don't both hold. A won game with
         poor execution is mixed (the WIN prevents 'cautionary'; the poor execution prevents
         'exemplary'). Use 'mixed' liberally — the library should be curated, not exhaustive.
       - If you disagree with the result (e.g. you think the game was "really" a loss because
         critical infrastructure was lost), you still MUST respect the result field. Tag weak
         individual doctrines via doctrine_ratings instead — that's the right channel for
         flagging bad play within an overall outcome.

    3. turning_points: 0-3 specific moments where the strategy should have pivoted differently (or
       where a good pivot happened). Skip if the game had no clear turning points.

    4. doctrine_ratings: one entry per DISTINCT doctrine NAME that appeared in the trajectory. Rate
       each as 'strong', 'adequate', or 'weak' with a one-sentence reasoning tied to game events.

    5. lessons: 1-4 durable lessons. Each lesson must be GENERALIZABLE to future games — not "we
       lost the tank on tick 3000". Frame trigger_text as a recognizable situation ("when enemy fields
       tesla coils before we have air") and guidance_text as a concrete response ("pivot to air or
       tech mammoths, don't keep sending tanks"). Confidence: 0.8+ only if you're sure the pattern
       generalizes; 0.4-0.7 for plausible patterns; below 0.4 is probably not worth saving.

    Be terse. This review is metadata that gets injected into future prompts — long prose wastes tokens.

    {{ ctx.output_format }}
  "#
}
