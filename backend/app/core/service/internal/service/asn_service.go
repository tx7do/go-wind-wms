package service

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"

	"go-wind-wms/app/core/service/internal/data"
	"go-wind-wms/app/core/service/internal/data/ent"
	"go-wind-wms/app/core/service/internal/data/ent/asn"
	"go-wind-wms/app/core/service/internal/data/ent/asnline"

	asnV1 "go-wind-wms/api/gen/go/asn/service/v1"
	inventoryV1 "go-wind-wms/api/gen/go/inventory/service/v1"
)

// AsnService 到货通知单服务（M1 入库链：预约 → 收货 → 上架）。
//
// 职责边界：
//   - 单据 CRUD 与状态机流转（announce/cancel/close）
//   - Receive：防超收 → 生成 INCOMING 拣货单（源=SUPPLIER 虚拟位置，
//     目的=仓库接收位置）并在同事务确认 moves —— 库存经拣货 Validate 落账
//   - Putaway：防超上架 → 生成 INTERNAL 拣货单（接收位置 → 目标库位）
//     并在同事务确认 moves —— 上架经拣货 Validate 移入库位
//
// 单据数量口径（防超收/防超上架）在本服务维护；库存数量口径由
// StockPickingService.Validate 维护，两者以拣货单为唯一桥梁。
type AsnService struct {
	asnV1.UnimplementedAsnServiceServer

	log *log.Helper

	asnRepo          *data.AsnRepo
	locationRepo     *data.LocationRepo
	stockPickingRepo *data.StockPickingRepo
	billing          *BillingGuard
}

func NewAsnService(
	ctx *bootstrap.Context,
	asnRepo *data.AsnRepo,
	locationRepo *data.LocationRepo,
	stockPickingRepo *data.StockPickingRepo,
	billing *BillingGuard,
) *AsnService {
	return &AsnService{
		log:              ctx.NewLoggerHelper("asn/service/core-service"),
		asnRepo:          asnRepo,
		locationRepo:     locationRepo,
		stockPickingRepo: stockPickingRepo,
		billing:          billing,
	}
}

func (s *AsnService) List(ctx context.Context, req *paginationV1.PagingRequest) (*asnV1.ListAsnResponse, error) {
	return s.asnRepo.List(ctx, req)
}

func (s *AsnService) Count(ctx context.Context, req *paginationV1.PagingRequest) (*asnV1.CountAsnResponse, error) {
	count, err := s.asnRepo.Count(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &asnV1.CountAsnResponse{Count: uint64(count)}, nil
}

func (s *AsnService) Get(ctx context.Context, req *asnV1.GetAsnRequest) (*asnV1.Asn, error) {
	return s.asnRepo.Get(ctx, req)
}

func (s *AsnService) GetByNumber(ctx context.Context, req *asnV1.GetAsnByNumberRequest) (*asnV1.Asn, error) {
	if req == nil || req.GetNumber() == "" {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}
	return s.asnRepo.GetByNumber(ctx, req.GetNumber())
}

func (s *AsnService) Create(ctx context.Context, req *asnV1.CreateAsnRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}

	if req.Data.GetWarehouseCode() == "" {
		return nil, asnV1.ErrorBadRequest("warehouse_code is required")
	}

	// SaaS 配额守卫：到货通知单计入"单据"口径。
	if err := s.billing.EnsureCanCreateOrder(ctx); err != nil {
		return nil, err
	}

	// 单号/状态由服务端生成，忽略客户端传入值。
	req.Data.AsnNumber = nil
	req.Data.Status = nil

	if _, err := s.asnRepo.Create(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *AsnService) Update(ctx context.Context, req *asnV1.UpdateAsnRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil || req.GetId() == 0 {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}

	old, err := s.asnRepo.Get(ctx, &asnV1.GetAsnRequest{QueryBy: &asnV1.GetAsnRequest_Id{Id: req.GetId()}})
	if err != nil {
		return nil, err
	}

	// 仅草稿可改。
	if old.GetStatus() != asnV1.AsnStatus_ASN_DRAFT {
		return nil, asnV1.ErrorConflict("only draft asn can be updated")
	}

	// 单号/状态不可改。
	req.Data.AsnNumber = nil
	req.Data.Status = nil

	if _, err := s.asnRepo.Update(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *AsnService) Delete(ctx context.Context, req *asnV1.DeleteAsnRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}

	old, err := s.asnRepo.Get(ctx, &asnV1.GetAsnRequest{QueryBy: &asnV1.GetAsnRequest_Id{Id: req.GetId()}})
	if err != nil {
		return nil, err
	}

	// 仅草稿/已取消可删。
	if old.GetStatus() != asnV1.AsnStatus_ASN_DRAFT &&
		old.GetStatus() != asnV1.AsnStatus_ASN_CANCELLED {
		return nil, asnV1.ErrorConflict("only draft or cancelled asn can be deleted")
	}

	if err := s.asnRepo.Delete(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// transition 通用状态流转（状态机校验 + 条件更新）。
func (s *AsnService) transition(ctx context.Context, id uint32, action string) (*emptypb.Empty, error) {
	to := asnActionTarget(action)

	old, err := s.asnRepo.Get(ctx, &asnV1.GetAsnRequest{QueryBy: &asnV1.GetAsnRequest_Id{Id: id}})
	if err != nil {
		return nil, err
	}

	from := old.GetStatus()
	if !asnCanTransition(from, to) {
		return nil, asnV1.ErrorConflict("invalid asn status transition (%s → %s)", from, to)
	}

	tx, err := s.asnRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { s.asnRepo.FinishTx(tx, err) }()

	if err = s.asnRepo.TransitionStatusTx(ctx, tx, id,
		entAsnStatus(from), entAsnStatus(to)); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *AsnService) Announce(ctx context.Context, req *asnV1.AnnounceAsnRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}
	return s.transition(ctx, req.GetId(), "announce")
}

func (s *AsnService) Cancel(ctx context.Context, req *asnV1.CancelAsnRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}
	return s.transition(ctx, req.GetId(), "cancel")
}

func (s *AsnService) Close(ctx context.Context, req *asnV1.CloseAsnRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}
	return s.transition(ctx, req.GetId(), "close")
}

