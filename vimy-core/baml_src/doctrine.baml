class TypeCount {
  type string @description("Internal type code, e.g. 'fact', '3tnk', 'e1'")
  count int
}

class PowerStatus {
  drained int
  provided int
  state string @description("Normal, Low, or Critical")
}

class ActiveProduction {
  queue string  @description("Queue type: Building, Defense, Vehicle, Infantry, Ship, Aircraft")
  item string   @description("Item currently being produced")
  progress int  @description("Completion percentage 0-100")
}

class SupportPowerStatus {
  key string    @description("e.g. 'NukeReady', 'IronCurtain'")
  status string @description("'READY', 'charging', or percentage like '75%'")
}

class SuperweaponFire {
  key string
  total_fires int
  recent_fires int @description("Fires since last doctrine evaluation")
}

class SquadInfo {
  name string
  role string @description("'attack', 'defend', or 'scout'")
  unit_count int
}

class EnemyBase {
  owner string
  x int
  y int
  last_seen_tick int
}

class GameEvent {
  kind string   @description("e.g. 'critical_building_lost', 'strategy_countered'")
  tick int
  detail string @description("Human-readable description of what happened")
}

class RecentDoctrine {
  tick int
  name string @description("Doctrine name from the prior attempt")
  shape string @description("Terse shape fingerprint, e.g. 'air=0.75 vehicle=0.4 ground_def=0.35 aggression=0.8'")
}

class CombatStats {
  infantry_lost int @description("Total infantry killed this game")
  vehicles_lost int @description("Total vehicles lost this game")
  aircraft_lost int @description("Total aircraft lost this game")
  naval_lost int @description("Total naval units lost this game")
}

class GameSituation {
  tick int
  phase string @description("'Early Game', 'Mid Game', or 'Late Game'")
  cash int
  resources int
  resource_capacity int
  power PowerStatus
  buildings TypeCount[]
  units TypeCount[]
  idle_unit_count int
  active_production ActiveProduction[]
  support_powers SupportPowerStatus[]
  superweapon_fires SuperweaponFire[]
  squads SquadInfo[]
  enemies_visible int
  enemy_units TypeCount[] @description("Currently visible enemy units grouped by type code")
  enemy_units_seen TypeCount[] @description("All enemy unit types ever observed, cumulative counts")
  enemy_buildings TypeCount[] @description("Currently visible enemy buildings grouped by type code")
  enemy_buildings_seen TypeCount[] @description("All enemy building types ever observed, cumulative counts")
  known_enemy_bases EnemyBase[]
  map_width int
  map_height int
  recent_events GameEvent[]
  combat_stats CombatStats | null
  recent_doctrines RecentDoctrine[] @description("Your OWN recent doctrine attempts this game, newest last. Used to detect and break out of convergent-shape loops.")
  burned_axes string[] @description("Axes (e.g. 'air', 'infantry') that have been pivoted to 2+ times this match AND each pivot was followed within a few doctrines by domain-specific counter events (aircraft losses, infantry losses to flame, etc.). Treat these as BURNED — do not pivot to them again for the rest of the match. Computed from full doctrine + event history, not just the recent_doctrines window.")
  capturables_visible TypeCount[] @description("Neutral capturable buildings currently visible on the map — e.g. oilb (oil derrick, passive income), fcom (forward command), miss (comm center), hosp (hospital). Their presence is a direct signal to raise capture_priority >= 0.3 so engineers/transports actually grab them.")
  being_rushed bool @description("True when early-game (tick < ~6000) AND base is still small (<= 6 buildings) AND harvesters have been attacked recently. Means the enemy is pushing units into our base before we have a perimeter. Triggers the RUSH RESPONSE prompt rule — counter-force, not turtle.")
  harvester_harassed bool @description("True when mid/late-game (tick >= ~6000) AND base is established (>= 8 buildings) AND there have been 2+ harvester_under_attack or 1+ harvester_lost events in the last ~2000 ticks. Triggers the HARVESTER HARASSMENT prompt rule — escorts, dispersal, defenses near refineries.")
  ground_squad_ready_ratio float @description("Current fraction (0.0-1.0) of the ground-attack squad's target size that is currently idle and ready to deploy. E.g. 0.5 means half the target squad is present and ready. Compare against your intended commit_ratio: if ready_ratio < commit_ratio, the squad won't deploy this tick — either wait, lower commit_ratio, or reduce ground_attack_group_size.")
  cash_burn_rate int @description("Net cash change over the last evaluation interval (positive = income exceeds spending, negative = losing cash). Signals whether current build order is sustainable. If negative and persistent, either economy is under attack, repair_budget_ratio is too high, or production caps are exceeded — respond by tightening repair or throttling production.")
  faction_side string @description("Which side your faction fights for: \"allied\" or \"soviet\". Read this rather than inferring it from the faction name — the roster lists below are keyed by side, not by country.")
  unbuildable_roles string[] @description("Roles your faction CANNOT build. Naming any of these is a wasted slot: it is dropped before production sees it, while still moving rule priorities on the strength of the name. Pick the equivalent from your own side instead — e.g. an Allied army wanting long-range ground fire takes artillery, not v2_launcher.")
  time_to_reach_enemy_estimate int @description("Rough tick-count estimate for a ground unit to walk from our base centroid to a known enemy base position. Positive when enemy_base_discovered has fired. -1 when no intel exists. Useful for planning commit_tick decisions: 'if reach estimate is 4000 and I want to attack by tick 8000, I should deploy by tick 4000.'")
}

