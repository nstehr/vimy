package rules

import "strings"

// typed is a generic constraint for any model type with a TypeName accessor.
type typed interface {
	TypeName() string
}

// matchesType checks if name matches type t, handling OpenRA faction variants.
// "afld.ukraine" matches base type "afld"; "afld" matches "afld" exactly.
func matchesType(name, t string) bool {
	if strings.EqualFold(name, t) {
		return true
	}
	// Check for faction variant: name starts with t followed by "."
	if len(name) > len(t) && strings.EqualFold(name[:len(t)], t) && name[len(t)] == '.' {
		return true
	}
	return false
}

// containsType returns true if any item's TypeName matches t (case-insensitive).
func containsType[T typed](items []T, t string) bool {
	for _, item := range items {
		if matchesType(item.TypeName(), t) {
			return true
		}
	}
	return false
}

// countType counts items whose TypeName matches t (case-insensitive).
func countType[T typed](items []T, t string) int {
	n := 0
	for _, item := range items {
		if matchesType(item.TypeName(), t) {
			n++
		}
	}
	return n
}

// containsAnyType returns true if any item's TypeName matches any of the given types.
func containsAnyType[T typed](items []T, types []string) bool {
	for _, item := range items {
		for _, t := range types {
			if matchesType(item.TypeName(), t) {
				return true
			}
		}
	}
	return false
}

// countAnyType counts items whose TypeName matches any of the given types.
func countAnyType[T typed](items []T, types []string) int {
	n := 0
	for _, item := range items {
		for _, t := range types {
			if matchesType(item.TypeName(), t) {
				n++
				break
			}
		}
	}
	return n
}

// Production queue type constants.
const (
	QueueBuilding = "Building"
	QueueDefense  = "Defense"
	QueueInfantry = "Infantry"
	QueueVehicle  = "Vehicle"
	QueueShip     = "Ship"
	QueueAircraft = "Aircraft"
)

// Unit type constants.
const (
	MCV           = "mcv"  // Mobile Construction Vehicle
	Harvester     = "harv" // Ore Harvester
	RifleInfantry = "e1"   // Rifle Infantry
	RocketSoldier = "e3"   // Rocket Soldier
	Engineer      = "e6"   // Engineer
	LightTank     = "1tnk" // Allied Light Tank
	MediumTank    = "2tnk" // Allied Medium Tank
	HeavyTank     = "3tnk" // Soviet Heavy Tank
	MammothTank   = "4tnk" // Soviet Mammoth Tank
	V2Launcher    = "v2rl" // V2 Rocket Launcher
	APC           = "apc"  // Armored Personnel Carrier
	FlakTruck     = "ftrk" // Flak Truck
	DemoTruck     = "dtrk" // Demolition Truck
	Ranger        = "jeep" // Allied Ranger
	Artillery     = "arty" // Allied Artillery
	BlackHawk     = "mh60" // Allied Black Hawk helicopter
	Longbow       = "heli" // Allied Longbow helicopter
	MiG           = "mig"  // Soviet MiG attack aircraft
	Yak           = "yak"  // Soviet Yak attack aircraft
	Flamethrower  = "e4"   // Flamethrower infantry
	ShockTrooper  = "shok" // Shock Trooper (Russia only)
	Tanya         = "e7"   // Tanya (Allied commando)
	Medic         = "medi" // Medic
	Submarine     = "ss"   // Soviet Submarine
	MissileSub    = "msub" // Soviet Missile Submarine
	Gunboat       = "pt"   // Allied Gunboat
	Destroyer     = "dd"   // Allied Destroyer
	Cruiser       = "ca"   // Allied Cruiser
)

// Building type constants.
const (
	ConstructionYard = "fact" // Construction Yard
	PowerPlant       = "powr" // Power Plant
	AdvancedPower    = "apwr" // Advanced Power Plant
	Refinery         = "proc" // Ore Refinery
	OreSilo          = "silo" // Ore Silo
	WarFactory       = "weap" // War Factory
	AlliedBarracks   = "tent" // Allied Barracks
	SovietBarracks   = "barr" // Soviet Barracks
	AlliedTechCenter = "atek" // Allied Tech Center
	SovietTechCenter = "stek" // Soviet Tech Center
	RadarDome        = "dome" // Radar Dome
	Airfield         = "afld" // Soviet Airfield
	Helipad          = "hpad" // Allied Helipad
	NavalYard        = "syrd" // Allied Naval Yard (Shipyard)
	SubPen           = "spen" // Soviet Sub Pen
	ServiceDepot     = "fix"  // Service Depot (unlocks Heavy Tank)
)

