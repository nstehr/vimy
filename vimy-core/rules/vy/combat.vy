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
  because "the group size is a FLOOR now, not the target. It is a constant — 5 or 6 in every game the strategist has written — while the army runs from 7 combat units to 27 at peak, so as a target it is a shrinking fraction of the army, and FormSquad set TargetSize once at formation and never revisited it. squadTarget now computes the real number from the committable force on every call: every combat ground unit not rostered to another squad, which is already the garrison. Game 150 is what the constant cost — the squad formed at a full 6, held formation, reached 0.143 of the map diagonal, and arrived as TWO units against 16 rocket soldiers. Scaling the knob by force-scale() was tried first and was the same mistake this session keeps finding: force-size is what the doctrine WANTS, and a constant folded at compile time cannot track an army that grows. The forming threshold stays on the raw knob and stays at 2, because unassigned-idle-ground runs a median of 0 and a maximum of 6 — waiting for a dozen simultaneously idle units would mean never forming at all"
  do form-squad(ground-attack, Ground, ground-attack-group-size, Attack)
  require (not squad-exists(ground-attack)
           and count(unassigned-idle-ground) >= ground-form-threshold())
       or (squad-needs-reinforcement(ground-attack) and count(unassigned-idle-ground) >= 1)
}

rule squad-attack {
  priority attack-priority()
  category ground-attack-choice exclusive
  because "squad-ready-ratio is living members over TargetSize now, so a squad under orders no longer reads as zero ready and a remnant no longer reads as one. That inversion is why activation() carried a clamp: it counted members flagged IDLE, Idle means no current order, and a squad walking to its target scored zero — game 129 cycled approach, rally, approach, rally without reaching strike, and game 127 peaked at 0.40 across 111510 ticks and never launched. The clamp was applied twice, to 0.5 and then 0.25, and this note called it a calibration and not a cure. The cure is in the measure, so the clamp is gone and commit-ratio binds again: the strategist asks for 0.70 every game and the prompt tells it to ask 0.6-0.8 against a fortified base, all of which was being discarded. Game 158 is what the stopgap cost — the squad decayed 15 to 6 to 2 to 1 and kept walking into the enemy base, army peak 5000 against game 155's 11800, because a remnant of one scored full marks. The default band drops from 0.6-1.0 to 0.4-0.8 because it now measures something reachable: presence, not stillness"
  do squad-attack-move(ground-attack)
  require squad-exists(ground-attack)
  require squad-ready-ratio(ground-attack) >= round2(activation())
  require exists best-ground-target or exists nearest-enemy
  require defense-floor-holds()
}

rule squad-reengage {
  priority attack-priority() - 2
  category combat
  because "catches stragglers finishing an order while the squad presses forward, and now commands ONLY them. It used squad-attack-move, which is the assault action and moves every member, aimed at bestTargetForSquad rather than the base the assault was marching on — so one idle straggler redirected the whole army. This rule is category combat, not the exclusive ground-attack-choice, so it acted alongside the exclusive winner instead of competing with it: two rules steering the same units at two different targets, every tick. Game 159 sawtoothed 51 units across 0.3 of the map diagonal, 38 cells forward and 38 back, in front of the enemy base for thousands of ticks without closing, at 210 base-attack fires against 104 of these. Stragglers are now steered at the squad's own centroid, because rejoining is the job"
  do squad-nudge-stragglers(ground-attack)
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