// Receive 收货（按明细增量）：
//  1. 校验单据状态（ANNOUNCED/RECEIVING）；
//  2. 防超收预检（纯函数快速失败）；
//  3. 先创建 INCOMING 拣货单（DRAFT），再在单事务内逐行原子累加已收数
//     （行级条件更新兜底并发）+ 首收迁移状态 + 确认 moves。
//
// 事务失败时拣货单停留 DRAFT（无库存影响，可人工取消）——单据数量与
// moves 状态原子一致，不存在半收账窗口。
func (s *AsnService) Receive(ctx context.Context, req *asnV1.ReceiveAsnRequest) (resp *asnV1.ReceiveAsnResponse, err error) {
	if req == nil || req.GetAsnId() == 0 || len(req.GetLines()) == 0 {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}
	for _, l := range req.GetLines() {
		if l.GetAsnLineId() == 0 || l.GetQuantity() <= 0 {
			return nil, asnV1.ErrorBadRequest("each receive line requires asn_line_id and positive quantity")
		}
	}

	// SaaS 配额守卫：收货生成拣货单，计入单量。
	if err = s.billing.EnsureCanCreateOrder(ctx); err != nil {
		return nil, err
	}

	old, lines, err := s.asnRepo.GetWithLinesTx(ctx, nil, req.GetAsnId())
	if err != nil {
		return nil, err
	}

	if old.Status == nil || (*old.Status != asn.StatusAnnounced && *old.Status != asn.StatusReceiving) {
		return nil, asnV1.ErrorConflict("asn is not receivable in current status")
	}

	// 防超收预检（并发下由 ApplyReceivedQtyTx 的行级条件更新兜底）。
	reqQty := make(map[uint32]int64, len(req.GetLines()))
	for _, l := range req.GetLines() {
		reqQty[l.GetAsnLineId()] += l.GetQuantity()
	}
	if violated := validateReceiveQuantities(asnLineFactsOf(lines), reqQty); len(violated) > 0 {
		return nil, asnV1.ErrorConflict(
			"receive exceeds expected quantity on line id(s) %v (over-receipt blocked)", violated)
	}

	// 解析位置：source = 租户供应商虚拟位置，dest = 仓库接收位置。
	sourceLocID, err := s.locationRepo.GetSupplierLocationID(ctx)
	if err != nil {
		return nil, err
	}
	destLocID, err := s.locationRepo.GetLocationID(ctx, deref(old.WarehouseCode))
	if err != nil {
		return nil, err
	}

	// 本次收货按 (SKU, 批次, 效期) 细分为 moves——批次信息随 move 走，
	// Validate 时据此 get-or-create 批次（多批次分次收互不覆盖）。
	moves := aggregateMoves(lines, req.GetLines())

	picking, err := s.stockPickingRepo.Create(ctx, &inventoryV1.CreateStockPickingRequest{
		Data: &inventoryV1.StockPicking{
			PickingType:           inventoryV1.StockPicking_INCOMING.Enum(),
			SourceLocationId:      &sourceLocID,
			DestinationLocationId: &destLocID,
			AsnId:                 &old.ID,
			PartnerCode:           old.PartnerCode,
			Moves:                 moves,
		},
	})
	if err != nil {
		return nil, err
	}

	tx, err := s.asnRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { s.asnRepo.FinishTx(tx, err) }()

	for _, l := range req.GetLines() {
		qc := asnV1.AsnQcStatus_QC_PASS
		if l.QcStatus != nil {
			qc = l.GetQcStatus()
		}
		var expiry *time.Time
		if l.ExpiryDate != nil {
			t := l.GetExpiryDate().AsTime()
			expiry = &t
		}
		if err = s.asnRepo.ApplyReceivedQtyTx(ctx, tx, l.GetAsnLineId(), l.GetQuantity(),
			entQcStatus(qc), l.LotName, expiry); err != nil {
			return nil, err
		}
	}

	if *old.Status == asn.StatusAnnounced {
		if err = s.asnRepo.TransitionStatusTx(ctx, tx, req.GetAsnId(),
			entAsnStatus(asnV1.AsnStatus_ASN_ANNOUNCED),
			entAsnStatus(asnV1.AsnStatus_ASN_RECEIVING)); err != nil {
			return nil, err
		}
	}

	// 确认拣货单 moves（DRAFT → CONFIRMED，同事务）。
	if err = s.confirmPickingMovesTx(ctx, tx, picking); err != nil {
		return nil, err
	}

	resp = &asnV1.ReceiveAsnResponse{PickingId: picking.GetId()}
	err = nil
	return resp, nil
}

