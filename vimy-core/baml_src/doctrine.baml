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
  known_enemy_bases EnemyBase[]
  map_width int
  map_height int
  recent_events GameEvent[]
}

class Doctrine {
  name string @description("Short name for this doctrine, e.g. 'Blitzkrieg', 'Turtle Defense', 'Guerrilla Raids'")
  rationale string @description("Brief explanation of why this doctrine fits the current situation and directive")
  economy_priority float @description("0.0-1.0: investment in refineries and harvesters")
  aggression float @description("0.0-1.0: offensive commitment level")
  ground_defense_priority float @description("0.0-1.0: ground base defense urgency — controls pillbox/turret/tesla caps and ground unit scramble threshold when base is attacked")
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
}

function GenerateDoctrine(directive: string, situation: GameSituation, faction: string) -> Doctrine {
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
    - ground_defense_priority controls ground base defense: pillbox/turret/tesla caps and how quickly ground units scramble to defend. High = scramble even 1 unit, low = wait for 3+. Always set above 0
    - air_defense_priority controls AA structures (SAM/AA gun caps) and how eagerly aircraft scramble to defend the base. Can be independent of ground defense
    - Aggression controls how eagerly you attack; low = defensive turtle, high = constant pressure
    - ground_attack_group_size: smaller = faster but riskier ground attacks, larger = slower but more decisive
    - air_attack_group_size: how many aircraft to accumulate before striking (1 = harass constantly, 8 = decisive air raids)
    - naval_attack_group_size: how many ships to accumulate before attacking
    - Scout priority: higher = more reconnaissance, important when enemy position unknown
    - Specialized infantry (flamethrowers, commandos, medics) require prerequisite structures — set above 0 only when those are likely available
    - Superweapon priority controls building the Missile Silo (nuke) and Iron Curtain — both require a tech center. Set above 0.3 to invest. Airfield support powers (spy plane, paratroopers, parabombs) come free with the airfield and are controlled by air_weight instead

    {{ ctx.output_format }}
  "#
}
