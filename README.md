# Vimy

An AI-controlled bot for OpenRA (Red Alert) that uses LLM-generated strategy compiled into runtime rules to play the game autonomously.

## How It Works

An LLM generates a high-level strategy (e.g., "Blitzkrieg", "Turtle Defense") which is compiled into rules that execute at game speed. The bot plays immediately with seed rules on startup, and adapts its strategy as game events unfold.

```
┌──────────────────────────────┐
│     LLM (OpenAI via BAML)    │
│  Generates strategy rules    │
│  Reassesses on game events   │
└──────────┬───────────────────┘
           │ async
┌──────────▼───────────────────┐
│     vimy-core (Go)           │
│  Receives game state         │
│  Executes rules via Grule    │
│  Sends commands back         │
└──────────┬───────────────────┘
           │ Unix domain socket
┌──────────▼───────────────────┐
│     OpenRA Mod (C#)          │
│  Serializes game state       │
│  Executes bot commands       │
└──────────────────────────────┘
```

## Components

### OpenRA Mod (`OpenRA.Mods.Vimy/`)

A C# mod built on the OpenRA engine (release-20250330). Acts as a stateless bridge — serializes game state as JSON and sends it over a Unix domain socket, receives command envelopes and translates them into game orders.

### vimy-core (`vimy-core/`)

The Go agent that drives the bot. Connects to the mod via Unix domain socket using a length-prefixed JSON envelope protocol. Processes game state, runs the rule engine, and sends commands back to the mod.

## Getting Started

### Prerequisites

- [.NET 6 SDK](https://dotnet.microsoft.com/download) (for the OpenRA mod)
- [Go 1.25+](https://golang.org/dl/) (for vimy-core)

### Build & Run

```bash
# Build the mod (fetches the OpenRA engine on first run)
make

# In a separate terminal, start vimy-core
cd vimy-core && go run .

# Launch the game
./launch-game.sh
```

Start a skirmish and select "Vimy AI" as an opponent.

## Repository Structure

The repository root doubles as the OpenRA Mod SDK workspace. Files like `Makefile`, `launch-game.sh`, `mod.config`, and `engine/` belong to the SDK and expect to live at the repo root — moving them into a subdirectory would break the SDK's relative path assumptions and create friction with future engine updates.

As a result, the mod files live at the top level alongside `vimy-core/`, rather than being nested under their own directory.

## License

The OpenRA engine and SDK scripts are made available under the [GPLv3](https://github.com/OpenRA/OpenRA/blob/bleed/COPYING) license.
