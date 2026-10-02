param economy-priority: float
param tech-priority: float

rule build-power {
  priority 785
  category economy exclusive
  because "BELOW build-advanced-power at 790, which is the correction to the first version of this retune. Both rules live in the exclusive `economy` category and an exclusive category picks the highest-priority rule whose condition HOLDS, so when this rule moved to the same `power-excess < 50` threshold advanced-power had used all along, it took every evaluation advanced-power used to win. Game 214 measured it: build-advanced-power went from 5-6 acts in games 210-213 to ZERO, and the base ran 14 basic plants against a baseline of 2-6, with a LOWER median excess (98 against 171 and 180) because the baseline games were drawing on a mix including 200-power advanced plants. Roughly 2400 credits of extra plants bought worse power. That is the same preemption the produce-heavy-vehicle entry records. Below 790 the advanced plant wins the headroom band whenever its own gates pass -- economy-priority above 0.3 or tech-priority above 0.5, not rushed, buildable -- and this rule fills in when they do not, which keeps the early firing that worked. builds on HEADROOM rather than on a deficit already arrived. RA sets LowPowerModifier 300, so the moment excess goes negative everything builds three times slower - including the plant that would fix it, which then holds the Building queue and shuts this rule's own `not queue-busy` gate. Game 212 spent 35920 ticks power-negative across 26 episodes, 1128 evaluations of them with a plant buildable, cash in hand, nothing queued and the queue busy. Measured warning: power decays THROUGH the thin band first, so the trigger has time to work. In the 2000 ticks before an episode game 211 ran median excess 80, then 65, then 38 in the last 300 with 71 percent of samples under 50, and game 212 ran 96, then 50, then 70 with about half under 50. A threshold of 50 fires in that window, several hundred ticks before the deficit, while the queue is still free - which is what makes it a fix rather than one more order queued behind a refinery. The threshold also bounds the spending by itself: once excess clears 50 the rule stops, so it cannot run away. Matches build-advanced-power, which has used `power-excess < 50` all along. NOT the alternative of dropping the queue gate: the Building queue is serial, so a plant queued behind a half-built refinery arrives no sooner"
  do produce-power-plant
  require not queue-busy(Building)
  require not queue-producing-role(power-plant)
  require can-build-role(power-plant)
  require power-excess < 50 or role-count(power-plant) == 0
  require cash >= lerp(500, 200, economy-priority)
}

rule build-refinery {
  priority 750
  category economy exclusive
  because "the first refinery is the whole economy, so it outranks everything but power"
  do produce-refinery
  require not queue-busy(Building)
  require can-build-role(refinery)
  require role-count(refinery) < 1
  require cash >= lerp(2000, 800, economy-priority)
  require not queue-producing-role(refinery)
}

rule build-second-refinery {
  priority lerp(560, 700, economy-priority) + 1
  because "above the tech centre when economy and tech are weighted the same: the tech centre is paid for out of an income that has to exist first"
  category economy exclusive
  do produce-refinery
  require economy-priority > 0.1
  require not is-rushed()
  require not queue-busy(Building)
  require can-build-role(refinery)
  require role-count(refinery) == 1
  require has-role(barracks) or has-role(war-factory)
  require cash >= lerp(1500, 500, economy-priority)
  require not queue-producing-role(refinery)
}

rule build-extra-refinery {
  priority lerp(520, 680, economy-priority) + 1
  because "above the tech centre when economy and tech are weighted the same"
  category economy exclusive
  do produce-refinery
  require economy-priority > 0.1
  require economy-priority > 0.37
  require not is-rushed()
  require not queue-busy(Building)
  require can-build-role(refinery)
  require role-count(refinery) >= 2
  require role-count(refinery) < lerp(1, 10, economy-priority)
  require has-role(barracks) or has-role(war-factory)
  require cash >= lerp(2000, 800, economy-priority)
  require not queue-producing-role(refinery)
}

rule build-advanced-power {
  priority 790
  category economy exclusive
  do produce-advanced-power
  require economy-priority > 0.3 or tech-priority > 0.5
  require not is-rushed()
  require not queue-busy(Building)
  require can-build-role(advanced-power)
  require power-excess < 50
  require cash >= 500
}

rule build-ore-silo {
  priority 300
  category economy exclusive
  do produce-ore-silo
  require economy-priority > 0.5
  require not is-rushed()
  require not queue-busy(Building)
  require can-build-role(ore-silo)
  require resources-near-cap()
  require role-count(ore-silo) < lerp(0, 2, economy-priority)
  require cash >= 150
  require not queue-producing-role(ore-silo)
}

rule produce-extra-harvester {
  priority 510
  category produce-vehicle exclusive
  because "above every combat-vehicle rule, so the exclusive queue cannot starve income. The ceiling stays at refineries plus two and is deliberately NOT two per refinery: that was tried between 8 and 10 September, put ten harvesters on the field, and game 98 lost nine of them to raids while a 540-credit pillbox was unaffordable in 90% of states. The mod gives one Vehicle queue however many war factories are built and this rule outranks every combat-vehicle rule in it, so a higher ceiling buys income with the army's queue time. Game 178 looked like a case for raising it - 6 refineries, 10 harvesters, 27 matches and 6 acts - but its fleet matched game 175's and its income was a third lower, because the harvesters spent the game fleeing: 47% of their time mining, 117 flee-harvesters acts. The ceiling was not the limit"
  do produce-harvester
  require economy-priority > 0.5
  require has-role(refinery)
  require has-role(war-factory)
  require queue-depth(Vehicle) < 2
  require can-build-role(harvester)
  require role-count(harvester) < role-count(refinery) + 2
  require combat-vehicle-count >= 2
       or role-count(harvester) <= role-count(refinery)
  require cash >= 600
}
