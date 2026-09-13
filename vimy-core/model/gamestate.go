package model

type GameState struct {
	Tick             int               `json:"tick"`
	Player           Player            `json:"player"`
	Buildings        []Building        `json:"buildings"`
	Units            []Unit            `json:"units"`
	ProductionQueues []ProductionQueue `json:"productionQueues"`
	Enemies          []Enemy           `json:"enemies"`
	Capturables      []Enemy           `json:"capturables"`
	SupportPowers    []SupportPower    `json:"supportPowers"`
	MapWidth         int               `json:"mapWidth"`
	MapHeight        int               `json:"mapHeight"`
}

type Player struct {
	Name             string `json:"name"`
	Cash             int    `json:"cash"`
	Resources        int    `json:"resources"`
	ResourceCapacity int    `json:"resourceCapacity"`
	PowerProvided    int    `json:"powerProvided"`
	PowerDrained     int    `json:"powerDrained"`
	PowerState       string `json:"powerState"`

	// Ground truth from the engine's PlayerStatistics, as against what the
	// sidecar infers by watching. Kills were deduced from enemies vanishing,
	// which cannot separate a death from a walk into fog, and the trade was read
	// as a count of units — a medium tank and a rifleman are one unit each and
	// 850 credits apart.
	UnitsKilled     int `json:"unitsKilled"`
	UnitsDead       int `json:"unitsDead"`
	BuildingsKilled int `json:"buildingsKilled"`
	BuildingsDead   int `json:"buildingsDead"`
	KillsCost       int `json:"killsCost"`
	DeathsCost      int `json:"deathsCost"`
	ArmyValue       int `json:"armyValue"`
	AssetsValue     int `json:"assetsValue"`
	Earned          int `json:"earned"`
}

type Unit struct {
	ID         int    `json:"id"`
	Type       string `json:"type"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	HP         int    `json:"hp"`
	MaxHP      int    `json:"maxHp"`
	Idle       bool   `json:"idle"`
	CargoCount int    `json:"cargoCount"`
}

func (u Unit) TypeName() string { return u.Type }

type Building struct {
	ID    int    `json:"id"`
	Type  string `json:"type"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	HP    int    `json:"hp"`
	MaxHP int    `json:"maxHp"`
}

func (b Building) TypeName() string { return b.Type }

type ProductionQueue struct {
	Type            string   `json:"type"`
	Items           []string `json:"items"`
	Buildable       []string `json:"buildable"`
	CurrentItem     string   `json:"currentItem"`
	CurrentProgress int      `json:"currentProgress"`
}

type Enemy struct {
	ID    int    `json:"id"`
	Owner string `json:"owner"`
	Type  string `json:"type"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	HP    int    `json:"hp"`
	MaxHP int    `json:"maxHp"`
}

func (e Enemy) TypeName() string { return e.Type }

type SupportPower struct {
	Key            string `json:"key"`
	Ready          bool   `json:"ready"`
	RemainingTicks int    `json:"remainingTicks"`
	TotalTicks     int    `json:"totalTicks"`
}
