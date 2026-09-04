package rules

import "strings"

// typed is a generic constraint for any model type with a TypeName accessor.
type typed interface {
	TypeName() string
}

// matchesType handles OpenRA's faction variant naming (e.g. "afld.ukraine"
// matches "afld"). Without this, faction-specific buildings would be invisible
// to role-based queries.
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

func containsType[T typed](items []T, t string) bool {
	for _, item := range items {
		if matchesType(item.TypeName(), t) {
			return true
		}
	}
	return false
}

func countType[T typed](items []T, t string) int {
	n := 0
	for _, item := range items {
		if matchesType(item.TypeName(), t) {
			n++
		}
	}
	return n
}

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

// Production queue type constants — must match OpenRA's queue type names.
const (
	QueueBuilding = "Building"
	QueueDefense  = "Defense"
	QueueInfantry = "Infantry"
	QueueVehicle  = "Vehicle"
	QueueShip     = "Ship"
	QueueAircraft = "Aircraft"
)

// Unit type constants — OpenRA internal names (not display names).
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
	Hind          = "hind" // Soviet Hind attack helicopter
	Flamethrower  = "e4"   // Flamethrower infantry
	ShockTrooper  = "shok" // Shock Trooper (Russia only)
	TeslaTank     = "ttnk" // Tesla Tank (Russia only)
	Tanya         = "e7"   // Tanya (Allied commando)
	Medic         = "medi" // Medic
	Submarine     = "ss"   // Soviet Submarine
	MissileSub    = "msub" // Soviet Missile Submarine
	Gunboat       = "pt"   // Allied Gunboat
	Destroyer     = "dd"   // Allied Destroyer
	Cruiser       = "ca"   // Allied Cruiser
	Grenadier     = "e2"   // Grenadier (Soviet)
	AttackDog     = "dog"  // Attack Dog
	Spy           = "spy"  // Spy (Allied)
	MADTank       = "qtnk" // MAD Tank (Soviet)
	Minelayer     = "mnly" // Minelayer
)

// Building type constants — OpenRA internal names (not display names).
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
	MissileSilo      = "mslo" // Soviet Missile Silo (Nuke)
	IronCurtain      = "iron" // Soviet Iron Curtain
	GapGenerator     = "gap"  // Allied Gap Generator (creates shroud)
	Kennel           = "kenn" // Soviet Kennel (dog production building)
)

// Defense type constants — OpenRA internal names.
const (
	Pillbox     = "pbox" // Allied Pillbox
	CamoPillbox = "hbox" // Allied Camo Pillbox
	Turret      = "gun"  // Allied Gun Turret
	TeslaCoil   = "tsla" // Soviet Tesla Coil
	AAGun       = "agun" // Allied AA Gun
	SAMSite     = "sam"  // Soviet SAM Site
	FlameTower  = "ftur" // Soviet Flame Tower
)

// displayNames maps internal type codes to human-readable names.
var displayNames = map[string]string{
	// Units
	MCV: "MCV", Harvester: "Ore Harvester",
	RifleInfantry: "Rifle Infantry", Grenadier: "Grenadier", RocketSoldier: "Rocket Soldier",
	Engineer: "Engineer", Flamethrower: "Flamethrower", ShockTrooper: "Shock Trooper",
	Tanya: "Tanya", Medic: "Medic", AttackDog: "Attack Dog", Spy: "Spy", "thf": "Thief",
	LightTank: "Light Tank", MediumTank: "Medium Tank",
	HeavyTank: "Heavy Tank", MammothTank: "Mammoth Tank",
	V2Launcher: "V2 Launcher", APC: "APC", FlakTruck: "Flak Truck",
	DemoTruck: "Demo Truck", Ranger: "Ranger", Artillery: "Artillery",
	TeslaTank: "Tesla Tank", Minelayer: "Minelayer", MADTank: "MAD Tank",
	BlackHawk: "Black Hawk", Longbow: "Longbow", MiG: "MiG", Yak: "Yak", Hind: "Hind",
	"badr": "Badger Bomber", "u2": "Spy Plane", "camera": "Camera",
	Submarine: "Submarine", MissileSub: "Missile Sub",
	Gunboat: "Gunboat", Destroyer: "Destroyer", Cruiser: "Cruiser",
	"tran": "Chinook Transport",
	// Buildings
	ConstructionYard: "Construction Yard", PowerPlant: "Power Plant",
	AdvancedPower: "Advanced Power", Refinery: "Ore Refinery",
	OreSilo: "Ore Silo", WarFactory: "War Factory",
	AlliedBarracks: "Allied Barracks", SovietBarracks: "Soviet Barracks",
	AlliedTechCenter: "Allied Tech Center", SovietTechCenter: "Soviet Tech Center",
	RadarDome: "Radar Dome", Airfield: "Airfield", Helipad: "Helipad",
	NavalYard: "Naval Yard", SubPen: "Sub Pen",
	ServiceDepot: "Service Depot", MissileSilo: "Missile Silo",
	IronCurtain: "Iron Curtain", GapGenerator: "Gap Generator", Kennel: "Kennel",
	// Defenses
	Pillbox: "Pillbox", CamoPillbox: "Camo Pillbox", Turret: "Gun Turret",
	TeslaCoil: "Tesla Coil", AAGun: "AA Gun", SAMSite: "SAM Site",
	FlameTower: "Flame Tower",
}

