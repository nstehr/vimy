param vehicle-weight: float
param air-weight: float
param naval-weight: float
param infantry-weight: float
param tech-priority: float
param ground-defense-priority: float
param air-defense-priority: float
param superweapon-priority: float
param transport-assault: float

param prefers-radar-gated-primary: int
param prefers-v2-launcher: int
param prefers-artillery: int

def any-ground-defense-buildable() =
  can-build-role(pillbox) or can-build-role(camo-pillbox) or can-build-role(turret)
  or can-build-role(flame-tower) or can-build-role(tesla-coil)

def ground-defense-count() =
  role-count(pillbox) + role-count(camo-pillbox) + role-count(turret)
  + role-count(flame-tower) + role-count(tesla-coil)

def defense-cap() = lerp(2, 20, ground-defense-priority)

def war-factory-base() = lerp(580, 730, vehicle-weight)


def service-depot-cash() = 1200

def war-factory-cash() =
  trunc(select(transport-assault > 0.2,
               min(lerp(2500, 2000, vehicle-weight),
                   lerp(2200, 2000, transport-assault)),
               lerp(2500, 2000, vehicle-weight)))

def affordable(cost: int) =
  cash >= cost
  and (vehicle-weight <= 0.1 or has-role(radar) or cash >= cost + 1000)
  and (income-rate > 0 or has-role(refinery)
       or cash >= cost + 1400)
  and (income-rate > 0 or has-role(war-factory) or vehicle-weight <= 0.1
       or cash >= cost + war-factory-cash())
  and (income-rate > 0 or has-role(service-depot) or vehicle-weight <= 0.3
       or cash >= cost + service-depot-cash())
  and (income-rate > 0 or has-role(radar) or tech-priority <= 0.2
       or cash >= cost + 1000)

rule build-radar {
  priority trunc(select(prefers-radar-gated-primary > 0, 710.0, 570.0))
  category economy exclusive
  because "radar is the tech gate for vehicles, aircraft and naval"
  do produce-radar
  require vehicle-weight > 0.1 or air-weight > 0.1 or naval-weight > 0.1 or tech-priority > 0.3
  require not is-rushed()
  require not queue-busy(Building)
  require can-build-role(radar)
  require not has-role(radar)
  require not queue-producing-role(radar)
  require has-role(barracks) or has-role(war-factory)
  require power-excess >= 0
  require cash >= 1000
}

rule build-barracks {
  priority trunc(max(745.0, max(lerp(600, 700, infantry-weight),
                                select(ground-defense-priority > 0.2,
                                       lerp(600, 700, ground-defense-priority), 0.0))))
  category economy exclusive
  because "barracks unblocks the whole infantry queue, so it outranks radar"
  do produce-barracks
  require infantry-weight > 0.1 or ground-defense-priority > 0.2
  require not queue-busy(Building)
  require can-build-role(barracks)
  require not has-role(barracks)
  require power-excess >= 0
  require cash >= 300
  require not queue-producing-role(barracks)
}

rule build-war-factory {
  priority trunc(max(select(naval-weight > 0.1 or air-weight > 0.1, 685.0, 0.0),
                     max(select(prefers-v2-launcher > 0 or prefers-artillery > 0, 720.0, 0.0),
                         select(transport-assault > 0.2,
                                max(war-factory-base(),
                                    lerp(600, 700, transport-assault))
                                  + lerp(0, 40, transport-assault),
                                war-factory-base()))))
  category economy exclusive
  because "vehicles are universally useful, so the war factory precedes naval and air yards. The cap replaces `not has-role(war-factory)`, which stopped at exactly one: build-war-factory fired once per game in 56 of 69 archived games and never again. A second factory adds NO parallel production - ClassicProductionQueue is per player per type, so there is one Vehicle queue however many are built, and a rule written on the assumption of parallelism would be wrong. What it adds is speed: ra/rules/player.yaml sets SpeedUp: True with BuildTimeSpeedReduction 100, 75, 60, 50, so build time falls to 75 percent at two factories, 60 at three and 50 at four - 1.33x, 1.67x and 2x the throughput of one queue, at the same credits per unit and with no change to any priority. Game 182 built one, produced 16 vehicles in 66320 ticks, earned 121689 credits and spent 19900 of it on armour: 16 percent of income, against twelve refineries and fifteen power plants. The queue was neither starved of orders nor short of cash - queue-depth(Vehicle) already stood at 1 in 61 of 63 sampled evaluations, and median cash on a produce fire was 1608 - it drained too slowly to replace losses, so production and death settled at about 35 units on the field while the enemy fielded mammoths that survived. Vimy built 231 riflemen and 16 tanks in that game and finished with none of either. Tied to refineries rather than to a constant because a factory the economy cannot feed is worse than none, and refineries are what feeds one. Thresholds measured against the streamed games rather than picked: counting refineries and factories per tick from the unit stream, 4 and 6 unlock the second factory in 7 of 9 games but late - tick 11500 of game 182's 66320, and 29480 of 79180 in the session before it. 3 and 5 unlock it in 8 of 9, at 10500 in game 182 with 84 percent of the game still to run and at 25660 with 68 percent left in the long one, and the third at 12620 in game 182. Only the game that peaked at 2 refineries declines, which is the gate working. Peak factories is 1 in every one of those games, so `not has-role(war-factory)` was the binding constraint and nothing else was. Capped at three rather than four: the fourth buys 10 percentage points against the third's 15 and the second's 25, and each one competes with armour for the Building queue. Written as a runtime or-chain rather than a def: vimyc refuses `max(1, min(3, trunc(role-count(refinery) / 2)))` because a def argument is fixed when the doctrine lands and so cannot read role-count, which is correct and is why the ladder is spelled out"
  do produce-war-factory
  require vehicle-weight > 0.1
  require not queue-busy(Building)
  require can-build-role(war-factory)
  require role-count(war-factory) < 1
       or (role-count(war-factory) < 2 and role-count(refinery) >= 3)
       or (role-count(war-factory) < 3 and role-count(refinery) >= 5)
  require power-excess >= 0
  require cash >= war-factory-cash()
  require not queue-producing-role(war-factory)
}

