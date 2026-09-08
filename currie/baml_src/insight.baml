// Reading a rule-set post mortem.
//
// The input is already the answer to "what stopped each rule" — counted, ranked
// and attributed to a line. What a model adds is the join back to strategy: why
// these particular blockages, together, produced the game that happened.

class BlockedClause {
  source string @description("the require, as written")
  file string
  line int
  sole int @description("states where this clause alone stopped the rule")
  sole_rate float @description("the same, per 100 states replayed. Compare THIS across games — a raw count is proportional to how long the game lasted")
  blocked int @description("states where it was false, alone or with others")
  rules string[] @description("the rules it blocks")
  working int @description("how many of those rules ALREADY fire and act in the real game. When this equals the length of `rules`, the line is not where a problem is — the sample missed the moments they fired")
}

class DeadRule {
  name string
  category string @description("what kind of rule it is. A `rebuild`, `aircraft-maintenance` or `setup` rule that never fires is usually good news — nothing was lost, nothing was stuck — and reading it as a starved production rule inverts the finding")
  // From the game itself, not the replay.
  matched int @description("times the condition held during the real game. -1 when unmeasured")
  acted int @description("times the action then did something. -1 when unmeasured. GREATER THAN ZERO MEANS THE RULE ALREADY WORKS")
  seen int
  culprit string @description("the require most often solely responsible")
  culprit_sole int
}

class ClauseSensitivity {
  source string
  file string
  line int
  rules int @description("how many rules this one line is inlined into")
  windows int @description("doctrine windows it was measured across")
  low_rate float @description("lowest block rate in any window, 0 to 1")
  high_rate float @description("highest block rate in any window, 0 to 1")
  doctrinal bool @description("true when one doctrine input clearly separates the windows where it blocked from the ones where it did not")
  knob string @description("the doctrine input that separates them, when doctrinal")
  knob_low float @description("mean value of that input in the low half")
  knob_high float
  rate_low float @description("block rate in the low half, 0 to 1")
  rate_high float
  verdict string @description("the plain-language reading")
}

class TunableGate {
  source string
  file string
  line int
  rules int
  at float @description("the value the gate has")
  blocked int @description("states it blocked at that value")
  relief int @description("states that moving it, without leaving the observed range, would stop blocking")
  relief_at float @description("the value that buys the relief")
  reached float @description("the highest value the compared side ever reached")
}

class DoctrineChoice {
  name string
  rationale string @description("the strategist's own account of why it chose these weights")
  infantry_weight float
  vehicle_weight float
  air_weight float
  tech_priority float
  economy_priority float
  aggression float
}

class MissingThing {
  rule string @description("the rule that could not fire")
  clause string @description("the existence test that stopped it — NOT a threshold to relax")
  thing string @description("what did not exist")
  maker string @description("the rule whose action would have produced it. Empty when the rule set has none")
  maker_matched int @description("times that rule's condition held in the real game. -1 when unmeasured")
  maker_culprit string @description("what stopped the maker, when it never fired")
  verdict string
}

class GameFacts {
  our_faction string
  opponent_faction string
  outcome string @description("win or loss")
  duration_ticks int
  doctrine_windows int @description("how many times the strategist swapped doctrine")
  states_replayed int
  rules_in_set int
  never_fired int @description("rules never satisfiable in any recorded state")
  no_single_cause int @description("rules blocked by several requirements at once")
  top_clauses BlockedClause[]
  dead_rules DeadRule[]
  doctrine_names string[] @description("the doctrines the strategist chose, in order")
  sensitivity ClauseSensitivity[] @description("whether each blocking line tracks a doctrine input or blocks regardless")
  gates TunableGate[] @description("numeric gates and what moving them would buy, measured only at values the compared side actually reached")
  faction string @description("the side Vimy played. Soviet factions build a different unit list from Allied ones")
  directive string @description("the line of prose the player gave the strategist. Empty when the game was played before it was recorded")
  doctrines DoctrineChoice[] @description("a sample of the doctrines the strategist wrote from that directive, with its stated reasoning")
  missing MissingThing[] @description("clauses that report a thing absent, followed one hop back to whatever was supposed to make it")
}

