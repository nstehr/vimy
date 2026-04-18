using System;
using System.IO;
using System.Net.Sockets;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using OpenRA.Mods.Common.Traits;
using OpenRA.Traits;

namespace OpenRA.Mods.Vimy
{
	[Desc("Bot module that bridges game state to an external sidecar process via Unix domain socket.")]
	public class VimyBotModuleInfo : ConditionalTraitInfo
	{
		[Desc("Path to the Unix domain socket for IPC with the sidecar.")]
		public readonly string PipePath = "/tmp/vimy.sock";

		[Desc("How often (in ticks) to send game state to the sidecar.")]
		public readonly int StateIntervalTicks = 10;

		public override object Create(ActorInitializer init) { return new VimyBotModule(init, this); }
	}

	public class VimyBotModule : ConditionalTrait<VimyBotModuleInfo>, IBotTick, IBotEnabled, INotifyWinStateChanged, INotifyActorDisposing
	{
		readonly World world;
		Socket socket;
		NetworkStream stream;
		int ticksSinceLastState;
		bool connected;
		bool helloSent;
		string playerName;

		public VimyBotModule(ActorInitializer init, VimyBotModuleInfo info)
			: base(info)
		{
			world = init.World;
		}

		void IBotEnabled.BotEnabled(IBot bot)
		{
			playerName = bot.Player.PlayerName;
			Log.Write("debug", $"VimyBotModule enabled for player {playerName}");
			TryConnect(bot);
		}

		void TryConnect(IBot bot)
		{
			if (connected)
				return;

			try
			{
				var endpoint = new UnixDomainSocketEndPoint(Info.PipePath);
				socket = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
				socket.Connect(endpoint);
				stream = new NetworkStream(socket, ownsSocket: false);
				connected = true;
				Log.Write("debug", $"Connected to sidecar at {Info.PipePath}");

				// Hello is deferred to the first BotTick — at BotEnabled time
				// world.Players may not be fully populated yet, which would
				// produce an empty opponents array.
			}
			catch (Exception ex)
			{
				Log.Write("debug", $"Failed to connect to sidecar at {Info.PipePath}: {ex.Message}");
				connected = false;
			}
		}

		void SendHello(IBot bot)
		{
			var faction = bot.Player.Faction.InternalName;

			Log.Write("debug", $"VimyBotModule SendHello: world.Players.Length={world.Players.Length}");

			var opponents = new JsonArray();
			foreach (var p in world.Players)
			{
				var rel = bot.Player.RelationshipWith(p);
				Log.Write("debug", $"VimyBotModule hello iter: player={p.PlayerName} faction={p.Faction.InternalName} nonCombatant={p.NonCombatant} rel={rel} self={(p == bot.Player)}");

				if (p == bot.Player || p.NonCombatant)
					continue;
				if (rel == PlayerRelationship.Ally)
					continue;
				opponents.Add(new JsonObject
				{
					["player"] = p.PlayerName,
					["faction"] = p.Faction.InternalName,
				});
			}

			var payload = new JsonObject
			{
				["player"] = bot.Player.PlayerName,
				["faction"] = faction,
				["opponents"] = opponents,
				["terrain"] = JsonNode.Parse(TerrainGridSerializer.Serialize(world)),
			};

			SendEnvelope("hello", payload.ToJsonString());
			Log.Write("debug", $"Sent hello for player {bot.Player.PlayerName}, faction {faction}, opponents={opponents.Count}");
		}

		void IBotTick.BotTick(IBot bot)
		{
			ticksSinceLastState++;

			if (!connected)
			{
				if (ticksSinceLastState % 100 == 0)
					TryConnect(bot);

				return;
			}

			// Send hello on the first tick after a successful connect — by
			// now world.Players is fully populated.
			if (!helloSent)
			{
				SendHello(bot);
				helloSent = true;
			}

			// Read any inbound messages from sidecar
			ReadMessages(bot);

			// Periodically send game state
			if (ticksSinceLastState >= Info.StateIntervalTicks)
			{
				ticksSinceLastState = 0;
				SendState(bot);
			}
		}

		void SendState(IBot bot)
		{
			if (!connected)
				return;

			try
			{
				var stateJson = GameStateSerializer.Serialize(world, bot);
				SendEnvelope("game_state", stateJson);
			}
			catch (Exception ex)
			{
				Log.Write("debug", $"Error sending state: {ex.Message}");
				Disconnect();
			}
		}