rule build-barracks-prereq {
  priority 600
  category economy exclusive
  because "radar needs a barracks, and without this the two can deadlock"
  do produce-barracks
  require vehicle-weight > 0.1 or air-weight > 0.1 or naval-weight > 0.1 or tech-priority > 0.3
  require not (infantry-weight > 0.1 or ground-defense-priority > 0.2)
  require vehicle-weight <= 0.1
  require not queue-busy(Building)
  require can-build-role(barracks)
  require not has-role(barracks)
  require power-excess >= 0
  require cash >= 300
  require not queue-producing-role(barracks)
}

rule build-airfield {
  priority lerp(580, 680, air-weight)
  category economy exclusive
  because "does not spend the war factory's money: an exclusive category picks the highest-priority rule whose condition HOLDS, so a cash gate keeps the dearer rule out of the running entirely and the category builds cheapest-first — the war factory outranks this by fifty points and in game 83 was built thirty-five points of game later"
  do produce-airfield
  require air-weight > 0.1
  require not is-rushed()
  require not queue-busy(Building)
  require can-build-role(airfield)
  require not has-role(airfield)
  require not queue-producing-role(airfield)
  require power-excess >= 0
  require cash >= 500
  require has-role(war-factory)
       or vehicle-weight <= 0.1
       or cash >= 500 + war-factory-cash()
  require has-role(service-depot)
       or vehicle-weight <= 0.3
       or cash >= 500 + service-depot-cash()
}

rule build-service-depot {
  priority trunc(select(vehicle-weight > 0.3, 679.0, 565.0))
  because "not a repair bay — fix is the prerequisite for the medium tank, the heavy tank and the mammoth, so without one the only armour either side can field is the Allied light tank; game 84 never built it, fought with light tanks and artillery, and met six tesla tanks and two mammoths"
  category economy exclusive
  do produce-service-depot
  require vehicle-weight > 0.1
  require not is-rushed()
  require not queue-busy(Building)
  require can-build-role(service-depot)
  require not has-role(service-depot)
  require has-role(war-factory)
  require power-excess >= 0
  require cash >= service-depot-cash()
  require not queue-producing-role(service-depot)
}

rule build-naval-yard {
  priority lerp(580, 680, naval-weight) - 1
  because "below the airfield when air and naval are weighted the same"
  category economy exclusive
  do produce-naval-yard
  require naval-weight > 0.1
  require not is-rushed()
  require map-has-water()
  require not queue-busy(Building)
  require can-build-role(naval-yard)
  require not has-role(naval-yard)
  require power-excess >= 0
  require cash >= 500
  require not queue-producing-role(naval-yard)
}

rule build-base-defense {
  priority lerp(400, 600, ground-defense-priority)
  category defense exclusive
  do produce-defense
  require ground-defense-priority > 0.2
  require not queue-busy(Defense)
  require power-excess >= 0
  require any-ground-defense-buildable()
  require ground-defense-count() < defense-cap()
  require affordable(lerp(1500, 300, ground-defense-priority))
}

