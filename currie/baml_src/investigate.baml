// The post mortem, as an investigation rather than a projection.
//
// ReadPostMortem hands the model one fixed bundle of facts, chosen in advance.
// That was deliberate — its comment says the model sees "the same numbers the
// page shows and only those, so it cannot make a claim the reader can't check"
// — and it holds, but the price is that the bundle has to anticipate the
// question. Every real diagnosis this project has produced came from following
// a thread: blockers, then the squad timeline, then the field at the tick where
// it collapsed. The bundle cannot be all of those at once.
//
// So the model asks instead, and every call and result is recorded and rendered
// beside the findings. The verifiability constraint survives, and strengthens:
// the reader sees the evidence chain rather than conclusions drawn from a
// projection they have to trust.

class AgentMessage {
  role string
  content string
}

// Investigative tools. Each reads the live telemetry streams, which the fixed
// projection has never included — transit, rallies, blockers, threat, and where
// every unit stood.

// Each tool carries a literal `tool` field naming itself.
//
// Without it three of these were structurally identical - a single `reason`
// string - and nothing in the union could tell them apart. The first live run
// asked for field_at six times in a row while every stated reason said "inspect
// the squad timeline": the model knew what it wanted and could only emit the
// one variant with a distinguishing shape. It spent the whole budget retrying
// and then concluded from evidence it had never received.

class SquadTimelineTool {
  tool "squad_timeline"
  reason string @description("What you expect to learn. One line, so the trace reads as an argument rather than a log.")
}

class StrikeBlockersTool {
  tool "strike_blockers"
  reason string @description("What you expect to learn.")
}

class FieldAtTool {
  tool "field_at"
  tick int @description("The tick to look at. Pick one you have a reason for — where the squad collapsed, where it stopped closing, where the army peaked — not a round number.")
  reason string @description("What you expect to learn.")
}

class CompareGamesTool {
  tool "compare_games"
  reason string @description("What you expect to learn. Use this to check whether something is particular to this game or true of the run.")
}

class FinalInsightTool {
  summary string @description("What happened, in the reader's terms. Not a list of what you queried.")
  findings Finding[]
  suggestion string @description("The single most useful change. One.")
  caveat string @description("What this analysis cannot see, or where you are guessing. Say it plainly; the archive is full of confident readings that were wrong.")
  directive_advice DirectiveAdvice?
}

function InvestigateGameStep(
  facts: GameFacts,
  tools: string,
  history: AgentMessage[]
) -> SquadTimelineTool | StrikeBlockersTool | FieldAtTool | CompareGamesTool | FinalInsightTool {
  client CustomGPT5Mini
  prompt #"
    {{ _.role("system") }}
    You are investigating why a rule-based RTS AI called Vimy lost a game.

    Vimy plays by a compiled rule set: each rule has a priority, a category and
    `require` clauses that must all hold for it to fire. A separate LLM
    strategist rewrites the doctrine — numeric weights — many times per game,
    and each doctrine compiles the same sources into different thresholds.

    You have the replay facts below, and tools that read telemetry the facts do
    not carry. Use them. The facts describe which rules were blocked; the tools
    describe what the army actually did.

    Work like an investigator, not a reporter. Follow one thread at a time and
    let each result choose the next call. A finding you reached by looking is
    worth more than one you inferred from the summary.

    Things that have misled readers of this data before, including careful ones:

    - A squad "arriving" can be two stragglers. Check members, not just distance.
    - Blocker COUNTS are not causes. A blocker that fires constantly may be
      benign, and one that fires four times may be the game.
    - An empty threat field means nothing was SCOUTED, not that the map is safe.
      Blank ground is ignorance.
    - Income per tick has predicted the outcome in every game measured. A game
      with no economy did not lose on tactics.

    Do not repeat a call you have already made. If a result did not answer your
    question, ask a DIFFERENT question rather than the same one again - the
    budget is small and a repeated call spends it without adding evidence.

    Stop when you can name a cause. Guessing costs more than a short answer:
    every confident wrong reading in this archive was produced by someone who
    had not looked at the next thing.

    {{ _.role("user") }}
    {{ facts }}

    Tools available:
    {{ tools }}

    {% for m in history %}
    {{ _.role(m.role) }}
    {{ m.content }}
    {% endfor %}

    {{ ctx.output_format }}
  "#
}
