param aggression: float
param ground-defense-priority: float
param air-defense-priority: float
param air-weight: float
param naval-weight: float
param superweapon-priority: float
param commit-ratio: float
param base-defense-floor: int
param ground-attack-group-size: int
param air-attack-group-size: int
param naval-attack-group-size: int

def attack-priority() = lerp(200, 400, aggression)

def defend-priority() = lerp(350, 500, ground-defense-priority)

def ground-form-threshold() = trunc(max(2.0, ground-attack-group-size * 3 / 10))

def defense-floor-holds() =
  base-defense-floor <= 0
  or squad-exists(ground-defense)
  or role-count(pillbox) + role-count(camo-pillbox) + role-count(turret)
     + role-count(flame-tower) + role-count(tesla-coil) >= base-defense-floor

rule form-defense-squad {
  priority defend-priority() + 5
  category squad-form
  because "formed at its own size — it used to demand enough surplus for an attack squad on top, which needed a pool of eight and so only ever formed once the base was already under attack"
  do form-squad(ground-defense, Ground, lerp(2, 5, ground-defense-priority), Defend)
  require ground-defense-priority > 0.3
  require (not squad-exists(ground-defense)
           and count(unassigned-idle-ground) >= lerp(2, 5, ground-defense-priority))
       or (squad-needs-reinforcement(ground-defense) and count(unassigned-idle-ground) >= 1)
}

rule defend-base {
  priority defend-priority() + 1
  category combat
  because "one rule where there were five. scramble-base-defense, emergency-base-defense, defend-base, squad-defend-base and defend-critical-building all sent units at a threat near home, with five triggers and four different unit pools, and three of them called this same action. One said so in its own because: above scramble-base-defense, which does the same thing with a looser condition. Nobody designed that, it accreted, and the cost was measurable — game 143 spent 2143 defensive acts against 248 attack acts, 8.6 to 1, while a squad setting out lost a third of itself in transit. Narrowing any one of them widened the next, three rounds of it. The escalation now lives in the action and is explicit: garrison, then unassigned, then anything not on the offensive, and the assault itself only when core infrastructure is actually being hit"
  do defend-base
  require base-under-attack() or critical-building-under-attack()
  require count(near-base-ground-units) > 0
}

rule defend-base-air {
  priority lerp(350, 500, air-defense-priority)
  category air-combat
  do air-defend-base
  require base-under-attack()
  require count(idle-combat-aircraft) > 0
}

rule form-ground-attack {
  priority attack-priority() + 5
  category squad-form
  because "the threshold is how many unassigned idle ground units Vimy ever HAS at one instant, not what a war party ought to number. At six tenths of a group size the strategist pins near eleven, forming demanded six simultaneously idle — and unassigned-idle-ground ran a median of 0 and a maximum of 6 across games 127 and 129, so the gate held in 0 of 1364 sampled states. Not rarely: never. Three tenths asks for three and holds in about one state in ten. The reason idle is always near zero is that defensive rules repossess the army continuously — emergency base defense fired 42 times and recall-stray-units 37 in one game — so a rendezvous condition counted on simultaneity was the wrong shape for this army whatever number it carried"
  do form-squad(ground-attack, Ground, ground-attack-group-size, Attack)
  require (not squad-exists(ground-attack)
           and count(unassigned-idle-ground) >= ground-form-threshold())
       or (squad-needs-reinforcement(ground-attack) and count(unassigned-idle-ground) >= 1)
}

rule squad-attack {
  priority attack-priority()
  category ground-attack-choice exclusive
  because "activation() caps the readiness demand, and the cap comes from the measured distribution rather than from the strategist, which asks for 0.76 to 0.80 every game. squad-ready-ratio counts squad members flagged IDLE, so a squad walking to its target reads as zero ready — which is why game 129's assault phases cycled approach, rally, approach, rally without once reaching strike. Game 127 peaked at 0.40 across 111510 ticks and never launched at all. At 0.5 the gate held in 0 percent of game 127's states and 7 percent of game 129's; at 0.25, 1 and 17 percent. Still tight, and deliberately so: the honest fix is that readiness should not mean standing still, and until it does not, this is a calibration and not a cure"
  do squad-attack-move(ground-attack)
  require squad-exists(ground-attack)
  require squad-ready-ratio(ground-attack) >= round2(activation())
  require exists best-ground-target or exists nearest-enemy
  require defense-floor-holds()
}

rule squad-reengage {
  priority attack-priority() - 2
  category combat
  because "catches stragglers finishing an order while the squad presses forward"
  do squad-attack-move(ground-attack)
  require squad-exists(ground-attack)
  require squad-idle-count(ground-attack) > 0
  require exists best-ground-target or exists nearest-enemy
}