rule build-base-defense-rush {
  priority lerp(400, 600, ground-defense-priority) + 100
  category defense exclusive
  because "under a rush, defenses go up before the next refinery"
  do produce-defense
  require ground-defense-priority > 0.2
  require is-rushed()
  require not queue-busy(Defense)
  require power-excess >= 0
  require any-ground-defense-buildable()
  require ground-defense-count() < defense-cap()
  require cash >= 200
}

rule build-aa-defense {
  priority lerp(400, 600, air-defense-priority) - 1
  because "below base defense when the doctrine weights air and ground the same, which is how the strategist usually sets them"
  category defense exclusive
  do produce-aa-defense
  require air-defense-priority > 0.3
  require not queue-busy(Defense)
  require power-excess >= 0
  require can-build-role(aa-defense)
  require role-count(aa-defense) < lerp(2, 5, air-defense-priority)
  require affordable(lerp(1200, 500, air-defense-priority))
  require not queue-producing-role(aa-defense)
}

rule build-gap-generator {
  priority lerp(400, 550, ground-defense-priority) - 2
  because "the least urgent of the three defenses, so it goes below both"
  category defense exclusive
  do produce-gap-generator
  require ground-defense-priority > 0.3
  require tech-priority > 0.3
  require not is-rushed()
  require not queue-busy(Defense)
  require power-excess >= 0
  require can-build-role(gap-generator)
  require has-role(tech-center)
  require role-count(gap-generator) < lerp(1, 2, ground-defense-priority)
  require affordable(800)
  require not queue-producing-role(gap-generator)
}

rule build-tech-center {
  priority lerp(600, 660, tech-priority)
  category economy exclusive
  do produce-tech-center
  require tech-priority > 0.4
  require not is-rushed()
  require not queue-busy(Building)
  require can-build-role(tech-center)
  require not has-role(tech-center)
  require has-role(radar)
  require power-excess >= 0
  require cash >= 1500
  require not queue-producing-role(tech-center)
}

rule build-missile-silo {
  priority 650
  category superweapon-build exclusive
  do produce-missile-silo
  require superweapon-priority > 0.3
  require not is-rushed()
  require not queue-busy(Defense)
  require can-build-role(missile-silo)
  require not has-role(missile-silo)
  require has-role(tech-center)
  require power-excess >= 0
  require cash >= 2500
  require not queue-producing-role(missile-silo)
}

rule build-iron-curtain {
  priority 640
  category superweapon-build exclusive
  do produce-iron-curtain
  require superweapon-priority > 0.3
  require not is-rushed()
  require not queue-busy(Defense)
  require can-build-role(iron-curtain)
  require not has-role(iron-curtain)
  require has-role(tech-center)
  require power-excess >= 0
  require cash >= 2500
  require not queue-producing-role(iron-curtain)
}

rule build-extra-barracks {
  priority 500
  category economy exclusive
  do produce-barracks
  require infantry-weight > 0.6
  require not is-rushed()
  require not queue-busy(Building)
  require can-build-role(barracks)
  require role-count(barracks) < lerp(1, 3, infantry-weight)
  require power-excess >= 0
  require cash >= 300
  require not queue-producing-role(barracks)
}

rule build-extra-war-factory {
  priority 490
  category economy exclusive
  do produce-war-factory
  require vehicle-weight > 0.6
  require not is-rushed()
  require not queue-busy(Building)
  require can-build-role(war-factory)
  require role-count(war-factory) < lerp(1, 2, vehicle-weight)
  require power-excess >= 0
  require cash >= 2000
  require not queue-producing-role(war-factory)
}

rule build-extra-airfield {
  priority 480
  category economy exclusive
  because "grow pads to match the doctrine's aircraft ambition, but only once the ones we have fill up"
  do produce-airfield
  require air-weight > 0.1
  require not is-rushed()
  require not queue-busy(Building)
  require can-build-role(airfield)
  require not queue-producing-role(airfield)
  require aircraft-capacity < lerp(2, 8, air-weight)
  require combat-aircraft-count >= aircraft-capacity - 1
  require power-excess >= 0
  require cash >= 500
}

rule build-extra-naval-yard {
  priority 470
  category economy exclusive
  do produce-naval-yard
  require naval-weight > 0.6
  require not is-rushed()
  require map-has-water()
  require not queue-busy(Building)
  require can-build-role(naval-yard)
  require role-count(naval-yard) < lerp(1, 2, naval-weight)
  require role-count(submarine) + role-count(destroyer) >= lerp(3, 8, naval-weight) - 1
  require power-excess >= 0
  require cash >= 500
  require not queue-producing-role(naval-yard)
}
