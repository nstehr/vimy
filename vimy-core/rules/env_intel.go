package rules

import (
	"log/slog"
	"math"
	"strings"

	"github.com/nstehr/vimy/vimy-core/model"
)

// CapturableCount is the neutral tech structures Vimy knows about, seen or
// remembered - not len(Capturables), which counted enemy tanks and husks and
// fell to zero the moment a scout drove past a derrick.
func (e RuleEnv) CapturableCount() int { return len(getCapturables(e.Memory)) }

// capturableValue ranks capturable types; unknown types get a baseline score.
var capturableValue = map[string]float64{
	"oilb": 10, // Oil derrick: continuous cash income
	"fcom": 8,  // Forward command: expands build area
	"miss": 5,  // Communications center: large radar reveal
	"bio":  4,  // Bio lab: provides prerequisite
	"hosp": 3,  // Hospital: heals infantry
}

const capturableValueDefault = 2

// IsNeutralTechStructure reports whether a type is one of the genuine neutral
// objectives, as opposed to whatever else turns up in GameState.Capturables.
//
// That list is not what its name suggests. In one game it carried the enemy's
// construction yard, refineries, power, a SAM site and a flame tower, plus
// their APCs, flak trucks, heavy tanks, harvesters and a tank husk - alongside
// four oil derricks. Everything in it is capturable by something; only a few of
// them are NEUTRAL, and only those are the objectives the capture rules and the
// doctrine's capture_priority are written about.
func IsNeutralTechStructure(t string) bool {
	_, ok := capturableValue[baseTypeName(t)]
	return ok
}

func (e RuleEnv) NearestCapturable() *model.Enemy {
	return e.BestCapturable()
}

// BestCapturable divides value by sqrt(distance), so a nearby cheap target can
// beat a distant valuable one whose trip isn't worth making.
func (e RuleEnv) BestCapturable() *model.Enemy {
	known := e.KnownCapturables()
	if len(known) == 0 {
		return nil
	}
	bx, by := 0, 0
	if len(e.State.Buildings) > 0 {
		bx = e.State.Buildings[0].X
		by = e.State.Buildings[0].Y
	}
	var best *model.Enemy
	bestScore := -1.0
	for i := range known {
		c := &known[i]
		if e.Terrain != nil {
			t := e.Terrain.AtMapPos(c.X, c.Y)
			if t != model.Land && t != model.Bridge {
				continue
			}
		}
		dx := float64(c.X - bx)
		dy := float64(c.Y - by)
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist < 1 {
			dist = 1
		}
		// An engineer cannot capture a tank.
		//
		// GameState.Capturables carries whatever something could take: in one
		// game the enemy's APCs, flak trucks, heavy tanks, harvesters and a
		// husk, alongside their base and four derricks. Everything unrecognised
		// scored capturableValueDefault, so an engineer walking at an enemy
		// heavy tank was a legal, and occasionally the best, choice.
		//
		// The test is structure, not neutrality: taking the enemy's refinery is
		// a real tactic and engineers do it. A husk fails this because
		// baseTypeName reduces "3tnk.husk" to a vehicle.
		if !IsNeutralTechStructure(c.Type) && !IsKnownBuildingType(c.Type) {
			continue
		}
		// Keyed on the base type. "oilb.ukraine" missed the table and took the
		// default, so a faction-suffixed derrick ranked level with an unknown.
		val := capturableValue[baseTypeName(c.Type)]
		if val == 0 {
			val = capturableValueDefault
		}
		score := val / math.Sqrt(dist)
		if score > bestScore {
			bestScore = score
			best = c
		}
	}
	return best
}

// EngineerNearCapturable catches an engineer just unloaded next to its target.
func (e RuleEnv) EngineerNearCapturable() bool {
	engineers := e.IdleEngineers()
	if len(engineers) == 0 {
		return false
	}
	known := e.KnownCapturables()
	for i := range known {
		c := &known[i]
		if !IsNeutralTechStructure(c.Type) && !IsKnownBuildingType(c.Type) {
			continue
		}
		for _, eng := range engineers {
			dx := float64(eng.X - c.X)
			dy := float64(eng.Y - c.Y)
			if dx*dx+dy*dy < 8*8 {
				return true
			}
		}
	}
	return false
}

