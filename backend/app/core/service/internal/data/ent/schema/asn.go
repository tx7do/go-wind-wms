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

// Asn holds the schema definition for the Asn entity.
// 到货通知单（Advance Shipping Notice）：M1 入库链的预约单据。
// 状态机：DRAFT → ANNOUNCED → RECEIVING → COMPLETED；ANNOUNCED/RECEIVING 可 CANCELLED。
type Asn struct {
	ent.Schema
}

func (Asn) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "wms_asns",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("到货通知单表"),
	}
}

// Fields of the Asn.
func (Asn) Fields() []ent.Field {
	return []ent.Field{
		field.String("asn_number").
			Comment("单号（服务端生成，租户范围内唯一）").
			Optional().
			Nillable(),

		field.String("warehouse_code").
			Comment("目的仓库编码").
			Optional().
			Nillable(),

		field.String("partner_code").
			Comment("发货方编码（供应商/上游仓库）").
			Optional().
			Nillable(),

		// status：单据状态机（服务端校验流转，禁止跳状态）。
		field.Enum("status").
			Comment("状态：DRAFT=草稿，ANNOUNCED=已宣告，RECEIVING=收货中，COMPLETED=已完结，CANCELLED=已取消").
			NamedValues(
				"Draft", "DRAFT",
				"Announced", "ANNOUNCED",
				"Receiving", "RECEIVING",
				"Completed", "COMPLETED",
				"Cancelled", "CANCELLED",
			).
			Default("DRAFT").
			Optional().
			Nillable(),

		field.Time("expected_date").
			Comment("预计到货时间").
			Optional().
			Nillable(),
	}
}

// Mixin of the Asn.
func (Asn) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{}, appmixin.SoftDeleteQuery{},
		mixin.OperatorID{},
		mixin.Remark{},
		mixin.TenantID[uint32]{},
	}
}

// Indexes of the Asn.
func (Asn) Indexes() []ent.Index {
	return []ent.Index{
		// 租户范围内单号唯一
		index.Fields("tenant_id", "asn_number").
			Unique().
			StorageKey("idx_wms_asn_tenant_number"),

		// 按仓库 + 状态查预约
		index.Fields("tenant_id", "warehouse_code", "status").
			StorageKey("idx_wms_asn_tenant_wh_status"),
	}
}