		void ReadMessages(IBot bot)
		{
			if (!connected || socket == null)
				return;

			try
			{
				while (socket.Available > 0)
				{
					var envelope = ReadEnvelope();
					if (envelope == null)
						continue;

					Log.Write("debug", $"Received message: type={envelope.Value.Type}, data={envelope.Value.Data}");

					switch (envelope.Value.Type)
					{
						case "ack":
							Log.Write("debug", $"Sidecar acknowledged: {envelope.Value.Data}");
							break;
						case "produce":
						case "place_building":
						case "attack_move":
						case "move":
						case "set_rally":
						case "deploy":
						case "repair_building":
						case "attack":
						case "cancel_production":
						case "harvest":
						case "capture":
						case "support_power":
						case "enter_transport":
						case "unload":
						case "repair_unit":
						case "place_minefield":
							CommandExecutor.Execute(envelope.Value.Type, envelope.Value.Data, world, bot);
							break;
						default:
							Log.Write("debug", $"Unknown message type: {envelope.Value.Type}");
							break;
					}
				}
			}
			catch (Exception ex)
			{
				Log.Write("debug", $"Error reading messages: {ex.Message}");
				Disconnect();
			}
		}

		void SendEnvelope(string type, string dataJson)
		{
			var envelope = $"{{\"type\":\"{type}\",\"data\":{dataJson}}}";
			SendRaw(envelope);
		}

		void SendRaw(string json)
		{
			var payload = Encoding.UTF8.GetBytes(json);
			var lengthBytes = BitConverter.GetBytes(payload.Length);
			if (!BitConverter.IsLittleEndian)
				Array.Reverse(lengthBytes);

			stream.Write(lengthBytes);
			stream.Write(payload);
			stream.Flush();
		}

		struct EnvelopeResult
		{
			public string Type;
			public string Data;
		}

		EnvelopeResult? ReadEnvelope()
		{
			var json = ReadRawMessage();
			if (json == null)
				return null;

			using var doc = JsonDocument.Parse(json);
			var root = doc.RootElement;

			if (!root.TryGetProperty("type", out var typeProp))
				return null;

			var type = typeProp.GetString();
			var data = root.TryGetProperty("data", out var dataProp)
				? dataProp.GetRawText()
				: "{}";

			return new EnvelopeResult { Type = type, Data = data };
		}

		string ReadRawMessage()
		{
			var lengthBuf = new byte[4];
			var bytesRead = stream.Read(lengthBuf);
			if (bytesRead < 4)
				return null;

			if (!BitConverter.IsLittleEndian)
				Array.Reverse(lengthBuf);

			var length = BitConverter.ToInt32(lengthBuf, 0);
			if (length <= 0 || length > 1024 * 1024)
				return null;

			var payload = new byte[length];
			var totalRead = 0;
			while (totalRead < length)
			{
				var read = stream.Read(payload, totalRead, length - totalRead);
				if (read == 0)
					return null;

				totalRead += read;
			}

			return Encoding.UTF8.GetString(payload);
		}

		void INotifyWinStateChanged.OnPlayerWon(Player winner)
		{
			SendGameEnd(winner.PlayerName);
		}

		void INotifyWinStateChanged.OnPlayerLost(Player loser)
		{
			// Find the winner. WinState.Won may not be set yet when OnPlayerLost
			// fires, so fall back to the first non-spectator player who isn't the loser.
			var winner = "";
			foreach (var player in world.Players)
			{
				if (player.WinState == WinState.Won)
				{
					winner = player.PlayerName;
					break;
				}
			}

			if (string.IsNullOrEmpty(winner))
			{
				foreach (var player in world.Players)
				{
					if (!player.NonCombatant && player != loser)
					{
						winner = player.PlayerName;
						break;
					}
				}
			}

			SendGameEnd(winner);
		}

		void SendGameEnd(string winner)
		{
			if (!connected)
				return;

			try
			{
				Log.Write("debug", $"Game over detected, winner: {winner}");
				SendEnvelope("game_end", $"{{\"winner\":\"{winner}\"}}");

				// Read the ack before disconnecting so the sidecar can process it.
				if (socket != null && socket.Poll(2000000, SelectMode.SelectRead))
					ReadEnvelope();
			}
			catch (Exception ex)
			{
				Log.Write("debug", $"Error sending game_end: {ex.Message}");
			}
		}

		void Disconnect()
		{
			connected = false;

			try
			{
				stream?.Dispose();
				socket?.Dispose();
			}
			catch { }

			stream = null;
			socket = null;
		}

		void INotifyActorDisposing.Disposing(Actor self)
		{
			Disconnect();
		}
	}
}
