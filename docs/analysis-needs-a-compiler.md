# What the compiler bought us

Vimy's rules used to be built by `CompileDoctrine`, a Go function that turned a
doctrine's weights into `expr` condition strings. It worked. It was replaced by
vimyc — a hand-written lexer, parser, type checker and evaluator for a small
language — and the stated reason at the time was that the rules deserved to be
*written* rather than assembled by string concatenation.

That reason was right and it undersold the result. The compiler turned out to be
the thing that made analysis possible at all, and this note records why, because
it is not obvious in advance and it generalises past this project.

## The question we could not answer

Vimy loses. Thirteen archived losses, and the firing counters said the busiest
rule in every one of them was `form-ground-attack` — 15,702 firings across those
games. It was doing nothing. The action it called refused to form a squad below
full strength while the rule's condition asked for 60% of one, so the rule
matched constantly and returned without effect.

Counters answer *what happened*. They cannot answer *what nearly happened*, and
in a rule-based AI that is where the answers live. A rule that never fires all
game is invisible in a log: there is no line for the thing that didn't occur.
The question we actually wanted was

> This rule never fired. Which of its requirements was false, and how often was
> it the only one?

## Why the runtime cannot answer it

The engine holds each rule's condition as a single string:

```go
type Rule struct {
    ConditionSrc string   // "cash >= 600 && !QueueBusy(\"Vehicle\") && RoleCount(...) < ..."
    program      *vm.Program
}
```

One flattened conjunction, compiled to one bytecode program. Ask it whether the
rule fires and it says yes or no. The clause boundaries are gone, and so is any
notion of *where* each clause came from. You could re-parse that string, but you
would be writing a parser for a language you already have a parser for, and it
would not recover the source position of anything.

This is what a string-concatenating compiler costs you. `CompileDoctrine` also
knew, momentarily, that a condition was five separate clauses — and then joined
them with `&&` and forgot. The structure existed only inside the function that
built it.

## What the compiler kept

vimyc lowers a rule to an IR that keeps the parts apart:

```rust
pub struct IrRule {
    pub priority: IrExpr,
    pub action: IrAction,
    /// Implicitly ANDed.
    pub requires: Vec<IrExpr>,
    pub span: Span,
}
```

`requires` is a `Vec`, and every `IrExpr` carries a `Span` into the source. Two
properties, and the whole of Currie falls out of them:

**The clauses are separable.** `eval::conjuncts` already evaluated each one
independently — it was written to measure which conjuncts a test corpus
exercised. Blame is that function plus counting:

```rust
let held = conjuncts(rule, params, state);
let failing = held.iter().filter(|ok| !**ok).count();
for (clause, ok) in record.clauses.iter_mut().zip(&held) {
    if !ok {
        clause.blocked += 1;
        if failing == 1 { clause.sole += 1 }   // satisfy this and the rule fires
    }
}
```

That `failing == 1` test is the useful part, and it only exists because the
clauses were never merged. A clause that fails alongside three others is not
worth fixing; a clause that stands alone between a rule and firing is exactly
what to fix. A flattened condition cannot tell the two apart.

**The spans survive to the report.** A count is a curiosity. `economy.vy:90` is
somewhere to go:

```
produce-extra-harvester   seen 522  held 0
  blocked 443  sole 66   economy.vy:90  require role-count(harvester) < role-count(refinery) + 1
  blocked 335  sole 16   economy.vy:91  require cash >= 600
```

This particular output corrected a change made an hour earlier that day. The
cash floor had been lowered from 1,400 to 600 on the strength of an aggregate
that showed cash starvation. The replay says the binding constraint was the
harvester cap, not the cash gate — the fix addressed the second-most-important
clause. Nobody would have found that by reading firing counts.

The spans also compose with something else the compiler gained recently. When
the rule set was split from one 1,477-line file into six topic files, vimyc
grew a `SourceMap` so a diagnostic could name the file it came from. The blame
report inherited file-and-line attribution for free, because a span resolves the
same way whether it is a type error or a post mortem.

## The line that only a compiler could find

Ranking blame by rule is the obvious view and it is not the interesting one. A
`require` written once inside a `def` is inlined into every rule that calls it,
so the same line appears dozens of times:

```
614 sole  production.vy:45  reserves(cost)          blocks 3 rules
530 sole  production.vy:26  cash >= cost            blocks 2 rules
457 sole  core.vy:11        cash >= cost            blocks 4 rules
```

Those are the top three blame sites in a whole game, and they are all the cash
reserve model — a savings mechanism added to stop cheap rules draining income
before a radar dome could be afforded. It does its job. It also, under a doctrine
that wants air *and* tech *and* vehicles, arms three reserve clauses at once and
prices the AI out of its own units.

Aggregating by source line is only meaningful because the analysis knows a line
exists. Per-rule counts would have shown five separate rules each blocked by
"cash" and nobody would have noticed they were the same line.

## The general shape

The three pieces now have clean roles, and the split is not arbitrary:

| | |
|---|---|
| `vimyc` | what the strategy **is** — and, because it kept the structure, what it *could not* do |
| `vimy-core` | executing it, and recording what happened |
| `currie` | replaying the recording against the rules that ran |

The transferable claim is narrow and, I think, true:

> A system that generates its behaviour by assembling strings can tell you what
> it did. A system that compiles its behaviour from a language it can still
> parse can tell you what it declined to do, and where the decision is written.

Vimy did not get a compiler in order to be analysable. It got one because the
rules were worth writing properly. The analysis was a consequence — the IR that
exists so `lower` can inline a def without capturing a variable is the same IR
that lets a post mortem name the line. That is the argument for building the
real thing rather than the string-templating shortcut, and it is much easier to
make now than it was beforehand.

