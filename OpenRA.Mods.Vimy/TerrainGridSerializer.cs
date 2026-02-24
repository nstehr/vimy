using System;
using System.Text.Json;
using System.Text.Json.Serialization;
using OpenRA.Traits;

namespace OpenRA.Mods.Vimy
{
	public class TerrainGridData
	{
		[JsonPropertyName("cols")]
		public int Cols { get; set; }

		[JsonPropertyName("rows")]
		public int Rows { get; set; }

		[JsonPropertyName("cellW")]
		public int CellW { get; set; }

		[JsonPropertyName("cellH")]
		public int CellH { get; set; }

		[JsonPropertyName("grid")]
		public int[] Grid { get; set; }
	}

	public static class TerrainGridSerializer
	{
		const int GridSize = 32;

		// Terrain categories matching the Go model.
		const int Land = 0;
		const int Water = 1;
		const int Cliff = 2;
		const int Bridge = 3;

		public static string Serialize(World world)
		{
			var map = world.Map;
			var mapW = map.MapSize.X;
			var mapH = map.MapSize.Y;

			var cellW = (int)Math.Ceiling((double)mapW / GridSize);
			var cellH = (int)Math.Ceiling((double)mapH / GridSize);

			var grid = new int[GridSize * GridSize];

			for (var row = 0; row < GridSize; row++)
			{
				for (var col = 0; col < GridSize; col++)
				{
					grid[row * GridSize + col] = ClassifyZone(map, col, row, cellW, cellH, mapW, mapH);
				}
			}

			var data = new TerrainGridData
			{
				Cols = GridSize,
				Rows = GridSize,
				CellW = cellW,
				CellH = cellH,
				Grid = grid
			};

			return JsonSerializer.Serialize(data);
		}

		static int ClassifyZone(Map map, int col, int row, int cellW, int cellH, int mapW, int mapH)
		{
			var startX = col * cellW;
			var startY = row * cellH;
			var endX = Math.Min(startX + cellW, mapW);
			var endY = Math.Min(startY + cellH, mapH);

			var total = 0;
			var waterCount = 0;
			var cliffCount = 0;
			var hasBridge = false;

			for (var y = startY; y < endY; y++)
			{
				for (var x = startX; x < endX; x++)
				{
					total++;
					var cell = new MPos(x, y);
					var tileInfo = map.Rules.TerrainInfo.GetTerrainInfo(map.Tiles[cell]);
					var typeName = map.Rules.TerrainInfo.TerrainTypes[tileInfo.TerrainType].Type;

					switch (typeName)
					{
						case "Bridge":
							hasBridge = true;
							break;
						case "Water":
							waterCount++;
							break;
						case "Rock":
						case "Tree":
						case "Wall":
							cliffCount++;
							break;
						// Clear, Road, Rough, Beach, River, Ore, Gems, etc. → Land
					}
				}
			}

			if (total == 0)
				return Land;

			// First match wins: bridge > water > cliff > land.
			if (hasBridge)
				return Bridge;
			if (waterCount * 2 >= total)
				return Water;
			if (cliffCount * 2 >= total)
				return Cliff;

			return Land;
		}
	}
}