class Finding {
  claim string @description("one sentence, the finding itself")
  evidence string @description("the numbers from the input that support it, quoted exactly")
  confidence float @description("0 to 1")
}

class DirectiveAdvice {
  diagnosis string @description("how the directive led the strategist to the weights that blocked things — quoting the directive and a rationale where they show the link")
  revision string @description("a directive the player could paste in its place, written in the same voice and length as theirs. Concrete about what to prioritise and what to hold back. Name capabilities rather than units — a directive outlives the game it was written for and gets reused on the other side")
  confidence float @description("0 to 1. Low when the directive was not recorded, or when the blocking was not doctrinal")
}

class Insight {
  summary string @description("two or three sentences: what this game's rule set could not do, and why that produced this outcome")
  findings Finding[] @description("at most four, ordered by how much they explain")
  suggestion string @description("the single change most likely to help. Must name the file and line, and must quote that line's `verdict` from the sensitivity block, so the recommendation and the evidence for it stay together")
  caveat string @description("what this data cannot show — say plainly if the evidence is thin")
  directive_advice DirectiveAdvice? @description("how to change the directive itself, when the blocking was doctrinal. Null when the fault is in the rules, because then the directive is not what to change")
}

function ReadPostMortem(facts: GameFacts) -> Insight {
  client CustomGPT5Mini
  prompt #"
    You are reading a post mortem of a rule-based RTS AI called Vimy.

    Vimy plays by a compiled rule set. Each rule has a priority, a category and
    a list of `require` clauses that must all hold for it to fire. A separate
    LLM strategist rewrites the doctrine — a set of numeric weights — many times
    per game, and each doctrine compiles the same rule sources into different
    thresholds.

    The analysis below replayed every recorded game state against the rule set
    that was actually running at that moment, and recorded which `require`
    stopped each rule from firing.

    Read these two numbers carefully, they are not the same thing:
    - `blocked` — the clause was false. It may have been one of several.
    - `sole` — the clause was the ONLY false one. Satisfy it and the rule fires.

    `sole` is the actionable number. A clause with a high `blocked` and a low
    `sole` is not worth fixing on its own, because the rule would still not fire.

    {{ facts }}

    The `sensitivity` block is how you tell a rule-set problem from a doctrine
    that priced itself out. The same sources compile to different thresholds
    under every doctrine, and this game ran dozens, so it is a real experiment:

    - `doctrinal: true` — the line blocks heavily under some doctrines and
      barely under others, and `knob` names the input that moves it. The rule is
      not wrong; the strategist asked for something that made it unsatisfiable.
      Recommend changing the directive or the weights, NOT the rule.
    - `doctrinal: false` with `low_rate` near `high_rate` — the line blocks
      whatever the doctrine does. It asks for something the game rarely
      provides, and the rule itself is what to change.
    - `doctrinal: false` with a wide `low_rate`–`high_rate` spread — it varies a
      lot and no single input explains it. Say so. Do not recommend editing it
      as though it were a constant blocker, and do not attribute it to a
      doctrine input the data does not support.

    `gates` answers the question `sensitivity` cannot: whether a threshold is
    mistuned or merely asking for something that did not happen. `relief` is the
    states that moving it would free, measured only at values the compared side
    actually reached — a cash gate improves all the way to zero on paper, and
    "set it to zero" is not a recommendation. A line absent from `gates` either
    is not a number or has no relief available.

    The last step is the directive. The player writes one line of prose; the
    strategist turns it into the weights in `doctrines`, and those weights
    decide which rules can fire. So a finding that a clause is `doctrinal` has a
    cause upstream of the weight, and `directive_advice` is where you say what
    it is.

    - Use the `rationale` fields. They are the strategist explaining, in its own
      words, why it chose those weights — the evidence linking the sentence to
      the numbers. Quote from one where it shows the link.
    - `revision` must be a directive the player can paste: the same voice and
      roughly the same length as theirs, not a specification. If theirs is one
      sentence, yours is one sentence.
    - **Name capabilities, not units.** Say "long-range rocket support", not
      "V2". Units are faction-locked — V2 launchers, flak trucks, APCs, tesla
      tanks, shock troopers and flamethrowers are Soviet; artillery, rangers,
      light tanks and Tanya are Allied — and a directive is reused across games
      and sides. This has already gone wrong: a revision naming the V2 was run
      by an Allied faction that cannot build one, and the strategist repeated
      the name into every doctrine it wrote.
    - If the directive you are revising names a unit `{{ facts.faction }}`
      cannot build, say so in `diagnosis`. It is a real defect in the directive
      and the player should know.
    - Set `directive_advice` to null when the blocking was not doctrinal.
      Rewriting the directive would not help, and offering a rewrite anyway is
      the most confident way to be useless here.
    - If `directive` is empty the game predates it being recorded. You may still
      advise from the rationales, but say the directive was not recorded and
      drop your confidence.

    A replay reads the *sampled* states in the recording, not every tick. So a
    rule that fired a handful of times across a whole game can be missing from
    every sample and appear never to have been satisfiable. `matched` and
    `acted` come from the game itself and settle it.

    - **Never recommend changing a line whose `working` equals the number of
      rules it blocks.** Every rule it gates already fires and already does
      something; the sample simply missed those moments. This is the single most
      likely way to give confidently wrong advice here.
    - **Never recommend unblocking a rule whose `acted` is above zero.** It
      already fires and already does something; the replay simply did not sample
      the moments it did. Recommending a change there is the clearest way to be
      wrong, and it has happened.
    - Weigh `category` before treating a silent rule as a problem. A `rebuild`
      rule that never fired means nothing was destroyed. An
      `aircraft-maintenance` rule that never fired means nothing was stuck.
      Silence there is the system working, not a blockage — and relaxing such a
      rule makes it fire when it should not.
    - `-1` means unmeasured, which is not zero. Say so rather than reading it as
      "did nothing".

    `missing` is the difference between a gate being too tight and a thing not
    existing. A clause like `count(idle-minelayers) > 0` or `has-role(airfield)`
    is a report, not a dial: no wording of it produces a minelayer.

    - **Never recommend relaxing or removing an existence test.** Report the
      `maker` and its `maker_culprit` instead — that is where the cause is. If
      `maker` is empty, the finding is that the rule set has no way to produce
      the thing at all, which is a bigger result than any threshold.
    - This mistake has been made three times on three different rules. It is the
      single most likely way to be wrong here.

    Counts scale with how long a game ran. When comparing anything across games
    use `sole_rate`, which is per 100 states; a game a third shorter yields a
    third fewer of everything, and reading two raw counts side by side reports
    the length difference as a finding.

    Rules for your answer:
    - Ground every claim in a number from the input, and quote it exactly.
    - A requirement written inside a shared def is inlined into many rules, so
      one line blocking several rules matters more than its per-rule counts show.
    - Do not recommend changing a clause whose `sole` is zero.
    - Say what the evidence does not support. Do not fill gaps with plausible
      RTS strategy advice — the value here is what the replay actually shows.
    - Never claim a rule "never fired" caused something without noting that a
      rule can be blocked for reasons outside the rule set, such as the action
      itself doing nothing.
    - Before recommending a change to a line, check its entry in `sensitivity`
      and quote that line's `verdict` in the suggestion itself.
    - You may NOT recommend editing a line whose verdict is "varies, but no
      single input explains it". Its blocking is real and neither the rule text
      nor any one doctrine input accounts for it — editing the threshold is as
      likely to do nothing as to help. Recommend the best-supported constant or
      doctrinal line instead, or say plainly that no single edit is supported by
      this data and name what would settle it.
    - A line with a high `sole` count is where the blocking happened; it is not
      automatically where the fault is. The sensitivity verdict is what tells
      you which.
    - When you recommend moving a threshold, take the value from `relief_at` and
      quote the `relief` it buys. Do not invent a value; one outside the range
      the compared side reached buys nothing whatever it looks like on paper.

    {{ ctx.output_format }}
  "#
}
