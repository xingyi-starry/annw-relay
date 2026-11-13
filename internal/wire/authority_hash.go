// Code generated from the canonical MP schema. DO NOT EDIT.
package wire

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
)

type fieldHasher struct {
	bytes.Buffer
	err error
}

func (h *fieldHasher) u32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	h.Write(b[:])
}
func (h *fieldHasher) u64(v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	h.Write(b[:])
}
func (h *fieldHasher) data(v []byte) { h.u32(uint32(len(v))); h.Write(v) }
func (h *fieldHasher) flag(v bool) {
	if v {
		h.WriteByte(1)
	} else {
		h.WriteByte(0)
	}
}

// AuthorityFrameHash encodes explicit v21 fields, independently of protobuf serialization.
func AuthorityFrameHash(frame *AuthorityFrameMessage) ([]byte, error) {
	h := &fieldHasher{}
	h.data([]byte("XingyiStarry.Mp/authority/v2"))
	h.data(frame.MatchId)
	h.u64(uint64(frame.FrameId))
	h.data(frame.PreviousHash)
	switch body := frame.Body.(type) {
	case *AuthorityFrameMessage_OperationBegin:
		h.u32(101)
		hashOperationBegin(h, body.OperationBegin)
	case *AuthorityFrameMessage_Resolution:
		h.u32(102)
		hashResolution(h, body.Resolution)
	case *AuthorityFrameMessage_OperationEnd:
		h.u32(103)
		hashOperationEnd(h, body.OperationEnd)
	case *AuthorityFrameMessage_TurnPhase:
		h.u32(104)
		hashAuthorityTurnPhase(h, body.TurnPhase)
	case *AuthorityFrameMessage_SeatChanged:
		h.u32(105)
		hashSeatControlChanged(h, body.SeatChanged)
	case *AuthorityFrameMessage_MatchEnded:
		h.u32(106)
		hashAuthorityMatchEnded(h, body.MatchEnded)
	case *AuthorityFrameMessage_OperationFailed:
		h.u32(107)
		hashOperationFailed(h, body.OperationFailed)
	default:
		return nil, errors.New("missing authority body")
	}
	if h.err != nil {
		return nil, h.err
	}
	hash := sha256.Sum256(h.Bytes())
	return hash[:], nil
}
func hashGameCommandMessage(h *fieldHasher, value *GameCommandMessage) {
	if value == nil {
		h.err = errors.New("nil GameCommandMessage")
		return
	}
	switch body := value.Body.(type) {
	case *GameCommandMessage_Move:
		h.u32(101)
		hashMoveCommand(h, body.Move)
	case *GameCommandMessage_Action:
		h.u32(102)
		hashActionCommand(h, body.Action)
	case *GameCommandMessage_EquipmentAction:
		h.u32(103)
		hashEquipmentActionCommand(h, body.EquipmentAction)
	case *GameCommandMessage_EquipmentMoveAction:
		h.u32(104)
		hashEquipmentMoveActionCommand(h, body.EquipmentMoveAction)
	case *GameCommandMessage_BuildWithMove:
		h.u32(105)
		hashBuildWithMoveCommand(h, body.BuildWithMove)
	case *GameCommandMessage_Skill:
		h.u32(106)
		hashSkillCommand(h, body.Skill)
	case *GameCommandMessage_UndoMove:
		h.u32(107)
		hashUndoMoveCommand(h, body.UndoMove)
	case *GameCommandMessage_EndTurn:
		h.u32(108)
		hashEndTurnCommand(h, body.EndTurn)
	case *GameCommandMessage_DebugAddResources:
		h.u32(111)
		hashDebugAddResourcesCommand(h, body.DebugAddResources)
	case *GameCommandMessage_DebugFillSkill:
		h.u32(112)
		hashDebugFillSkillCommand(h, body.DebugFillSkill)
	case *GameCommandMessage_AiUnitAction:
		h.u32(113)
		hashAiUnitActionCommand(h, body.AiUnitAction)
	case *GameCommandMessage_AiSkill:
		h.u32(114)
		hashAiSkillCommand(h, body.AiSkill)
	case *GameCommandMessage_TurnAdvance:
		h.u32(115)
		hashTurnAdvanceCommand(h, body.TurnAdvance)
	case *GameCommandMessage_Surrender:
		h.u32(116)
		hashSurrenderCommand(h, body.Surrender)
	case *GameCommandMessage_ToggleStandby:
		h.u32(117)
		hashToggleStandbyCommand(h, body.ToggleStandby)
	case *GameCommandMessage_ToggleSleep:
		h.u32(118)
		hashToggleSleepCommand(h, body.ToggleSleep)
	case *GameCommandMessage_SelfDestruct:
		h.u32(120)
		hashSelfDestructCommand(h, body.SelfDestruct)
	case *GameCommandMessage_ResolvedUnitActions:
		h.u32(121)
		hashResolvedUnitActionsCommand(h, body.ResolvedUnitActions)
	default:
		h.err = errors.New("missing oneof")
	}
}
func hashResolvedUnitAction(h *fieldHasher, value *ResolvedUnitAction) {
	if value == nil {
		h.err = errors.New("nil ResolvedUnitAction")
		return
	}
	h.u64(uint64(value.UnitId))
	h.flag(value.SkipAnimations)
	switch body := value.Body.(type) {
	case *ResolvedUnitAction_Move:
		h.u32(100)
		hashResolvedMove(h, body.Move)
	case *ResolvedUnitAction_Action:
		h.u32(101)
		hashResolvedAction(h, body.Action)
	case *ResolvedUnitAction_Create:
		h.u32(102)
		hashResolvedCreate(h, body.Create)
	case *ResolvedUnitAction_Unload:
		h.u32(103)
		hashResolvedUnload(h, body.Unload)
	default:
		h.err = errors.New("missing oneof")
	}
}
func hashSeatControlChanged(h *fieldHasher, value *SeatControlChanged) {
	if value == nil {
		h.err = errors.New("nil SeatControlChanged")
		return
	}
	h.data(value.SeatId)
	h.u32(uint32(value.PlayerIndex))
	h.flag(value.AiControlled)
	h.data([]byte(value.Reason))
}
func hashRandomRecordMessage(h *fieldHasher, value *RandomRecordMessage) {
	if value == nil {
		h.err = errors.New("nil RandomRecordMessage")
		return
	}
	h.data([]byte(value.CallSite))
	h.u32(uint32(value.Ordinal))
	switch body := value.Value.(type) {
	case *RandomRecordMessage_Integer:
		h.u32(6)
		h.u64(uint64(body.Integer))
	case *RandomRecordMessage_Floating:
		h.u32(7)
		h.u64(math.Float64bits(body.Floating))
	default:
		h.err = errors.New("missing oneof")
	}
}
func hashResolution(h *fieldHasher, value *Resolution) {
	if value == nil {
		h.err = errors.New("nil Resolution")
		return
	}
	h.data(value.OperationId)
	h.flag(value.Event != nil)
	if value.Event != nil {
		hashSettlementEvent(h, value.Event)
	}
}
func hashOperationBegin(h *fieldHasher, value *OperationBegin) {
	if value == nil {
		h.err = errors.New("nil OperationBegin")
		return
	}
	h.data(value.OperationId)
	h.data(value.SeatId)
	h.u64(uint64(value.RequestId))
	h.u32(uint32(value.Round))
	h.flag(value.Command != nil)
	if value.Command != nil {
		hashGameCommandMessage(h, value.Command)
	}
}
func hashOperationEnd(h *fieldHasher, value *OperationEnd) {
	if value == nil {
		h.err = errors.New("nil OperationEnd")
		return
	}
	h.data(value.OperationId)
}
func hashOperationFailed(h *fieldHasher, value *OperationFailed) {
	if value == nil {
		h.err = errors.New("nil OperationFailed")
		return
	}
	h.data(value.OperationId)
	h.data([]byte(value.Reason))
}
func hashAuthorityFrameMessage(h *fieldHasher, value *AuthorityFrameMessage) {
	if value == nil {
		h.err = errors.New("nil AuthorityFrameMessage")
		return
	}
	h.data(value.MatchId)
	h.u64(uint64(value.FrameId))
	h.data(value.PreviousHash)
	h.data(value.Hash)
	switch body := value.Body.(type) {
	case *AuthorityFrameMessage_OperationBegin:
		h.u32(101)
		hashOperationBegin(h, body.OperationBegin)
	case *AuthorityFrameMessage_Resolution:
		h.u32(102)
		hashResolution(h, body.Resolution)
	case *AuthorityFrameMessage_OperationEnd:
		h.u32(103)
		hashOperationEnd(h, body.OperationEnd)
	case *AuthorityFrameMessage_TurnPhase:
		h.u32(104)
		hashAuthorityTurnPhase(h, body.TurnPhase)
	case *AuthorityFrameMessage_SeatChanged:
		h.u32(105)
		hashSeatControlChanged(h, body.SeatChanged)
	case *AuthorityFrameMessage_MatchEnded:
		h.u32(106)
		hashAuthorityMatchEnded(h, body.MatchEnded)
	case *AuthorityFrameMessage_OperationFailed:
		h.u32(107)
		hashOperationFailed(h, body.OperationFailed)
	default:
		h.err = errors.New("missing oneof")
	}
}
func hashMoveCommand(h *fieldHasher, value *MoveCommand) {
	if value == nil {
		h.err = errors.New("nil MoveCommand")
		return
	}
	h.u32(uint32(len(value.UnitIds)))
	for _, item := range value.UnitIds {
		h.u64(uint64(item))
	}
	h.u32(uint32(len(value.UnitTargetXs)))
	for _, item := range value.UnitTargetXs {
		h.u32(uint32(item))
	}
	h.u32(uint32(len(value.UnitTargetYs)))
	for _, item := range value.UnitTargetYs {
		h.u32(uint32(item))
	}
}
func hashActionCommand(h *fieldHasher, value *ActionCommand) {
	if value == nil {
		h.err = errors.New("nil ActionCommand")
		return
	}
	h.u32(uint32(len(value.UnitIds)))
	for _, item := range value.UnitIds {
		h.u64(uint64(item))
	}
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
	h.u32(uint32(len(value.UnitTargetXs)))
	for _, item := range value.UnitTargetXs {
		h.u32(uint32(item))
	}
	h.u32(uint32(len(value.UnitTargetYs)))
	for _, item := range value.UnitTargetYs {
		h.u32(uint32(item))
	}
	h.u32(uint32(value.ActionCategory))
	h.data([]byte(value.TemplateId))
	h.u64(uint64(value.PassengerUnitId))
	h.flag(value.DesiredToggleState)
}
func hashEquipmentActionCommand(h *fieldHasher, value *EquipmentActionCommand) {
	if value == nil {
		h.err = errors.New("nil EquipmentActionCommand")
		return
	}
	h.u32(uint32(len(value.UnitIds)))
	for _, item := range value.UnitIds {
		h.u64(uint64(item))
	}
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
	h.u32(uint32(value.ActionCategory))
	h.data([]byte(value.ActionId))
	h.data([]byte(value.TemplateId))
}
func hashEquipmentMoveActionCommand(h *fieldHasher, value *EquipmentMoveActionCommand) {
	if value == nil {
		h.err = errors.New("nil EquipmentMoveActionCommand")
		return
	}
	h.u32(uint32(len(value.UnitIds)))
	for _, item := range value.UnitIds {
		h.u64(uint64(item))
	}
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
	h.u32(uint32(value.ActionCategory))
	h.data([]byte(value.ActionId))
	h.data([]byte(value.TemplateId))
	h.flag(value.MovePosition != nil)
	if value.MovePosition != nil {
		hashActionMovePosition(h, value.MovePosition)
	}
}
func hashBuildWithMoveCommand(h *fieldHasher, value *BuildWithMoveCommand) {
	if value == nil {
		h.err = errors.New("nil BuildWithMoveCommand")
		return
	}
	h.u32(uint32(len(value.UnitIds)))
	for _, item := range value.UnitIds {
		h.u64(uint64(item))
	}
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
	h.u32(uint32(value.ActionCategory))
	h.data([]byte(value.TemplateId))
	h.flag(value.MovePosition != nil)
	if value.MovePosition != nil {
		hashActionMovePosition(h, value.MovePosition)
	}
}
func hashSkillCommand(h *fieldHasher, value *SkillCommand) {
	if value == nil {
		h.err = errors.New("nil SkillCommand")
		return
	}
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
}
func hashUndoMoveCommand(h *fieldHasher, value *UndoMoveCommand) {
	if value == nil {
		h.err = errors.New("nil UndoMoveCommand")
		return
	}
}
func hashEndTurnCommand(h *fieldHasher, value *EndTurnCommand) {
	if value == nil {
		h.err = errors.New("nil EndTurnCommand")
		return
	}
}
func hashDebugAddResourcesCommand(h *fieldHasher, value *DebugAddResourcesCommand) {
	if value == nil {
		h.err = errors.New("nil DebugAddResourcesCommand")
		return
	}
	h.u32(uint32(value.DebugPlayerIndex))
	h.u32(uint32(value.DebugMetalDelta))
	h.u32(uint32(value.DebugPowerDelta))
}
func hashDebugFillSkillCommand(h *fieldHasher, value *DebugFillSkillCommand) {
	if value == nil {
		h.err = errors.New("nil DebugFillSkillCommand")
		return
	}
	h.u32(uint32(value.DebugPlayerIndex))
}
func hashAiUnitActionCommand(h *fieldHasher, value *AiUnitActionCommand) {
	if value == nil {
		h.err = errors.New("nil AiUnitActionCommand")
		return
	}
	h.u32(uint32(len(value.UnitIds)))
	for _, item := range value.UnitIds {
		h.u64(uint64(item))
	}
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
	h.u32(uint32(len(value.UnitTargetXs)))
	for _, item := range value.UnitTargetXs {
		h.u32(uint32(item))
	}
	h.u32(uint32(len(value.UnitTargetYs)))
	for _, item := range value.UnitTargetYs {
		h.u32(uint32(item))
	}
	h.u32(uint32(value.ActionCategory))
	h.data([]byte(value.TemplateId))
	h.u64(uint64(value.PassengerUnitId))
	h.flag(value.DesiredToggleState)
	h.u32(uint32(value.AiActionType))
}
func hashAiSkillCommand(h *fieldHasher, value *AiSkillCommand) {
	if value == nil {
		h.err = errors.New("nil AiSkillCommand")
		return
	}
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
	h.flag(value.DesiredToggleState)
}
func hashTurnAdvanceCommand(h *fieldHasher, value *TurnAdvanceCommand) {
	if value == nil {
		h.err = errors.New("nil TurnAdvanceCommand")
		return
	}
}
func hashSurrenderCommand(h *fieldHasher, value *SurrenderCommand) {
	if value == nil {
		h.err = errors.New("nil SurrenderCommand")
		return
	}
	h.u32(uint32(value.TargetX))
}
func hashToggleStandbyCommand(h *fieldHasher, value *ToggleStandbyCommand) {
	if value == nil {
		h.err = errors.New("nil ToggleStandbyCommand")
		return
	}
	h.u32(uint32(len(value.UnitIds)))
	for _, item := range value.UnitIds {
		h.u64(uint64(item))
	}
	h.flag(value.DesiredToggleState)
}
func hashToggleSleepCommand(h *fieldHasher, value *ToggleSleepCommand) {
	if value == nil {
		h.err = errors.New("nil ToggleSleepCommand")
		return
	}
	h.u32(uint32(len(value.UnitIds)))
	for _, item := range value.UnitIds {
		h.u64(uint64(item))
	}
	h.flag(value.DesiredToggleState)
}
func hashSelfDestructCommand(h *fieldHasher, value *SelfDestructCommand) {
	if value == nil {
		h.err = errors.New("nil SelfDestructCommand")
		return
	}
	h.u32(uint32(len(value.UnitIds)))
	for _, item := range value.UnitIds {
		h.u64(uint64(item))
	}
}
func hashResolvedUnitActionsCommand(h *fieldHasher, value *ResolvedUnitActionsCommand) {
	if value == nil {
		h.err = errors.New("nil ResolvedUnitActionsCommand")
		return
	}
	h.u32(uint32(len(value.UnitIds)))
	for _, item := range value.UnitIds {
		h.u64(uint64(item))
	}
	h.u32(uint32(len(value.ResolvedActions)))
	for _, item := range value.ResolvedActions {
		hashResolvedUnitAction(h, item)
	}
}
func hashRandomRecordList(h *fieldHasher, value *RandomRecordList) {
	if value == nil {
		h.err = errors.New("nil RandomRecordList")
		return
	}
	h.u32(uint32(len(value.Records)))
	for _, item := range value.Records {
		hashRandomRecordMessage(h, item)
	}
}
func hashUnitRandomResult(h *fieldHasher, value *UnitRandomResult) {
	if value == nil {
		h.err = errors.New("nil UnitRandomResult")
		return
	}
	h.u64(uint64(value.UnitId))
	h.flag(value.Values != nil)
	if value.Values != nil {
		hashRandomRecordList(h, value.Values)
	}
}
func hashTileRandomResult(h *fieldHasher, value *TileRandomResult) {
	if value == nil {
		h.err = errors.New("nil TileRandomResult")
		return
	}
	h.u32(uint32(value.X))
	h.u32(uint32(value.Y))
	h.flag(value.Values != nil)
	if value.Values != nil {
		hashRandomRecordList(h, value.Values)
	}
}
func hashRandomResult(h *fieldHasher, value *RandomResult) {
	if value == nil {
		h.err = errors.New("nil RandomResult")
		return
	}
	h.flag(value.DamageRoll != nil)
	if value.DamageRoll != nil {
		hashRandomRecordMessage(h, value.DamageRoll)
	}
	h.flag(value.DamageVariation != nil)
	if value.DamageVariation != nil {
		hashRandomRecordMessage(h, value.DamageVariation)
	}
	h.flag(value.DeathWreckScale != nil)
	if value.DeathWreckScale != nil {
		hashRandomRecordMessage(h, value.DeathWreckScale)
	}
	h.u32(uint32(len(value.Wrecks)))
	for _, item := range value.Wrecks {
		hashTileRandomResult(h, item)
	}
	h.u32(uint32(len(value.CommanderPositions)))
	for _, item := range value.CommanderPositions {
		hashUnitRandomResult(h, item)
	}
	h.u32(uint32(len(value.ExperienceShares)))
	for _, item := range value.ExperienceShares {
		hashUnitRandomResult(h, item)
	}
}
func hashSettlementEvent(h *fieldHasher, value *SettlementEvent) {
	if value == nil {
		h.err = errors.New("nil SettlementEvent")
		return
	}
	h.u64(uint64(value.SourceUnitId))
	h.u64(uint64(value.TargetUnitId))
	h.flag(value.RandomResult != nil)
	if value.RandomResult != nil {
		hashRandomResult(h, value.RandomResult)
	}
	h.u32(uint32(len(value.Children)))
	for _, item := range value.Children {
		hashSettlementEvent(h, item)
	}
	h.u32(uint32(value.PlayerIndex))
	switch body := value.Body.(type) {
	case *SettlementEvent_AttackStarted:
		h.u32(101)
		hashAttackStartedEvent(h, body.AttackStarted)
	case *SettlementEvent_AttackCell:
		h.u32(102)
		hashAttackCellEvent(h, body.AttackCell)
	case *SettlementEvent_AttackFinished:
		h.u32(103)
		hashAttackFinishedEvent(h, body.AttackFinished)
	case *SettlementEvent_Damage:
		h.u32(104)
		hashDamageEvent(h, body.Damage)
	case *SettlementEvent_DeathStarted:
		h.u32(105)
		hashDeathStartedEvent(h, body.DeathStarted)
	case *SettlementEvent_DeathCommitted:
		h.u32(106)
		hashDeathCommittedEvent(h, body.DeathCommitted)
	case *SettlementEvent_PlayerDefeated:
		h.u32(107)
		hashPlayerDefeatedEvent(h, body.PlayerDefeated)
	case *SettlementEvent_CleanupCompleted:
		h.u32(108)
		hashCleanupCompletedEvent(h, body.CleanupCompleted)
	case *SettlementEvent_Effect:
		h.u32(109)
		hashEffectEvent(h, body.Effect)
	case *SettlementEvent_Spawn:
		h.u32(110)
		hashSpawnEvent(h, body.Spawn)
	case *SettlementEvent_Heal:
		h.u32(111)
		hashHealEvent(h, body.Heal)
	case *SettlementEvent_Resource:
		h.u32(112)
		hashResourceEvent(h, body.Resource)
	case *SettlementEvent_CommanderPosition:
		h.u32(113)
		hashCommanderPositionEvent(h, body.CommanderPosition)
	case *SettlementEvent_ForcedAttack:
		h.u32(115)
		hashForcedAttackEvent(h, body.ForcedAttack)
	case *SettlementEvent_Pulse:
		h.u32(116)
		hashPulseEvent(h, body.Pulse)
	case *SettlementEvent_Wreck:
		h.u32(117)
		hashWreckEvent(h, body.Wreck)
	case *SettlementEvent_Shield:
		h.u32(118)
		hashShieldEvent(h, body.Shield)
	case *SettlementEvent_EffectMutation:
		h.u32(119)
		hashEffectMutationEvent(h, body.EffectMutation)
	case *SettlementEvent_Experience:
		h.u32(120)
		hashExperienceEvent(h, body.Experience)
	case *SettlementEvent_SkillFinished:
		h.u32(121)
		hashSkillFinishedEvent(h, body.SkillFinished)
	case *SettlementEvent_ActionPreparation:
		h.u32(122)
		hashActionPreparationEvent(h, body.ActionPreparation)
	case *SettlementEvent_ActionCallbacks:
		h.u32(123)
		hashActionCallbacksEvent(h, body.ActionCallbacks)
	default:
		h.err = errors.New("missing oneof")
	}
}
func hashAttackStartedEvent(h *fieldHasher, value *AttackStartedEvent) {
	if value == nil {
		h.err = errors.New("nil AttackStartedEvent")
		return
	}
	h.data([]byte(value.ActionName))
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
}
func hashAttackCellEvent(h *fieldHasher, value *AttackCellEvent) {
	if value == nil {
		h.err = errors.New("nil AttackCellEvent")
		return
	}
	h.data([]byte(value.ActionName))
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
	h.flag(value.OriginX != nil)
	if value.OriginX != nil {
		h.u32(uint32(*value.OriginX))
	}
	h.flag(value.OriginY != nil)
	if value.OriginY != nil {
		h.u32(uint32(*value.OriginY))
	}
	h.flag(value.Data != nil)
	if value.Data != nil {
		h.u32(math.Float32bits(*value.Data))
	}
	h.data([]byte(value.ActionType))
	h.u32(uint32(value.SkillIndex))
	h.u32(uint32(len(value.Targets)))
	for _, item := range value.Targets {
		hashAttackTargetResult(h, item)
	}
}
func hashAttackFinishedEvent(h *fieldHasher, value *AttackFinishedEvent) {
	if value == nil {
		h.err = errors.New("nil AttackFinishedEvent")
		return
	}
	h.data([]byte(value.ActionName))
	h.flag(value.CheckAfterAction)
}
func hashDamageEvent(h *fieldHasher, value *DamageEvent) {
	if value == nil {
		h.err = errors.New("nil DamageEvent")
		return
	}
	h.flag(value.Damage != nil)
	if value.Damage != nil {
		h.u32(math.Float32bits(*value.Damage))
	}
	h.data([]byte(value.ActionName))
	h.flag(value.CenterX != nil)
	if value.CenterX != nil {
		h.u32(uint32(*value.CenterX))
	}
	h.flag(value.CenterY != nil)
	if value.CenterY != nil {
		h.u32(uint32(*value.CenterY))
	}
	h.flag(value.UsePredamage)
	h.flag(value.DoRandom)
	h.flag(value.NoKill)
}
func hashDeathStartedEvent(h *fieldHasher, value *DeathStartedEvent) {
	if value == nil {
		h.err = errors.New("nil DeathStartedEvent")
		return
	}
	h.u32(uint32(value.Reason))
	h.flag(value.Damage != nil)
	if value.Damage != nil {
		h.u32(math.Float32bits(*value.Damage))
	}
	h.data([]byte(value.ActionName))
	h.flag(value.GenerateWreck)
	h.flag(value.CenterX != nil)
	if value.CenterX != nil {
		h.u32(uint32(*value.CenterX))
	}
	h.flag(value.CenterY != nil)
	if value.CenterY != nil {
		h.u32(uint32(*value.CenterY))
	}
}
func hashDeathCommittedEvent(h *fieldHasher, value *DeathCommittedEvent) {
	if value == nil {
		h.err = errors.New("nil DeathCommittedEvent")
		return
	}
	h.u32(uint32(value.Reason))
	h.u32(uint32(value.KillerPlayer))
	h.flag(value.DescendOnly)
	h.flag(value.CallbacksOnly)
}
func hashPlayerDefeatedEvent(h *fieldHasher, value *PlayerDefeatedEvent) {
	if value == nil {
		h.err = errors.New("nil PlayerDefeatedEvent")
		return
	}
	h.u32(uint32(value.PlayerIndex))
}
func hashCleanupCompletedEvent(h *fieldHasher, value *CleanupCompletedEvent) {
	if value == nil {
		h.err = errors.New("nil CleanupCompletedEvent")
		return
	}
	h.u32(uint32(value.PlayerIndex))
}
func hashEffectEvent(h *fieldHasher, value *EffectEvent) {
	if value == nil {
		h.err = errors.New("nil EffectEvent")
		return
	}
	h.data([]byte(value.EffectName))
	h.u32(uint32(value.EffectIndex))
	h.data([]byte(value.Callback))
	h.u64(uint64(value.OtherUnitId))
	h.flag(value.Damage != nil)
	if value.Damage != nil {
		h.u32(math.Float32bits(*value.Damage))
	}
	h.flag(value.Killed)
	h.u32(uint32(value.Reason))
	h.flag(value.Value != nil)
	if value.Value != nil {
		h.u32(math.Float32bits(*value.Value))
	}
	h.flag(value.Preview)
	h.data([]byte(value.EffectType))
	h.u32(uint32(value.X))
	h.u32(uint32(value.Y))
}
func hashSpawnEvent(h *fieldHasher, value *SpawnEvent) {
	if value == nil {
		h.err = errors.New("nil SpawnEvent")
		return
	}
	h.data([]byte(value.TemplateId))
	h.u32(uint32(value.X))
	h.u32(uint32(value.Y))
	h.u32(uint32(value.PlayerIndex))
	h.flag(value.Building)
	h.flag(value.Spawned)
	h.u32(uint32(value.CreateReason))
	h.flag(value.TriggerPassives)
	h.flag(value.HasPosition)
	h.flag(value.Moved)
	h.flag(value.Actioned)
}
func hashHealEvent(h *fieldHasher, value *HealEvent) {
	if value == nil {
		h.err = errors.New("nil HealEvent")
		return
	}
	h.flag(value.Amount != nil)
	if value.Amount != nil {
		h.u32(math.Float32bits(*value.Amount))
	}
}
func hashResourceEvent(h *fieldHasher, value *ResourceEvent) {
	if value == nil {
		h.err = errors.New("nil ResourceEvent")
		return
	}
	h.u32(uint32(value.PlayerIndex))
	h.u32(uint32(value.Reason))
	h.u32(uint32(value.Metal))
	h.u32(uint32(value.Power))
}
func hashCommanderPositionEvent(h *fieldHasher, value *CommanderPositionEvent) {
	if value == nil {
		h.err = errors.New("nil CommanderPositionEvent")
		return
	}
	h.u32(uint32(value.PlayerIndex))
}
func hashForcedAttackEvent(h *fieldHasher, value *ForcedAttackEvent) {
	if value == nil {
		h.err = errors.New("nil ForcedAttackEvent")
		return
	}
	h.u32(uint32(value.ActionCategory))
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
}
func hashPulseEvent(h *fieldHasher, value *PulseEvent) {
	if value == nil {
		h.err = errors.New("nil PulseEvent")
		return
	}
	h.u32(uint32(value.EffectIndex))
}
func hashWreckEvent(h *fieldHasher, value *WreckEvent) {
	if value == nil {
		h.err = errors.New("nil WreckEvent")
		return
	}
	h.u32(uint32(value.X))
	h.u32(uint32(value.Y))
	h.u32(uint32(value.Amount))
	h.flag(value.UseRandom)
}
func hashShieldEvent(h *fieldHasher, value *ShieldEvent) {
	if value == nil {
		h.err = errors.New("nil ShieldEvent")
		return
	}
	h.u32(uint32(value.Damage))
	h.flag(value.Preview)
}
func hashEffectMutationEvent(h *fieldHasher, value *EffectMutationEvent) {
	if value == nil {
		h.err = errors.New("nil EffectMutationEvent")
		return
	}
	h.data([]byte(value.EffectName))
	h.u32(uint32(value.Layer))
	h.u64(uint64(value.CasterUnitId))
	h.u32(uint32(value.CasterPlayer))
	h.flag(value.Remove)
	h.u32(uint32(value.EffectIndex))
}
func hashExperienceEvent(h *fieldHasher, value *ExperienceEvent) {
	if value == nil {
		h.err = errors.New("nil ExperienceEvent")
		return
	}
	h.flag(value.Amount != nil)
	if value.Amount != nil {
		h.u32(math.Float32bits(*value.Amount))
	}
	h.flag(value.UseFilter)
	h.flag(value.BasePercent)
}
func hashSkillFinishedEvent(h *fieldHasher, value *SkillFinishedEvent) {
	if value == nil {
		h.err = errors.New("nil SkillFinishedEvent")
		return
	}
	h.u32(uint32(value.PlayerIndex))
}
func hashResolvedMove(h *fieldHasher, value *ResolvedMove) {
	if value == nil {
		h.err = errors.New("nil ResolvedMove")
		return
	}
	h.u32(uint32(value.X))
	h.u32(uint32(value.Y))
}
func hashResolvedAction(h *fieldHasher, value *ResolvedAction) {
	if value == nil {
		h.err = errors.New("nil ResolvedAction")
		return
	}
	h.u32(uint32(value.MoveX))
	h.u32(uint32(value.MoveY))
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
	h.u32(uint32(value.ActionCategory))
}
func hashResolvedCreate(h *fieldHasher, value *ResolvedCreate) {
	if value == nil {
		h.err = errors.New("nil ResolvedCreate")
		return
	}
	h.u32(uint32(value.MoveX))
	h.u32(uint32(value.MoveY))
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
	h.u32(uint32(value.ActionCategory))
	h.data([]byte(value.TemplateId))
}
func hashResolvedUnload(h *fieldHasher, value *ResolvedUnload) {
	if value == nil {
		h.err = errors.New("nil ResolvedUnload")
		return
	}
	h.u32(uint32(value.MoveX))
	h.u32(uint32(value.MoveY))
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
	h.u32(uint32(value.ActionCategory))
	h.u64(uint64(value.PassengerUnitId))
}
func hashAuthorityTurnPhase(h *fieldHasher, value *AuthorityTurnPhase) {
	if value == nil {
		h.err = errors.New("nil AuthorityTurnPhase")
		return
	}
	h.u32(uint32(value.Phase))
}
func hashAuthorityMatchEnded(h *fieldHasher, value *AuthorityMatchEnded) {
	if value == nil {
		h.err = errors.New("nil AuthorityMatchEnded")
		return
	}
	h.flag(value.LocalVictory)
}
func hashActionMovePosition(h *fieldHasher, value *ActionMovePosition) {
	if value == nil {
		h.err = errors.New("nil ActionMovePosition")
		return
	}
	h.u32(uint32(value.X))
	h.u32(uint32(value.Y))
}
func hashActionPreparationEvent(h *fieldHasher, value *ActionPreparationEvent) {
	if value == nil {
		h.err = errors.New("nil ActionPreparationEvent")
		return
	}
	h.data([]byte(value.ActionName))
	h.u32(uint32(value.TargetX))
	h.u32(uint32(value.TargetY))
}
func hashActionCallbacksEvent(h *fieldHasher, value *ActionCallbacksEvent) {
	if value == nil {
		h.err = errors.New("nil ActionCallbacksEvent")
		return
	}
	h.data([]byte(value.ActionName))
}
func hashAttackTargetResult(h *fieldHasher, value *AttackTargetResult) {
	if value == nil {
		h.err = errors.New("nil AttackTargetResult")
		return
	}
	h.flag(value.Position != nil)
	if value.Position != nil {
		hashActionMovePosition(h, value.Position)
	}
	h.flag(value.Damage != nil)
	if value.Damage != nil {
		h.u32(math.Float32bits(*value.Damage))
	}
	switch body := value.Recipient.(type) {
	case *AttackTargetResult_Unit:
		h.u32(101)
		hashAttackUnitRecipient(h, body.Unit)
	case *AttackTargetResult_Shield:
		h.u32(102)
		hashAttackShieldRecipient(h, body.Shield)
	default:
		h.err = errors.New("missing oneof")
	}
}
func hashAttackUnitRecipient(h *fieldHasher, value *AttackUnitRecipient) {
	if value == nil {
		h.err = errors.New("nil AttackUnitRecipient")
		return
	}
	h.u64(uint64(value.UnitId))
}
func hashAttackShieldRecipient(h *fieldHasher, value *AttackShieldRecipient) {
	if value == nil {
		h.err = errors.New("nil AttackShieldRecipient")
		return
	}
	h.u64(uint64(value.CasterUnitId))
}
