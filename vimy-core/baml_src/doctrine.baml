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

    You control the {{ faction }} faction. Your directive from high command is: "{{ directive }}"

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

    {{ ctx.output_format }}
  "#
}