class Doctrine {
  name string @description("Short name for this doctrine, e.g. 'Blitzkrieg', 'Turtle Defense', 'Guerrilla Raids'")
  rationale string @description("Brief explanation of why this doctrine fits the current situation and directive")
  economy_priority float @description("0.0-1.0: investment in refineries and harvesters")
  aggression float @description("0.0-1.0: offensive commitment level")
  ground_defense_priority float @description("0.0-1.0: ground base defense urgency — controls pillbox/turret/tesla caps, ground unit scramble threshold when base is attacked, and minelayer production for area denial (above 0.3)")
  air_defense_priority float @description("0.0-1.0: anti-air defense urgency — controls AA structure caps and aircraft scramble priority when base is attacked")
  tech_priority float @description("0.0-1.0: investment in tech buildings")
  infantry_weight float @description("0.0-1.0: infantry production preference")
  vehicle_weight float @description("0.0-1.0: vehicle production preference")
  air_weight float @description("0.0-1.0: air unit production preference")
  naval_weight float @description("0.0-1.0: naval unit production preference")
  ground_attack_group_size int @description("Minimum ground units (infantry+vehicles) before launching a ground attack (3-15)")
  air_attack_group_size int @description("Minimum combat aircraft before launching an air strike (1-8)")
  naval_attack_group_size int @description("Minimum naval units before launching a naval attack (2-10)")
  scout_priority float @description("0.0-1.0: reconnaissance investment")
  specialized_infantry_weight float @description("0.0-1.0: investment in elite infantry (flamethrowers, shock troopers, Tanya, medics) — requires prerequisite buildings already built via other priorities")
  superweapon_priority float @description("0.0-1.0: investment in superweapon buildings (Missile Silo, Iron Curtain) and eagerness to use them — requires tech center already built via tech_priority. Set above 0.3 to build superweapon structures. Airfield powers (spy plane, paratroopers) are governed by air_weight instead")
  capture_priority float @description("0.0-1.0: eagerness to capture neutral tech buildings with engineers — controls engineer and APC/ranger production. Set above 0.1 to produce engineers, above 0.3 to also produce transport vehicles for delivery. Soviet uses APCs (apc role), Allies use Rangers (ranger role). Set to 0 for pure defense doctrines that shouldn't waste the Infantry queue on engineers")
  transport_assault float @description("0.0-1.0: rush infantry into the enemy base using transport vehicles. Soviet uses APCs (apc role, carries 5 infantry), Allies use Rangers (ranger role, carries 1 infantry). Requires war factory and infantry production. Pair with preferred_infantry to choose which troops to load. Set above 0.1 to produce assault transports. Higher = more transports (up to 3). Set to 0 for standard play without transport rushes")
  preferred_infantry string[] @description("Ordered list of infantry roles to prioritize. Soviet: flamethrower, shock_trooper, medic, engineer. Allied: tanya, medic, engineer. Both: rocket_soldier. Only list roles available to your faction. Empty = default priority.")
  preferred_vehicle string[] @description("Ordered list of vehicle roles to prioritize. Soviet: heavy_tank, medium_tank, tesla_tank, v2_launcher, flak_truck, demo_truck, apc. Allied: medium_tank, light_tank, artillery, ranger. IMPORTANT: apc is Soviet-only, ranger is Allied-only. Only list roles available to your faction. Empty = default priority.")
  preferred_aircraft string[] @description("Ordered list of aircraft roles to prioritize. Soviet: basic_aircraft (Yak/MiG), advanced_aircraft (MiG/Hind). Allied: basic_aircraft (Black Hawk), advanced_aircraft (Longbow). Empty = default priority.")
  preferred_naval string[] @description("Ordered list of naval roles to prioritize. Soviet: submarine, missile_sub (requires tech center, can attack land targets). Allied: gunboat, destroyer, cruiser. Empty = default priority.")
  ground_target_aa_priority float @description("0.0-1.0: how aggressively ground forces should target enemy AA defenses (AA guns, SAM sites) to protect your aircraft. Set high when relying on air power. 0 = default targeting, 1 = AA becomes top ground priority")
  air_target_ground_def_priority float @description("0.0-1.0: how aggressively air forces should target enemy ground defenses (pillboxes, turrets, tesla coils, flame towers) to clear the path for your ground troops. Set high when relying on infantry/vehicle pushes. 0 = default targeting, 1 = ground defenses become top air priority")
  commit_ratio float @description("0.0-1.0: fraction of the ground-attack squad that must be ready (idle) before deploying. 0 = disabled (use system default ~0.5). Directly controls whether squad-attack / squad-attack-known-base fires. IMPORTANT: this knob interacts with what you're attacking. Rush vs unfortified enemy (no enemy_buildings_seen listing pillbox/turret/tesla/flame_tower, or you got there before defenses came online): set 0.3-0.4 for fast partial-squad pressure. Rush vs FORTIFIED enemy (enemy_buildings_seen shows pillbox/tesla/flame_tower/turret/sam/agun): set 0.6-0.8 — small waves die piecemeal on the pillbox line, you need overwhelming mass per wave to break through. Turtle/careful doctrines: set 0.8-0.9 (full squad or nothing). Match your setting to what you're actually pushing INTO — a 2-rifle wave vs a pillbox row wastes the rifles.")
  base_defense_floor int @description("0-15: minimum number of static ground defenses (pillbox + camo_pillbox + turret + flame_tower + tesla_coil, summed) that must exist before offensive squad rules fire. 0 = disabled (attack whenever ready). A turtle sets 6-8, a rush sets 0-1, a balanced push sets 2-3. Prevents committing forward while the base is naked; if the floor isn't met, the squad stays home until enough defenses are built.")
  repair_budget_ratio float @description("0.0-1.0: cap on repair spending as a fraction of cash on hand. 0 = disabled (unlimited repair — current behavior). 1.0 = repair spend not capped. 0.2 = only repair if we can pay for repairs with 20% of cash, keeping 80% for production. Aggressive doctrines set 0.2-0.4 to preserve production cash; defensive doctrines can leave at 0 or 1.0.")
  scout_reach_priority float @description("0.0-1.0: how aggressively scouts probe toward the known/likely enemy position vs. passive perimeter patrol. 0 = disabled (default perimeter patrol). 1.0 = probe hard toward last-known enemy contact direction. Rush doctrines need enemy base found FAST — set 0.7-0.9. Turtle doctrines don't care where the enemy is — leave at 0. Balanced doctrines 0.4-0.5.")
}