// KnownCapturables is what is visible now plus what has been seen before.
//
// A capture objective is a building: it does not move, and once a scout has
// driven past a derrick its position is known for the rest of the game.
// GameState.Capturables is vision-only, so the count spiked as a unit passed a
// derrick and fell back to zero as it drove on, taking capture_priority and any
// engineer already walking toward it with it.
//
// Remembered entries carry no HP or owner - a remembered building is a position
// and a type, which is all the capture path reads.
func (e RuleEnv) KnownCapturables() []model.Enemy {
	out := make([]model.Enemy, 0, len(e.State.Capturables))
	seen := make(map[int]bool, len(e.State.Capturables))
	for _, c := range e.State.Capturables {
		out = append(out, c)
		seen[c.ID] = true
	}
	for _, m := range getCapturables(e.Memory) {
		if seen[m.ActorID] {
			continue
		}
		out = append(out, model.Enemy{ID: m.ActorID, Type: m.Type, X: m.X, Y: m.Y})
	}
	return out
}

func getCapturables(memory map[string]any) map[int]EnemyDefenseIntel {
	if v, ok := memory["capturables"].(map[int]EnemyDefenseIntel); ok {
		return v
	}
	return make(map[int]EnemyDefenseIntel)
}

// RememberedCapturables is the neutral tech structures Vimy believes are still
// standing and still up for grabs. What the prompt and the map should draw:
// sight is fleeting, the derrick is not.
func RememberedCapturables(memory map[string]any) map[int]EnemyDefenseIntel {
	return getCapturables(memory)
}

// updateCapturableIntel remembers where the neutral tech structures are.
//
// Only those. The rest of GameState.Capturables is enemy armour, harvesters and
// husks, which do move; remembering a tank's last known position as a capture
// objective would be worse than forgetting it.
func updateCapturableIntel(env RuleEnv) {
	caps := getCapturables(env.Memory)
	visible := make(map[int]bool, len(env.State.Capturables))
	for i := range env.State.Capturables {
		c := &env.State.Capturables[i]
		visible[c.ID] = true
		if !IsNeutralTechStructure(c.Type) {
			continue
		}
		caps[c.ID] = EnemyDefenseIntel{
			ActorID: c.ID, Type: baseTypeName(c.Type), X: c.X, Y: c.Y, Tick: env.State.Tick,
		}
	}
	// Cleared on the same evidence standard as defences and structures: one of
	// ours standing where it should be while it is not on the visible list -
	// captured by someone else, or destroyed.
	const clearRadiusSq = 10 * 10
	const minAge = 300
	for id, intel := range caps {
		if visible[id] || env.State.Tick-intel.Tick < minAge {
			continue
		}
		for _, u := range env.State.Units {
			dx, dy := u.X-intel.X, u.Y-intel.Y
			if dx*dx+dy*dy < clearRadiusSq {
				delete(caps, id)
				break
			}
		}
	}
	env.Memory["capturables"] = caps
}

// airTargetValue ranks air-strike targets. Defenses score high because aircraft
// can reach them ahead of a ground push that otherwise has to eat them.
var airTargetValue = map[string]float64{
	// Win-condition target — same reasoning as groundTargetValue.
	ConstructionYard: 12, "afac": 12,
	// Defense structures — clear them so we can stay over the base.
	TeslaCoil: 10, Turret: 8, Pillbox: 7, CamoPillbox: 7, FlameTower: 6,
	// Superweapons
	MissileSilo: 9, IronCurtain: 9,
	// Production
	WarFactory: 5, Airfield: 5, Helipad: 5,
	SovietBarracks: 4, AlliedBarracks: 4,
	// Economy
	Refinery: 6,
	// AA defenses (risky but worth removing)
	AAGun: 4, SAMSite: 4,
	// Power / support
	AdvancedPower: 3, PowerPlant: 2, RadarDome: 3, AlliedTechCenter: 3, SovietTechCenter: 3,
	SubPen: 4, NavalYard: 4,
}

const airTargetValueDefault = 1.0

// EnemyBaseIntel records a known enemy base position.
type EnemyBaseIntel struct {
	Owner         string
	X             int
	Y             int
	Tick          int
	FromBuildings bool // true if derived from building sightings (high confidence)
}

// enemyHarvesterIntel persists last-known enemy harvester positions. Harvesters
// orbit refineries inside the enemy base, so a stale sighting still locates it.
type enemyHarvesterIntel struct {
	X, Y, Tick int
}

// IntelCenter estimates the enemy base from all accumulated sightings.
// Confidence scales 0..1 with weighted sighting mass — one harvester is weak
// evidence, several buildings strong.
type IntelCenter struct {
	X, Y       int
	Confidence float64
	Sources    int
}