// Putaway 上架（按明细增量）：
//  1. 校验单据状态（RECEIVING——ANNOUNCED 无已收库存）；
//  2. 防超上架预检（质检闸门 + 余量）；
//  3. 目标库位必须为 INTERNAL；
//  4. 按目标库位分组生成 INTERNAL 拣货单（接收位置 → 目标库位），
//     单事务内原子累加上架数 + 确认 moves。
func (s *AsnService) Putaway(ctx context.Context, req *asnV1.PutawayAsnRequest) (resp *asnV1.PutawayAsnResponse, err error) {
	if req == nil || req.GetAsnId() == 0 || len(req.GetLines()) == 0 {
		return nil, asnV1.ErrorBadRequest("invalid parameter")
	}
	for _, l := range req.GetLines() {
		if l.GetAsnLineId() == 0 || l.GetQuantity() <= 0 || l.GetTargetLocationId() == 0 {
			return nil, asnV1.ErrorBadRequest("each putaway line requires asn_line_id, positive quantity and target_location_id")
		}
	}

	old, lines, err := s.asnRepo.GetWithLinesTx(ctx, nil, req.GetAsnId())
	if err != nil {
		return nil, err
	}

	if old.Status == nil || *old.Status != asn.StatusReceiving {
		return nil, asnV1.ErrorConflict("asn must be in RECEIVING status for putaway")
	}

	// 防超上架预检（质检闸门 + 余量；并发由 ApplyPutawayQtyTx 兜底）。
	reqQty := make(map[uint32]int64, len(req.GetLines()))
	for _, l := range req.GetLines() {
		reqQty[l.GetAsnLineId()] += l.GetQuantity()
	}
	if violated := validatePutawayQuantities(asnLineFactsOf(lines), reqQty); len(violated) > 0 {
		return nil, asnV1.ErrorConflict(
			"putaway blocked on line id(s) %v (QC failed or exceeds received quantity)", violated)
	}

	// 目标库位必须是 INTERNAL（真实库位）。
	for _, l := range req.GetLines() {
		usage, uerr := s.locationRepo.GetUsageTx(ctx, nil, l.GetTargetLocationId())
		if uerr != nil {
			return nil, uerr
		}
		if usage != inventoryV1.StockLocation_INTERNAL {
			return nil, asnV1.ErrorConflict("target location %d is not an internal location", l.GetTargetLocationId())
		}
	}

	// source = 仓库接收位置；明细按目标库位分组（每组一张 INTERNAL picking）。
	sourceLocID, err := s.locationRepo.GetLocationID(ctx, deref(old.WarehouseCode))
	if err != nil {
		return nil, err
	}

	// 按目标库位分组 → 组内按 SKU 聚合 moves。
	byTarget := aggregatePutawayMoves(lines, req.GetLines())

	tx, err := s.asnRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { s.asnRepo.FinishTx(tx, err) }()

	var firstID uint32
	for target, moves := range byTarget {
		picking, perr := s.stockPickingRepo.Create(ctx, &inventoryV1.CreateStockPickingRequest{
			Data: &inventoryV1.StockPicking{
				PickingType:           inventoryV1.StockPicking_INTERNAL.Enum(),
				SourceLocationId:      &sourceLocID,
				DestinationLocationId: &target,
				AsnId:                 &old.ID,
				PartnerCode:           old.PartnerCode,
				Moves:                 moves,
			},
		})
		if perr != nil {
			err = perr
			return nil, err
		}
		if err = s.confirmPickingMovesTx(ctx, tx, picking); err != nil {
			return nil, err
		}
		if firstID == 0 {
			firstID = picking.GetId()
		}
	}

	for _, l := range req.GetLines() {
		if err = s.asnRepo.ApplyPutawayQtyTx(ctx, tx, l.GetAsnLineId(), l.GetQuantity()); err != nil {
			return nil, err
		}
	}

	resp = &asnV1.PutawayAsnResponse{PickingId: firstID}
	err = nil
	return resp, nil
}

