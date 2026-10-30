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

// Wave holds the schema definition for the Wave entity.
// 波次（M2b 出库效率编排）：分组执行 N 张 CONFIRMED 出库拣货单。
// 状态机：DRAFT → RUNNING → COMPLETED；DRAFT 可 CANCELLED。
// 引擎零改动——成员单据仍逐张走拣货校验落账。
type Wave struct {
	ent.Schema
}

func (Wave) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "wms_waves",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("波次表"),
	}
}

// Fields of the Wave.
func (Wave) Fields() []ent.Field {
	return []ent.Field{
		field.String("wave_number").
			Comment("波次号（服务端生成，租户范围内唯一）").
			Optional().
			Nillable(),

		field.Enum("status").
			Comment("状态：DRAFT=草稿，RUNNING=执行中，COMPLETED=已完成，CANCELLED=已取消").
			NamedValues(
				"Draft", "DRAFT",
				"Running", "RUNNING",
				"Completed", "COMPLETED",
				"Cancelled", "CANCELLED",
			).
			Default("DRAFT").
			Optional().
			Nillable(),
	}
}

// Mixin of the Wave.
func (Wave) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.TimeAt{}, appmixin.SoftDeleteQuery{},
		mixin.OperatorID{},
		mixin.Remark{},
		mixin.TenantID[uint32]{},
	}
}

// Indexes of the Wave.
func (Wave) Indexes() []ent.Index {
	return []ent.Index{
		// 租户范围内波次号唯一
		index.Fields("tenant_id", "wave_number").
			Unique().
			StorageKey("idx_wms_wave_tenant_number"),
	}
}
