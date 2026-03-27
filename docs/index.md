---
layout: home
title: ""

---

# LLM Driven RTS Doctrine

LLMs have been improving exponentially over the last few years. While their capabilities have been growing, there are still two major
challenges: determinism and latency. This project explores how to build an RTS agent that can leverage the strategic reasoning capabilities of LLMs while mitigating these challenges.

## The Key Idea

> The LLM never directly controls the game.
> It produces a doctrine, a set of weights.

That doctrine is compiled into deterministic rules, and only those rules are allowed to act.

## High Level Approach
There have been examples of LLMs playing games and they have been capable of generating decent gameplay. An example of this is:
[hallucinating-splines](https://github.com/andrewedunn/hallucinating-splines) where an agent can play Micropolis. This works because the game is turn based so
the latency doesn't impact it as much. For an RTS, having the agent involved in every game tick is not feasible.

The other challenge is determinism. LLMs are not deterministic and can generate different outputs for the same input. This is a problem for an RTS agent because it needs to be able to predict the outcome of its actions and plan accordingly.

To address these challenges, the approach taken in this project is to use the LLM to generate a high level doctrine (aggression, economy, unit composition, attack timing) and then compile that into deterministic rules that can be executed at game speed. The agent will play immediately with seed rules on startup and then adapt its strategy as game events unfold.

## Doctrine Generation
The doctrine generation is done via goroutine in the agent. It is initially seeded at start up with a directive. The directive
is plain english statement(s) that describe the strategy the bot should follow. For example, "Play a defensive turtle strategy, focus on economy and tech, and only attack when you have a significant advantage." or "Dominance through superweapons. Protect the base but rush superweapon tech and unleash devastation on the enemy."
This directive is then fed to the LLM along with game state and instructions on what to output. The LLM is instructed to output a series of weights that can then be applied to affect the direction of the game.

An example prompt would be something like:

```
You are a military strategist AI for Command & Conquer: Red Alert, a classic real-time strategy game.

    You control the germany faction. Your directive from high command is: "Early overwhelming. Rush to get a bulk of infantry and light armour to swarm and overwhelm before the enemy can get to leveled up"

    Current battlefield situation:
    Tick: 13261 | Phase: Mid Game
    Cash: 0 | Resources: 1334/2000
    Power: 210/299 (Normal)

    Buildings: 3x powr 1x pbox 1x hbox 1x dome 1x fact 1x proc 3x tent 1x gun
    Units: 3x harv 2x e6 (2 idle)

    Queue Building: producing apwr (58%)




    Enemies visible: 0

    Known enemy base [bot-turtle-ai.name]: (25, 35) last seen tick 8011

    Map: 130x130


    Combat Stats (lifetime): Lost 6 infantry, 0 vehicles, 0 aircraft

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
    - ground_defense_priority controls ground base defense: pillbox/turret/tesla caps and how quickly ground units scramble to defend. High = scramble even 1 unit, low = wait for 3+. Always set above 0
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
    - Transport assault loads infantry into APCs and drives them to the enemy base for a surprise attack. Pair with infantry_weight and preferred_infantry (e.g. ["flamethrower"]) to choose troops. Requires war factory and scouted enemy base. Best as an early/mid-game rush — late game the APCs get destroyed en route

    Answer in JSON using this schema:
    {
      // Short name for this doctrine, e.g. 'Blitzkrieg', 'Turtle Defense', 'Guerrilla Raids'
      name: string,
      // Brief explanation of why this doctrine fits the current situation and directive
      rationale: string,
      // 0.0-1.0: investment in refineries and harvesters
      economy_priority: float,
      // 0.0-1.0: offensive commitment level
      aggression: float,
      // 0.0-1.0: ground base defense urgency — controls pillbox/turret/tesla caps and ground unit scramble threshold when base is attacked
      ground_defense_priority: float,
      // 0.0-1.0: anti-air defense urgency — controls AA structure caps and aircraft scramble priority when base is attacked
      air_defense_priority: float,
      // 0.0-1.0: investment in tech buildings
      tech_priority: float,
      // 0.0-1.0: infantry production preference
      infantry_weight: float,
      // 0.0-1.0: vehicle production preference
      vehicle_weight: float,
      // 0.0-1.0: air unit production preference
      air_weight: float,
      // 0.0-1.0: naval unit production preference
      naval_weight: float,
      // Minimum ground units (infantry+vehicles) before launching a ground attack (3-15)
      ground_attack_group_size: int,
      // Minimum combat aircraft before launching an air strike (1-8)
      air_attack_group_size: int,
      // Minimum naval units before launching a naval attack (2-10)
      naval_attack_group_size: int,
      // 0.0-1.0: reconnaissance investment
      scout_priority: float,
      // 0.0-1.0: investment in elite infantry (flamethrowers, shock troopers, Tanya, medics) — requires prerequisite buildings already built via other priorities
      specialized_infantry_weight: float,
      // 0.0-1.0: investment in superweapon buildings (Missile Silo, Iron Curtain) and eagerness to use them — requires tech center already built via tech_priority. Set above 0.3 to build superweapon structures. Airfield powers (spy plane, paratroopers) are governed by air_weight instead
      superweapon_priority: float,
      // 0.0-1.0: eagerness to capture neutral tech buildings with engineers — controls engineer and APC production. Set above 0.1 to produce engineers, above 0.3 to also produce APCs for delivery. Set to 0 for pure defense doctrines that shouldn't waste the Infantry queue on engineers
      capture_priority: float,
      // 0.0-1.0: use APCs to rush infantry into the enemy base. Requires war factory and infantry production. Pair with preferred_infantry to choose which troops to load (e.g. flamethrowers). Set above 0.1 to produce assault APCs. Higher = more APCs (up to 3). Set to 0 for standard play without APC rushes
      transport_assault: float,
      // Ordered list of infantry roles to prioritize. Soviet: flamethrower, shock_trooper, medic, engineer. Allied: tanya, medic, engineer. Both: rocket_soldier. Only list roles available to your faction. Empty = default priority.
      preferred_infantry: string[],
      // Ordered list of vehicle roles to prioritize. Soviet: heavy_tank, medium_tank, v2_launcher, flak_truck, demo_truck, apc. Allied: medium_tank, light_tank, artillery, ranger, apc. Only list roles available to your faction. Empty = default priority.
      preferred_vehicle: string[],
      // Ordered list of aircraft roles to prioritize. Soviet: basic_aircraft (Yak/MiG), advanced_aircraft (MiG/Hind). Allied: basic_aircraft (Black Hawk), advanced_aircraft (Longbow). Empty = default priority.
      preferred_aircraft: string[],
      // Ordered list of naval roles to prioritize. Soviet: submarine. Allied: gunboat, destroyer, cruiser. Empty = default priority.
      preferred_naval: string[],
    }
```


And the output from the LLM would be:
```
 {
      "name": "Blitzswarm",
      "rationale": "Mid-game with modest resources and intact power; directive demands early overwhelming pressure. Maintain a strong economy while massing cheap infantry reinforced with light/medium armour and APC assaults to swarm the enemy before they tech up. Keep scouting high to locate the enemy, moderate tech to unlock mid-game tanks, and light base defenses so forces can be forwarded quickly.",
      "economy_priority": 0.70,
      "aggression": 0.90,
      "ground_defense_priority": 0.50,
      "air_defense_priority": 0.20,
      "tech_priority": 0.40,
      "infantry_weight": 0.60,
      "vehicle_weight": 0.30,
      "air_weight": 0.10,
      "naval_weight": 0.00,
      "ground_attack_group_size": 6,
      "air_attack_group_size": 1,
      "naval_attack_group_size": 2,
      "scout_priority": 0.70,
      "specialized_infantry_weight": 0.20,
      "superweapon_priority": 0.05,
      "capture_priority": 0.35,
      "transport_assault": 0.40,
      "preferred_infantry": [
        "rifleman",
        "rocket_soldier",
        "engineer"
      ],
      "preferred_vehicle": [
        "light_tank",
        "medium_tank",
        "apc"
      ],
      "preferred_aircraft": [],
      "preferred_naval": []
    }
```

We have a decent amount of knobs to tweak, but not too many that it becomes overwhelming. You can see in the output how this strategy was to be land based with `infantry_weight` relatively high, `vehicle_weight` medium and `naval_weight` zero.

Outputting fixed weights helps to minimize hallucinations. We can programatically apply these to the rules instead of having the LLM produce the rules themselves.

From an implementation perspective, this is all implemented using [BAML](https://docs.boundaryml.com/home). BAML has been my go to tool for integrating LLMs. It uses code generation and structured inputs and outputs. It lets me treat interactions with LLMs as 'just another function call' and the structured input and output give an elegant way to model our interactions. You can see the BAML used for the agent [here](https://github.com/nstehr/vimy/blob/main/vimy-core/baml_src/doctrine.baml)

From a gameplay perspective there is also one key insight. The agent keeps track of unit losses and other game details and feeds those back as part of the prompt. This forces the LLM to ground the initial directive against the realities of what is happening in the game.

## Rule Engine

The doctrine weights on their own can't affect the game. They need to be translated into concrete, deterministic actions that can execute at game speed. This is where the rule engine comes in.

### Rules

A rule is a simple pairing: a **condition** and an **action**. Each game tick, the engine evaluates every rule's condition against the current game state. If the condition is true, the action fires. Rules also have a **priority** (higher evaluates first) and a **category**. Within a category, a rule can be marked **exclusive**, meaning once it fires, no lower-priority rule in the same category can fire that tick. This prevents conflicts like trying to queue two buildings on the same production queue.

Conditions are written using the [expr](https://github.com/expr-lang/expr) expression language. This gives us a concise, readable way to express game logic as boolean expressions. For example:

```
HasUnit("mcv") && !HasBuilding("fact")
```

This checks if we have a Mobile Construction Vehicle but haven't deployed it yet. The expressions can call methods on a rich environment object that exposes game state queries like `Cash()`, `PowerExcess()`, `UnitCount("e1")`, `HasRole("barracks")`, `BaseUnderAttack()`, `len(IdleGroundUnits())`, and many more. The `HasRole` and `CanBuildRole` functions abstract away faction differences, for example  `"war_factory"` maps to the correct internal building type whether you're playing Allied or Soviet.

When a condition evaluates to true, the corresponding action sends commands back to the game: produce a unit, place a building, attack-move units to a position, repair a structure, and so on.

### Seed Rules

The agent doesn't need the LLM to play. On startup, a set of **seed rules** provide a baseline strategy that handles the fundamentals: deploy the MCV, build power and a refinery, construct a barracks and war factory, produce basic infantry, defend the base if attacked, scout when no enemies are visible, and send idle harvesters back to work. These seed rules ensure the agent is playing immediately from tick one while the LLM generates its first doctrine.

### OpenRA Integration

The agent communicates with the game through a [custom OpenRA mod](https://github.com/nstehr/vimy/tree/main/openra-mod). The mod connects to the agent over a Unix domain socket (`/tmp/vimy.sock`) using a simple protocol: length-prefixed JSON messages. Each game tick, the mod sends the full game state; the buildings, units, production queues, visible enemies, support powers, and map dimensions. The agent evaluates its rules against this state and sends back commands (produce, place building, attack-move, repair, etc.) over the same connection. I tried to keep this side of the design as "dumb" as possible. Just producing and sending game state and executing commands. I also tried to preserve things like the fog of war so that our agent is close to the human experience when playing the game.

### The Compiler

The compiler is the bridge between the LLM's strategic reasoning and the rule engine's deterministic execution. It takes a doctrine (the set of weights from the LLM) and compiles it into a complete rule set.

The compilation process uses **threshold gates** to decide which rules to include. A weight below 0.1 means that capability is effectively disabled and no rules are generated for it. As the weight increases, more sophisticated rules are added. For example, if `vehicle_weight` is above 0.1, the compiler adds rules for a war factory and basic vehicle production. Above 0.2, siege vehicle rules are included. Above 0.3, a service depot is added for repairs.

The weights also control **numeric parameters** within rules. The `infantry_weight` scales the infantry production cap (from 8 units at 0.0 to 20 at 1.0). The `aggression` weight determines attack squad sizes and how eagerly units are sent forward. The `ground_defense_priority` controls how many defensive structures get built and how quickly units scramble to defend when the base is attacked.

To make this concrete, consider this snippet from a doctrine:

```json
{
  "infantry_weight": 0.6,
  "vehicle_weight": 0.3
}
```

The compiler takes `infantry_weight: 0.6` and produces a rule like this:

```go
if d.InfantryWeight > DoctrineEnabled { // 0.6 > 0.1, so this block runs
    infantryCap := lerp(8, 20, d.InfantryWeight) // lerp(8, 20, 0.6) = 15
    rules = append(rules, &Rule{
        Name:         "produce-infantry",
        Priority:     infantryBasePri,
        Category:     CatProduceInfantry,
        Exclusive:    true,
        ConditionSrc: `HasRole("barracks") && !QueueBusy("Infantry") &&
                       CanBuild("Infantry","e1") && UnitCount("e1") < 15 && Cash() >= 100`,
        Action:       ActionProduceInfantry,
    })
}
```

The `0.6` weight became a concrete unit cap of 15 baked into a deterministic rule condition. At game time, the rule engine just evaluates `UnitCount("e1") < 15`

Once compiled, the new rule set is atomically swapped into the engine via `Engine.Swap()`, replacing the previous rules mid-game. The game state is treated as the input, and the rule engine takes that and evaluates and executes.

### Determinism Boundary

The compiler enforces strict guarantees: all rule conditions are generated via templates so there are no invalid expressions, numeric values are bounded through `lerp` and validation, and the LLM never directly controls actions. Rule execution is fully deterministic per game tick.

This means the LLM can suggest strategy, but it cannot produce undefined or unsafe behavior.

## Dashboard

To observe the system in action, the agent serves a live dashboard over HTTP. It provides a window into the agent's strategic thinking as a game unfolds.

![Directive]({{ site.baseurl }}/assets/Screenshot%202026-03-26%20at%2012.14.00%E2%80%AFPM.png)

The current doctrine panel shows the latest weights produced by the LLM, organized by category: economy and tech, combat composition, aggression and defense, squad sizes, and unit preferences. The rationale at the top explains *why* the LLM chose these weights given the current game state.  You can see some adaptation here as the LLM has detected that infantry is being hard-countered and is shifting toward naval and vehicle production.


![Current Doctrine]({{ site.baseurl }}/assets/Screenshot%202026-03-26%20at%203.50.30%E2%80%AFPM.png)

The doctrine evolution chart tracks how weights change over the course of a game. Each point is a new doctrine generated by the LLM in response to changing conditions.

![Doctrine Evolution]({{ site.baseurl }}/assets/Screenshot%202026-03-26%20at%2012.13.53%E2%80%AFPM.png)

The compiled rules panel shows the actual rules that the compiler produced from the current doctrine. Rules are grouped by category and you can see their priority, exclusivity, and the full `expr` condition strings.

![Compiled Rules]({{ site.baseurl }}/assets/Screenshot%202026-03-26%20at%2010.27.04%E2%80%AFPM.png)

## Gameplay
TODO

## Final Thoughts
So far this system has been working very well, or at least well enough to produce interesting matches for me to watch as Vimy takes on the built in AI. One of the main pieces of work is the tweaking of the engine itself. To make sure it covers scenarios properly and has the right set of rules and the environment has the right set of functions to allow the rules to be expressive enough. From an RTS perspective, one of the main downsides is that we do give up some of the micro that is a key element of these types of games. We can model some of it into the rule engine (return to base when damaged, targeting rules for units and buildings) but unit level individual commands are not really feasible with this architecture. The upside is that we get a strategic agent that can adapt its strategy mid game and can leverage the LLM's reasoning capabilities without being hampered by latency or hallucinations.

Stepping outside of gameplaying use cases I find this architecture to be a compelling way to map LLM reasoning to deterministic outcomes. As we build more and more agents for our day to day work, it is a nice pattern to adopt to leverage LLMs safely and cost effectively. The LLM can be the "brain" that generates high level strategies and adapts to changing circumstances, while the rule engine is the "muscle" that executes those strategies in a predictable way. This separation of concerns allows us to get the best of both worlds: the creativity and reasoning of LLMs without sacrificing reliability and trust in the output.
