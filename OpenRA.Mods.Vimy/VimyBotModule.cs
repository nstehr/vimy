using System;
using System.IO;
using System.Net.Sockets;
using System.Text;
using System.Text.Json;
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

	public class VimyBotModule : ConditionalTrait<VimyBotModuleInfo>, IBotTick, IBotEnabled, INotifyActorDisposing
	{
		readonly World world;
		Socket socket;
		NetworkStream stream;
		int ticksSinceLastState;
		bool connected;
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

				SendHello(bot);
			}
			catch (Exception ex)
			{
				Log.Write("debug", $"Failed to connect to sidecar at {Info.PipePath}: {ex.Message}");
				connected = false;
			}
		}

		void SendHello(IBot bot)
		{
			var data = $"{{\"player\":\"{bot.Player.PlayerName}\"}}";
			SendEnvelope("hello", data);
			Log.Write("debug", $"Sent hello for player {bot.Player.PlayerName}");
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
