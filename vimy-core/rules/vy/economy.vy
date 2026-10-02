param economy-priority: float
param tech-priority: float

rule build-power {
  priority 800
  category economy exclusive
  because "REVERTED to the reactive trigger on 2026-10-02 after two games on a headroom version. The headroom (power-excess < 50) worked on its own terms - build-power first acted at tick 3030 against a baseline of 5630-12910, and the held signature from vimy-ccvb fell from 235 and 115 to 34 - and it was net harmful anyway, because it consumed the trigger build-advanced-power needs. That rule asks for cash >= 500 while this one asks lerp(500,200,economy-priority), about 260, so the cheap plant clears the deficit before cash ever reaches 500 with excess still under 50. Measured: advanced power plants present in the field went from 10674 unit-samples in game 211 to ZERO in games 214 and 215, basic plants rose from a 2-6 baseline to 14 and 8, and median power excess FELL to 98 and 70 against 171 and 180. Two wrong diagnoses were shipped before that one: exclusive-category preemption (refuted - advanced-power was evaluated 3396 times in game 215 and skipped only 0.7 percent, the same as baseline) and a priority reorder to 785 to fix it (did nothing, for the same reason). Any redesign has to handle the cash-floor interaction, not the priority. Do not reach for `power-excess < 50` again without it"
  do produce-power-plant
  require not queue-busy(Building)
  require not queue-producing-role(power-plant)
  require power-excess < 0 or role-count(power-plant) == 0
  require can-build-role(power-plant)
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
