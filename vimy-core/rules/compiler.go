package rules

import (
	"fmt"
)

// Doctrine gate thresholds control which rule blocks CompileDoctrine emits.
// Named constants make the vocabulary self-documenting across 40+ guard checks.
const (
	DoctrineEnabled     = 0.1 // non-trivial weight; include basic rules
	DoctrineModerate    = 0.2 // enough priority to warrant moderate investment
	DoctrineSignificant = 0.3 // warrants dedicated buildings or rule blocks
	DoctrineHigh        = 0.4 // advanced capabilities (tech center, attack aircraft)
	DoctrineDominant    = 0.5 // heavy investment (economy scaling, extra buildings)
	DoctrineExtreme     = 0.6 // extra production buildings for this domain
)

// Priority offsets encode relative rule ordering constraints that aren't
// obvious from bare integer literals.
const (
	SquadFormBonus    = 5  // form-squad fires just above its squad-act rule
	ReengageDiscount  = 2  // re-engage fires just below coordinated attack
	KnownBaseDiscount = 10 // attack-known-base fires below direct-enemy attack
	AirDomainOffset   = 5  // air attack base priority below ground
	NavalDomainOffset = 15 // naval attack base priority below ground
)

// Per-queue production categories. Exclusive within each queue so only the
// highest-priority rule whose conditions are met fires per tick. This prevents
// multiple items being queued per tick (e.g. engineer+flamethrower+rifle all
// queued on the Infantry queue in one tick) and ensures army composition
// matches doctrine priority ordering.
const (
	CatProduceInfantry = "produce_infantry"
	CatProduceVehicle  = "produce_vehicle"
	CatProduceAircraft = "produce_aircraft"
	CatProduceShip     = "produce_ship"
)

// Gameplay thresholds used inside expr condition strings (require fmt.Sprintf).
const (
	LowPowerHeadroom    = 50 // PowerExcess below this triggers advanced power
	IronCurtainMinUnits = 3  // minimum idle ground units to fire iron curtain
)

// buildingSaving prevents unit production from consuming cash needed for
// a high-value building (e.g. tech center at 1500 credits). Once the
// building exists, the savings constraint is released.
type buildingSaving struct {
	existsExpr string // expr condition that the building already exists
	cost       int    // cash needed to queue the building
}

// doctrineCompiler holds the shared state used across all rule-generation
// methods. Each add* method appends rules to c.rules using the doctrine
// and savings slices.
type doctrineCompiler struct {
	d               Doctrine
	rules           []*Rule
	savings         []buildingSaving
	infantrySavings []buildingSaving

	// Shared combat parameters, computed once in addCombatRules and
	// reused in addMicroRules for focus-fire priority.
	attackPriority      int
	activationThreshold float64
}

// buildCashCondition generates a cash check that prevents unit production
// from starving out expensive buildings. Without this, a doctrine wanting a
// tech center might never save 1500 credits because infantry keep spending
// at 100 each.
func buildCashCondition(unitCost int, savings []buildingSaving) string {
	cond := fmt.Sprintf("Cash() >= %d", unitCost)
	for _, s := range savings {
		cond += fmt.Sprintf(` && (%s || Cash() >= %d)`, s.existsExpr, unitCost+s.cost)
	}
	return cond
}

// prefersInfantry returns true if the given role appears in the doctrine's
// preferred infantry list. Used to gate prerequisite building rules on
// specific specialist preferences.
func (c *doctrineCompiler) prefersInfantry(role string) bool {
	for _, r := range c.d.PreferredInfantry {
		if r == role {
			return true
		}
	}
	return false
}

// initSavings computes the building savings and infantry savings slices
// from doctrine weights. These prevent unit spam from starving expensive
// queued buildings.
func (c *doctrineCompiler) initSavings() {
	// Each savings clause is gated on having the prerequisite building so that
	// the reserve doesn't block unit production before the expensive building
	// can actually be queued. Without these gates, moderate tech/superweapon
	// priorities (0.3-0.5) create enormous cash thresholds (2300+ for a tank)
	// that prevent any army from being built in early/mid game.
	if c.d.VehicleWeight > DoctrineModerate {
		// War factory requires radar. Don't reserve 2000 until radar exists.
		c.savings = append(c.savings, buildingSaving{`HasRole("war_factory") || !HasRole("radar")`, 2000})
	}
	if c.d.TechPriority > DoctrineHigh {
		// Tech center requires radar. Don't reserve 1500 until radar exists.
		// Threshold matches build-tech-center rule (DoctrineHigh) so we never
		// reserve cash for a tech center the doctrine won't actually build.
		c.savings = append(c.savings, buildingSaving{`HasRole("tech_center") || !HasRole("radar")`, 1500})
	}
	if c.d.SuperweaponPriority > DoctrineHigh {
		// Superweapons require tech center. Don't reserve 2500 until it exists.
		c.savings = append(c.savings, buildingSaving{
			`HasRole("missile_silo") || HasRole("iron_curtain") || !HasRole("tech_center")`, 2500,
		})
	}

	// War-factory reservation: when the doctrine wants vehicles, infantry
	// rules save cash for the war factory building (2000 credits). Without
	// this, infantry production drains cash below the war factory threshold
	// and the war factory is never built — especially in rush doctrines.
	c.infantrySavings = append([]buildingSaving(nil), c.savings...)
	if c.d.VehicleWeight > DoctrineEnabled {
		c.infantrySavings = append(c.infantrySavings, buildingSaving{
			existsExpr: `HasRole("war_factory")`,
			cost:       2000,
		})
	}

	// Vehicle-cost reservation: when the doctrine wants both infantry and
	// vehicles, infantry rules save 800 cash headroom so vehicle production
	// can start. The reservation releases once vehicles reach their cap.
	if c.d.VehicleWeight > DoctrineModerate {
		vehicleCapForSaving := lerp(3, 10, c.d.VehicleWeight)
		c.infantrySavings = append(c.infantrySavings, buildingSaving{
			existsExpr: fmt.Sprintf("CombatVehicleCount() >= %d", vehicleCapForSaving),
			cost:       800,
		})
	}
}

// CompileDoctrine translates a Doctrine's continuous 0–1 weights into a
// discrete rule set. Each weight controls which rules are included, their
// relative priorities, and the thresholds in their conditions.
// All expr strings are constructed via fmt.Sprintf — never from user input.
func CompileDoctrine(d Doctrine) []*Rule {
	d.Validate()
	c := &doctrineCompiler{d: d}
	c.initSavings()
	c.addCoreRules()
	c.addEconomyRules()
	c.addBuildingRules()
	c.addProductionRules()
	c.addCombatRules()
	c.addMicroRules()
	return c.rules
}