// role maps a logical role name to its production queue and all faction-variant type names.
type role struct {
	queue string   // production queue constant
	types []string // all faction variants for this role
}

// roles is the static registry of logical roles to concrete type names.
var roles = map[string]role{
	"barracks":          {queue: QueueBuilding, types: []string{AlliedBarracks, SovietBarracks}},
	"power_plant":       {queue: QueueBuilding, types: []string{PowerPlant}},
	"refinery":          {queue: QueueBuilding, types: []string{Refinery}},
	"war_factory":       {queue: QueueBuilding, types: []string{WarFactory}},
	"construction_yard": {queue: QueueBuilding, types: []string{ConstructionYard}},
	"tech_center":       {queue: QueueBuilding, types: []string{AlliedTechCenter, SovietTechCenter}},
	"radar":             {queue: QueueBuilding, types: []string{RadarDome}},
	"airfield":          {queue: QueueBuilding, types: []string{Airfield, Helipad}},
	"naval_yard":        {queue: QueueBuilding, types: []string{NavalYard, SubPen}},
	"service_depot":     {queue: QueueBuilding, types: []string{ServiceDepot}},
	"basic_aircraft":    {queue: QueueAircraft, types: []string{BlackHawk, Yak}},
	"advanced_aircraft": {queue: QueueAircraft, types: []string{Longbow, MiG}},
	"light_tank":        {queue: QueueVehicle, types: []string{LightTank}},
	"medium_tank":       {queue: QueueVehicle, types: []string{MediumTank, HeavyTank}},
	"heavy_tank":        {queue: QueueVehicle, types: []string{MammothTank}},
	"v2_launcher":       {queue: QueueVehicle, types: []string{V2Launcher}},
	"apc":               {queue: QueueVehicle, types: []string{APC}},
	"flak_truck":        {queue: QueueVehicle, types: []string{FlakTruck}},
	"demo_truck":        {queue: QueueVehicle, types: []string{DemoTruck}},
	"ranger":            {queue: QueueVehicle, types: []string{Ranger}},
	"artillery":         {queue: QueueVehicle, types: []string{Artillery}},
	"rocket_soldier":    {queue: QueueInfantry, types: []string{RocketSoldier}},
	"flamethrower":      {queue: QueueInfantry, types: []string{Flamethrower}},
	"shock_trooper":     {queue: QueueInfantry, types: []string{ShockTrooper}},
	"tanya":             {queue: QueueInfantry, types: []string{Tanya}},
	"medic":             {queue: QueueInfantry, types: []string{Medic}},
	"engineer":          {queue: QueueInfantry, types: []string{Engineer}},
	"submarine":         {queue: QueueShip, types: []string{Submarine, MissileSub}},
	"destroyer":         {queue: QueueShip, types: []string{Destroyer}},
	"cruiser":           {queue: QueueShip, types: []string{Cruiser}},
	"gunboat":           {queue: QueueShip, types: []string{Gunboat}},
	"pillbox":           {queue: QueueDefense, types: []string{"pbox", "hbox"}},
	"turret":            {queue: QueueDefense, types: []string{"gun"}},
	"tesla_coil":        {queue: QueueDefense, types: []string{"tsla"}},
	"aa_defense":        {queue: QueueDefense, types: []string{"agun", "sam"}},
	"flame_tower":       {queue: QueueDefense, types: []string{"ftur"}},
	"advanced_power":    {queue: QueueBuilding, types: []string{AdvancedPower}},
	"ore_silo":          {queue: QueueBuilding, types: []string{OreSilo}},
	"harvester":         {queue: QueueVehicle, types: []string{Harvester}},
}

// combatVehicleRoles lists roles that represent combat vehicles, ordered by preference.
// Used by generic vehicle production helpers to work across all factions.
var combatVehicleRoles = []string{
	"heavy_tank", "medium_tank", "light_tank",
	"v2_launcher", "artillery", "ranger",
	"flak_truck", "apc", "demo_truck",
}

// combatAircraftRoles lists roles that represent combat aircraft, ordered by preference.
var combatAircraftRoles = []string{
	"advanced_aircraft", "basic_aircraft",
}

// combatNavalRoles lists roles that represent combat naval units, ordered by preference.
var combatNavalRoles = []string{
	"cruiser", "destroyer", "submarine", "gunboat",
}

// specialistInfantryRoles lists roles for elite infantry, ordered by priority (best first).
var specialistInfantryRoles = []string{
	"tanya", "shock_trooper", "flamethrower", "medic",
}