// DisplayName returns a human-readable name for an internal type code.
// Faction variants like "afld.ukraine" are resolved to the base type.
// Unknown codes are returned as-is.
func DisplayName(code string) string {
	if name, ok := displayNames[code]; ok {
		return name
	}
	// Handle faction variants (e.g. "afld.ukraine" → "afld")
	if dot := strings.IndexByte(code, '.'); dot > 0 {
		if name, ok := displayNames[code[:dot]]; ok {
			return name
		}
	}
	return code
}

// role abstracts over faction-specific type names. The compiler and env
// methods use roles so rules say "barracks" instead of checking for both
// "barr" (Soviet) and "tent" (Allied).
type role struct {
	queue string   // which production queue builds this
	types []string // all faction variants (e.g. barr + tent for barracks)
}

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
	"missile_silo":      {queue: QueueDefense, types: []string{MissileSilo}},
	"iron_curtain":      {queue: QueueDefense, types: []string{IronCurtain}},
	"basic_aircraft":    {queue: QueueAircraft, types: []string{BlackHawk, Yak, Hind}},
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
	"tesla_tank":        {queue: QueueVehicle, types: []string{TeslaTank}},
	"tanya":             {queue: QueueInfantry, types: []string{Tanya}},
	"medic":             {queue: QueueInfantry, types: []string{Medic}},
	"engineer":          {queue: QueueInfantry, types: []string{Engineer}},
	"submarine":         {queue: QueueShip, types: []string{Submarine}},
	"missile_sub":       {queue: QueueShip, types: []string{MissileSub}},
	"destroyer":         {queue: QueueShip, types: []string{Destroyer}},
	"cruiser":           {queue: QueueShip, types: []string{Cruiser}},
	"gunboat":           {queue: QueueShip, types: []string{Gunboat}},
	"pillbox":           {queue: QueueDefense, types: []string{Pillbox}},
	"camo_pillbox":      {queue: QueueDefense, types: []string{CamoPillbox}},
	"turret":            {queue: QueueDefense, types: []string{Turret}},
	"tesla_coil":        {queue: QueueDefense, types: []string{TeslaCoil}},
	"aa_defense":        {queue: QueueDefense, types: []string{AAGun, SAMSite}},
	"flame_tower":       {queue: QueueDefense, types: []string{FlameTower}},
	"gap_generator":     {queue: QueueDefense, types: []string{GapGenerator}},
	"advanced_power":    {queue: QueueBuilding, types: []string{AdvancedPower}},
	"ore_silo":          {queue: QueueBuilding, types: []string{OreSilo}},
	"harvester":         {queue: QueueVehicle, types: []string{Harvester}},
	"grenadier":         {queue: QueueInfantry, types: []string{Grenadier}},
	"attack_dog":        {queue: QueueInfantry, types: []string{AttackDog}},
	"spy":               {queue: QueueInfantry, types: []string{Spy}},
	"mad_tank":          {queue: QueueVehicle, types: []string{MADTank}},
	"minelayer":         {queue: QueueVehicle, types: []string{Minelayer}},
	"kennel":            {queue: QueueBuilding, types: []string{Kennel}},
}

// combatVehicleRoles determines production priority — first buildable role wins.
// Order: heaviest armor first, then support vehicles.
// APC is excluded — it's a transport with dedicated capture/assault production rules.
var combatVehicleRoles = []string{
	"heavy_tank", "medium_tank", "tesla_tank", "light_tank",
	"v2_launcher", "artillery", "ranger",
	"flak_truck", "demo_truck", "mad_tank",
}
// Note: minelayer is excluded from combatVehicleRoles — it has dedicated
// production and mine-laying rules gated on GroundDefensePriority.

// combatAircraftRoles: advanced (longbow/MiG) preferred over basic (blackhawk/yak).
var combatAircraftRoles = []string{
	"advanced_aircraft", "basic_aircraft",
}

// combatNavalRoles: heaviest firepower first.
var combatNavalRoles = []string{
	"cruiser", "missile_sub", "destroyer", "submarine", "gunboat",
}

// specialistInfantryRoles: most impactful unit first.
var specialistInfantryRoles = []string{
	"tanya", "shock_trooper", "flamethrower", "medic",
}
