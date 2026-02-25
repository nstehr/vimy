using System.Collections.Generic;
using System.Linq;
using System.Text.Json;
using System.Text.Json.Serialization;
using OpenRA.Mods.Common;
using OpenRA.Mods.Common.Activities;
using OpenRA.Mods.Common.Traits;
using OpenRA.Traits;

namespace OpenRA.Mods.Vimy
{
	public class PlayerData
	{
		[JsonPropertyName("name")]
		public string Name { get; set; }

		[JsonPropertyName("cash")]
		public int Cash { get; set; }

		[JsonPropertyName("resources")]
		public int Resources { get; set; }

		[JsonPropertyName("resourceCapacity")]
		public int ResourceCapacity { get; set; }

		[JsonPropertyName("powerProvided")]
		public int PowerProvided { get; set; }

		[JsonPropertyName("powerDrained")]
		public int PowerDrained { get; set; }

		[JsonPropertyName("powerState")]
		public string PowerState { get; set; }
	}

	public class ActorData
	{
		[JsonPropertyName("type")]
		public string Type { get; set; }

		[JsonPropertyName("id")]
		public uint Id { get; set; }

		[JsonPropertyName("x")]
		public int X { get; set; }

		[JsonPropertyName("y")]
		public int Y { get; set; }

		[JsonPropertyName("hp")]
		public int Hp { get; set; }

		[JsonPropertyName("maxHp")]
		public int MaxHp { get; set; }
	}

	public class UnitData : ActorData
	{
		[JsonPropertyName("idle")]
		public bool Idle { get; set; }

		[JsonPropertyName("cargoCount")]
		public int CargoCount { get; set; }
	}

	public class EnemyActorData : ActorData
	{
		[JsonPropertyName("owner")]
		public string Owner { get; set; }
	}

	public class SupportPowerData
	{
		[JsonPropertyName("key")]
		public string Key { get; set; }

		[JsonPropertyName("ready")]
		public bool Ready { get; set; }

		[JsonPropertyName("remainingTicks")]
		public int RemainingTicks { get; set; }

		[JsonPropertyName("totalTicks")]
		public int TotalTicks { get; set; }
	}

	public class ProductionQueueData
	{
		[JsonPropertyName("type")]
		public string Type { get; set; }

		[JsonPropertyName("items")]
		public List<string> Items { get; set; } = new();

		[JsonPropertyName("buildable")]
		public List<string> Buildable { get; set; } = new();

		[JsonPropertyName("currentItem")]
		public string CurrentItem { get; set; }

		[JsonPropertyName("currentProgress")]
		public int CurrentProgress { get; set; }
	}

	public class GameStateData
	{
		[JsonPropertyName("tick")]
		public int Tick { get; set; }

		[JsonPropertyName("player")]
		public PlayerData Player { get; set; }

		[JsonPropertyName("buildings")]
		public List<ActorData> Buildings { get; set; } = new();

		[JsonPropertyName("units")]
		public List<UnitData> Units { get; set; } = new();

		[JsonPropertyName("productionQueues")]
		public List<ProductionQueueData> ProductionQueues { get; set; } = new();

		[JsonPropertyName("enemies")]
		public List<EnemyActorData> Enemies { get; set; } = new();

		[JsonPropertyName("capturables")]
		public List<EnemyActorData> Capturables { get; set; } = new();

		[JsonPropertyName("supportPowers")]
		public List<SupportPowerData> SupportPowers { get; set; } = new();

		[JsonPropertyName("mapWidth")]
		public int MapWidth { get; set; }

		[JsonPropertyName("mapHeight")]
		public int MapHeight { get; set; }
	}

	public static class GameStateSerializer
	{
		static readonly JsonSerializerOptions JsonOptions = new()
		{
			DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull
		};

		public static string Serialize(World world, IBot bot)
		{
			var state = new GameStateData
			{
				Tick = world.WorldTick,
				Player = SerializePlayer(bot),
				Buildings = SerializeBuildings(world, bot),
				Units = SerializeUnits(world, bot),
				ProductionQueues = SerializeProductionQueues(bot),
				Enemies = SerializeEnemies(world, bot),
				Capturables = SerializeCapturables(world, bot),
				SupportPowers = SerializeSupportPowers(bot),
				MapWidth = world.Map.MapSize.X,
				MapHeight = world.Map.MapSize.Y
			};

			return JsonSerializer.Serialize(state, JsonOptions);
		}

		static PlayerData SerializePlayer(IBot bot)
		{
			var player = bot.Player;
			var resources = player.PlayerActor.Trait<PlayerResources>();
			var power = player.PlayerActor.TraitOrDefault<PowerManager>();

			return new PlayerData
			{
				Name = player.PlayerName,
				Cash = resources.Cash,
				Resources = resources.Resources,
				ResourceCapacity = resources.ResourceCapacity,
				PowerProvided = power?.PowerProvided ?? 0,
				PowerDrained = power?.PowerDrained ?? 0,
				PowerState = power?.PowerState.ToString() ?? "Normal"
			};
		}

		static List<ActorData> SerializeBuildings(World world, IBot bot)
		{
			var buildings = new List<ActorData>();

			foreach (var actor in world.ActorsHavingTrait<Building>())
			{
				if (actor.Owner != bot.Player || actor.IsDead || !actor.IsInWorld)
					continue;

				var health = actor.TraitOrDefault<Health>();
				buildings.Add(new ActorData
				{
					Type = actor.Info.Name,
					Id = actor.ActorID,
					X = actor.Location.X,
					Y = actor.Location.Y,
					Hp = health?.HP ?? 0,
					MaxHp = health?.MaxHP ?? 0
				});
			}

			return buildings;
		}

