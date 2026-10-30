package data

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	entCrud "github.com/tx7do/go-crud/entgo"

	"github.com/tx7do/go-utils/mapper"

	"go-wind-wms/app/core/service/internal/data/ent"
	"go-wind-wms/app/core/service/internal/data/ent/predicate"
	"go-wind-wms/app/core/service/internal/data/ent/stockpicking"
	"go-wind-wms/app/core/service/internal/data/ent/wave"

	waveV1 "go-wind-wms/api/gen/go/wave/service/v1"
)

// WaveRepo 波次仓储：单据 CRUD + 成员编排的事务操作。
// 成员资格校验（OUTGOING/CONFIRMED/未入波次）依赖行级条件更新，并发安全。
type WaveRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *log.Helper


	mapper *mapper.CopierMapper[waveV1.Wave, ent.Wave]

	repository *entCrud.Repository[
		ent.WaveQuery, ent.WaveSelect,
		ent.WaveCreate, ent.WaveCreateBulk,
		ent.WaveUpdate, ent.WaveUpdateOne,
		ent.WaveDelete,
		predicate.Wave,
		waveV1.Wave, ent.Wave,
	]
}

func NewWaveRepo(ctx *bootstrap.Context, entClient *entCrud.EntClient[*ent.Client]) *WaveRepo {
	repo := &WaveRepo{
		log:       ctx.NewLoggerHelper("wave/repo/core-service"),
		entClient: entClient,
	}

	repo.init()

	return repo
}

// backfillStatus 回填波次状态（Nillable 指针字段与值类型转换 pair 不匹配，
// copier 静默错值——显式按 id 回填）。
func (r *WaveRepo) backfillStatus(ctx context.Context, items []*waveV1.Wave) {
	if len(items) == 0 {
		return
	}

	ids := make([]uint32, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.GetId())
	}

	rows, err := r.entClient.Client().Wave.Query().
		Where(wave.IDIn(ids...)).
		Select(wave.FieldID, wave.FieldStatus).
		All(ctx)
	if err != nil {
		r.log.Errorf("backfill wave status failed: %s", err.Error())
		return
	}

	byID := make(map[uint32]*ent.Wave, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	for _, it := range items {
		if row := byID[it.GetId()]; row != nil && row.Status != nil {
			it.Status = waveStatusToProto(*row.Status)
		}
	}
}

func (r *WaveRepo) init() {

	r.mapper = mapper.NewCopierMapper[waveV1.Wave, ent.Wave]()

	r.repository = entCrud.NewRepository[
		ent.WaveQuery, ent.WaveSelect,
		ent.WaveCreate, ent.WaveCreateBulk,
		ent.WaveUpdate, ent.WaveUpdateOne,
		ent.WaveDelete,
		predicate.Wave,
		waveV1.Wave, ent.Wave,
	](r.mapper)
}

// waveStatusToProto ent 状态 → proto 状态。两侧枚举字符串不同（ent 存
// "RUNNING"，proto 为 "WAVE_RUNNING"），通用转换器按 proto 值查 ent 值必
// miss；copier 的 string→int32 静默错值更危险——必须显式映射。
func waveStatusToProto(s wave.Status) *waveV1.WaveStatus {
	switch s {
	case wave.StatusDraft:
		return waveV1.WaveStatus_WAVE_DRAFT.Enum()
	case wave.StatusRunning:
		return waveV1.WaveStatus_WAVE_RUNNING.Enum()
	case wave.StatusCompleted:
		return waveV1.WaveStatus_WAVE_COMPLETED.Enum()
	case wave.StatusCancelled:
		return waveV1.WaveStatus_WAVE_CANCELLED.Enum()
	default:
		return nil
	}
}

func (r *WaveRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.entClient.Client().Wave.Query()
	if len(whereCond) != 0 {
		builder.Modify(whereCond...)
	}

	count, err := builder.Count(ctx)
	if err != nil {
		r.log.Errorf("query count failed: %s", err.Error())
		return 0, waveV1.ErrorInternalServerError("query count failed")
	}

	return count, nil
}