## What it actually found, one day in

The claim above is worth testing against what happened rather than restating.
This is the accounting for the first day the three pieces existed together, and
it does not all point one way.

**Currie found outright: one thing, and it corrected me.**
`produce-extra-harvester` was blocked 443 times by `role-count(harvester) <
role-count(refinery) + 1` and only 335 by its cash floor, with the cap solely
responsible 66 times against the floor's 16. An hour earlier I had lowered that
cash floor on the strength of an aggregate showing the economy starved. I had
fixed the second-most-important clause. The replay said so on its first run.

**Currie narrowed, but did not find: two things.** The reserve tax and the
attack-choice threshold both came from reading `.vy` after the tool made me ask
the right question. The sensitivity read is what said `cash >= cost` was *not*
doctrine-sensitive — that no directive would fix it — and that genuinely
redirected the work. But narrowing is not discovery, and the doc should not
claim otherwise.

**Currie did not find the two largest fixes at all.** `FormSquad` refusing to
form a squad below full strength — which is why the attack squad existed in
under a tenth of sampled states across thirteen games — came from reading the
action next to the rule that called it. `Swap` deleting every squad on every
doctrine change came from noticing that formation timestamps clustered 0.09
seconds after swap timestamps in a log. Neither needed the tool.

**Currie was confidently wrong three times.** It recommended relaxing
`queue-ready(Aircraft)`, then `count(idle-minelayers) > 0`, then the cash gate —
each time targeting the clause that *reports* an absence rather than the reason
for it. The first was worse than useless: `cancel-stuck-aircraft` had matched
nine times and acted nine times, and relaxing its gate would have cancelled more
aircraft in a game whose directive was air supremacy and which built none.

That failure has a shape, and naming it was the day's most useful result: **a
blame count cannot distinguish "this gate is too tight" from "the thing does not
exist".** The fixes — existence-gate chaining, annotating a site with how many of
its rules already work, reconciling the replay against what actually fired — all
came from a human refusing an answer, not from the tool noticing.

## The part that carried its weight

Underneath all of it, the compiler kept paying for itself in ways that had
nothing to do with the analysis it was built for:

- The frozen acceptance corpus caught a reserve change that made a
  1,500-credit aircraft *stricter* rather than cheaper — precisely backwards for
  the doctrine being tested.
- The `RETUNED` rot-check caught two entries going stale the moment the reserve
  was capped, because the rules they exempted no longer differed from Go.
- The multi-file `SourceMap`, built to let six topic files replace one, is why a
  blame site resolves to `economy.vy:90` rather than an offset.
- `IrRule.requires` staying a `Vec<IrExpr>` — a decision made so `lower` could
  inline a def — is the whole of why blame exists.

None of those were analysis features. They are what a real compiler gives you
for free once it exists, and they are the strongest evidence for the claim above.

## What none of it had done, until it did

For most of a day this section read: five games, five losses, `squad-attack`
never once fired, and not one finding here had produced a game Vimy won. That
was worth writing down, and it is worth keeping the correction next to it.

Game 88 was a win. The first in eleven games, and the numbers say what carried
it:

| | games 85-87 | game 88 |
|---|---|---|
| harvesters | median 5, max 5 | median 9, max 9 |
| combat vehicles | max 2 | max 5 |
| `squad-attack-known-base` | 0-2 firings | 58 firings, from 12% |
| result | loss | **win** |

The last constraint was one clause. `produce-extra-harvester` carried
`role-count(harvester) < role-count(refinery) + 1`, which permits about one
harvester per refinery where the game wants two, so the rule acted exactly once
in each of games 85, 86 and 87 and the economy stood at four refineries and
five harvesters in all three. Changing `+ 1` to `* 2` moved it to nine, and
everything downstream that had been an argument with the cash gate stopped
being one.

Two things that matters for, and one it does not.

It is **one game**, against one opponent, and germany-vs-russia is the same
matchup that lost 83 and 86. A single win does not separate "the cap was the
binding constraint" from "this opponent played badly". The measured claims are
narrower and they hold regardless: the economy moved from five harvesters to
nine, the vehicle ceiling broke for the first time in the series, and the AI
attacked fifty-eight times having managed none in the game before.

It also took **the whole stack**. Scouting found the enemy at tick 1770 instead
of 13250. The airfield stopped spending the war factory's money, so the factory
arrived at 4% instead of 57%. The service depot was priced as the tech gate it
is, unlocking every real tank in the game. `FormSquad` formed partial squads,
`Swap` stopped deleting them, and `form-defense-squad` stopped demanding a pool
of eight. Each of those was necessary and none was sufficient — every one of
them was in place for games 86 and 87, which lost.

The claim this document makes is not "the compiler wins games". It is that a
compiler which keeps its structure can tell you which clause is standing in the
way, one at a time, until none is. That is what happened, and the last one was
a `+ 1`.

## What is still missing, and why it is the same lesson

The archive records the *doctrine* a game window ran under, but not the rule set
that doctrine compiled to. Recompile an old doctrine and you get today's
sources. So the moment the `.vy` files are edited, an exact replay of an older
game is gone — the fingerprint in the recording matches nothing.

Currie detects this and says so rather than quietly reporting against the wrong
rules. The fix is to archive the compiled artifact per window, which
`rules.ToVimyc` already produces for the differential dump.

It is the same mistake in a different place: keeping the inputs and throwing away
the structure they produced. Worth fixing before there is a year of games to
regret it with.
