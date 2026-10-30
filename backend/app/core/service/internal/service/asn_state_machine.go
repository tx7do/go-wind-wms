package service

import (
	asnV1 "go-wind-wms/api/gen/go/asn/service/v1"
)

// ASN 状态机（纯函数，独立于仓储，便于单测）。
// 流转图：
//
//	DRAFT ──announce──→ ANNOUNCED ──receive──→ RECEIVING ──close──→ COMPLETED
//	  │                     │    ↖ receive ──┘ │
//	  │                     └───cancel────────┤（→ CANCELLED）
//	  ├──cancel──→ CANCELLED                  └───close──→ COMPLETED（允许短收完结）
//
// COMPLETED / CANCELLED 为终态，任何流转都被拒绝。

// asnCanTransition 判定状态流转是否合法。
func asnCanTransition(from, to asnV1.AsnStatus) bool {
	switch from {
	case asnV1.AsnStatus_ASN_DRAFT:
		return to == asnV1.AsnStatus_ASN_ANNOUNCED ||
			to == asnV1.AsnStatus_ASN_CANCELLED
	case asnV1.AsnStatus_ASN_ANNOUNCED:
		return to == asnV1.AsnStatus_ASN_RECEIVING ||
			to == asnV1.AsnStatus_ASN_COMPLETED ||
			to == asnV1.AsnStatus_ASN_CANCELLED
	case asnV1.AsnStatus_ASN_RECEIVING:
		return to == asnV1.AsnStatus_ASN_COMPLETED ||
			to == asnV1.AsnStatus_ASN_CANCELLED
	default:
		// COMPLETED / CANCELLED 为终态
		return false
	}
}

// asnActionTarget 状态动作 → 目标状态映射。
func asnActionTarget(action string) asnV1.AsnStatus {
	switch action {
	case "announce":
		return asnV1.AsnStatus_ASN_ANNOUNCED
	case "cancel":
		return asnV1.AsnStatus_ASN_CANCELLED
	case "close":
		return asnV1.AsnStatus_ASN_COMPLETED
	default:
		return asnV1.AsnStatus_ASN_DRAFT
	}
}

// asnReceiveTarget 收货动作允许的来源状态（收货是幂等多次操作，不改状态
// 语义：ANNOUNCED → RECEIVING 仅首收触发；RECEIVING 保持不变）。
func asnReceiveAllowed(from asnV1.AsnStatus) bool {
	return from == asnV1.AsnStatus_ASN_ANNOUNCED ||
		from == asnV1.AsnStatus_ASN_RECEIVING
}

// asnLineFacts 明细数量口径快照（纯数据，脱离 proto/ent 依赖，便于单测）。
type asnLineFacts struct {
	ID          uint32
	ExpectedQty int64
	ReceivedQty int64
	PutawayQty  int64
	QcStatus    asnV1.AsnQcStatus
}

// validateReceiveQuantities 防超收口径校验（纯函数）。
// 对每行：本次数量必须为正；累计已收 + 本次 ≤ 预期。
// 返回违规的明细 ID，空切片表示全部通过。
func validateReceiveQuantities(lines []asnLineFacts, reqLines map[uint32]int64) []uint32 {
	var violated []uint32
	for _, line := range lines {
		delta, ok := reqLines[line.ID]
		if !ok || delta == 0 {
			continue
		}
		if delta < 0 {
			violated = append(violated, line.ID)
			continue
		}
		if line.ReceivedQty+delta > line.ExpectedQty {
			violated = append(violated, line.ID)
		}
	}
	return violated
}

// validatePutawayQuantities 防超上架口径校验（纯函数）。
// 对每行：本次数量必须为正；质检必须通过；累计已上架 + 本次 ≤ 累计已收。
func validatePutawayQuantities(lines []asnLineFacts, reqLines map[uint32]int64) []uint32 {
	var violated []uint32
	for _, line := range lines {
		delta, ok := reqLines[line.ID]
		if !ok || delta == 0 {
			continue
		}
		if delta < 0 {
			violated = append(violated, line.ID)
			continue
		}
		if line.QcStatus != asnV1.AsnQcStatus_QC_PASS {
			violated = append(violated, line.ID)
			continue
		}
		if line.PutawayQty+delta > line.ReceivedQty {
			violated = append(violated, line.ID)
		}
	}
	return violated
}
