using System;
using System.Linq;
using System.Text.Json;
using OpenRA.Mods.Common;
using OpenRA.Mods.Common.Traits;
using OpenRA.Traits;

namespace OpenRA.Mods.Vimy
{
	public static class CommandExecutor
	{
		public static void Execute(string commandType, string dataJson, World world, IBot bot)
		{
			try
			{
				switch (commandType)
				{
					case "produce":
						ExecuteProduce(dataJson, world, bot);
						break;
					case "place_building":
						ExecutePlaceBuilding(dataJson, world, bot);
						break;
					case "attack_move":
						ExecuteAttackMove(dataJson, world, bot);
						break;
					case "move":
						ExecuteMove(dataJson, world, bot);
						break;
					case "set_rally":
						ExecuteSetRally(dataJson, world, bot);
						break;
					case "deploy":
						ExecuteDeploy(dataJson, world, bot);
						break;
					case "repair_building":
						ExecuteRepairBuilding(dataJson, world, bot);
						break;
					case "attack":
						ExecuteAttack(dataJson, world, bot);
						break;
					case "cancel_production":
						ExecuteCancelProduction(dataJson, world, bot);
						break;
					case "harvest":
						ExecuteHarvest(dataJson, world, bot);
						break;
					case "capture":
						ExecuteCapture(dataJson, world, bot);
						break;
					case "enter_transport":
						ExecuteEnterTransport(dataJson, world, bot);
						break;
					case "unload":
						ExecuteUnload(dataJson, world, bot);
						break;
					case "support_power":
						ExecuteSupportPower(dataJson, world, bot);
						break;
					default:
						Log.Write("debug", $"CommandExecutor: unknown command type '{commandType}'");
						break;
				}
			}
			catch (Exception ex)
			{
				Log.Write("debug", $"CommandExecutor: error executing '{commandType}': {ex.Message}");
			}
		}

		static void ExecuteProduce(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var queueType = root.GetProperty("queue").GetString();
			var item = root.GetProperty("item").GetString();
			var count = root.TryGetProperty("count", out var countProp) ? countProp.GetInt32() : 1;

			var queue = FindQueue(bot, queueType);
			if (queue == null)
			{
				Log.Write("debug", $"CommandExecutor: produce — no queue of type '{queueType}'");
				return;
			}

			bot.QueueOrder(Order.StartProduction(queue.Actor, item, count));
			Log.Write("debug", $"CommandExecutor: produce {count}x {item} from {queueType}");
		}

		static void ExecutePlaceBuilding(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var queueType = root.GetProperty("queue").GetString();
			var item = root.GetProperty("item").GetString();

			// Optional placement hint from sidecar.
			CPos? hint = null;
			if (root.TryGetProperty("hint_x", out var hx) && root.TryGetProperty("hint_y", out var hy)
				&& hx.GetInt32() != 0 && hy.GetInt32() != 0)
			{
				hint = new CPos(hx.GetInt32(), hy.GetInt32());
			}

			var queue = FindQueue(bot, queueType);
			if (queue == null)
			{
				Log.Write("debug", $"CommandExecutor: place_building — no queue of type '{queueType}'");
				return;
			}

			var actorInfo = world.Map.Rules.Actors[item];
			var bi = actorInfo.TraitInfoOrDefault<BuildingInfo>();
			if (bi == null)
			{
				Log.Write("debug", $"CommandExecutor: place_building — '{item}' has no BuildingInfo");
				return;
			}

			var location = ChooseBuildLocation(world, bot.Player, actorInfo, bi, hint);
			if (location == null)
			{
				Log.Write("debug", $"CommandExecutor: place_building — no valid location for '{item}'");
				bot.QueueOrder(Order.CancelProduction(queue.Actor, item, 1));
				return;
			}

			bot.QueueOrder(new Order("PlaceBuilding", bot.Player.PlayerActor, Target.FromCell(world, location.Value), false)
			{
				TargetString = item,
				ExtraLocation = CPos.Zero,
				ExtraData = queue.Actor.ActorID,
				SuppressVisualFeedback = true
			});

			Log.Write("debug", $"CommandExecutor: place_building '{item}' at ({location.Value.X},{location.Value.Y})");
		}

		static void ExecuteAttackMove(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var actorIdsProp = root.GetProperty("actor_ids");
			var x = root.GetProperty("x").GetInt32();
			var y = root.GetProperty("y").GetInt32();

			var actors = actorIdsProp.EnumerateArray()
				.Select(id => world.GetActorById(id.GetUInt32()))
				.Where(a => IsValidOwnedActor(a, bot))
				.ToArray();

			if (actors.Length == 0)
			{
				Log.Write("debug", "CommandExecutor: attack_move — no valid actors");
				return;
			}

			var target = Target.FromCell(world, new CPos(x, y));
			bot.QueueOrder(new Order("AttackMove", null, target, false, groupedActors: actors));
			Log.Write("debug", $"CommandExecutor: attack_move {actors.Length} units to ({x},{y})");
		}

