package data

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"google.golang.org/protobuf/types/known/timestamppb"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	entCrud "github.com/tx7do/go-crud/entgo"

	"github.com/tx7do/go-utils/copierutil"
	"github.com/tx7do/go-utils/mapper"

	"go-wind-wms/app/core/service/internal/data/ent"
	"go-wind-wms/app/core/service/internal/data/ent/asn"
	"go-wind-wms/app/core/service/internal/data/ent/asnline"
	"go-wind-wms/app/core/service/internal/data/ent/predicate"

	asnV1 "go-wind-wms/api/gen/go/asn/service/v1"
)

// AsnRepo 到货通知单仓储：单据 CRUD + 收货/上架的事务内原子数量操作。
// 防超收/防超上架依赖数据库行级条件更新（UPDATE ... WHERE 余量足够），
// 并发下由受影响行数判定成败，无检查-写入竞态窗口。
type AsnRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *log.Helper

	statusConverter *mapper.EnumTypeConverter[asnV1.AsnStatus, asn.Status]
	qcConverter     *mapper.EnumTypeConverter[asnV1.AsnQcStatus, asnline.QcStatus]

	mapper *mapper.CopierMapper[asnV1.Asn, ent.Asn]

	repository *entCrud.Repository[
		ent.AsnQuery, ent.AsnSelect,
		ent.AsnCreate, ent.AsnCreateBulk,
		ent.AsnUpdate, ent.AsnUpdateOne,
		ent.AsnDelete,
		predicate.Asn,
		asnV1.Asn, ent.Asn,
	]
}

func NewAsnRepo(ctx *bootstrap.Context, entClient *entCrud.EntClient[*ent.Client]) *AsnRepo {
	repo := &AsnRepo{
		log:       ctx.NewLoggerHelper("asn/repo/core-service"),
		entClient: entClient,
	}

	repo.init()

	return repo
}

