// Memory librarian. Runs once per game (at first evaluation). Given the
// current directive + matchup and a set of candidate memory items retrieved
// from the store, decides which are actually relevant for THIS game and
// produces a brief explaining the alignment (or lack thereof) between the
// current directive and the precedent.
//
// The goal is anti-anchoring: when the directive diverges from prior games,
// the librarian explicitly filters those memories out and tells the
// strategist to treat the precedent as non-applicable.

class CandidateExemplar {
  id int @description("Stable identifier for referencing in output")
  context string @description("Brief matchup summary, e.g. 'vs england, won — rating: strong'")
  rationale string @description("The doctrine's original rationale text")
  doctrine_summary string @description("Tight field summary, e.g. 'aggression=0.6, vehicle=0.7'")
}

class CandidateCautionary {
  id int
  context string @description("Matchup and outcome, e.g. 'vs england, lost'")
  failed_pattern string
  failure_reason string
}

class CandidateLesson {
  id int
  trigger string
  guidance string
  confidence float
}

class MemoryCandidates {
  exemplars CandidateExemplar[]
  cautionaries CandidateCautionary[]
  lessons CandidateLesson[]
}

class SelectedExemplar {
  id int @description("References the CandidateExemplar.id from the input")
  why_relevant string @description("One sentence on why this precedent applies to the current directive")
  what_to_emulate string @description("The specific aspect worth carrying forward")
}

class SelectedCautionary {
  id int @description("References the CandidateCautionary.id from the input")
  why_relevant string @description("One sentence on why this failure is a risk for the current directive")
  what_to_avoid string @description("The specific shape or behavior to steer away from")
}

class RelevantMemory {
  alignment_note string @description("ONE paragraph assessing how the current directive relates to the precedent. If the directive diverges strongly from all prior games, say so explicitly — this is the most important output field when memory is misaligned.")
  exemplars SelectedExemplar[] @description("Zero or more exemplars that genuinely apply. Better to return an empty list than to stretch a weak match.")
  cautionaries SelectedCautionary[] @description("Zero or more cautionaries worth warning about. Include even if the current directive differs, IF the LLM might still drift into the cautionary pattern.")
  lesson_ids int[] @description("IDs of lessons from the candidate list that genuinely apply. Drop anything directive-specific that doesn't transfer.")
}

function SelectRelevantMemory(
    directive: string,
    our_faction: string,
    opponent_faction: string,
    candidates: MemoryCandidates
) -> RelevantMemory {
  client CustomGPT5Mini
  prompt #"
    You are the memory librarian for a Red Alert AI commander. Your job is to
    decide which past-game memories are ACTUALLY relevant to the current
    game's strategic intent, and which are noise that would anchor the
    commander toward the wrong strategy.

    CURRENT GAME
    Directive (strategic intent from high command): "{{ directive }}"
    Matchup: {{ our_faction }} vs {{ opponent_faction }}

    CANDIDATE MEMORIES FROM THE LIBRARY

    {% if candidates.exemplars %}
    Exemplars (doctrines that worked in past games):
    {% for e in candidates.exemplars %}
    [exemplar #{{ e.id }}] ({{ e.context }})
      Summary: {{ e.doctrine_summary }}
      Rationale: {{ e.rationale }}
    {% endfor %}
    {% else %}
    Exemplars: (none available)
    {% endif %}

    {% if candidates.cautionaries %}
    Cautionary patterns (doctrines that failed):
    {% for c in candidates.cautionaries %}
    [cautionary #{{ c.id }}] {{ c.context }}
      Pattern: {{ c.failed_pattern }}
      Why it failed: {{ c.failure_reason }}
    {% endfor %}
    {% else %}
    Cautionary patterns: (none available)
    {% endif %}

    {% if candidates.lessons %}
    Lessons:
    {% for l in candidates.lessons %}
    [lesson #{{ l.id }}] (confidence {{ l.confidence }}) When {{ l.trigger }} → {{ l.guidance }}
    {% endfor %}
    {% else %}
    Lessons: (none available)
    {% endif %}

    YOUR JOB

    Produce a RelevantMemory with:

    1. alignment_note (CRITICAL): ONE paragraph describing how the current
       directive relates to the precedent. If the prior games pursued a
       different strategy than the current directive, say so clearly. If
       they match, note the overlap. This note gets shown to the strategist
       so it understands what the memory means for THIS game.

    2. exemplars: Select only those past doctrines whose strategic shape is
       genuinely applicable to the current directive. An "engineer rush with
       APC transport" precedent is NOT a good exemplar for an "early
       overwhelming infantry swarm" directive — those are different tactics
       that happen to share aggression. Empty list is BETTER than a stretched
       match.

    3. cautionaries: Include cautionary patterns if the commander might drift
       INTO them under the current directive (e.g. if prior APC-heavy games
       failed and the LLM has a known tendency to reach for APC solutions,
       keep the cautionary even under a swarm directive). Otherwise drop.

    4. lesson_ids: Select lessons that generalize across directives. Keep
       lessons about economy discipline, scouting, reacting to specific
       enemy signals. Drop lessons that are only useful for a specific tactic
       the current directive doesn't use.

    BE CONSERVATIVE. When in doubt about relevance, drop the item. Injecting
    irrelevant memory is worse than injecting none — it anchors the
    strategist toward the wrong shape.

    {{ ctx.output_format }}
  "#
}
