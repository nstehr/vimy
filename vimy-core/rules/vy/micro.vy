param aggression: float
param economy-priority: float
param scout-priority: float
param commit-ratio: float
param ground-attack-group-size: int

def retreat-threshold() = lerpf(0.5, 0.15, aggression)
def retreat-priority() = lerp(380, 450, 1.0 - aggression)
def leash() = lerpf(0.25, 0.5, aggression)

def harvester-danger() = round2(lerpf(0.025, 0.05, economy-priority))

rule retreat-damaged-units {
  priority retreat-priority()
  category micro
  do retreat-damaged-units(retreat-threshold())
  require count(damaged-combat-units(round2(retreat-threshold()))) > 0
}

rule clear-healed-units {
  priority 500
  category micro
  because "runs every tick so healed units rejoin the combat pool promptly"
  do clear-healed-units(retreat-threshold())
  require has-retreating-units()
}

rule recall-overextended-ground-attack {
  priority retreat-priority() - 10
  category micro
  because "aggressive doctrines let units roam further before the leash pulls"
  do recall-overextended(ground-attack, leash())
  require squad-exists(ground-attack)
  require count(overextended-squad-members(ground-attack, round2(leash()))) > 0
}

rule recall-overextended-naval-attack {
  priority retreat-priority() - 11
  because "below its ground mirror, so the two do not tie on every doctrine"
  category micro
  do recall-overextended(naval-attack, leash())
  require squad-exists(naval-attack)
  require count(overextended-squad-members(naval-attack, round2(leash()))) > 0
}

rule squad-disengage-ground-attack {
  priority retreat-priority() - 5
  category micro
  because "pure aggression never disengages, so this is absent at aggression 1"
  do squad-disengage(ground-attack)
  require aggression < 1.0
  require squad-exists(ground-attack)
  require squad-away-from-base(ground-attack, 0.1)
  require squad-threat-ratio(ground-attack, 0.1) > round2(lerpf(1.5, 3.0, aggression))
}

rule squad-disengage-naval-attack {
  priority retreat-priority() - 6
  because "below its ground mirror, so the two do not tie on every doctrine"
  category micro
  do squad-disengage(naval-attack)
  require aggression < 1.0
  require squad-exists(naval-attack)
  require squad-away-from-base(naval-attack, 0.1)
  require squad-threat-ratio(naval-attack, 0.1) > round2(lerpf(1.5, 3.0, aggression))
}

rule squad-focus-fire {
  priority lerp(200, 360, aggression) + 1
  category micro
  because "capped below the retreat band: keeping a unit alive outranks improving what it shoots at"
  do squad-focus-fire(ground-attack)
  require aggression > 0.2
  require squad-exists(ground-attack)
  require squad-ready-ratio(ground-attack) >= round2(activation())
  require exists best-ground-target
}

rule flee-harvesters {
  priority lerp(150, 300, economy-priority)
  category micro
  because "the radius is what a harvester can be SHOT from, not what it can see. At lerpf(0.05, 0.15, ..) an economy doctrine fled anything within 0.13 of the map diagonal — 24 cells on a 128 map, a quarter of the width — and game 129 fled 436 times in 57340 ticks, once every 130 ticks, against only 121 orders sent to resume harvesting. It kept 11 harvesters and 9 refineries alive and earned 1.68 credits a tick, a fraction of what that infrastructure should return, because the harvesters spent the game running rather than hauling. Vimy was broke for it: the war factory stood idle 47 percent of the game and could afford a tank in only 31 percent of it, so the army never compounded past 6 vehicles. Ground weapons reach 4 to 7 cells, so 0.045 of the diagonal is about 8 cells on a 128 map — close enough to be in real danger, far enough to get clear. Losing a harvester sometimes is the price, and a harvester is 1100 against an economy this was costing far more than that"
  do flee-harvesters(harvester-danger())
  require economy-priority > 0.1
  require enemies-visible
  require count(harvesters-in-danger(round2(harvester-danger()))) > 0
}

rule scout-with-scouts {
  priority lerp(250, 400, scout-priority) + 5
  category recon
  because "rangers keep patrolling so intel stays fresh rather than going stale"
  do scout-patrol
  require count(idle-scouts) > 0
}

rule scout-with-idle-units {
  priority lerp(250, 400, scout-priority)
  category recon
  because "two idle units, because the action sends at most two — it used to ask for a whole attack group, so the only path to early intel waited on six spare units the squads were consuming, and game 83 saw nothing at all until tick 13250 of 24020"
  do scout
  require not enemies-visible
  require not has-enemy-intel()
  require count(unassigned-idle-ground) >= 2
}

rule scramble-to-harvesters {
  priority lerp(362, 432, economy-priority)
  category harvester-defense
  because "the built-in AI lists harvesters first in ProtectionTypes and answers a raid out of its general squad pool, paying nothing until something is attacked. Vimy pre-committed a harvester-guard squad instead, reserved so the attack rules could not poach it back, to cover six harvesters at separate ore patches: the one win saw 26 flee events against 97 and 126 in the losses either side. This replaced it and then subsumed it — the compiler found guard-harvesters could never fire, since this sits above it in the same exclusive category on strictly weaker conditions, so the squad had no consumer left and form-harvester-guard was sequestering 2-4 ground units from an army already outvalued 2:1. Both are gone. This pulls the NEAREST combat units from the whole army and holds them only long enough to arrive and fight, so whatever squad they came from reclaims them when the hold lapses"
  do scramble-to-harvesters(harvester-danger(), lerp(2, 8, economy-priority))
  require economy-priority > 0.3
  require count(harvesters-in-danger(round2(harvester-danger()))) > 0
}
