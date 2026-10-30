package service

import (
	"testing"

	asnV1 "go-wind-wms/api/gen/go/asn/service/v1"
)

func TestAsnCanTransition(t *testing.T) {
	cases := []struct {
		name string
		from asnV1.AsnStatus
		to   asnV1.AsnStatus
		want bool
	}{
		{"draft→announced", asnV1.AsnStatus_ASN_DRAFT, asnV1.AsnStatus_ASN_ANNOUNCED, true},
		{"draft→cancelled", asnV1.AsnStatus_ASN_DRAFT, asnV1.AsnStatus_ASN_CANCELLED, true},
		{"draft→receiving(禁止跳状态)", asnV1.AsnStatus_ASN_DRAFT, asnV1.AsnStatus_ASN_RECEIVING, false},
		{"draft→completed(禁止跳状态)", asnV1.AsnStatus_ASN_DRAFT, asnV1.AsnStatus_ASN_COMPLETED, false},
		{"announced→receiving", asnV1.AsnStatus_ASN_ANNOUNCED, asnV1.AsnStatus_ASN_RECEIVING, true},
		{"announced→completed(短收完结)", asnV1.AsnStatus_ASN_ANNOUNCED, asnV1.AsnStatus_ASN_COMPLETED, true},
		{"announced→cancelled", asnV1.AsnStatus_ASN_ANNOUNCED, asnV1.AsnStatus_ASN_CANCELLED, true},
		{"receiving→completed", asnV1.AsnStatus_ASN_RECEIVING, asnV1.AsnStatus_ASN_COMPLETED, true},
		{"receiving→cancelled", asnV1.AsnStatus_ASN_RECEIVING, asnV1.AsnStatus_ASN_CANCELLED, true},
		{"receiving→announced(禁止回退)", asnV1.AsnStatus_ASN_RECEIVING, asnV1.AsnStatus_ASN_ANNOUNCED, false},
		{"completed→任意(终态)", asnV1.AsnStatus_ASN_COMPLETED, asnV1.AsnStatus_ASN_ANNOUNCED, false},
		{"cancelled→任意(终态)", asnV1.AsnStatus_ASN_CANCELLED, asnV1.AsnStatus_ASN_DRAFT, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := asnCanTransition(tc.from, tc.to); got != tc.want {
				t.Fatalf("asnCanTransition(%s → %s) = %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestAsnReceiveAllowed(t *testing.T) {
	for _, s := range []asnV1.AsnStatus{asnV1.AsnStatus_ASN_ANNOUNCED, asnV1.AsnStatus_ASN_RECEIVING} {
		if !asnReceiveAllowed(s) {
			t.Fatalf("asnReceiveAllowed(%s) = false, want true", s)
		}
	}
	for _, s := range []asnV1.AsnStatus{asnV1.AsnStatus_ASN_DRAFT, asnV1.AsnStatus_ASN_COMPLETED, asnV1.AsnStatus_ASN_CANCELLED} {
		if asnReceiveAllowed(s) {
			t.Fatalf("asnReceiveAllowed(%s) = true, want false", s)
		}
	}
}

func TestValidateReceiveQuantities(t *testing.T) {
	lines := []asnLineFacts{
		{ID: 1, ExpectedQty: 100, ReceivedQty: 0},
		{ID: 2, ExpectedQty: 50, ReceivedQty: 40},
		{ID: 3, ExpectedQty: 10, ReceivedQty: 10},
	}

	// 正常：行 1 全收 + 行 2 收余量
	if got := validateReceiveQuantities(lines, map[uint32]int64{1: 100, 2: 10}); len(got) != 0 {
		t.Fatalf("valid receive flagged lines %v", got)
	}

	// 防超收：行 1 收 101 > 100
	got := validateReceiveQuantities(lines, map[uint32]int64{1: 101})
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("over-receipt on line 1: got %v, want [1]", got)
	}

	// 防超收：行 2 已收 40 再收 11（40+11>50）
	got = validateReceiveQuantities(lines, map[uint32]int64{2: 11})
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("cumulative over-receipt on line 2: got %v, want [2]", got)
	}

	// 已收满的行再收（幂等重复提交防护）
	got = validateReceiveQuantities(lines, map[uint32]int64{3: 1})
	if len(got) != 1 || got[0] != 3 {
		t.Fatalf("re-receive on full line 3: got %v, want [3]", got)
	}

	// 负数数量
	got = validateReceiveQuantities(lines, map[uint32]int64{1: -5})
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("negative qty on line 1: got %v, want [1]", got)
	}
}

func TestValidatePutawayQuantities(t *testing.T) {
	lines := []asnLineFacts{
		{ID: 1, ExpectedQty: 100, ReceivedQty: 80, PutawayQty: 0, QcStatus: asnV1.AsnQcStatus_QC_PASS},
		{ID: 2, ExpectedQty: 50, ReceivedQty: 30, PutawayQty: 10, QcStatus: asnV1.AsnQcStatus_QC_PASS},
		{ID: 3, ExpectedQty: 10, ReceivedQty: 10, PutawayQty: 0, QcStatus: asnV1.AsnQcStatus_QC_FAIL},
		{ID: 4, ExpectedQty: 10, ReceivedQty: 10, PutawayQty: 0, QcStatus: asnV1.AsnQcStatus_QC_PENDING},
	}

	// 正常：行 1 上架 80（≤ 已收）
	if got := validatePutawayQuantities(lines, map[uint32]int64{1: 80}); len(got) != 0 {
		t.Fatalf("valid putaway flagged lines %v", got)
	}

	// 余量口径：行 2 已上架 10 再上 21（10+21>30 已收）
	got := validatePutawayQuantities(lines, map[uint32]int64{2: 21})
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("putaway exceeds received on line 2: got %v, want [2]", got)
	}

	// 质检闸门：QC_FAIL 不可上架
	got = validatePutawayQuantities(lines, map[uint32]int64{3: 1})
	if len(got) != 1 || got[0] != 3 {
		t.Fatalf("qc-fail line 3 putaway: got %v, want [3]", got)
	}

	// 质检闸门：QC_PENDING 不可上架
	got = validatePutawayQuantities(lines, map[uint32]int64{4: 1})
	if len(got) != 1 || got[0] != 4 {
		t.Fatalf("qc-pending line 4 putaway: got %v, want [4]", got)
	}

	// 超上架：行 1 上架 81 > 已收 80
	got = validatePutawayQuantities(lines, map[uint32]int64{1: 81})
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("over-putaway on line 1: got %v, want [1]", got)
	}
}

func TestAsnActionTarget(t *testing.T) {
	if asnActionTarget("announce") != asnV1.AsnStatus_ASN_ANNOUNCED {
		t.Fatal("announce target mismatch")
	}
	if asnActionTarget("cancel") != asnV1.AsnStatus_ASN_CANCELLED {
		t.Fatal("cancel target mismatch")
	}
	if asnActionTarget("close") != asnV1.AsnStatus_ASN_COMPLETED {
		t.Fatal("close target mismatch")
	}
}
