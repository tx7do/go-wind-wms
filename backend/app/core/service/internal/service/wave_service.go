package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"github.com/tx7do/go-utils/trans"
	"google.golang.org/protobuf/types/known/emptypb"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"

	"go-wind-wms/app/core/service/internal/data"
	"go-wind-wms/app/core/service/internal/data/ent"
	"go-wind-wms/app/core/service/internal/data/ent/wave"

	inventoryV1 "go-wind-wms/api/gen/go/inventory/service/v1"
	waveV1 "go-wind-wms/api/gen/go/wave/service/v1"
)

// WaveService 波次服务（M2b 出库效率编排）。
//
// 职责边界：波次只做分组与执行编排——
//   - Create：校验成员资格（OUTGOING + CONFIRMED + 未入波次，行级条件更新
//     并发安全）并绑定；单事务，任一单不合格整体回滚
//   - Run：DRAFT→RUNNING 后逐张调拣货校验引擎落账；单张失败不阻塞其他张
//     （可重试，Run 再次调用会处理剩余 CONFIRMED 单）；全部 DONE 后自动
//     RUNNING→COMPLETED。库存扣减引擎零改动。
//   - Cancel：仅 DRAFT；成员释放回 CONFIRMED（wave_id 清 0）。
type WaveService struct {
	waveV1.UnimplementedWaveServiceServer

	log *log.Helper

	waveRepo          *data.WaveRepo
	stockPickingRepo  *data.StockPickingRepo
	stockPickingSvc   *StockPickingService
}

func NewWaveService(
	ctx *bootstrap.Context,
	waveRepo *data.WaveRepo,
	stockPickingRepo *data.StockPickingRepo,
	stockPickingSvc *StockPickingService,
) *WaveService {
	return &WaveService{
		log:              ctx.NewLoggerHelper("wave/service/core-service"),
		waveRepo:         waveRepo,
		stockPickingRepo: stockPickingRepo,
		stockPickingSvc:  stockPickingSvc,
	}
}

func (s *WaveService) List(ctx context.Context, req *paginationV1.PagingRequest) (*waveV1.ListWaveResponse, error) {
	return s.waveRepo.List(ctx, req)
}

