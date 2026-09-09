package rules

import (
	"sort"
	"strings"
)

// typed is a generic constraint for any model type with a TypeName accessor.
type typed interface {
	TypeName() string
}

// matchesType resolves OpenRA's faction variants ("afld.ukraine" matches
// "afld"), which role-based queries would otherwise miss entirely.
func matchesType(name, t string) bool {
	if strings.EqualFold(name, t) {
		return true
	}
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

// DisplayName resolves faction variants to their base type and returns unknown
// codes unchanged.
func DisplayName(code string) string {
	if name, ok := displayNames[code]; ok {
		return name
	}
	if dot := strings.IndexByte(code, '.'); dot > 0 {
		if name, ok := displayNames[code[:dot]]; ok {
			return name
		}
	}
	return code
}

// role abstracts over faction-specific type names, so a rule says "barracks"
// rather than testing for both "barr" and "tent".
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

// combatVehicleRoles is production priority order, heaviest armor first. APC and
// minelayer are absent: both have dedicated production rules.
var combatVehicleRoles = []string{
	"heavy_tank", "medium_tank", "tesla_tank", "light_tank",
	"v2_launcher", "artillery", "ranger",
	"flak_truck", "demo_truck", "mad_tank",
}

// cappedVehicleRoles each have a dedicated production rule whose cap comes from
// a doctrine weight. produce-vehicle must not build them: it caps on
// CombatVehicleCount instead, laundering past whatever the dedicated rule
// computed — flak trucks reached 5 alive against a cap of 2 that way, and the
// governing knob had no effect at all.
//
// demo_truck is deliberately absent: it has no rule of its own, so excluding it
// would mean nothing builds it.
var cappedVehicleRoles = map[string]bool{
	"flak_truck": true,
	"mad_tank":   true,
}

// genericVehicleRoles is what `produce-vehicle` may choose from.
func genericVehicleRoles(roles []string) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		if !cappedVehicleRoles[r] {
			out = append(out, r)
		}
	}
	return out
}

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

// Roles only one side can build.
//
// The strategist is told which these are and lists the other side's anyway — a
// prompt is not a constraint. Harmless where a unit is chosen, since
// bestBuildableFrom skips what the faction can't make; not harmless in
// DoctrineParams, which reads the raw list and moves rule priorities on the
// strength of a name. Hence the filter upstream of both.
//
// Only genuinely locked roles belong here: medium_tank covers both sides' mid
// tanks and stays.
var factionLockedRoles = map[string]string{
	"v2_launcher":   "soviet",
	"apc":           "soviet",
	"flak_truck":    "soviet",
	"demo_truck":    "soviet",
	"tesla_tank":    "soviet",
	"shock_trooper": "soviet",
	"flamethrower":  "soviet",
	"heavy_tank":    "soviet",
	"artillery":     "allied",
	"ranger":        "allied",
	"light_tank":    "allied",
	"tanya":         "allied",
	"missile_sub":   "soviet",
	"cruiser":       "allied",
	"destroyer":     "allied",
}

// sovietFactions build from the Soviet tree; anything else is treated as
// Allied, so an unknown faction loses only the Soviet-locked roles.
var sovietFactions = map[string]bool{
	"soviet": true, "russia": true, "ukraine": true, "iraq": true, "russians": true,
}

// SideOf reports which tree a faction builds from.
func SideOf(faction string) string {
	if sovietFactions[strings.ToLower(strings.TrimSpace(faction))] {
		return "soviet"
	}
	return "allied"
}

// BuildableByFaction reports whether a role is available to a side.
func BuildableByFaction(role, faction string) bool {
	locked, ok := factionLockedRoles[strings.ToLower(role)]
	return !ok || locked == SideOf(faction)
}

// FilterPreferences drops roles the faction cannot build and returns them, so
// the caller can attribute them to a doctrine. How often the strategist names
// the other side's units is worth knowing; a silent filter would hide it.
func FilterPreferences(d Doctrine, faction string) (Doctrine, []string) {
	var dropped []string
	keep := func(list []string) []string {
		if len(list) == 0 {
			return list
		}
		out := make([]string, 0, len(list))
		for _, role := range list {
			if BuildableByFaction(role, faction) {
				out = append(out, role)
				continue
			}
			dropped = append(dropped, role)
		}
		return out
	}
	d.PreferredInfantry = keep(d.PreferredInfantry)
	d.PreferredVehicle = keep(d.PreferredVehicle)
	d.PreferredAircraft = keep(d.PreferredAircraft)
	d.PreferredNaval = keep(d.PreferredNaval)
	return d, dropped
}

// IsRole reports whether a name is something the rule set builds.
//
// Exported for analysis, which otherwise reads count(unassigned-idle-ground) as
// a test for a unit called "ground" and reports that nothing produces one.
func IsRole(name string) bool {
	_, ok := roles[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// UnbuildableRoles lists what this faction cannot build, sorted so the prompt it
// feeds is stable between windows.
//
// Rosters keyed by "Soviet:" and "Allied:" ask the strategist to know that
// germany is Allied, and it doesn't — it asked for v2_launcher four windows
// running. Naming the forbidden units outright is cheaper than filtering after.
func UnbuildableRoles(faction string) []string {
	side := SideOf(faction)
	var out []string
	for role, locked := range factionLockedRoles {
		if locked != side {
			out = append(out, role)
		}
	}
	sort.Strings(out)
	return out
}

// supportPowerCountries records which factions can ever hold a support power,
// read from the mod's prerequisites:
//
//	SovietParatroopers  aircraft.soviet
//	SovietSpyPlane      structures.russia
//	UkraineParabombs    aircraft.ukraine
//
// Country-gated, not side-gated: a Ukrainian player cannot call the Russian spy
// plane, so "Soviet" is too coarse an answer.
//
// Absence means unknown, not impossible — NukePowerInfoOrder has no prerequisite
// we could find, and calling an idle rule unsatisfiable is exactly the error
// this table exists to prevent.
var supportPowerCountries = map[string]map[string]bool{
	"SovietParatroopers": sovietFactions,
	"SovietSpyPlane":     {"russia": true},
	"UkraineParabombs":   {"ukraine": true},
}

// SupportPowerReachable is true for powers unknown to the table: silence is no
// evidence, and only evidence should let a caller call a rule impossible.
func SupportPowerReachable(power, faction string) bool {
	allowed, known := supportPowerCountries[power]
	if !known {
		return true
	}
	return allowed[strings.ToLower(strings.TrimSpace(faction))]
}

// IsUnarmedStructure reports whether an enemy actor type is a building that
// cannot shoot back — a refinery, a power plant, a war factory.
//
// It exists because SquadThreatRatio sums the HP of everything the enemy owns
// within a radius, and the mod deliberately includes structures in the enemy
// list ("IOccupySpace covers both buildings and mobile units"). So a squad sent
// to attack a base found the base itself counted as the threat: game 94's
// ground-attack squad read a median threat ratio of 7.96 and a p90 of 21 while
// engaged, and squad-disengage — which fires above about 2.5 — decided to
// withdraw 31 times against 15 attacks. It was retreating from its own
// objective.
//
// Defensive structures are NOT unarmed and still count: a pillbox is a real
// reason for a squad to leave. The distinction is whether the thing shoots
// back, which the queue already encodes.
func IsUnarmedStructure(actorType string) bool {
	for _, spec := range roles {
		if spec.queue != QueueBuilding {
			continue
		}
		for _, t := range spec.types {
			if matchesType(actorType, t) {
				return true
			}
		}
	}
	return false
}