// confirmPickingMovesTx 将拣货单下所有 DRAFT move 迁至 CONFIRMED（同事务，
// 镜像 StockPickingService.Confirm 的语义但不另开事务）。
func (s *AsnService) confirmPickingMovesTx(ctx context.Context, tx *ent.Tx, picking *inventoryV1.StockPicking) error {
	for _, move := range picking.GetMoves() {
		if move == nil || move.GetState() != inventoryV1.StockMove_DRAFT {
			continue
		}
		if err := s.stockPickingRepo.TransitionMoveStateTx(ctx, tx, move.GetId(),
			inventoryV1.StockMove_DRAFT, inventoryV1.StockMove_CONFIRMED); err != nil {
			return err
		}
	}
	return nil
}

// aggregateMoves 按 (SKU, 批次, 效期) 聚合收货 moves。
// 批次/效期事实来自本次收货请求行（ReceiveAsnLine），而非 ASN 行快照——
// 同一行多批分次收时 ASN 行是"最近一次"口径，历史批次关联会丢。
// 每个组合一条 move（批次信息随 move 走，Validate 落批次表）；
// 未带批次的行归入 (sku, "", nil) 组（未跟踪库存）。
func aggregateMoves(lines []*ent.AsnLine, reqLines []*asnV1.ReceiveAsnLine) []*inventoryV1.StockMove {
	lineByID := make(map[uint32]*ent.AsnLine, len(lines))
	for _, l := range lines {
		lineByID[l.ID] = l
	}

	type moveKey struct {
		sku    string
		lot    string
		expiry string
	}
	merged := make(map[moveKey]int64)
	var keyOrder []moveKey
	for _, rl := range reqLines {
		line, ok := lineByID[rl.GetAsnLineId()]
		if !ok {
			continue
		}
		k := moveKey{sku: deref(line.SkuCode)}
		if rl.LotName != nil {
			k.lot = rl.GetLotName()
		}
		if rl.ExpiryDate != nil {
			k.expiry = rl.GetExpiryDate().AsTime().UTC().Format(time.RFC3339)
		}
		if _, seen := merged[k]; !seen {
			keyOrder = append(keyOrder, k)
		}
		merged[k] += rl.GetQuantity()
	}

	moves := make([]*inventoryV1.StockMove, 0, len(keyOrder))
	for _, k := range keyOrder {
		qty := merged[k]
		mv := &inventoryV1.StockMove{
			ProductCode:     &k.sku,
			PlannedQuantity: &qty,
		}
		if k.lot != "" {
			mv.LotName = &k.lot
		}
		if k.expiry != "" {
			t, _ := time.Parse(time.RFC3339, k.expiry)
			mv.ExpiryDate = timestamppb.New(t)
		}
		moves = append(moves, mv)
	}
	return moves
}