func (s *WaveService) Count(ctx context.Context, req *paginationV1.PagingRequest) (*waveV1.CountWaveResponse, error) {
	count, err := s.waveRepo.Count(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &waveV1.CountWaveResponse{Count: uint64(count)}, nil
}

// Get 波次详情：聚合成员拣货单摘要（单号/客户/派生态/失败原因）与
// SKU 聚合动线（波次拣货一次拿全 SKU）。
func (s *WaveService) Get(ctx context.Context, req *waveV1.GetWaveRequest) (*waveV1.Wave, error) {
	id := req.GetId()
	if id == 0 {
		return nil, waveV1.ErrorBadRequest("invalid parameter")
	}

	dto, err := s.waveRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	_, pickings, err := s.waveRepo.GetWithPickingsTx(ctx, nil, id)
	if err != nil {
		return nil, err
	}

	dto.Pickings = s.toWavePickings(ctx, pickings)
	return dto, nil
}

// Create 创建波次。
func (s *WaveService) Create(ctx context.Context, req *waveV1.CreateWaveRequest) (*emptypb.Empty, error) {
	if req == nil || len(req.GetPickingIds()) == 0 {
		return nil, waveV1.ErrorBadRequest("picking_ids are required")
	}

	// 去重（幂等防呆）。
	seen := make(map[uint32]struct{}, len(req.GetPickingIds()))
	ids := make([]uint32, 0, len(req.GetPickingIds()))
	for _, id := range req.GetPickingIds() {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, waveV1.ErrorBadRequest("picking_ids are required")
	}

	callerUserID, _ := viewerUserIDFromContext(ctx)
	if _, err := s.waveRepo.CreateTx(ctx, ids, req.Remark, &callerUserID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// Run 执行波次（可重试）：逐张校验落账，单张失败不阻塞；无剩余
// CONFIRMED 单时波次自动完结。
func (s *WaveService) Run(ctx context.Context, req *waveV1.RunWaveRequest) (*waveV1.RunWaveResponse, error) {
	if req == nil || req.GetId() == 0 {
		return nil, waveV1.ErrorBadRequest("invalid parameter")
	}

	t, pickings, err := s.waveRepo.GetWithPickingsTx(ctx, nil, req.GetId())
	if err != nil {
		return nil, err
	}

	if t.Status == nil || (*t.Status != wave.StatusDraft && *t.Status != wave.StatusRunning) {
		return nil, waveV1.ErrorConflict("wave is not runnable in current status")
	}

	// DRAFT→RUNNING 先独立事务提交——成员 Validate 与完结分支必须在
	// 状态迁移落库之后执行，否则完结的条件更新会撞行锁并被 ctx 超时取消
	// （曾因此 504 且波次停留在 DRAFT/RUNNING）。
	if *t.Status == wave.StatusDraft {
		tx, terr := s.waveRepo.BeginTx(ctx)
		if terr != nil {
			return nil, terr
		}
		if cerr := s.waveRepo.TransitionStatusTx(ctx, tx, req.GetId(),
			wave.StatusDraft, wave.StatusRunning); cerr != nil {
			s.waveRepo.FinishTx(tx, cerr)
			return nil, cerr
		}
		s.waveRepo.FinishTx(tx, nil)
	}

	resp := &waveV1.RunWaveResponse{}
	var failures []*waveV1.WavePicking

	for _, p := range pickings {
		picking := p
		derived := derivePickingState(s.stockPickingRepo, ctx, picking.ID)
		if derived != inventoryV1.StockPicking_CONFIRMED {
			continue // DONE/CANCELLED 跳过
		}

		_, verr := s.stockPickingSvc.Validate(ctx, &inventoryV1.ValidateStockPickingRequest{
			Id: picking.ID,
		})
		if verr != nil {
			resp.Failed++
			failures = append(failures, &waveV1.WavePicking{
				PickingId:    picking.ID,
				PickingNumber: derefString(picking.PickingNumber),
				PartnerCode:  derefString(picking.PartnerCode),
				DerivedState: derived.String(),
				ErrorMessage: trans.Ptr(verr.Error()),
			})
			s.log.Errorf("wave %d picking %d validate failed: %s", req.GetId(), picking.ID, verr.Error())
			continue
		}
		resp.Succeeded++
	}

	// 无剩余 CONFIRMED → 自动完结（条件更新，并发安全）。
	remaining, cerr := s.countConfirmedPickings(ctx, req.GetId())
	if cerr != nil {
		s.log.Errorf("wave %d count confirmed pickings failed: %s", req.GetId(), cerr.Error())
	}
	s.log.Infof("wave %d run summary: succeeded=%d failed=%d remaining_confirmed=%d",
		req.GetId(), resp.GetSucceeded(), resp.GetFailed(), remaining)
	if remaining == 0 {
		tx2, terr := s.waveRepo.BeginTx(ctx)
		if terr != nil {
			s.log.Errorf("wave %d begin auto-complete tx failed: %s", req.GetId(), terr.Error())
		} else if cerr := s.waveRepo.CompleteTx(ctx, tx2, req.GetId()); cerr != nil {
			s.log.Errorf("wave %d auto-complete transition failed: %s", req.GetId(), cerr.Error())
			s.waveRepo.FinishTx(tx2, cerr)
		} else {
			s.waveRepo.FinishTx(tx2, nil)
			s.log.Infof("wave %d auto-completed", req.GetId())
		}
	}

	resp.Failures = failures
	return resp, nil
}

// Cancel 取消波次（仅 DRAFT）：成员释放回 CONFIRMED。
func (s *WaveService) Cancel(ctx context.Context, req *waveV1.CancelWaveRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, waveV1.ErrorBadRequest("invalid parameter")
	}

	t, _, err := s.waveRepo.GetWithPickingsTx(ctx, nil, req.GetId())
	if err != nil {
		return nil, err
	}
	if t.Status == nil || *t.Status != wave.StatusDraft {
		return nil, waveV1.ErrorConflict("only draft wave can be cancelled")
	}

	tx, err := s.waveRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	var txErr error
	defer func() { s.waveRepo.FinishTx(tx, txErr) }()

	if txErr = s.waveRepo.ReleasePickingsTx(ctx, tx, req.GetId()); txErr != nil {
		return nil, txErr
	}
	if txErr = s.waveRepo.TransitionStatusTx(ctx, tx, req.GetId(),
		wave.StatusDraft, wave.StatusCancelled); txErr != nil {
		return nil, txErr
	}

	return &emptypb.Empty{}, nil
}

// countConfirmedPickings 统计波次内仍为 CONFIRMED 的成员数。
func (s *WaveService) countConfirmedPickings(ctx context.Context, waveID uint32) (int, error) {
	pickings, err := s.waveRepoGetPickings(ctx, waveID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range pickings {
		if derivePickingState(s.stockPickingRepo, ctx, p.ID) == inventoryV1.StockPicking_CONFIRMED {
			n++
		}
	}
	return n, nil
}

func (s *WaveService) waveRepoGetPickings(ctx context.Context, waveID uint32) ([]*ent.StockPicking, error) {
	_, pickings, err := s.waveRepo.GetWithPickingsTx(ctx, nil, waveID)
	return pickings, err
}

// derivePickingState 从子 moves 聚合派生态（与 StockPickingRepo.Get 同口径）。
func derivePickingState(repo *data.StockPickingRepo, ctx context.Context, id uint32) inventoryV1.StockPicking_DerivedState {
	dto, err := repo.Get(ctx, &inventoryV1.GetStockPickingRequest{
		QueryBy: &inventoryV1.GetStockPickingRequest_Id{Id: id},
	})
	if err != nil || dto == nil {
		return inventoryV1.StockPicking_DRAFT
	}
	if dto.DerivedState == nil {
		return inventoryV1.StockPicking_DRAFT
	}
	return *dto.DerivedState
}

// toWavePickings ent 拣货单 → 波次摘要（含 SKU 聚合动线）。
func (s *WaveService) toWavePickings(ctx context.Context, pickings []*ent.StockPicking) []*waveV1.WavePicking {
	out := make([]*waveV1.WavePicking, 0, len(pickings))
	for _, p := range pickings {
		dto, err := s.stockPickingRepo.Get(ctx, &inventoryV1.GetStockPickingRequest{
			QueryBy: &inventoryV1.GetStockPickingRequest_Id{Id: p.ID},
		})
		if err != nil || dto == nil {
			continue
		}

		merged := make(map[string]int64)
		var skuOrder []string
		for _, m := range dto.GetMoves() {
			sku := m.GetProductCode()
			if _, seen := merged[sku]; !seen {
				skuOrder = append(skuOrder, sku)
			}
			merged[sku] += m.GetPlannedQuantity()
		}
		lines := make([]*waveV1.WaveLine, 0, len(skuOrder))
		for _, sku := range skuOrder {
			lines = append(lines, &waveV1.WaveLine{
				SkuCode:  sku,
				TotalQty: merged[sku],
			})
		}

		wp := &waveV1.WavePicking{
			PickingId:     p.ID,
			PickingNumber: derefString(p.PickingNumber),
			PartnerCode:   derefString(p.PartnerCode),
			Lines:         lines,
		}
		if dto.DerivedState != nil {
			wp.DerivedState = dto.GetDerivedState().String()
		}
		out = append(out, wp)
	}
	return out
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