// knownBuildingTypes separates structures from mobile units in the Enemies
// list: a building fixes a base, a unit may just be a force passing through.
var knownBuildingTypes = map[string]bool{
	// Production
	ConstructionYard: true, SovietBarracks: true, AlliedBarracks: true, WarFactory: true, Kennel: true,
	Airfield: true, Helipad: true, NavalYard: true, SubPen: true,
	// Economy
	PowerPlant: true, AdvancedPower: true, Refinery: true, OreSilo: true,
	// Tech / Support
	RadarDome: true, AlliedTechCenter: true, SovietTechCenter: true, ServiceDepot: true,
	// Superweapons
	IronCurtain: true, "pdox": true, MissileSilo: true, GapGenerator: true,
	// Defenses
	Pillbox: true, CamoPillbox: true, Turret: true, FlameTower: true,
	TeslaCoil: true, AAGun: true, SAMSite: true,
}

// IsKnownBuildingType handles faction variants (e.g. "afld.ukraine" → "afld").
func IsKnownBuildingType(t string) bool {
	base := strings.ToLower(t)
	if idx := strings.IndexByte(base, '.'); idx >= 0 {
		base = base[:idx]
	}
	return knownBuildingTypes[base]
}

// updateIntel maintains known enemy base positions. Buildings always update;
// units only seed, so a roaming attack force can't overwrite a real base.
func updateIntel(env RuleEnv) {
	type acc struct {
		sumX, sumY, count int
	}
	buildingsByOwner := make(map[string]*acc)
	unitsByOwner := make(map[string]*acc)

	for _, e := range env.State.Enemies {
		if IsKnownBuildingType(e.Type) {
			a, ok := buildingsByOwner[e.Owner]
			if !ok {
				a = &acc{}
				buildingsByOwner[e.Owner] = a
			}
			a.sumX += e.X
			a.sumY += e.Y
			a.count++
		} else {
			a, ok := unitsByOwner[e.Owner]
			if !ok {
				a = &acc{}
				unitsByOwner[e.Owner] = a
			}
			a.sumX += e.X
			a.sumY += e.Y
			a.count++
		}
	}

	bases := getEnemyBases(env.Memory)

	for owner, a := range buildingsByOwner {
		bases[owner] = EnemyBaseIntel{
			Owner:         owner,
			X:             a.sumX / a.count,
			Y:             a.sumY / a.count,
			Tick:          env.State.Tick,
			FromBuildings: true,
		}
	}

	for owner, a := range unitsByOwner {
		if _, exists := bases[owner]; exists {
			continue
		}
		bases[owner] = EnemyBaseIntel{
			Owner:         owner,
			X:             a.sumX / a.count,
			Y:             a.sumY / a.count,
			Tick:          env.State.Tick,
			FromBuildings: false,
		}
	}

	// Forget a base only after our units have stood near it for a sustained
	// stretch without seeing anything. A single-tick check lets a scout passing
	// through wipe good intel the moment the enemy sits behind fog, and the
	// attack rules downstream then have no target at all.
	const intelClearRadius = 10
	const intelMinAge = 3000
	const intelClearSustain = 500 // ticks of continuous confirmation
	intelClearRadiusSq := intelClearRadius * intelClearRadius

	sustain := memoryMap[string, int](env.Memory, "enemyBaseIntelClearSustain")

	for owner, intel := range bases {
		if !intel.FromBuildings {
			continue // only clear high-confidence intel
		}
		if env.State.Tick-intel.Tick < intelMinAge {
			continue // too fresh — could be behind fog of war
		}
		ourUnitNearby := false
		for _, u := range env.State.Units {
			dx := u.X - intel.X
			dy := u.Y - intel.Y
			if dx*dx+dy*dy < intelClearRadiusSq {
				ourUnitNearby = true
				break
			}
		}
		if !ourUnitNearby {
			delete(sustain, owner) // reset counter when we leave
			continue
		}
		enemyNearby := false
		for _, e := range env.State.Enemies {
			dx := e.X - intel.X
			dy := e.Y - intel.Y
			if dx*dx+dy*dy < intelClearRadiusSq {
				enemyNearby = true
				break
			}
		}
		if enemyNearby {
			delete(sustain, owner) // reset counter when enemies appear
			continue
		}
		startTick, ok := sustain[owner]
		if !ok {
			sustain[owner] = env.State.Tick
			continue
		}
		if env.State.Tick-startTick < intelClearSustain {
			continue // not sustained long enough yet
		}
		slog.Info("clearing stale enemy base intel", "owner", owner, "x", intel.X, "y", intel.Y, "sustainTicks", env.State.Tick-startTick)
		delete(bases, owner)
		delete(sustain, owner)
	}

	env.Memory["enemyBases"] = bases

	harvesters := memoryMap[int, enemyHarvesterIntel](env.Memory, "enemyHarvesterIntel")
	for _, e := range env.State.Enemies {
		if matchesType(e.Type, Harvester) {
			harvesters[e.ID] = enemyHarvesterIntel{X: e.X, Y: e.Y, Tick: env.State.Tick}
		}
	}

	updateDefenseIntel(env)
	updateCapturableIntel(env)

	// Units dedupe by ID; buildings track a high-water mark instead, since they
	// are destroyed and rebuilt under fresh IDs.
	seenIDs := getEnemySeenIDs(env.Memory)
	unitsSeen := GetEnemyUnitsSeen(env.Memory)
	buildingsSeen := GetEnemyBuildingsSeen(env.Memory)

	curBuildingCounts := make(map[string]int)
	for _, e := range env.State.Enemies {
		if IsKnownBuildingType(e.Type) {
			curBuildingCounts[e.Type]++
		} else {
			if seenIDs[e.ID] {
				continue
			}
			seenIDs[e.ID] = true
			unitsSeen[e.Type]++
		}
	}
	for t, c := range curBuildingCounts {
		if c > buildingsSeen[t] {
			buildingsSeen[t] = c
		}
	}
	env.Memory["enemySeenIDs"] = seenIDs
	env.Memory["enemyUnitsSeen"] = unitsSeen
	env.Memory["enemyBuildingsSeen"] = buildingsSeen
}

