package ipc

// Command type constants — must stay in sync with C# CommandExecutor.
const (
	TypeProduce          = "produce"
	TypePlaceBuilding    = "place_building"
	TypeAttackMove       = "attack_move"
	TypeMove             = "move"
	TypeSetRally         = "set_rally"
	TypeDeploy           = "deploy"
	TypeRepairBuilding   = "repair_building"
	TypeAttack           = "attack"
	TypeCancelProduction = "cancel_production"
	TypeHarvest          = "harvest"
	TypeCapture          = "capture"
	TypeSupportPower     = "support_power"
)

type ProduceCommand struct {
	Queue string `json:"queue"`
	Item  string `json:"item"`
	Count int    `json:"count,omitempty"`
}

type PlaceBuildingCommand struct {
	Queue string `json:"queue"`
	Item  string `json:"item"`
}

type AttackMoveCommand struct {
	ActorIDs []uint32 `json:"actor_ids"`
	X        int      `json:"x"`
	Y        int      `json:"y"`
}

type MoveCommand struct {
	ActorID uint32 `json:"actor_id"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
}

type SetRallyCommand struct {
	ActorID uint32 `json:"actor_id"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
}

type DeployCommand struct {
	ActorID uint32 `json:"actor_id"`
}

type RepairBuildingCommand struct {
	ActorID uint32 `json:"actor_id"`
}

type AttackCommand struct {
	ActorID  uint32 `json:"actor_id"`
	TargetID uint32 `json:"target_id"`
}

type CancelProductionCommand struct {
	Queue string `json:"queue"`
	Item  string `json:"item"`
	Count int    `json:"count,omitempty"`
}

type HarvestCommand struct {
	ActorID uint32 `json:"actor_id"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
}

type CaptureCommand struct {
	ActorID  uint32 `json:"actor_id"`
	TargetID uint32 `json:"target_id"`
}

type SupportPowerCommand struct {
	PowerKey string `json:"power_key"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
}
