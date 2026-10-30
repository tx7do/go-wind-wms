package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-crud/entgo/mixin"
	appmixin "go-wind-wms/pkg/entgo/mixin"
)

// AsnLine holds the schema definition for the AsnLine entity.
// 到货通知单明细：预期/已收/已上架三数量口径 + 质检状态。
// 防超收：累计 received_qty ≤ expected_qty（服务端在事务内原子累加）。
// 防超上架：累计 putaway_qty ≤ received_qty 且仅质检通过的明细可上架。
type AsnLine struct {
	ent.Schema
}

func (AsnLine) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "wms_asn_lines",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("到货通知单明细表"),
	}
}

// Fields of the AsnLine.
func (AsnLine) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("asn_id").
			Comment("所属到货通知单ID").
			Default(0).
			Optional().
			Nillable(),

		field.String("sku_code").
			Comment("SKU 编码").
			Optional().
			Nillable(),

		field.Int64("expected_qty").
			Comment("预期数量").
			Default(0).
			Optional().
			Nillable(),

		field.Int64("received_qty").
			Comment("已收数量（累计）").
			Default(0).
			Optional().
			Nillable(),

		field.Int64("putaway_qty").
			Comment("已上架数量（累计）").
			Default(0).
			Optional().
			Nillable(),

		// qc_status：最近一次收货的质检状态；QC_FAIL 的数量不参与上架。
		field.Enum("qc_status").
			Comment("质检状态：QC_PENDING=未检，QC_PASS=通过，QC_FAIL=不合格").
			NamedValues(
				"QCPending", "QC_PENDING",
				"QCPass", "QC_PASS",
				"QCFail", "QC_FAIL",
			).
			Default("QC_PENDING").
			Optional().
			Nillable(),

		field.String("lot_name").
			Comment("批次号（最近一次收货登记，可选）").
			Optional().
			Nillable(),
	}
}

// Mixin of the AsnLine.
func (AsnLine) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{}, appmixin.SoftDeleteQuery{},
		mixin.OperatorID{},
		mixin.Remark{},
		mixin.TenantID[uint32]{},
	}
}

// Indexes of the AsnLine.
func (AsnLine) Indexes() []ent.Index {
	return []ent.Index{
		// 按单据取明细
		index.Fields("tenant_id", "asn_id").
			StorageKey("idx_wms_asn_line_tenant_asn"),

		// 按 SKU 追溯到货历史
		index.Fields("tenant_id", "sku_code").
			StorageKey("idx_wms_asn_line_tenant_sku"),
	}
}