// GetEnemyUnitsSeen returns the cumulative count of enemy units observed by type.
func GetEnemyUnitsSeen(memory map[string]any) map[string]int {
	if v, ok := memory["enemyUnitsSeen"].(map[string]int); ok {
		return v
	}
	return make(map[string]int)
}

// GetEnemyBuildingsSeen returns the cumulative count of enemy buildings observed by type.
func GetEnemyBuildingsSeen(memory map[string]any) map[string]int {
	if v, ok := memory["enemyBuildingsSeen"].(map[string]int); ok {
		return v
	}
	return make(map[string]int)
}

func getEnemySeenIDs(memory map[string]any) map[int]bool {
	if v, ok := memory["enemySeenIDs"].(map[int]bool); ok {
		return v
	}
	return make(map[int]bool)
}

func getEnemyBases(memory map[string]any) map[string]EnemyBaseIntel {
	if v, ok := memory["enemyBases"].(map[string]EnemyBaseIntel); ok {
		return v
	}
	return make(map[string]EnemyBaseIntel)
}

// HasEnemyIntel requires a building sighting — scouting continues until we
// find the base itself, not just its patrols.
func (e RuleEnv) HasEnemyIntel() bool {
	for _, base := range getEnemyBases(e.Memory) {
		if base.FromBuildings {
			return true
		}
	}
	return false
}

// InferredEnemyBaseCenter is the weighted centroid of every enemy sighting we
// hold. The persistent base centroid weighs most (it already aggregates many
// building sightings), then visible buildings, then harvesters, which decay
// with age. Lets siege and rally rules aim at a base seen only in fragments.
func (e RuleEnv) InferredEnemyBaseCenter() *IntelCenter {
	sumX, sumY, totalW := 0.0, 0.0, 0.0
	sources := 0

	for _, enemy := range e.State.Enemies {
		var w float64
		switch {
		case IsKnownBuildingType(enemy.Type):
			w = 3.0
		case matchesType(enemy.Type, Harvester):
			w = 1.0
		}
		if w == 0 {
			continue
		}
		sumX += float64(enemy.X) * w
		sumY += float64(enemy.Y) * w
		totalW += w
		sources++
	}

	for _, harv := range memoryMap[int, enemyHarvesterIntel](e.Memory, "enemyHarvesterIntel") {
		w := 1.0
		if e.State.Tick-harv.Tick > 500 {
			w = 0.5
		}
		sumX += float64(harv.X) * w
		sumY += float64(harv.Y) * w
		totalW += w
		sources++
	}

	for _, base := range getEnemyBases(e.Memory) {
		if !base.FromBuildings {
			continue
		}
		sumX += float64(base.X) * 5.0
		sumY += float64(base.Y) * 5.0
		totalW += 5.0
		sources++
	}

	if totalW == 0 {
		return nil
	}
	conf := totalW / 15.0 // 15 ≈ three full-weight buildings
	if conf > 1.0 {
		conf = 1.0
	}
	return &IntelCenter{
		X:          int(sumX / totalW),
		Y:          int(sumY / totalW),
		Confidence: conf,
		Sources:    sources,
	}
}