func (r *AsnRepo) init() {
	r.statusConverter = mapper.NewEnumTypeConverter[asnV1.AsnStatus, asn.Status](asnV1.AsnStatus_name, asnV1.AsnStatus_value)
	r.qcConverter = mapper.NewEnumTypeConverter[asnV1.AsnQcStatus, asnline.QcStatus](asnV1.AsnQcStatus_name, asnV1.AsnQcStatus_value)

	r.mapper = mapper.NewCopierMapper[asnV1.Asn, ent.Asn]()

	r.repository = entCrud.NewRepository[
		ent.AsnQuery, ent.AsnSelect,
		ent.AsnCreate, ent.AsnCreateBulk,
		ent.AsnUpdate, ent.AsnUpdateOne,
		ent.AsnDelete,
		predicate.Asn,
		asnV1.Asn, ent.Asn,
	](r.mapper)

	r.mapper.AppendConverters(r.statusConverter.NewConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
}

// asnStatusToProto ent 状态 → proto 状态。两侧枚举字符串不同（ent 存
// "DRAFT"，proto 为 "ASN_DRAFT"——NamedValues 与 proto value 前缀差异），
// 不能走通用 EnumTypeConverter（valueMap 按 proto 值建，查 ent 值必 miss）。
func asnStatusToProto(s asn.Status) *asnV1.AsnStatus {
	switch s {
	case asn.StatusDraft:
		return asnV1.AsnStatus_ASN_DRAFT.Enum()
	case asn.StatusAnnounced:
		return asnV1.AsnStatus_ASN_ANNOUNCED.Enum()
	case asn.StatusReceiving:
		return asnV1.AsnStatus_ASN_RECEIVING.Enum()
	case asn.StatusCompleted:
		return asnV1.AsnStatus_ASN_COMPLETED.Enum()
	case asn.StatusCancelled:
		return asnV1.AsnStatus_ASN_CANCELLED.Enum()
	default:
		return nil
	}
}

func timeToTimestamppb(t time.Time) *timestamppb.Timestamp {
	return timestamppb.New(t)
}

func derefUint32(v *uint32) uint32 {
	if v == nil {
		return 0
	}
	return *v
}

func (r *AsnRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.entClient.Client().Asn.Query()
	if len(whereCond) != 0 {
		builder.Modify(whereCond...)
	}

	count, err := builder.Count(ctx)
	if err != nil {
		r.log.Errorf("query count failed: %s", err.Error())
		return 0, asnV1.ErrorInternalServerError("query count failed")
	}

	return count, nil
}

func (r *AsnRepo) List(ctx context.Context, req *paginationV1.PagingRequest) (ret *asnV1.ListAsnResponse, err error) {
	if req == nil {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().Asn.Query()

	ret2, err := r.repository.ListWithPaging(ctx, builder, builder.Clone(), req)
	if err != nil {
		return nil, err
	}
	if ret2 == nil {
		return &asnV1.ListAsnResponse{Total: 0, Items: nil}, nil
	}

	r.backfillStatus(ctx, ret2.Items)
	if err = r.attachLines(ctx, ret2.Items); err != nil {
		return nil, err
	}

	_ = ret2
	return &asnV1.ListAsnResponse{
		Total: ret2.Total,
		Items: ret2.Items,
	}, nil
}

func (r *AsnRepo) Get(ctx context.Context, req *asnV1.GetAsnRequest) (dto *asnV1.Asn, err error) {
	if req == nil {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().Asn.Query()

	var whereCond []func(s *sql.Selector)
	if id := req.GetId(); id != 0 {
		whereCond = append(whereCond, asn.IDEQ(id))
	}

	dto, err = r.repository.Get(ctx, builder, nil, whereCond...)
	if err != nil {
		return nil, err
	}

	r.backfillStatus(ctx, []*asnV1.Asn{dto})
	if err = r.attachLines(ctx, []*asnV1.Asn{dto}); err != nil {
		return nil, err
	}

	return dto, nil
}

func (r *AsnRepo) GetByNumber(ctx context.Context, number string) (dto *asnV1.Asn, err error) {
	if number == "" {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().Asn.Query().
		Where(asn.AsnNumberEQ(number))

	dto, err = r.repository.Get(ctx, builder, nil)
	if err != nil {
		return nil, err
	}

	r.backfillStatus(ctx, []*asnV1.Asn{dto})
	if err = r.attachLines(ctx, []*asnV1.Asn{dto}); err != nil {
		return nil, err
	}

	return dto, nil
}

// backfillStatus 回填单据状态/审计字段：ent 的 Nillable 字段是指针类型，
// CopierMapper 注册的值类型枚举转换 pair 匹配不到指针字段，status 会被
// 静默丢弃——这里按 id 从 ent 批量取回补上（List/Get/Create/Update 统一走此路径）。
func (r *AsnRepo) backfillStatus(ctx context.Context, items []*asnV1.Asn) {
	if len(items) == 0 {
		return
	}

	ids := make([]uint32, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.GetId())
	}

	rows, err := r.entClient.Client().Asn.Query().
		Where(asn.IDIn(ids...)).
		Select(asn.FieldID, asn.FieldStatus, asn.FieldExpectedDate).
		All(ctx)
	if err != nil {
		r.log.Errorf("backfill asn status failed: %s", err.Error())
		return
	}

	byID := make(map[uint32]*ent.Asn, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	for _, it := range items {
		row, ok := byID[it.GetId()]
		if !ok {
			continue
		}
		if row.Status != nil {
			it.Status = asnStatusToProto(*row.Status)
		}
		if row.ExpectedDate != nil && it.ExpectedDate == nil {
			it.ExpectedDate = timeToTimestamppb(*row.ExpectedDate)
		}
	}
}

// attachLines 批量装配明细（按 asn_id 一次查询，按单分组回填）。
func (r *AsnRepo) attachLines(ctx context.Context, items []*asnV1.Asn) error {
	if len(items) == 0 {
		return nil
	}

	ids := make([]uint32, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.GetId())
	}

	lines, err := r.entClient.Client().AsnLine.Query().
		Where(asnline.AsnIDIn(ids...)).
		Order(ent.Asc(asnline.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query asn lines failed: %s", err.Error())
		return asnV1.ErrorInternalServerError("query asn lines failed")
	}

	byAsn := make(map[uint32][]*asnV1.AsnLine, len(items))
	for _, l := range lines {
		dto := r.lineToDTO(l)
		tid := derefUint32(l.AsnID)
		byAsn[tid] = append(byAsn[tid], dto)
	}
	for _, it := range items {
		it.Lines = append(it.Lines, byAsn[it.GetId()]...)
	}

	return nil
}

func (r *AsnRepo) lineToDTO(t *ent.AsnLine) *asnV1.AsnLine {
	dto := &asnV1.AsnLine{
		Id:          &t.ID,
		AsnId:       t.AsnID,
		SkuCode:     t.SkuCode,
		ExpectedQty: t.ExpectedQty,
		ReceivedQty: t.ReceivedQty,
		PutawayQty:  t.PutawayQty,
		LotName:     t.LotName,
	}
	if t.QcStatus != nil {
		if v := r.qcConverter.ToDTO(t.QcStatus); v != nil {
			dto.QcStatus = v
		}
	}
	return dto
}

func (r *AsnRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.entClient.Client().Asn.Query().
		Where(asn.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, asnV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

// Create 创建到货通知单（草稿）+ 嵌套明细（单事务）。
// 单号服务端生成（ASN + 毫秒时间戳），忽略客户端传入值。
func (r *AsnRepo) Create(ctx context.Context, req *asnV1.CreateAsnRequest) (_ *asnV1.Asn, err error) {
	if req == nil || req.Data == nil {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}

	asnNumber := "ASN" + fmt.Sprintf("%d", time.Now().UnixMilli())

	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf("start transaction failed: %s", err.Error())
		return nil, asnV1.ErrorInternalServerError("start transaction failed")
	}
	defer func() { r.FinishTx(tx, err) }()

	builder := tx.Asn.Create().
		SetNillableTenantID(req.Data.TenantId).
		SetNillableWarehouseCode(req.Data.WarehouseCode).
		SetNillablePartnerCode(req.Data.PartnerCode).
		SetStatus(asn.StatusDraft).
		SetAsnNumber(asnNumber).
		SetNillableRemark(req.Data.Remark).
		SetNillableCreatedBy(req.Data.CreatedBy).
		SetCreatedAt(time.Now())

	if req.Data.ExpectedDate != nil {
		builder.SetExpectedDate(req.Data.ExpectedDate.AsTime())
	}
	if req.Data.Id != nil {
		builder.SetID(req.GetData().GetId())
	}

	t, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf("insert asn failed: %s", err.Error())
		return nil, asnV1.ErrorInternalServerError("insert asn failed")
	}

	if err = r.createLines(ctx, tx, t.ID, derefUint32(t.TenantID), req.Data.Lines, req.Data.CreatedBy); err != nil {
		return nil, err
	}

	dto := r.mapper.ToDTO(t)
	dto.Status = asnV1.AsnStatus_ASN_DRAFT.Enum()
	if err = r.attachLinesTx(ctx, tx, []*asnV1.Asn{dto}); err != nil {
		return nil, err
	}

	return dto, nil
}

// createLines 批量创建明细。
func (r *AsnRepo) createLines(ctx context.Context, tx *ent.Tx, asnID uint32, tenantID uint32, lines []*asnV1.AsnLine, createdBy *uint32) error {
	if len(lines) == 0 {
		return asnV1.ErrorBadRequest("asn lines are required")
	}

	for _, l := range lines {
		if l == nil || l.GetSkuCode() == "" || l.GetExpectedQty() <= 0 {
			return asnV1.ErrorBadRequest("each asn line requires sku_code and positive expected_qty")
		}

		lb := tx.AsnLine.Create().
			SetNillableTenantID(&tenantID).
			SetAsnID(asnID).
			SetNillableSkuCode(l.SkuCode).
			SetExpectedQty(l.GetExpectedQty()).
			SetReceivedQty(0).
			SetPutawayQty(0).
			SetNillableLotName(l.LotName).
			SetNillableRemark(l.Remark).
			SetNillableCreatedBy(createdBy).
			SetCreatedAt(time.Now())
		if l.QcStatus != nil {
			entQc := r.qcConverter.ToEntity(l.QcStatus)
			if entQc == nil {
				return asnV1.ErrorBadRequest("invalid qc_status")
			}
			lb.SetQcStatus(*entQc)
		}
		if _, err := lb.Save(ctx); err != nil {
			r.log.Errorf("insert asn line failed: %s", err.Error())
			return asnV1.ErrorInternalServerError("insert asn line failed")
		}
	}

	return nil
}

// Update 更新单据头 + 整体替换明细（仅草稿；状态机校验在服务层）。
func (r *AsnRepo) Update(ctx context.Context, req *asnV1.UpdateAsnRequest) (result *asnV1.Asn, err error) {
	if req == nil || req.Data == nil {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}

	tid, hasTenant := maybeTenantFromViewer(ctx)
	callerUserID, hasUser := viewerUserIDFromContext(ctx)

	builder := r.entClient.Client().Asn.UpdateOneID(req.GetId())
	builder.Where(asn.IDEQ(req.GetId()))
	if hasTenant {
		builder.Where(asn.TenantIDEQ(tid))
	}

	if req.Data.ExpectedDate != nil {
		builder.SetExpectedDate(req.Data.ExpectedDate.AsTime())
	}

	result, err = r.repository.UpdateOne(ctx, builder, req.Data, req.GetUpdateMask(),
		func(dto *asnV1.Asn) {
			builder.
				SetNillableWarehouseCode(req.Data.WarehouseCode).
				SetNillablePartnerCode(req.Data.PartnerCode).
				SetNillableRemark(req.Data.Remark).
				SetUpdatedAt(time.Now())

			// updated_by 强制由服务端 viewer context 推导，忽略客户端传入值
			if hasUser {
				builder.SetUpdatedBy(callerUserID)
			}
		},
		func(s *sql.Selector) {
			s.Where(sql.EQ(asn.FieldID, req.GetId()))
		},
	)
	if err != nil {
		return nil, err
	}

	// 明细整体替换（仅草稿；调用方保证）。单事务内先删后建。
	if req.Data.Lines != nil {
		tx, terr := r.entClient.Client().Tx(ctx)
		if terr != nil {
			return nil, asnV1.ErrorInternalServerError("start transaction failed")
		}
		defer func() { r.FinishTx(tx, err) }()

		if _, err = tx.AsnLine.Delete().
			Where(asnline.AsnIDEQ(req.GetId())).
			Exec(ctx); err != nil {
			r.log.Errorf("delete asn lines failed: %s", err.Error())
			return nil, asnV1.ErrorInternalServerError("delete asn lines failed")
		}

		if err = r.createLines(ctx, tx, req.GetId(), result.GetTenantId(), req.Data.Lines, result.CreatedBy); err != nil {
			return nil, err
		}
	}

	r.backfillStatus(ctx, []*asnV1.Asn{result})
	if err = r.attachLines(ctx, []*asnV1.Asn{result}); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *AsnRepo) Delete(ctx context.Context, req *asnV1.DeleteAsnRequest) error {
	if req == nil {
		return asnV1.ErrorBadRequest("invalid parameter")
	}

	tid, hasTenant := maybeTenantFromViewer(ctx)

	// 级联删除明细
	lineDel := r.entClient.Client().AsnLine.Delete().
		Where(asnline.AsnIDEQ(req.GetId()))
	if hasTenant {
		lineDel.Where(asnline.TenantIDEQ(tid))
	}
	if _, err := lineDel.Exec(ctx); err != nil {
		r.log.Errorf("delete asn lines failed: %s", err.Error())
		return asnV1.ErrorInternalServerError("delete failed")
	}

	delBuilder := r.entClient.Client().Asn.Delete()
	delBuilder.Where(asn.IDEQ(req.GetId()))
	if hasTenant {
		delBuilder.Where(asn.TenantIDEQ(tid))
	}
	if _, err := delBuilder.Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return asnV1.ErrorNotFound("asn not found")
		}

		r.log.Errorf("delete one data failed: %s", err.Error())

		return asnV1.ErrorInternalServerError("delete failed")
	}

	return nil
}

// ═══════════════════════ 收货/上架事务操作 ═══════════════════════

// GetWithLinesTx 事务内取单据 + 明细（收货/上架主流程用）。
func (r *AsnRepo) GetWithLinesTx(ctx context.Context, tx *ent.Tx, id uint32) (*ent.Asn, []*ent.AsnLine, error) {
	client := r.entClient.Client()
	if tx != nil {
		client = tx.Client()
	}

	t, err := client.Asn.Query().
		Where(asn.IDEQ(id)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil, asnV1.ErrorNotFound("asn not found")
		}
		r.log.Errorf("query asn failed: %s", err.Error())
		return nil, nil, asnV1.ErrorInternalServerError("query asn failed")
	}

	lines, err := client.AsnLine.Query().
		Where(asnline.AsnIDEQ(id)).
		Order(ent.Asc(asnline.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query asn lines failed: %s", err.Error())
		return nil, nil, asnV1.ErrorInternalServerError("query asn lines failed")
	}

	return t, lines, nil
}

// attachLinesTx 事务内装配明细（Create 主流程用，读未提交数据）。
func (r *AsnRepo) attachLinesTx(ctx context.Context, tx *ent.Tx, items []*asnV1.Asn) error {
	if len(items) == 0 {
		return nil
	}

	ids := make([]uint32, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.GetId())
	}

	lines, err := tx.AsnLine.Query().
		Where(asnline.AsnIDIn(ids...)).
		Order(ent.Asc(asnline.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query asn lines failed: %s", err.Error())
		return asnV1.ErrorInternalServerError("query asn lines failed")
	}

	byAsn := make(map[uint32][]*asnV1.AsnLine, len(items))
	for _, l := range lines {
		tid := derefUint32(l.AsnID)
		byAsn[tid] = append(byAsn[tid], r.lineToDTO(l))
	}
	for _, it := range items {
		it.Lines = append(it.Lines, byAsn[it.GetId()]...)
	}

	return nil
}

// TransitionStatusTx 状态机流转（服务端条件更新：仅 from 态可迁 to 态，
// 并发下 0 行命中即冲突）。
func (r *AsnRepo) TransitionStatusTx(ctx context.Context, tx *ent.Tx, id uint32, from, to asn.Status) error {
	n, err := tx.Asn.Update().
		Where(
			asn.IDEQ(id),
			asn.StatusEQ(from),
		).
		SetStatus(to).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("transition asn status failed: %s", err.Error())
		return asnV1.ErrorInternalServerError("transition asn status failed")
	}
	if n == 0 {
		return asnV1.ErrorConflict("asn status changed concurrently or invalid transition")
	}
	return nil
}

// ApplyReceivedQtyTx 原子累加收货数：仅当 累计收货 + delta ≤ 预期 时生效
// （防超收，行级条件更新）。同时回写质检状态与批次（最近一次收货口径）。
// 0 行命中返回 CONFLICT。
func (r *AsnRepo) ApplyReceivedQtyTx(ctx context.Context, tx *ent.Tx, lineID uint32, delta int64, qc asnline.QcStatus, lotName *string) error {
	guard := func(s *sql.Selector) {
		// received + delta ≤ expected
		s.Where(sql.ExprP(
			fmt.Sprintf("%s + %d <= %s", asnline.FieldReceivedQty, delta, asnline.FieldExpectedQty),
		))
	}

	query := tx.AsnLine.Update().
		Where(
			asnline.IDEQ(lineID),
			guard,
		).
		AddReceivedQty(delta).
		SetQcStatus(qc).
		SetUpdatedAt(time.Now())
	if lotName != nil {
		query.SetLotName(*lotName)
	}

	n, err := query.Save(ctx)
	if err != nil {
		r.log.Errorf("apply received qty failed: %s", err.Error())
		return asnV1.ErrorInternalServerError("apply received qty failed")
	}
	if n == 0 {
		return asnV1.ErrorConflict("receive quantity exceeds expected quantity (over-receipt blocked)")
	}
	return nil
}

// ApplyPutawayQtyTx 原子累加上架数：仅当质检通过且 累计上架 + delta ≤ 已收 时生效。
func (r *AsnRepo) ApplyPutawayQtyTx(ctx context.Context, tx *ent.Tx, lineID uint32, delta int64) error {
	guard := func(s *sql.Selector) {
		// putaway + delta ≤ received
		s.Where(sql.ExprP(
			fmt.Sprintf("%s + %d <= %s", asnline.FieldPutawayQty, delta, asnline.FieldReceivedQty),
		))
	}

	n, err := tx.AsnLine.Update().
		Where(
			asnline.IDEQ(lineID),
			asnline.QcStatusEQ(asnline.QcStatusQCPass),
			guard,
		).
		AddPutawayQty(delta).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("apply putaway qty failed: %s", err.Error())
		return asnV1.ErrorInternalServerError("apply putaway qty failed")
	}
	if n == 0 {
		return asnV1.ErrorConflict("putaway quantity exceeds received quantity or line failed QC")
	}
	return nil
}

// BeginTx 开启事务。
func (r *AsnRepo) BeginTx(ctx context.Context) (*ent.Tx, error) {
	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf("start transaction failed: %s", err.Error())
		return nil, asnV1.ErrorInternalServerError("start transaction failed")
	}
	return tx, nil
}

// FinishTx 提交/回滚事务（err 非 nil 回滚）。
func (r *AsnRepo) FinishTx(tx *ent.Tx, err error) {
	if tx == nil {
		return
	}
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			r.log.Errorf("transaction rollback failed: %s", rollbackErr.Error())
		}
		return
	}
	if commitErr := tx.Commit(); commitErr != nil {
		r.log.Errorf("transaction commit failed: %s", commitErr.Error())
	}
}
