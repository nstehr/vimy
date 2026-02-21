package rules

import "strings"

// typed is a generic constraint for any model type with a TypeName accessor.
type typed interface {
	TypeName() string
}

// containsType returns true if any item's TypeName matches t (case-insensitive).
func containsType[T typed](items []T, t string) bool {
	for _, item := range items {
		if strings.EqualFold(item.TypeName(), t) {
			return true
		}
	}
	return false
}

// countType counts items whose TypeName matches t (case-insensitive).
func countType[T typed](items []T, t string) int {
	n := 0
	for _, item := range items {
		if strings.EqualFold(item.TypeName(), t) {
			n++
		}
	}
	return n
}

// containsAnyType returns true if any item's TypeName matches any of the given types.
func containsAnyType[T typed](items []T, types []string) bool {
	for _, item := range items {
		for _, t := range types {
			if strings.EqualFold(item.TypeName(), t) {
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
			if strings.EqualFold(item.TypeName(), t) {
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
)

// Building type constants.
const (
	ConstructionYard = "fact" // Construction Yard
	PowerPlant       = "powr" // Power Plant
	Refinery         = "proc" // Ore Refinery
	WarFactory       = "weap" // War Factory
	AlliedBarracks   = "tent" // Allied Barracks
	SovietBarracks   = "barr" // Soviet Barracks
	AlliedTechCenter = "atek" // Allied Tech Center
	SovietTechCenter = "stek" // Soviet Tech Center
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
}