		static List<UnitData> SerializeUnits(World world, IBot bot)
		{
			var units = new List<UnitData>();

			foreach (var actor in world.ActorsHavingTrait<IPositionable>())
			{
				if (actor.Owner != bot.Player || actor.IsDead || !actor.IsInWorld)
					continue;

				// Skip buildings — they're serialized separately
				if (actor.Info.HasTraitInfo<BuildingInfo>())
					continue;

				var health = actor.TraitOrDefault<Health>();
				units.Add(new UnitData
				{
					Type = actor.Info.Name,
					Id = actor.ActorID,
					X = actor.Location.X,
					Y = actor.Location.Y,
					Hp = health?.HP ?? 0,
					MaxHp = health?.MaxHP ?? 0,
					Idle = IsEffectivelyIdle(actor),
					CargoCount = actor.TraitOrDefault<Cargo>()?.PassengerCount ?? 0
				});
			}

			return units;
		}

		// Aircraft never truly become idle — they always have an activity:
		// ReturnToBase → Resupply → TakeOff → FlyIdle (circling), or
		// ReturnToBase → FlyIdle. Treat fully-armed aircraft in any of
		// these holding-pattern activities as idle.
		static bool IsEffectivelyIdle(Actor actor)
		{
			if (actor.IsIdle)
				return true;

			if (actor.Info.HasTraitInfo<AircraftInfo>() && actor.CurrentActivity != null)
			{
				// FlyIdle = circling with nothing to do — always idle.
				if (actor.CurrentActivity.ActivitiesImplementing<FlyIdle>().Any())
					return true;

				// ReturnToBase/Resupply cycle — idle once fully rearmed.
				var isRearming = actor.CurrentActivity.ActivitiesImplementing<Resupply>().Any()
					|| actor.CurrentActivity.ActivitiesImplementing<ReturnToBase>().Any();
				if (isRearming)
				{
					var ammoPools = actor.TraitsImplementing<AmmoPool>();
					return !ammoPools.Any() || ammoPools.All(a => a.HasFullAmmo);
				}
			}

			return false;
		}

		static List<ProductionQueueData> SerializeProductionQueues(IBot bot)
		{
			var queues = new List<ProductionQueueData>();
			var queuesByCategory = AIUtils.FindQueuesByCategory(bot.Player);

			foreach (var group in queuesByCategory)
			{
				foreach (var queue in group)
				{
					var queueData = new ProductionQueueData
					{
						Type = queue.Info.Type
					};

					var currentItem = queue.CurrentItem();
					if (currentItem != null)
					{
						queueData.CurrentItem = currentItem.Item;
						queueData.CurrentProgress = currentItem.TotalTime > 0
							? 100 - (100 * currentItem.RemainingTime / currentItem.TotalTime)
							: 0;
					}

					foreach (var item in queue.AllQueued())
						queueData.Items.Add(item.Item);

					foreach (var item in queue.BuildableItems())
						queueData.Buildable.Add(item.Name);

					queues.Add(queueData);
				}
			}

			return queues;
		}

		static List<SupportPowerData> SerializeSupportPowers(IBot bot)
		{
			var powers = new List<SupportPowerData>();
			var spm = bot.Player.PlayerActor.TraitOrDefault<SupportPowerManager>();
			if (spm == null)
				return powers;

			foreach (var kvp in spm.Powers)
			{
				var instance = kvp.Value;
				if (instance.Disabled)
					continue;

				powers.Add(new SupportPowerData
				{
					Key = instance.Key,
					Ready = instance.Ready,
					RemainingTicks = instance.RemainingTicks,
					TotalTicks = instance.TotalTicks
				});
			}
			return powers;
		}

		static List<EnemyActorData> SerializeCapturables(World world, IBot bot)
		{
			var capturables = new List<EnemyActorData>();

			foreach (var actor in world.ActorsHavingTrait<Capturable>())
			{
				if (actor.Owner == bot.Player || actor.IsDead || !actor.IsInWorld)
					continue;

				if (!actor.CanBeViewedByPlayer(bot.Player))
					continue;

				var health = actor.TraitOrDefault<Health>();
				capturables.Add(new EnemyActorData
				{
					Type = actor.Info.Name,
					Id = actor.ActorID,
					Owner = actor.Owner.PlayerName,
					X = actor.Location.X,
					Y = actor.Location.Y,
					Hp = health?.HP ?? 0,
					MaxHp = health?.MaxHP ?? 0
				});
			}

			return capturables;
		}

		static List<EnemyActorData> SerializeEnemies(World world, IBot bot)
		{
			var enemies = new List<EnemyActorData>();

			// IOccupySpace covers both buildings (Building trait) and mobile units
			// (IPositionable extends IOccupySpace). Using IOccupySpace ensures
			// enemy structures are included for intel gathering.
			foreach (var actor in world.ActorsHavingTrait<IOccupySpace>())
			{
				if (actor.IsDead || !actor.IsInWorld)
					continue;

				if (bot.Player.RelationshipWith(actor.Owner) != PlayerRelationship.Enemy)
					continue;

				// Fog-of-war filter: only include visible enemies
				if (!actor.CanBeViewedByPlayer(bot.Player))
					continue;

				var health = actor.TraitOrDefault<Health>();
				enemies.Add(new EnemyActorData
				{
					Type = actor.Info.Name,
					Id = actor.ActorID,
					Owner = actor.Owner.PlayerName,
					X = actor.Location.X,
					Y = actor.Location.Y,
					Hp = health?.HP ?? 0,
					MaxHp = health?.MaxHP ?? 0
				});
			}

			return enemies;
		}
	}
}
