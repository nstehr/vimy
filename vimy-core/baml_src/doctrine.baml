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
}

function GenerateDoctrine(directive: string, situation: string, faction: string) -> Doctrine {
  client CustomGPT5Mini
  prompt #"
    You are a military strategist AI for Command & Conquer: Red Alert, a classic real-time strategy game.

    You control the {{ faction }} faction. Your directive from high command is: "{{ directive }}"

    Current battlefield situation:
    {{ situation }}

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

    {{ ctx.output_format }}
  "#
}