class PastDoctrineExample {
  context string @description("Brief matchup summary, e.g. 'vs allies on small map, won — rating: strong'")
  rationale string @description("The doctrine's original rationale text")
  doctrine_summary string @description("1-2 line tight summary of key fields, e.g. 'aggression=0.8, vehicle=0.7, scout=0.6'")
}

class CautionaryPattern {
  context string @description("Matchup and outcome, e.g. 'vs england, lost'")
  failed_pattern string @description("Terse description of the doctrine SHAPE that failed, e.g. 'aggression=0.85 + infantry=0.75 + low ground_def'")
  failure_reason string @description("Why this pattern failed — the reviewer's judgment. NOT the original rationale (which would seduce the reader into repeating the mistake).")
}

class MemoryLesson {
  trigger string
  guidance string
}

class MemoryContext {
  exemplar_doctrines PastDoctrineExample[]
  cautionary_patterns CautionaryPattern[]
  lessons MemoryLesson[]
}

function GenerateDoctrine(directive: string, situation: GameSituation, faction: string, memory: MemoryContext | null) -> Doctrine {
  client CustomGPT5Mini
  prompt #"
    You are a military strategist AI for Command & Conquer: Red Alert, a classic real-time strategy game.

    You control the {{ faction }} faction, which fights for the {{ situation.faction_side }} side.
    {% if situation.unbuildable_roles %}
    You CANNOT build these roles: {% for r in situation.unbuildable_roles %}{{ r }}{% if not loop.last %}, {% endif %}{% endfor %}.
    Never list one in preferred_infantry, preferred_vehicle, preferred_aircraft or preferred_naval. If your plan calls for
    one, substitute your own side's equivalent so the plan survives — a doctrine whose central unit is dropped keeps every
    weight that assumed it and pursues the same aims with the wrong tools.
    {% endif %} Your directive from high command is: "{{ directive }}"

    Current battlefield situation:
    Tick: {{ situation.tick }} | Phase: {{ situation.phase }}
    Cash: {{ situation.cash }} | Resources: {{ situation.resources }}/{{ situation.resource_capacity }}
    Power: {{ situation.power.drained }}/{{ situation.power.provided }} ({{ situation.power.state }})

    {% if situation.buildings %}
    Buildings:{% for b in situation.buildings %} {{ b.count }}x {{ b.type }}{% endfor %}
    {% endif %}

    {% if situation.units %}
    Units:{% for u in situation.units %} {{ u.count }}x {{ u.type }}{% endfor %} ({{ situation.idle_unit_count }} idle)
    {% endif %}

    {% for pq in situation.active_production %}
    Queue {{ pq.queue }}: producing {{ pq.item }} ({{ pq.progress }}%)
    {% endfor %}

    {% if situation.support_powers %}
    Support Powers:{% for sp in situation.support_powers %} {{ sp.key }}({{ sp.status }}){% endfor %}
    {% endif %}

    {% if situation.superweapon_fires %}
    Superweapon launches:{% for sw in situation.superweapon_fires %} {{ sw.key }}={{ sw.total_fires }}{% if sw.recent_fires > 0 %} (+{{ sw.recent_fires }} since last eval){% endif %}{% endfor %}
    {% endif %}

    {% if situation.squads %}
    Squads:{% for sq in situation.squads %} {{ sq.name }}({{ sq.role }}, {{ sq.unit_count }} units){% endfor %}
    {% endif %}

    Enemies visible: {{ situation.enemies_visible }}
    {% if situation.enemy_units %}
    Enemy units (visible now):{% for u in situation.enemy_units %} {{ u.count }}x {{ u.type }}{% endfor %}
    {% endif %}
    {% if situation.enemy_units_seen %}
    Enemy units (all observed):{% for u in situation.enemy_units_seen %} {{ u.count }}x {{ u.type }}{% endfor %}
    {% endif %}
    {% if situation.enemy_buildings %}
    Enemy buildings (visible now):{% for b in situation.enemy_buildings %} {{ b.count }}x {{ b.type }}{% endfor %}
    {% endif %}
    {% if situation.enemy_buildings_seen %}
    Enemy buildings (all observed):{% for b in situation.enemy_buildings_seen %} {{ b.count }}x {{ b.type }}{% endfor %}
    {% endif %}

    {% if situation.capturables_visible %}
    Capturable neutral buildings visible:{% for c in situation.capturables_visible %} {{ c.count }}x {{ c.type }}{% endfor %} — raise capture_priority >= 0.3 to build engineers (and transport_assault > 0 for APC/ranger delivery) to take them.
    {% endif %}

    {% if situation.known_enemy_bases %}
    {% for eb in situation.known_enemy_bases %}
    Known enemy base [{{ eb.owner }}]: ({{ eb.x }}, {{ eb.y }}) last seen tick {{ eb.last_seen_tick }}
    {% endfor %}
    {% else %}
    Known enemy bases: none (not yet scouted)
    {% endif %}

    Map: {{ situation.map_width }}x{{ situation.map_height }}

    {% if situation.recent_events %}
    Recent Events:
    {% for e in situation.recent_events %}
    - [tick {{ e.tick }}] {{ e.kind }}: {{ e.detail }}
    {% endfor %}
    {% endif %}

    {% if situation.recent_doctrines %}
    YOUR RECENT DOCTRINE ATTEMPTS THIS GAME (newest last):
    {% for rd in situation.recent_doctrines %}
    - [tick {{ rd.tick }}] {{ rd.name }} — {{ rd.shape }}
    {% endfor %}
    {% endif %}

    {% if situation.combat_stats %}
    Combat Stats (lifetime): Lost {{ situation.combat_stats.infantry_lost }} infantry, {{ situation.combat_stats.vehicles_lost }} vehicles, {{ situation.combat_stats.aircraft_lost }} aircraft, {{ situation.combat_stats.naval_lost }} naval
    {% endif %}

    {% if memory %}
    {% if memory.lessons %}
    LESSONS FROM PRIOR GAMES (advisory — current battlefield overrides):
    {% for l in memory.lessons %}
    - When {{ l.trigger }}: {{ l.guidance }}
    {% endfor %}
    {% endif %}

    {% if memory.exemplar_doctrines %}
    EXAMPLES OF DOCTRINES THAT WORKED IN SIMILAR MATCHUPS:
    {% for d in memory.exemplar_doctrines %}
    - ({{ d.context }}) {{ d.doctrine_summary }}
      Rationale: {{ d.rationale }}
    {% endfor %}
    {% endif %}

    {% if memory.cautionary_patterns %}
    PATTERNS THAT FAILED IN SIMILAR MATCHUPS — HARD CONSTRAINTS, NOT ADVICE:
    {% for c in memory.cautionary_patterns %}
    - {{ c.context }}: {{ c.failed_pattern }}
      Why it failed: {{ c.failure_reason }}
    {% endfor %}

    ENFORCEMENT RULES for cautionary patterns (these override the permissive
    tone elsewhere in this prompt):

    1. A cautionary pattern is a PROHIBITION, not a warning. If it mentions
       a tactic or doctrine shape, the fields implementing that tactic must
       be zeroed or strongly de-emphasized in your output — UNLESS the
       current directive explicitly names that tactic as required.

    2. Partial reproduction is still reproduction. Adding your directive's
       intent ON TOP of the failed shape does NOT satisfy the constraint.
       "The swarm I was told to do, PLUS the APC-engineer raiding from the
       cautionary" is still reproducing the cautionary. You must REPLACE
       the failed tactic, not blend it in.

    3. Concrete field mappings (apply when a cautionary warns against the
       corresponding tactic AND the current directive does not require it):
       - Transport/APC-drop tactics warned against → transport_assault = 0,
         capture_priority < 0.2, do NOT list "apc" in preferred_vehicle.
       - Engineer-capture tactics warned against → capture_priority = 0,
         do NOT list "engineer" as preferred_infantry[0].
       - Air-heavy openings warned against → air_weight < 0.15.
       - Turtle/base-camping warned against → aggression > 0.6,
         ground_defense_priority < 0.5.

    4. If you find yourself wanting to include a tactic that's under a
       cautionary, and the directive does NOT require it, the correct
       response is to leave that field at its neutral default (0 or its
       baseline) and compensate with tactics the directive DOES mention.
    {% endif %}

    Treat EXEMPLAR examples as advisory. Treat CAUTIONARY patterns as hard
    constraints per the rules above. Current battlefield conditions override
    exemplar patterns.
    {% endif %}

    CRITICAL ADAPTATION RULES:
    - Your directive is STRATEGIC INTENT, not a rigid constraint. Adapt to battlefield reality.
    - If recent_events show "strategy_countered", you MUST change your doctrine — do not repeat the same failing strategy.
    - If combat_stats show sustained high losses in one domain (e.g., 20+ infantry lost), that domain is being hard-countered.
    - When infantry dies to static defenses (tesla coils, flame towers), shift weight to vehicles (0.5+) or air (0.3+) to break through, OR increase ground_attack_group_size to 10+ to overwhelm with numbers.
    - When vehicles are countered by tesla coils, shift to air strikes or build up tech for mammoth tanks.
    - CAUTIONARY-AWARE OPENER: This rule applies ONLY to the opening doctrine of a match (situation.tick is small — roughly under 5000 — AND recent_doctrines is empty or has fewer than 2 entries). When cautionary_patterns are present whose failure_reason or failed_pattern explicitly mentions base raids, harvester harassment / harvester_lost, critical_building_lost, or low ground_def correlated with infrastructure damage — that opponent has a demonstrated raid game. The opener MUST set ground_defense_priority >= 0.6 and economy_priority >= 0.6, even if the directive emphasizes aggression. The directive's offensive intent can resume in subsequent doctrines once the base is hardened. This is not a generic default — only fires when the cautionaries you were given specifically warn about base/economy harassment for this matchup. Without this, by the time reactive evals raise ground_def, the first critical building has already been lost (game 20 vs russia: opener gd=0.35, lost critical building at tick 16510 despite 4 cautionaries warning about exactly this pattern).
    - CONVERGENCE BREAK: If recent_doctrines shows 4+ attempts with a similar dominant axis (e.g., air_weight >= 0.55 repeatedly) AND recent_events include 'strategy_countered' or repeated harvester_under_attack / critical_building_lost, the repeated shape is failing. You MUST pick a doctrine whose DOMINANT axis differs — e.g., drop the previously-dominant weight to <= 0.25 and raise a different axis (vehicle_weight or ground_defense_priority) to > 0.5. Rationale-on-top pivots ("still air-heavy, but now with more ground") do NOT satisfy this rule. The goal is a structurally different plan, not a reskin. The directive is strategic intent, not a prohibition on pivoting — if the directive's preferred approach is being hard-countered, the correct response is to pivot and prove the counter wrong by a different route. Explicitly note the pivot in your rationale.
    - STICKY PIVOT: A pivot is only useful if it STICKS. If recent_doctrines shows that you recently pivoted (your previous doctrine had a substantially different dominant axis than the one before it — e.g., naval dropped from 0.60 to 0.25 while vehicle rose above 0.5) AND recent_events STILL contain counter signals (strategy_countered, harvester_under_attack, critical_building_lost within the last few entries — or the underlying condition hasn't visibly resolved, e.g., harvester count hasn't recovered, no new base kills), the battlefield hasn't stabilized yet. You MUST maintain the pivoted shape (keep the formerly-dominant axis <= 0.30 and the new emphasis above 0.5) rather than reverting to the directive's preferred shape. Only revert once counter events have been absent for at least 2-3 consecutive evaluations AND no new economic/infrastructure losses have occurred. Reverting too soon is how a one-shot pivot fails — treat the pivot like a committed build order, not a hedged gesture.
    - BURNED AXIS: situation.burned_axes lists axes that have been pivoted to 2+ times this match AND each was followed by domain-specific counter events (aircraft losses to SAM/Flak; infantry losses to flame/Tesla; vehicle pushes followed by army_devastated). These are HARD CONSTRAINTS — do not raise the corresponding weight above 0.25 for the rest of the match, regardless of directive. Air burned means air_weight <= 0.25 (and don't list aircraft as preferred). Infantry burned means infantry_weight <= 0.25. Vehicle burned means vehicle_weight <= 0.25 (lean on infantry/air/naval/siege instead). Pick a non-burned axis as your dominant force. If recent_doctrines also shows a recent attempt on a burned axis, that's evidence the rule has already been violated — do not repeat. Note explicitly in rationale which axis is burned and the alternative you chose.
    - RUSH RESPONSE: When situation.being_rushed is true, the enemy is pushing units into a base that doesn't yet have a perimeter. Harvesters under attack here is NORMAL, not an emergency — turtling won't fix it. The right response is COUNTER-FORCE: kill the rushers before they kill the economy. Hard constraints when this flag is true: infantry_weight >= 0.5 (rifles + dogs spam quickly for body count), ground_defense_priority >= 0.6 (one or two pillboxes at the threatened side, not a fortress), aggression in the 0.4-0.6 range (counter-attack the rushers when you have numbers; do NOT drop to 0 and turtle), capture_priority = 0, transport_assault = 0, no expensive tech detours (tech_priority <= 0.3, superweapon_priority = 0). preferred_infantry MUST start with cheap defenders — Soviet: ["rifleman", "attack_dog", "rocket_soldier"], Allied: ["rifleman", "attack_dog", "rocket_soldier"]. Hold this shape for 2-3 evals or until being_rushed clears (base grows past 6 buildings or tick >= 6000). The intent is to survive the opening AND retain the ability to push back, not to fortify.
    - HARVESTER HARASSMENT: When situation.harvester_harassed is true (mid/late-game, established base, sustained harvester losses), the enemy is running raid harassment against your economy. Different from a rush — you have a base, but it's leaking. Hard constraints when true: ground_defense_priority >= 0.7 (build pillboxes/turrets near refineries, not just at the front), economy_priority >= 0.7 (replace lost harvesters and consider a 3rd refinery for dispersed harvesting), capture_priority <= 0.2, transport_assault <= 0.1 (don't open new fronts while bleeding at home). Aggression can stay moderate (0.4-0.6) to keep counter-pressure. Keep this shape until harvester_under_attack and harvester_lost events have been absent for 2+ evals.
    - ANTI-OSCILLATION: Pivoting is good. Whip-sawing between two shapes is not. Examine recent_doctrines for a flip pattern — e.g., infantry-anchor → vehicle-siege → infantry-anchor → vehicle-siege, or any A→B→A→B alternation across the last 4 doctrines where each adjacent pair has substantially different dominant axes. This pattern produces incoherent force composition: you spend on tanks, then infantry, then tanks again, and never accumulate enough of either to win an engagement (army_devastated events are the typical downstream signal). If you detect this pattern, you MUST commit to ONE of the modes for at least 3 consecutive evaluations even if fresh counter events are present. Pick the mode whose primary units are cheaper, faster to mass, or already partially built (check current Units/Buildings). Sustained mediocre execution beats oscillating commitment. Do NOT introduce a third shape (air pivot, etc.) to "break the cycle" — that's just oscillating across more axes. Pick A or B, hold it.
    - When the enemy clusters AA (sam, agun) alongside ground defenses (pbox, gun, ftur, tsla) so that air can't clear the AA without being shot down and ground can't close on the defenses without dying — stand off with long-range siege (v2_launcher/artillery) that outranges both. Signals: high aircraft_lost with repeated first_contact events at the enemy base, or sustained inability to reduce enemy defense counts despite air+ground pressure. Add v2_launcher / artillery to preferred_vehicle and raise tech_priority so they become available.
    - High losses with no progress = strategy failing, adapt immediately. Low losses = strategy working, maintain course.

    Based on the directive and current situation, produce a strategic doctrine.
    All weight values must be between 0.0 and 1.0.
    ground_attack_group_size must be between 3 and 15.
    air_attack_group_size must be between 1 and 8.
    naval_attack_group_size must be between 2 and 10.

    Key considerations:
    - Economy (refineries/harvesters) fuels everything — neglect it and you stall
    - Infantry is cheap and fast to produce, good for early pressure and scouting
    - Vehicles are expensive but powerful — tanks dominate mid-game
    - Air units require an airfield and are expensive but bypass ground defenses
    - Naval units require a naval yard and water on the map — set to 0 if no water
    - ground_defense_priority controls ground base defense: pillbox/turret/tesla caps, how quickly ground units scramble to defend (high = scramble even 1 unit, low = wait for 3+), and minelayer production above 0.3 for laying mines between your base and the enemy. Always set above 0
    - air_defense_priority controls AA structures (SAM/AA gun caps) and how eagerly aircraft scramble to defend the base. Can be independent of ground defense
    - Aggression controls how eagerly you attack; low = defensive turtle, high = constant pressure
    - ground_attack_group_size: smaller = faster but riskier ground attacks, larger = slower but more decisive
    - air_attack_group_size: how many aircraft to accumulate before striking (1 = harass constantly, 8 = decisive air raids)
    - naval_attack_group_size: how many ships to accumulate before attacking
    - Scout priority: higher = more reconnaissance, important when enemy position unknown
    - Specialized infantry (flamethrowers, commandos, medics) require prerequisite structures — set above 0 only when those are likely available
    - Superweapon priority controls building the Missile Silo (nuke) and Iron Curtain — both require a tech center. Set above 0.3 to invest. Airfield support powers (spy plane, paratroopers, parabombs) come free with the airfield and are controlled by air_weight instead
    - Capture priority controls engineer production for seizing neutral tech buildings (oil derricks, hospitals). Set above 0 only if neutral buildings are likely present on the map. High capture priority pairs well with aggression for "rush and steal" plays
    - Unit preferences (preferred_infantry, preferred_vehicle, preferred_aircraft, preferred_naval) let you steer which specific units get built within each category. List role names in priority order. First buildable role wins. Empty list = default priority. Examples: ["light_tank", "medium_tank"] for a light tank rush, ["flamethrower", "shock_trooper"] for flame-heavy infantry
    - Transport assault loads infantry into transport vehicles and drives them to the enemy base for a surprise attack. Soviet uses APCs (apc role, carries 5), Allies use Rangers (ranger role, carries 1). Pair with infantry_weight and preferred_infantry (e.g. ["flamethrower"]) to choose troops. Requires war factory and scouted enemy base. Best as an early/mid-game rush — late game the transports get destroyed en route. IMPORTANT: never put "apc" in preferred_vehicle for Allies or "ranger" for Soviet — they are faction-locked
    - ground_target_aa_priority: set high (0.5+) when your strategy relies on aircraft — your ground forces will prioritize destroying enemy AA guns and SAM sites to clear the skies
    - air_target_ground_def_priority: set high (0.5+) when your strategy relies on ground pushes — your aircraft will prioritize destroying enemy pillboxes, turrets, tesla coils, and flame towers to clear the path
    - commit_ratio: when to launch the ground-attack squad. Default 0 (uses system baseline ~0.5). The correct value depends on WHAT you're attacking, not just your directive:
       * Rush vs UNFORTIFIED enemy (no defenses scouted yet, or early enough that they haven't built any): 0.3-0.4. Small fast waves deny the enemy time to fortify. Multiple pressure points.
       * Rush vs FORTIFIED enemy (enemy_buildings_seen or enemy_buildings contains pillbox / camo_pillbox / turret / tesla_coil / flame_tower / sam_site / gunboat_turret / aa_gun): 0.6-0.8. Small waves die on the pillbox line without breaking through. You need overwhelming mass per commit. If you also lack a breakthrough element (no vehicles / no siege / no rocket_soldier / no air), your rush WILL NOT succeed regardless of commit_ratio — add vehicles or artillery to the doctrine first.
       * Turtle / careful: 0.8-0.9. Full squad or nothing.
       * A common failure: commit_ratio 0.3 vs turtle AI. Small waves reach pillbox line, die, retreat, repeat. Attacks squad-disengage without progress. Match the ratio to the actual defense state you're pushing INTO.
    - base_defense_floor: minimum pillboxes/turrets/tesla_coils before the ground-attack squad deploys. Default 0 (no floor — attack whenever). Rush: 0-1. Balanced push: 2-3. Turtle: 6-8. Prevents committing the army forward while the base is naked. If floor isn't met, squad stays home until enough defenses build up.
    - repair_budget_ratio: cap repair spending as fraction of cash. Default 0 (unlimited — current behavior). Rush doctrines that need every $50 for rifles should set 0.2 (only repair with 20% of cash on hand, keeping 80% for production). Turtle doctrines that need buildings alive can leave at 0 or 1.
    - scout_reach_priority: how aggressively scouts probe toward known enemy vs. passive perimeter patrol. Default 0. Rush doctrines need enemy base found fast — set 0.7-0.9 (probe direct toward last-known enemy contact). Turtle doctrines don't need to locate the enemy — leave at 0. Balanced doctrines 0.4-0.5.

    Key situation signals to use with these knobs:
    - situation.ground_squad_ready_ratio: current fraction of the ground-attack squad that's idle and ready. Compare to your commit_ratio — if ready_ratio is chronically below commit_ratio, either lower commit_ratio or reduce ground_attack_group_size.
    - situation.cash_burn_rate: net cash change per eval. If persistently negative, either the economy is under attack, your repair_budget_ratio is too generous, or production caps are exceeded. Tighten one of them.
    - situation.time_to_reach_enemy_estimate: rough travel time from our base to a known enemy base. -1 if no intel. Useful for planning: if the estimate is 4000 ticks and you want to attack by tick 8000, deploy the squad by tick 4000.

    {{ ctx.output_format }}
  "#
}