		static void ExecuteMove(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var actorId = root.GetProperty("actor_id").GetUInt32();
			var x = root.GetProperty("x").GetInt32();
			var y = root.GetProperty("y").GetInt32();

			var actor = world.GetActorById(actorId);
			if (!IsValidOwnedActor(actor, bot))
			{
				Log.Write("debug", $"CommandExecutor: move — invalid actor {actorId}");
				return;
			}

			bot.QueueOrder(new Order("Move", actor, Target.FromCell(world, new CPos(x, y)), false));
			Log.Write("debug", $"CommandExecutor: move actor {actorId} to ({x},{y})");
		}

		static void ExecuteSetRally(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var actorId = root.GetProperty("actor_id").GetUInt32();
			var x = root.GetProperty("x").GetInt32();
			var y = root.GetProperty("y").GetInt32();

			var actor = world.GetActorById(actorId);
			if (!IsValidOwnedActor(actor, bot))
			{
				Log.Write("debug", $"CommandExecutor: set_rally — invalid actor {actorId}");
				return;
			}

			bot.QueueOrder(new Order("SetRallyPoint", actor, Target.FromCell(world, new CPos(x, y)), false)
			{
				SuppressVisualFeedback = true
			});

			Log.Write("debug", $"CommandExecutor: set_rally actor {actorId} to ({x},{y})");
		}

		static void ExecuteDeploy(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var actorId = root.GetProperty("actor_id").GetUInt32();

			var actor = world.GetActorById(actorId);
			if (!IsValidOwnedActor(actor, bot))
			{
				Log.Write("debug", $"CommandExecutor: deploy — invalid actor {actorId}");
				return;
			}

			bot.QueueOrder(new Order("DeployTransform", actor, true));
			Log.Write("debug", $"CommandExecutor: deploy actor {actorId}");
		}

		static void ExecuteRepairBuilding(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var actorId = root.GetProperty("actor_id").GetUInt32();

			var actor = world.GetActorById(actorId);
			if (!IsValidOwnedActor(actor, bot))
			{
				Log.Write("debug", $"CommandExecutor: repair_building — invalid actor {actorId}");
				return;
			}

			bot.QueueOrder(new Order("RepairBuilding", bot.Player.PlayerActor, Target.FromActor(actor), false));
			Log.Write("debug", $"CommandExecutor: repair_building actor {actorId}");
		}

		static void ExecuteAttack(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var actorId = root.GetProperty("actor_id").GetUInt32();
			var targetId = root.GetProperty("target_id").GetUInt32();

			var actor = world.GetActorById(actorId);
			if (!IsValidOwnedActor(actor, bot))
			{
				Log.Write("debug", $"CommandExecutor: attack — invalid actor {actorId}");
				return;
			}

			var target = world.GetActorById(targetId);
			if (target == null || target.IsDead || !target.IsInWorld)
			{
				Log.Write("debug", $"CommandExecutor: attack — invalid target {targetId}");
				return;
			}

			bot.QueueOrder(new Order("Attack", actor, Target.FromActor(target), false));
			Log.Write("debug", $"CommandExecutor: attack actor {actorId} -> target {targetId}");
		}

		static void ExecuteCancelProduction(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var queueType = root.GetProperty("queue").GetString();
			var item = root.GetProperty("item").GetString();
			var count = root.TryGetProperty("count", out var countProp) ? countProp.GetInt32() : 1;

			var queue = FindQueue(bot, queueType);
			if (queue == null)
			{
				Log.Write("debug", $"CommandExecutor: cancel_production — no queue of type '{queueType}'");
				return;
			}

			bot.QueueOrder(Order.CancelProduction(queue.Actor, item, count));
			Log.Write("debug", $"CommandExecutor: cancel_production {count}x {item} from {queueType}");
		}

		static void ExecuteHarvest(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var actorId = root.GetProperty("actor_id").GetUInt32();
			var x = root.GetProperty("x").GetInt32();
			var y = root.GetProperty("y").GetInt32();

			var actor = world.GetActorById(actorId);
			if (!IsValidOwnedActor(actor, bot))
			{
				Log.Write("debug", $"CommandExecutor: harvest — invalid actor {actorId}");
				return;
			}

			bot.QueueOrder(new Order("Harvest", actor, Target.FromCell(world, new CPos(x, y)), false));
			Log.Write("debug", $"CommandExecutor: harvest actor {actorId} to ({x},{y})");
		}