rule squad-attack-known-base {
  priority trunc(select(aggression >= 0.32,
                        attack-priority() + 5,
                        attack-priority() - 10))
  category ground-attack-choice exclusive
  because "aggressive doctrines press the base and let base defenses handle raiders. This threshold has now been wrong in both directions. At 0.3 it sat below anything the strategist chose, so this always won and squad-attack never fired in eighty games — and it was moved to 0.6, which is ABOVE anything the strategist chooses: across 1079 doctrines since game 135 aggression ran 0.15 to 0.50, mean 0.31, and reached 0.6 exactly never. So the base assault was permanently demoted below squad-attack in the same exclusive category, and squad-attack holds whenever any enemy is in sight, which against an infantry swarm is always. Game 148 shows the result: the squad formed well and held together — 5.2 members with 4.4 inside 8 cells — and 76 of 81 transit samples were still FAR from the target, 5 at mid range, and not one within 0.20 of the enemy base across 118920 ticks. squad-attack acted 257 times against this rule's 151. 0.32 is the median of what the strategist actually picks, so roughly 41 percent of doctrines press the base and the rest hold the line: a contest rather than an always or a never"
  do squad-attack-known-base(ground-attack, aggression)
  require squad-exists(ground-attack)
  require squad-ready-ratio(ground-attack) >= round2(activation())
  require has-enemy-intel()
  require defense-floor-holds()
}

rule form-air-attack {
  priority attack-priority() - 5 + 5
  category squad-form
  do form-squad(air-attack, Air, air-attack-group-size, Attack)
  require air-weight > 0.1
  require (not squad-exists(air-attack)
           and count(unassigned-idle-air) >= trunc(max(2.0, air-attack-group-size * 6 / 10)))
       or (squad-needs-reinforcement(air-attack) and count(unassigned-idle-air) >= 1)
}

rule squad-air-attack {
  priority attack-priority() - 5
  category air-combat
  do squad-air-strike(air-attack)
  require air-weight > 0.1
  require squad-exists(air-attack)
  require squad-ready-ratio(air-attack) >= round2(activation())
  require exists best-air-target
}

rule squad-air-reengage {
  priority attack-priority() - 5 - 2
  category air-combat
  do squad-air-strike(air-attack)
  require air-weight > 0.1
  require squad-exists(air-attack)
  require squad-idle-count(air-attack) > 0
  require exists best-air-target
}

rule squad-air-attack-known-base {
  priority attack-priority() - 5 - 10
  category air-combat
  do squad-attack-known-base(air-attack, aggression)
  require air-weight > 0.1
  require squad-exists(air-attack)
  require squad-ready-ratio(air-attack) >= round2(activation())
  require not enemies-visible
  require has-enemy-intel()
}

rule form-naval-attack {
  priority attack-priority() - 15 + 5
  category squad-form
  do form-squad(naval-attack, Naval, naval-attack-group-size, Attack)
  require naval-weight > 0.1
  require map-has-water()
  require (not squad-exists(naval-attack)
           and count(unassigned-idle-naval) >= trunc(max(2.0, naval-attack-group-size * 6 / 10)))
       or (squad-needs-reinforcement(naval-attack) and count(unassigned-idle-naval) >= 1)
}

rule squad-naval-attack {
  priority attack-priority() - 15
  category naval-combat
  do squad-attack-move(naval-attack)
  require naval-weight > 0.1
  require map-has-water()
  require squad-exists(naval-attack)
  require squad-ready-ratio(naval-attack) >= round2(activation())
  require exists nearest-enemy
}

rule squad-naval-reengage {
  priority attack-priority() - 15 - 2
  category naval-combat
  do squad-attack-move(naval-attack)
  require naval-weight > 0.1
  require map-has-water()
  require squad-exists(naval-attack)
  require squad-idle-count(naval-attack) > 0
  require exists nearest-enemy
}

rule squad-naval-attack-known-base {
  priority attack-priority() - 15 - 10
  category naval-combat
  do squad-attack-known-base(naval-attack, aggression)
  require naval-weight > 0.1
  require map-has-water()
  require squad-exists(naval-attack)
  require squad-ready-ratio(naval-attack) >= round2(activation())
  require not enemies-visible
  require has-enemy-intel()
}

rule fire-nuke {
  priority 880
  category superweapon exclusive
  do fire-nuke
  require superweapon-priority > 0.1
  require support-power-ready(NukePowerInfoOrder)
}

rule fire-iron-curtain {
  priority 870
  category superweapon exclusive
  do fire-iron-curtain
  require superweapon-priority > 0.1
  require support-power-ready(GrantExternalConditionPowerInfoOrder)
  require count(idle-ground-units) >= 3
}

rule fire-spy-plane {
  priority 860
  category superweapon exclusive
  do fire-spy-plane
  require air-weight > 0.1
  require support-power-ready(SovietSpyPlane)
  require not has-enemy-intel()
}

rule fire-spy-plane-update {
  priority 250
  category superweapon exclusive
  do fire-spy-plane
  require air-weight > 0.1
  require support-power-ready(SovietSpyPlane)
  require has-enemy-intel()
  require not enemies-visible
}

rule fire-paratroopers {
  priority 855
  category superweapon exclusive
  do fire-paratroopers
  require air-weight > 0.1
  require support-power-ready(SovietParatroopers)
  require has-enemy-intel() or enemies-visible
}

rule fire-parabombs {
  priority 845
  category superweapon exclusive
  do fire-parabombs
  require air-weight > 0.1
  require support-power-ready(UkraineParabombs)
  require has-enemy-intel() or enemies-visible
}