// NearestEnemyBase returns the closest remembered enemy base for fog-of-war attacks.
func (e RuleEnv) NearestEnemyBase() *EnemyBaseIntel {
	bases := getEnemyBases(e.Memory)
	if len(bases) == 0 {
		return nil
	}

	bx, by := 0, 0
	if len(e.State.Buildings) > 0 {
		bx = e.State.Buildings[0].X
		by = e.State.Buildings[0].Y
	}

	var nearest *EnemyBaseIntel
	bestDist := math.MaxFloat64
	for _, base := range bases {
		dx := float64(base.X - bx)
		dy := float64(base.Y - by)
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			b := base
			nearest = &b
		}
	}
	return nearest
}

func (e RuleEnv) EnemyBaseCount() int {
	return len(getEnemyBases(e.Memory))
}

// EnemyDefenseIntel outlives vision so path planning can route around defenses
// currently behind fog.
type EnemyDefenseIntel struct {
	ActorID int
	Type    string
	X, Y    int
	Tick    int
}

// defenseThreatWeight is painted onto the threat field, ordered by rough
// DPS × range.
var defenseThreatWeight = map[string]float64{
	Pillbox:     1.0,
	CamoPillbox: 1.0,
	Turret:      1.2,
	FlameTower:  1.4,
	TeslaCoil:   2.5,
	AAGun:       0.4, // anti-air, not ground threat
	SAMSite:     0.4,
}

func isEnemyDefenseType(t string) bool {
	_, ok := defenseThreatWeight[baseTypeName(t)]
	return ok
}

// BaseTypeName strips a faction suffix: "oilb.ukraine" is an oil derrick.
func BaseTypeName(t string) string { return baseTypeName(t) }

func baseTypeName(t string) string {
	base := strings.ToLower(t)
	if idx := strings.IndexByte(base, '.'); idx >= 0 {
		base = base[:idx]
	}
	return base
}

// updateDefenseIntel refreshes remembered defenses. Stale records clear only
// when one of our units stands where the defense should be and doesn't see it —
// same evidence standard as base intel.
func updateDefenseIntel(env RuleEnv) {
	defs := getEnemyDefenses(env.Memory)
	structures := getEnemyStructures(env.Memory)
	for _, e := range env.State.Enemies {
		// Every enemy building's POSITION, not just the defences.
		//
		// The sighting code beside this one records buildings as
		// enemyBuildingsSeen, a map of type to count, and discards x and y. So
		// Vimy could know it had seen a refinery and never where. Targeting
		// reads currently-visible enemies, which means a building that goes
		// back into fog stops existing, and the base assault compensates by
		// spiralling outward from a remembered centroid - huntOffset exists
		// because of this gap. blind-at-base, 37 times in game 173, is the
		// squad standing on the enemy base with nothing visible and nothing
		// remembered to shoot at.
		if IsKnownBuildingType(e.Type) {
			structures[e.ID] = EnemyDefenseIntel{
				ActorID: e.ID, Type: baseTypeName(e.Type), X: e.X, Y: e.Y, Tick: env.State.Tick,
			}
		}
		if !isEnemyDefenseType(e.Type) {
			continue
		}
		defs[e.ID] = EnemyDefenseIntel{
			ActorID: e.ID, Type: baseTypeName(e.Type), X: e.X, Y: e.Y, Tick: env.State.Tick,
		}
	}
	// Structures are cleared on the same evidence standard as defences: our own
	// unit standing where one should be and not seeing it.
	pruneStaleStructures(env, structures)
	env.Memory["enemyStructures"] = structures

	const clearRadiusSq = 10 * 10
	const minAge = 300
	for id, intel := range defs {
		if env.State.Tick-intel.Tick < minAge {
			continue
		}
		ourNearby := false
		for _, u := range env.State.Units {
			dx, dy := u.X-intel.X, u.Y-intel.Y
			if dx*dx+dy*dy < clearRadiusSq {
				ourNearby = true
				break
			}
		}
		if !ourNearby {
			continue
		}
		stillThere := false
		for _, e := range env.State.Enemies {
			if e.ID == intel.ActorID {
				stillThere = true
				break
			}
		}
		if !stillThere {
			delete(defs, id)
		}
	}
	env.Memory["enemyDefenses"] = defs
}