		static void ExecuteCapture(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var actorId = root.GetProperty("actor_id").GetUInt32();
			var targetId = root.GetProperty("target_id").GetUInt32();

			var actor = world.GetActorById(actorId);
			if (!IsValidOwnedActor(actor, bot))
			{
				Log.Write("debug", $"CommandExecutor: capture — invalid actor {actorId}");
				return;
			}

			var target = world.GetActorById(targetId);
			if (target == null || target.IsDead || !target.IsInWorld)
			{
				Log.Write("debug", $"CommandExecutor: capture — invalid target {targetId}");
				return;
			}

			bot.QueueOrder(new Order("CaptureActor", actor, Target.FromActor(target), true));
			Log.Write("debug", $"CommandExecutor: capture actor {actorId} -> target {targetId}");
		}

		static void ExecuteEnterTransport(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var actorId = root.GetProperty("actor_id").GetUInt32();
			var transportId = root.GetProperty("transport_id").GetUInt32();

			var actor = world.GetActorById(actorId);
			if (!IsValidOwnedActor(actor, bot))
			{
				Log.Write("debug", $"CommandExecutor: enter_transport — invalid actor {actorId}");
				return;
			}

			var transport = world.GetActorById(transportId);
			if (!IsValidOwnedActor(transport, bot))
			{
				Log.Write("debug", $"CommandExecutor: enter_transport — invalid transport {transportId}");
				return;
			}

			bot.QueueOrder(new Order("EnterTransport", actor, Target.FromActor(transport), false));
			Log.Write("debug", $"CommandExecutor: enter_transport actor {actorId} -> transport {transportId}");
		}

		static void ExecuteUnload(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var actorId = root.GetProperty("actor_id").GetUInt32();

			var actor = world.GetActorById(actorId);
			if (!IsValidOwnedActor(actor, bot))
			{
				Log.Write("debug", $"CommandExecutor: unload — invalid actor {actorId}");
				return;
			}

			bot.QueueOrder(new Order("Unload", actor, false));
			Log.Write("debug", $"CommandExecutor: unload actor {actorId}");
		}

		static void ExecuteSupportPower(string dataJson, World world, IBot bot)
		{
			using var doc = JsonDocument.Parse(dataJson);
			var root = doc.RootElement;

			var powerKey = root.GetProperty("power_key").GetString();
			var x = root.GetProperty("x").GetInt32();
			var y = root.GetProperty("y").GetInt32();

			bot.QueueOrder(new Order(powerKey, bot.Player.PlayerActor, Target.FromCell(world, new CPos(x, y)), false)
			{
				SuppressVisualFeedback = true,
				ExtraData = uint.MaxValue
			});

			Log.Write("debug", $"CommandExecutor: support_power '{powerKey}' at ({x},{y})");
		}

		static ProductionQueue FindQueue(IBot bot, string queueType)
		{
			return AIUtils.FindQueuesByCategory(bot.Player)
				.Where(q => q.Key == queueType)
				.SelectMany(g => g)
				.FirstOrDefault(q => q.CurrentItem() == null || q.CurrentItem().Done);
		}

		static bool IsValidOwnedActor(Actor actor, IBot bot)
		{
			return actor != null && !actor.IsDead && actor.IsInWorld && actor.Owner == bot.Player;
		}

		static CPos? ChooseBuildLocation(World world, Player player, ActorInfo actorInfo, BuildingInfo bi, CPos? hint = null)
		{
			// Find center of our base from existing buildings
			var ownBuildings = world.ActorsHavingTrait<Building>()
				.Where(a => a.Owner == player && !a.IsDead && a.IsInWorld)
				.ToArray();

			if (ownBuildings.Length == 0)
				return null;

			// Use hint as search center when provided, otherwise base centroid.
			CPos center;
			if (hint.HasValue)
				center = hint.Value;
			else
			{
				var centerX = (int)ownBuildings.Average(a => a.Location.X);
				var centerY = (int)ownBuildings.Average(a => a.Location.Y);
				center = new CPos(centerX, centerY);
			}

			// Search outward from center
			foreach (var cell in world.Map.FindTilesInAnnulus(center, 0, 20))
			{
				if (!world.CanPlaceBuilding(cell, actorInfo, bi, null))
					continue;

				if (!bi.IsCloseEnoughToBase(world, player, actorInfo, cell))
					continue;

				return cell;
			}

			return null;
		}
	}
}
