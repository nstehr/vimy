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
	CatProduceInfantry = "produce-infantry"
	CatProduceVehicle  = "produce-vehicle"
	CatProduceAircraft = "produce-aircraft"
	CatProduceShip     = "produce-ship"
)

// Gameplay thresholds used inside expr condition strings (require fmt.Sprintf).
const (
	LowPowerHeadroom    = 50   // PowerExcess below this triggers advanced power
	IronCurtainMinUnits = 3    // minimum idle ground units to fire iron curtain
	RadarCost           = 1000 // what build-radar needs, and so what a reserve must leave
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
	return buildCashConditionScaled(unitCost, savings, 1.0)
}

// buildCashConditionScaled is like buildCashCondition but multiplies each
// savings reserve's cost by `scale` before adding it to the threshold. Used
// by produce-vehicle under high VehicleWeight: the LLM explicitly said
// vehicles are the plan, so reserves for future buildings shouldn't fully
// lock out combat-vehicle production — a human would trade a little future
// tech delay for having tanks on the field now. scale=1.0 preserves existing
// behavior; scale=0.5 halves each reserve's weight; scale=0.0 removes them
// entirely. A savings clause whose scaled cost falls to zero is dropped.
func buildCashConditionScaled(unitCost int, savings []buildingSaving, scale float64) string {
	cond := fmt.Sprintf("Cash() >= %d", unitCost)
	for _, s := range savings {
		scaledCost := int(float64(s.cost) * scale)
		if scaledCost <= 0 {
			continue
		}
		cond += fmt.Sprintf(` && (%s || Cash() >= %d)`, s.existsExpr, unitCost+scaledCost)
	}
	return cond
}

// radarReserve is the cash a building rule must leave untouched so the radar can
// still be afforded.
//
// The savings stack protects the radar from unit production, because only the
// produce-* rules call buildCashCondition. Nothing protected it from buildings:
// with groundDefense 0.75 the defense rule spends at a 600 floor while
// build-radar needs 1000, they sit in different categories so exclusivity cannot
// arbitrate, and the cheaper rule wins the race every tick. Games 71 and 72 both
// spent their income on tesla coils and never teched — the doctrine asked for a
// fortified perimeter AND heavy tanks and got only the perimeter (vimy-4j5).
//
// Only while the doctrine wants vehicles, and only until the radar exists.
func (c *doctrineCompiler) buildingCashCondition(cost int) string {
	cond := fmt.Sprintf("Cash() >= %d", cost)
	if c.d.VehicleWeight > DoctrineEnabled {
		cond += fmt.Sprintf(` && (HasRole("radar") || Cash() >= %d)`, cost+RadarCost)
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

func (c *doctrineCompiler) prefersVehicle(role string) bool {
	for _, r := range c.d.PreferredVehicle {
		if r == role {
			return true
		}
	}
	return false
}

// radarGatedVehicles lists combat vehicles that require a radar dome to
// produce. Matches RA's stock build tree for Soviets and Allies. Used to
// decide whether a doctrine should prioritize radar construction (because
// radar is the enablement step for its primary combat unit) vs. defer
// radar (because its primary unit — APC, flak_truck, light_tank — doesn't
// need it).
var radarGatedVehicles = map[string]bool{
	"v2_launcher":  true,
	"artillery":    true,
	"heavy_tank":   true,
	"medium_tank":  true,
	"tesla_tank":   true,
	"mammoth_tank": true,
}

// prefersRadarGatedPrimary reports whether the doctrine's top-ranked
// preferred vehicle is one that requires radar. Only the first entry
// counts — secondary prefs are fallbacks, not critical path. This narrow
// trigger is what keeps APC-rush doctrines (which list tanks as secondary
// prefs) from being pulled into radar-first build orders.
func prefersRadarGatedPrimary(preferred []string) bool {
	if len(preferred) == 0 {
		return false
	}
	return radarGatedVehicles[preferred[0]]
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
	// Radar first, because it is the tech gate for every combat vehicle worth
	// building — without a dome, RA never offers 3tnk or v2rl at all. The
	// reserves below are each disarmed while `!HasRole("radar")`, which left
	// the window where radar is the thing we need with an empty savings list:
	// income arrived in 250-500 dribbles and infantry, defenses and flak trucks
	// drained it before it could reach 1000. Observed as a permanent deadlock —
	// zero tanks in a 21,000-tick game whose doctrine asked for heavy and
	// medium tanks (vimy-tex).
	if c.d.VehicleWeight > DoctrineEnabled {
		c.savings = append(c.savings, buildingSaving{`HasRole("radar")`, 1000})
	}
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
	//
	// Aggressive doctrines (Aggression >= DoctrineSignificant) skip this
	// reserve: they need bodies on the field *now* to defend the base while
	// the war factory is being built. Engineer-rush gets swarmed if rifles
	// are cash-gated above 2100 through the whole early game.
	c.infantrySavings = append([]buildingSaving(nil), c.savings...)
	if c.d.VehicleWeight > DoctrineEnabled && c.d.Aggression < DoctrineSignificant {
		c.infantrySavings = append(c.infantrySavings, buildingSaving{
			existsExpr: `HasRole("war_factory")`,
			cost:       2000,
		})
	}

	// Vehicle-cost reservation: infantry rules save cash headroom so vehicle
	// production can start. Historical bug (game 64): the fixed 800-cost
	// reservation meant infantry needed cash >= 900 while vehicles fired at
	// cash >= 800, so vehicle always drained cash first and infantry never
	// spawned in combined-arms doctrines. Fix: scale reservation by
	// max(0, vehicle_weight - infantry_weight), so balanced doctrines don't
	// starve infantry. Pure-vehicle doctrines (infantry_weight=0.1) still
	// get the full reservation; combined-arms (infantry=0.3, vehicle=0.6)
	// get a proportional reduction; infantry-heavy (infantry>vehicle) gets
	// zero reservation.
	if c.d.VehicleWeight > DoctrineModerate {
		vehicleCapForSaving := lerp(3, 10, c.d.VehicleWeight)
		reserveScale := c.d.VehicleWeight - c.d.InfantryWeight
		if reserveScale < 0 {
			reserveScale = 0
		}
		reserveCost := int(800.0 * reserveScale)
		if reserveCost > 0 {
			c.infantrySavings = append(c.infantrySavings, buildingSaving{
				existsExpr: fmt.Sprintf("CombatVehicleCount() >= %d", vehicleCapForSaving),
				cost:       reserveCost,
			})
		}
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