func (r *WaveRepo) List(ctx context.Context, req *paginationV1.PagingRequest) (*waveV1.ListWaveResponse, error) {
	if req == nil {
		return nil, waveV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.entClient.Client().Wave.Query()

	ret, err := r.repository.ListWithPaging(ctx, builder, builder.Clone(), req)
	if err != nil {
		return nil, err
	}
	if ret == nil {
		return &waveV1.ListWaveResponse{Total: 0, Items: nil}, nil
	}

	r.backfillStatus(ctx, ret.Items)

	return &waveV1.ListWaveResponse{
		Total: ret.Total,
		Items: ret.Items,
	}, nil
}

func (r *WaveRepo) Get(ctx context.Context, id uint32) (*waveV1.Wave, error) {
	dto, err := r.repository.Get(ctx, r.entClient.Client().Wave.Query(), nil, wave.IDEQ(id))
	if err != nil {
		return nil, err
	}

	r.backfillStatus(ctx, []*waveV1.Wave{dto})

	return dto, nil
}

// CreateTx 创建波次（DRAFT）+ 绑定成员拣货单（单事务）。
// 成员资格由行级条件更新把关：仅 OUTGOING + CONFIRMED + 未入波次 的单据
// 会被绑定；受影响行数不足即返回 CONFLICT（并发下安全）。
func (r *WaveRepo) CreateTx(ctx context.Context, pickingIDs []uint32, remark *string, createdBy *uint32) (*ent.Wave, error) {
	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf("start transaction failed: %s", err.Error())
		return nil, waveV1.ErrorInternalServerError("start transaction failed")
	}
	var txErr error
	defer func() { r.FinishTx(tx, txErr) }()

	waveNumber := "WV" + fmt.Sprintf("%d", time.Now().UnixMilli())

	builder := tx.Wave.Create().
		SetWaveNumber(waveNumber).
		SetStatus(wave.StatusDraft).
		SetNillableRemark(remark).
		SetNillableCreatedBy(createdBy).
		SetCreatedAt(time.Now())

	t, txErr := builder.Save(ctx)
	if txErr != nil {
		r.log.Errorf("insert wave failed: %s", txErr.Error())
		return nil, waveV1.ErrorInternalServerError("insert wave failed")
	}

	// 绑定成员：行级条件更新（OUTGOING + CONFIRMED + wave_id=0）。
	n, txErr := tx.StockPicking.Update().
		Where(
			stockpicking.IDIn(pickingIDs...),
			stockpicking.PickingTypeEQ(stockpicking.PickingTypeOutgoing),
			stockpicking.WaveIDEQ(0),
		).
		SetWaveID(t.ID).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if txErr != nil {
		r.log.Errorf("bind pickings to wave failed: %s", txErr.Error())
		return nil, waveV1.ErrorInternalServerError("bind pickings failed")
	}
	if int(n) != len(pickingIDs) {
		// 有单不满足成员资格（类型/状态/已入波次），整体回滚。
		txErr = waveV1.ErrorConflict(
			"only OUTGOING pickings in CONFIRMED state and not in another wave can join; %d of %d bound", n, len(pickingIDs))
		return nil, txErr
	}

	txErr = nil
	return t, nil
}

// GetWithPickingsTx 取波次 + 成员拣货单 ent（事务内外均可）。
func (r *WaveRepo) GetWithPickingsTx(ctx context.Context, tx *ent.Tx, id uint32) (*ent.Wave, []*ent.StockPicking, error) {
	client := r.entClient.Client()
	if tx != nil {
		client = tx.Client()
	}

	t, err := client.Wave.Query().
		Where(wave.IDEQ(id)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil, waveV1.ErrorNotFound("wave not found")
		}
		r.log.Errorf("query wave failed: %s", err.Error())
		return nil, nil, waveV1.ErrorInternalServerError("query wave failed")
	}

	pickings, err := client.StockPicking.Query().
		Where(stockpicking.WaveIDEQ(id)).
		Order(ent.Asc(stockpicking.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query wave pickings failed: %s", err.Error())
		return nil, nil, waveV1.ErrorInternalServerError("query wave pickings failed")
	}

	return t, pickings, nil
}

// TransitionStatusTx 状态机流转（条件更新：仅 from 态可迁 to 态）。
func (r *WaveRepo) TransitionStatusTx(ctx context.Context, tx *ent.Tx, id uint32, from, to wave.Status) error {
	n, err := tx.Wave.Update().
		Where(
			wave.IDEQ(id),
			wave.StatusEQ(from),
		).
		SetStatus(to).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("transition wave status failed: %s", err.Error())
		return waveV1.ErrorInternalServerError("transition wave status failed")
	}
	if n == 0 {
		return waveV1.ErrorConflict("wave status changed concurrently or invalid transition")
	}
	return nil
}

// CompleteTx 自动完结：DRAFT/RUNNING → COMPLETED（幂等，条件更新）。
func (r *WaveRepo) CompleteTx(ctx context.Context, tx *ent.Tx, id uint32) error {
	n, err := tx.Wave.Update().
		Where(
			wave.IDEQ(id),
			wave.StatusIn(wave.StatusDraft, wave.StatusRunning),
		).
		SetStatus(wave.StatusCompleted).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("complete wave failed: %s", err.Error())
		return waveV1.ErrorInternalServerError("complete wave failed")
	}
	if n == 0 {
		r.log.Warnf("complete wave %d matched 0 rows (tenant=%v)", id, "ctx")
	}
	return nil
}

// ReleasePickingsTx 释放成员（wave_id 置 0），供取消波次用。
func (r *WaveRepo) ReleasePickingsTx(ctx context.Context, tx *ent.Tx, waveID uint32) error {
	_, err := tx.StockPicking.Update().
		Where(stockpicking.WaveIDEQ(waveID)).
		SetWaveID(0).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("release wave pickings failed: %s", err.Error())
		return waveV1.ErrorInternalServerError("release pickings failed")
	}
	return nil
}

// BeginTx 开启事务。
func (r *WaveRepo) BeginTx(ctx context.Context) (*ent.Tx, error) {
	tx, err := r.entClient.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf("start transaction failed: %s", err.Error())
		return nil, waveV1.ErrorInternalServerError("start transaction failed")
	}
	return tx, nil
}

// FinishTx 提交/回滚事务（err 非 nil 回滚）。
func (r *WaveRepo) FinishTx(tx *ent.Tx, err error) {
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