// aggregatePutawayMoves 按目标库位分组聚合上架 moves：
// 组 = 目标库位，组内同 SKU 的上架数量合并为一条 move。
func aggregatePutawayMoves(lines []*ent.AsnLine, reqLines []*asnV1.PutawayAsnLine) map[uint32][]*inventoryV1.StockMove {
	lineByID := make(map[uint32]*ent.AsnLine, len(lines))
	for _, l := range lines {
		lineByID[l.ID] = l
	}

	// (target, sku) → qty
	type groupKey struct {
		target uint32
		sku    string
	}
	qtys := make(map[groupKey]int64)
	var skuOrderPerTarget = make(map[uint32][]string)

	for _, rl := range reqLines {
		line, ok := lineByID[rl.GetAsnLineId()]
		if !ok {
			continue
		}
		target := rl.GetTargetLocationId()
		sku := deref(line.SkuCode)
		k := groupKey{target, sku}
		if _, seen := qtys[k]; !seen {
			skuOrderPerTarget[target] = append(skuOrderPerTarget[target], sku)
		}
		qtys[k] += rl.GetQuantity()
	}

	result := make(map[uint32][]*inventoryV1.StockMove, len(skuOrderPerTarget))
	for target, skus := range skuOrderPerTarget {
		moves := make([]*inventoryV1.StockMove, 0, len(skus))
		for _, sku := range skus {
			qty := qtys[groupKey{target, sku}]
			moves = append(moves, &inventoryV1.StockMove{
				ProductCode:     &sku,
				PlannedQuantity: &qty,
			})
		}
		result[target] = moves
	}
	return result
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// asnLineFactsOf ent 明细 → 数量口径快照（纯函数校验入参）。
func asnLineFactsOf(lines []*ent.AsnLine) []asnLineFacts {
	facts := make([]asnLineFacts, 0, len(lines))
	for _, l := range lines {
		f := asnLineFacts{
			ID:          l.ID,
			ExpectedQty: derefInt64(l.ExpectedQty),
			ReceivedQty: derefInt64(l.ReceivedQty),
			PutawayQty:  derefInt64(l.PutawayQty),
			QcStatus:    asnV1.AsnQcStatus_QC_PENDING,
		}
		if l.QcStatus != nil {
			switch *l.QcStatus {
			case asnline.QcStatusQCPass:
				f.QcStatus = asnV1.AsnQcStatus_QC_PASS
			case asnline.QcStatusQCFail:
				f.QcStatus = asnV1.AsnQcStatus_QC_FAIL
			}
		}
		facts = append(facts, f)
	}
	return facts
}

// entAsnStatus proto → ent 状态枚举。
func entAsnStatus(s asnV1.AsnStatus) asn.Status {
	switch s {
	case asnV1.AsnStatus_ASN_DRAFT:
		return asn.StatusDraft
	case asnV1.AsnStatus_ASN_ANNOUNCED:
		return asn.StatusAnnounced
	case asnV1.AsnStatus_ASN_RECEIVING:
		return asn.StatusReceiving
	case asnV1.AsnStatus_ASN_COMPLETED:
		return asn.StatusCompleted
	case asnV1.AsnStatus_ASN_CANCELLED:
		return asn.StatusCancelled
	default:
		return asn.StatusDraft
	}
}

// entQcStatus proto → ent 质检枚举。
func entQcStatus(s asnV1.AsnQcStatus) asnline.QcStatus {
	switch s {
	case asnV1.AsnQcStatus_QC_PASS:
		return asnline.QcStatusQCPass
	case asnV1.AsnQcStatus_QC_FAIL:
		return asnline.QcStatusQCFail
	default:
		return asnline.QcStatusQCPending
	}
}
