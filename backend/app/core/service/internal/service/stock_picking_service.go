package service

import (
	"context"

	"time"

	"github.com/go-kratos/kratos/v2/log"
	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tx7do/go-utils/trans"

	"go-wind-wms/app/core/service/internal/data"
	"go-wind-wms/app/core/service/internal/data/ent"

	inventoryV1 "go-wind-wms/api/gen/go/inventory/service/v1"
)

// StockPickingService 拣货单服务（借鉴 Odoo stock.picking：一等文档，有生命周期）。
// 拣货单状态从子 moves 派生（不存储）。确认（Confirm）将 DRAFT moves 迁至
// CONFIRMED；校验（Validate）在单事务内创建 move-lines、更新 stock_quant、
// 回写采购收货量（借鉴 Odoo button_validate / _action_done）；取消（Cancel）
// 将非终态 moves 迁至 CANCELLED。
type StockPickingService struct {
	inventoryV1.UnimplementedStockPickingServiceServer

	log *log.Helper

	stockPickingRepo  *data.StockPickingRepo
	stockQuantRepo    *data.StockQuantRepo
	stockMoveLineRepo *data.StockMoveLineRepo
	stockLotRepo      *data.StockLotRepo
	locationRepo      *data.LocationRepo
	asnRepo           *data.AsnRepo
	billing           *BillingGuard
}

func NewStockPickingService(
	ctx *bootstrap.Context,
	stockPickingRepo *data.StockPickingRepo,
	stockQuantRepo *data.StockQuantRepo,
	stockMoveLineRepo *data.StockMoveLineRepo,
	stockLotRepo *data.StockLotRepo,
	locationRepo *data.LocationRepo,
	asnRepo *data.AsnRepo,
	billing *BillingGuard,
) *StockPickingService {
	svc := &StockPickingService{
		log:               ctx.NewLoggerHelper("stock_picking/service/core-service"),
		stockPickingRepo:  stockPickingRepo,
		stockQuantRepo:    stockQuantRepo,
		stockMoveLineRepo: stockMoveLineRepo,
		stockLotRepo:      stockLotRepo,
		locationRepo:      locationRepo,
		asnRepo:           asnRepo,
		billing:           billing,
	}

	return svc
}

func (s *StockPickingService) List(ctx context.Context, req *paginationV1.PagingRequest) (*inventoryV1.ListStockPickingResponse, error) {
	return s.stockPickingRepo.List(ctx, req)
}

func (s *StockPickingService) Count(ctx context.Context, req *paginationV1.PagingRequest) (*inventoryV1.CountStockPickingResponse, error) {
	count, err := s.stockPickingRepo.Count(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &inventoryV1.CountStockPickingResponse{Count: uint64(count)}, nil
}

func (s *StockPickingService) Get(ctx context.Context, req *inventoryV1.GetStockPickingRequest) (*inventoryV1.StockPicking, error) {
	return s.stockPickingRepo.Get(ctx, req)
}

// Create 创建拣货单（含嵌套 moves）。type=INTERNAL 调拨 / OUTGOING 出库 /
// INVENTORY_ADJUSTMENT 盘点时客户端提供明细；type=INCOMING 入库由 ASN 收货
// 生成（客户端不应直接调）。source/dest location 由服务层按 type + 仓库
// 推导（客户端不提供）。
// GetStockPickingByNumber 按拣货单号查单（PDA 扫码入口；含 moves）。
func (s *StockPickingService) GetStockPickingByNumber(ctx context.Context, req *inventoryV1.GetStockPickingByNumberRequest) (*inventoryV1.StockPicking, error) {
	if req == nil || req.GetNumber() == "" {
		return nil, inventoryV1.ErrorBadRequest("invalid parameter")
	}
	return s.stockPickingRepo.GetByNumber(ctx, req.GetNumber())
}

func (s *StockPickingService) Create(ctx context.Context, req *inventoryV1.CreateStockPickingRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, inventoryV1.ErrorBadRequest("invalid parameter")
	}

	// SaaS 配额守卫：订阅到期/本月单量超限则拒绝开单。
	if err := s.billing.EnsureCanCreateOrder(ctx); err != nil {
		return nil, err
	}

	pickingType := req.Data.GetPickingType()

	// source/dest location 由服务层按 type 推导落库（客户端不提供）。
	switch pickingType {
	case inventoryV1.StockPicking_INCOMING:
		// 入库：source = 租户供应商位置，dest = 目的仓库对应的
		// 接收位置。入库拣货单由 ASN 收货生成（M1 入库链）——客户端
		// 直接调此接口创建入库拣货单不允许。
		return nil, inventoryV1.ErrorBadRequest("incoming pickings are created automatically on ASN receive")

	case inventoryV1.StockPicking_OUTGOING:
		// 出库（M2 直发链）：source = 出库仓库的接收位置（INTERNAL），
		// dest = 租户客户虚拟位置（CUSTOMER）。客户端提供 from_warehouse_code、
		// partner_code（收货客户）与 moves（product + planned_quantity）；
		// FEFO 批次扣减与防负库存由 Validate 统一把关。
		return s.createOutgoingPicking(ctx, req)

	case inventoryV1.StockPicking_INTERNAL:
		// 调拨：source = fromWarehouse 的接收位置，dest = toWarehouse 的
		// 接收位置。客户端在 moves 里提供 per-move 的 product+planned_quantity，
		// 但不提供 per-move location（从 picking 继承）。
		return s.createInternalPicking(ctx, req)

	case inventoryV1.StockPicking_INVENTORY_ADJUSTMENT:
		// 盘点：moves 的 planned_quantity = 带符号差异数（正=盘盈，负=盘亏）。
		// 同一单内必须同号；盘盈 INVENTORY_LOSS→仓库，盘亏 仓库→INVENTORY_LOSS。
		return s.createAdjustmentPicking(ctx, req)

	default:
		return nil, inventoryV1.ErrorBadRequest("unsupported picking type")
	}
}

// createAdjustmentPicking 创建盘点拣货单（INVENTORY_ADJUSTMENT）。
// 借鉴 Odoo InventoryLoss 虚拟位置：盘盈 = 虚拟位置→仓库（入库腿），
// 盘亏 = 仓库→虚拟位置（出库腿，冻结当前均价成本）。同一单必须全正或全负
// （picking 的 source/dest 是单值，混合符号需拆成两张单）。Validate 的
// 双轨 quant 逻辑零改动即可执行（虚拟位置腿自动跳过）。
func (s *StockPickingService) createAdjustmentPicking(ctx context.Context, req *inventoryV1.CreateStockPickingRequest) (*emptypb.Empty, error) {
	whCode := req.Data.GetFromWarehouseCode()
	if whCode == "" {
		return nil, inventoryV1.ErrorBadRequest("from_warehouse_code is required for inventory adjustment")
	}

	whLocID, err := s.locationRepo.GetLocationID(ctx, whCode)
	if err != nil {
		return nil, err
	}
	lossLocID, err := s.locationRepo.GetInventoryLossLocationID(ctx)
	if err != nil {
		return nil, err
	}

	moves := req.Data.GetMoves()
	if len(moves) == 0 {
		return nil, inventoryV1.ErrorBadRequest("at least one move is required")
	}

	hasPositive, hasNegative := false, false
	for _, m := range moves {
		if m == nil || m.GetProductCode() == "" || m.GetPlannedQuantity() == 0 {
			return nil, inventoryV1.ErrorBadRequest("each move must have a valid product_code and non-zero signed planned_quantity")
		}
		if m.GetPlannedQuantity() > 0 {
			hasPositive = true
		} else {
			hasNegative = true
		}
	}
	if hasPositive && hasNegative {
		return nil, inventoryV1.ErrorBadRequest("an adjustment picking must be all-gain or all-loss; split mixed differences into separate pickings")
	}

	// 盘盈：INVENTORY_LOSS→仓库；盘亏：仓库→INVENTORY_LOSS。
	// move 的 planned_quantity 一律取绝对值（Validate 拒绝非正数量），
	// 方向由 source/dest 表达——与 INCOMING/OUTGOING 的语义一致。
	var srcLocID, dstLocID uint32
	absMoves := make([]*inventoryV1.StockMove, 0, len(moves))
	if hasPositive {
		srcLocID, dstLocID = lossLocID, whLocID
		absMoves = moves
	} else {
		srcLocID, dstLocID = whLocID, lossLocID
		for _, m := range moves {
			absMoves = append(absMoves, &inventoryV1.StockMove{
				ProductCode:         m.ProductCode,
				PlannedQuantity:     trans.Ptr(-m.GetPlannedQuantity()),
			})
		}
	}

	_, err = s.stockPickingRepo.Create(ctx, &inventoryV1.CreateStockPickingRequest{
		Data: &inventoryV1.StockPicking{
			PickingType:           inventoryV1.StockPicking_INVENTORY_ADJUSTMENT.Enum(),
			SourceLocationId:      trans.Ptr(srcLocID),
			DestinationLocationId: trans.Ptr(dstLocID),
			Moves:                 absMoves,
			Remark:                req.Data.Remark,
		},
	})
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// createOutgoingPicking 创建出库拣货单（OUTGOING，M2 直发链）：
// source = 出库仓库接收位置（INTERNAL），dest = 租户客户虚拟位置（CUSTOMER）。
// moves 由客户端提供（product + planned_quantity）。库存扣减在 Validate
// 单事务内执行：source quant 原子防负扣减 + FEFO 批次自动拆分（无指派时）
// + move-line 执行记录。客户虚拟位无 quant，dest 腿自动跳过——库存从
// 边界"消失"，符合 Odoo 出库语义。
func (s *StockPickingService) createOutgoingPicking(ctx context.Context, req *inventoryV1.CreateStockPickingRequest) (*emptypb.Empty, error) {
	whCode := req.Data.GetFromWarehouseCode()
	partnerCode := req.Data.GetPartnerCode()
	if whCode == "" {
		return nil, inventoryV1.ErrorBadRequest("from_warehouse_code is required for outgoing picking")
	}
	if partnerCode == "" {
		return nil, inventoryV1.ErrorBadRequest("partner_code is required for outgoing picking")
	}

	// source = 出库仓库的接收位置（INTERNAL）。
	sourceLocID, err := s.locationRepo.GetLocationID(ctx, whCode)
	if err != nil {
		return nil, err
	}
	// dest = 租户客户虚拟位置（CUSTOMER，无 quant）。
	destLocID, err := s.locationRepo.GetCustomerLocationID(ctx)
	if err != nil {
		return nil, err
	}

	// moves：客户端提供 per-move 的 product+planned_quantity。
	moves := req.Data.GetMoves()
	if len(moves) == 0 {
		return nil, inventoryV1.ErrorBadRequest("at least one move is required")
	}
	for _, m := range moves {
		if m == nil || m.GetProductCode() == "" || m.GetPlannedQuantity() <= 0 {
			return nil, inventoryV1.ErrorBadRequest("each move must have a valid product_code and positive planned_quantity")
		}
	}

	if _, err = s.stockPickingRepo.Create(ctx, &inventoryV1.CreateStockPickingRequest{
		Data: &inventoryV1.StockPicking{
			PickingType:           inventoryV1.StockPicking_OUTGOING.Enum(),
			SourceLocationId:      trans.Ptr(sourceLocID),
			DestinationLocationId: trans.Ptr(destLocID),
			PartnerCode:           trans.Ptr(partnerCode),
			Moves:                 moves,
			Remark:                req.Data.Remark,
		},
	}); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *StockPickingService) createInternalPicking(ctx context.Context, req *inventoryV1.CreateStockPickingRequest) (*emptypb.Empty, error) {
	fromWh := req.Data.GetFromWarehouseCode()
	toWh := req.Data.GetToWarehouseCode()
	if fromWh == "" || toWh == "" {
		return nil, inventoryV1.ErrorBadRequest("from_warehouse_code and to_warehouse_code are required for internal transfer")
	}
	if fromWh == toWh {
		return nil, inventoryV1.ErrorBadRequest("source and destination warehouses must differ")
	}

	// 推导 source/dest location（各自仓库的 receiving_location_id）。
	sourceLocID, err := s.locationRepo.GetLocationID(ctx, fromWh)
	if err != nil {
		return nil, err
	}
	destLocID, err := s.locationRepo.GetLocationID(ctx, toWh)
	if err != nil {
		return nil, err
	}

	// 构建 moves：客户端提供 per-move 的 product+planned_quantity，
	// location 从 picking 继承。
	moves := req.Data.GetMoves()
	if len(moves) == 0 {
		return nil, inventoryV1.ErrorBadRequest("at least one move is required")
	}
	for _, m := range moves {
		if m == nil || m.GetProductCode() == "" || m.GetPlannedQuantity() <= 0 {
			return nil, inventoryV1.ErrorBadRequest("each move must have a valid product_code and positive planned_quantity")
		}
	}

	_, err = s.stockPickingRepo.Create(ctx, &inventoryV1.CreateStockPickingRequest{
		Data: &inventoryV1.StockPicking{
			PickingType:           inventoryV1.StockPicking_INTERNAL.Enum(),
			SourceLocationId:      trans.Ptr(sourceLocID),
			DestinationLocationId: trans.Ptr(destLocID),
			Moves:                 moves,
			Remark:                req.Data.Remark,
		},
	})
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// Confirm 确认拣货单：将该 picking 下所有 DRAFT move 迁至 CONFIRMED
// （借鉴 Odoo action_confirm）。在事务内批量迁移。
func (s *StockPickingService) Confirm(ctx context.Context, req *inventoryV1.ConfirmStockPickingRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, inventoryV1.ErrorBadRequest("invalid request")
	}

	picking, err := s.stockPickingRepo.Get(ctx, &inventoryV1.GetStockPickingRequest{
		QueryBy: &inventoryV1.GetStockPickingRequest_Id{Id: req.GetId()},
	})
	if err != nil {
		return nil, err
	}

	// 仅 DRAFT 态可确认。
	if picking.GetDerivedState() != inventoryV1.StockPicking_DRAFT {
		return nil, inventoryV1.ErrorConflict("picking is not in draft state")
	}

	// 在事务内将所有 DRAFT move 迁至 CONFIRMED。
	tx, terr := s.stockPickingRepo.BeginTx(ctx)
	if terr != nil {
		return nil, terr
	}
	defer func() { s.stockPickingRepo.FinishTx(tx, terr) }()

	for _, move := range picking.GetMoves() {
		if move == nil || move.GetState() != inventoryV1.StockMove_DRAFT {
			continue
		}
		if err := s.stockPickingRepo.TransitionMoveStateTx(ctx, tx, move.GetId(),
			inventoryV1.StockMove_DRAFT, inventoryV1.StockMove_CONFIRMED); err != nil {
			terr = err
			return nil, err
		}
	}

	terr = nil
	return &emptypb.Empty{}, nil
}

// Validate 校验拣货单：在单事务内对每个 CONFIRMED move 创建 move-line、
// 更新 stock_quant（source 减、dest 加）。任一步骤失败 → 整事务回滚，
// 无补偿、无不一致窗口。
func (s *StockPickingService) Validate(ctx context.Context, req *inventoryV1.ValidateStockPickingRequest) (ret *emptypb.Empty, err error) {
	if req == nil || req.GetId() == 0 {
		return nil, inventoryV1.ErrorBadRequest("invalid request")
	}

	picking, err := s.stockPickingRepo.Get(ctx, &inventoryV1.GetStockPickingRequest{
		QueryBy: &inventoryV1.GetStockPickingRequest_Id{Id: req.GetId()},
	})
	if err != nil {
		return nil, err
	}

	// 仅 CONFIRMED 态可校验。
	if picking.GetDerivedState() != inventoryV1.StockPicking_CONFIRMED {
		return nil, inventoryV1.ErrorConflict("picking is not in confirmed state")
	}

	// 开启事务：全部腿的 quant 回写、move-line 落库、收货量回写均在
	// 事务内执行。任一腿失败则整体回滚，无补偿、无不一致窗口。
	tx, terr := s.stockPickingRepo.BeginTx(ctx)
	if terr != nil {
		return nil, terr
	}

	// defer 按 LIFO 顺序执行：事务提交/回滚（FinishTx）。
	defer func() { s.stockPickingRepo.FinishTx(tx, err) }()

	// 取该 picking 中所有 CONFIRMED moves（事务内）。
	confirmedMoves, cerr := s.stockPickingRepo.GetConfirmedMovesTx(ctx, tx, req.GetId())
	if cerr != nil {
		err = cerr
		return nil, err
	}

	// 批次指派索引：productCode → assignment（重复以最后一条为准）。
	assignments := map[string]*inventoryV1.LotAssignment{}
	for _, a := range req.GetLotAssignments() {
		if a.GetProductCode() != "" && a.GetLotName() != "" {
			assignments[a.GetProductCode()] = a
		}
	}

	// ASN 收货链：调用方未显式指派批次时，自动从到货明细回填
	// （收货时登记的 lot_name 随 ASN 行持久化）——否则批次表为空，
	// 后续出库 FEFO 无批次可拆。
	if picking.GetAsnId() != 0 && len(assignments) == 0 {
		if _, lines, lerr := s.asnRepo.GetWithLinesTx(ctx, nil, picking.GetAsnId()); lerr == nil {
			for _, l := range lines {
				if l.SkuCode != nil && *l.SkuCode != "" && l.LotName != nil && *l.LotName != "" {
					lotName := *l.LotName
					sku := *l.SkuCode
					la := &inventoryV1.LotAssignment{
						ProductCode: sku,
						LotName:     lotName,
					}
					if l.ExpiryDate != nil {
						la.ExpiryDate = timestamppb.New(*l.ExpiryDate)
					}
					assignments[sku] = la
				}
			}
		}
	}

	for _, move := range confirmedMoves {
		if move == nil {
			continue
		}

		sourceLocID := move.GetSourceLocationId()
		destLocID := move.GetDestinationLocationId()
		productCode := move.GetProductCode()
		executedQty := move.GetPlannedQuantity()

		if executedQty <= 0 {
			err = inventoryV1.ErrorBadRequest("planned quantity must be positive")
			return nil, err
		}

		// 查 source/dest 的 usage——借鉴 Odoo stock.location.usage：
		// SUPPLIER/CUSTOMER = 虚拟位置（无 quant，跳过该腿的 quant 回写），
		// INTERNAL = 真实位置（有 quant，执行回写）。这是双轨制库存的核心：
		// 只有真实位置之间的移动才同时做 source 减 / dest 加；涉及虚拟
		// 位置的腿跳过 quant 回写（库存从边界 "出现" 或 "消失"）。
		srcUsage, uerr := s.locationRepo.GetUsageTx(ctx, tx, sourceLocID)
		if uerr != nil {
			err = uerr
			return nil, err
		}
		dstUsage, uerr := s.locationRepo.GetUsageTx(ctx, tx, destLocID)
		if uerr != nil {
			err = uerr
			return nil, err
		}

		// 确定 unit_cost：出库/调拨腿从源位置 quant 的 cost_price 冻结
		// （COGS 核算用）；入库/盘盈回货（source 虚拟 → dest INTERNAL）取
		// dest quant 当前均价——按当前均价入库使加权平均保持稳定，避免零成本
		// 稀释。新 SKU 首次入库无均价可取则 unit_cost=0（WMS 后续由 ASN/
		// 入库单显式携带单价后在此取值）。
		var unitCost int64
		if srcUsage == inventoryV1.StockLocation_INTERNAL {
			srcQuantForCost, cerr := s.stockQuantRepo.FindByLocationProductTx(ctx, tx, sourceLocID, productCode)
			if cerr != nil {
				err = cerr
				return nil, err
			}
			unitCost, _ = s.stockQuantRepo.GetCostPriceTx(ctx, tx, srcQuantForCost.GetId())
		} else if dstUsage == inventoryV1.StockLocation_INTERNAL {
			// 盘盈 / 退货回货：source 是 INVENTORY_LOSS/CUSTOMER 虚拟位置，
			// 按 dest 当前均价入库（quant 行不存在则 0）。
			if dstQuantForCost, qerr := s.stockQuantRepo.FindByLocationProductTx(ctx, tx, destLocID, productCode); qerr == nil {
				unitCost, _ = s.stockQuantRepo.GetCostPriceTx(ctx, tx, dstQuantForCost.GetId())
			}
		}

		// 创建 move-line（执行记录，借鉴 Odoo stock.move.line._action_done）。
		// move-line 始终记录完整的 source→dest 轨迹（含虚拟位置），供审计追溯。
		// unit_cost 冻结执行时的单位成本（出库/调拨=源位置 quant 的加权平均
		// 成本），供成本核算。
		// 批次（记录式）：入库腿登记/复用批次；出库腿按指派（余量校验）或
		// FEFO 自动拆分扣减；未跟踪部分落 lotID=0。
		if err = s.recordMoveLinesWithLot(ctx, tx, move, req.GetId(),
			srcUsage, dstUsage, executedQty, unitCost,
			assignments[productCode]); err != nil {
			return nil, err
		}

		// 更新 source quant：quantity -= executedQty（防负，原子条件更新）。
		// 仅对真实位置（INTERNAL）执行——虚拟位置（SUPPLIER）无 quant 行，
		// 跳过（借鉴 Odoo _action_done 对虚拟 source 的处理）。
		if srcUsage == inventoryV1.StockLocation_INTERNAL {
			srcQuant, qerr := s.stockQuantRepo.FindByLocationProductTx(ctx, tx, sourceLocID, productCode)
			if qerr != nil {
				err = qerr
				return nil, err
			}
			if _, err = s.stockQuantRepo.ApplyDeltaTx(ctx, tx, srcQuant.GetId(), -executedQty); err != nil {
				return nil, err
			}
		}

		// 更新 dest quant：quantity += executedQty（EnsureForLocation 先确保行存在）。
		// 仅对真实位置（INTERNAL）执行——虚拟位置无 quant 行，跳过。
		// 使用 ApplyInboundWithCostTx 同时更新 quantity 和 cost_price（加权平均），
		// unitCost 为上面冻结的源位置成本或入库均价。
		if dstUsage == inventoryV1.StockLocation_INTERNAL {
			if err = s.stockQuantRepo.EnsureForLocationTx(ctx, tx, destLocID, productCode); err != nil {
				return nil, err
			}
			dstQuant, qerr := s.stockQuantRepo.FindByLocationProductTx(ctx, tx, destLocID, productCode)
			if qerr != nil {
				err = qerr
				return nil, err
			}
			if err = s.stockQuantRepo.ApplyInboundWithCostTx(ctx, tx, dstQuant.GetId(), executedQty, unitCost); err != nil {
				return nil, err
			}
		}

		// move 状态 → DONE（事务内）。
		if err = s.stockPickingRepo.TransitionMoveStateTx(ctx, tx, move.GetId(),
			inventoryV1.StockMove_CONFIRMED, inventoryV1.StockMove_DONE); err != nil {
			return nil, err
		}
	}

	// 事务提交（由 FinishTx 执行）。
	ret = &emptypb.Empty{}
	err = nil

	return ret, err
}

// recordMoveLinesWithLot 创建 move-line 并挂批次（记录式批次/效期）。
//
//   - 入库腿（dest INTERNAL，source 虚拟）：有指派 → get-or-create 批次并挂；
//     无指派 → 未跟踪行（lotID=0）。
//   - 出库腿（source INTERNAL，dest 虚拟）：有指派 → 批次必须存在且余量足够
//     （不足 409）；无指派 → FEFO 自动拆分：按效期升序逐批扣到 0，余量不足
//     的尾差落未跟踪行（兼容历史无批次库存，批次账目自洽）。
//   - 调拨等其余腿：单行；指派批次存在则挂上（不改变批次余量）。
func (s *StockPickingService) recordMoveLinesWithLot(
	ctx context.Context,
	tx *ent.Tx,
	move *inventoryV1.StockMove,
	pickingID uint32,
	srcUsage inventoryV1.StockLocation_Usage,
	dstUsage inventoryV1.StockLocation_Usage,
	executedQty int64,
	unitCost int64,
	assignment *inventoryV1.LotAssignment,
) error {
	srcInternal := srcUsage == inventoryV1.StockLocation_INTERNAL
	dstInternal := dstUsage == inventoryV1.StockLocation_INTERNAL

	createLine := func(qty int64, lotID uint32) error {
		return s.stockMoveLineRepo.CreateTx(ctx, tx, move.GetId(), pickingID,
			move.GetProductCode(), move.GetSourceLocationId(), move.GetDestinationLocationId(),
			qty, unitCost, lotID)
	}

	// 入库腿：登记批次。优先用 move 自带的批次信息（ASN 收货链：
	// 每 (sku, lot, expiry) 一条 move，多批次分次收互不覆盖），
	// 其次走调用方显式指派（LotAssignments）；都没有则未跟踪行。
	if dstInternal && !srcInternal {
		if move.GetLotName() != "" {
			var expiry *time.Time
			if move.GetExpiryDate() != nil {
				t := move.GetExpiryDate().AsTime()
				expiry = &t
			}
			lotID, lerr := s.stockLotRepo.GetOrCreateTx(ctx, tx,
				move.GetProductCode(), move.GetLotName(), expiry)
			if lerr != nil {
				return lerr
			}
			return createLine(executedQty, lotID)
		}
		if assignment != nil {
			var expiry *time.Time
			if assignment.GetExpiryDate() != nil {
				t := assignment.GetExpiryDate().AsTime()
				expiry = &t
			}
			lotID, lerr := s.stockLotRepo.GetOrCreateTx(ctx, tx,
				move.GetProductCode(), assignment.GetLotName(), expiry)
			if lerr != nil {
				return lerr
			}
			return createLine(executedQty, lotID)
		}
		return createLine(executedQty, 0)
	}

	// 出库腿：指定批次（余量校验）或 FEFO 自动拆分。
	if srcInternal && !dstInternal {
		if assignment != nil {
			lotID, err := s.stockLotRepo.FindIdTx(ctx, tx,
				move.GetProductCode(), assignment.GetLotName())
			if err != nil {
				return err
			}
			if lotID == 0 {
				return inventoryV1.ErrorBadRequest(
					"批次不存在：%s / %s", move.GetProductCode(), assignment.GetLotName())
			}
			remaining, err := s.stockLotRepo.LotRemainingTx(ctx, tx, lotID)
			if err != nil {
				return err
			}
			if remaining < executedQty {
				return inventoryV1.ErrorConflict(
					"批次 %s 余量不足：%d < %d", assignment.GetLotName(), remaining, executedQty)
			}
			return createLine(executedQty, lotID)
		}

		lots, err := s.stockLotRepo.ListFefoTx(ctx, tx, move.GetProductCode())
		if err != nil {
			return err
		}
		left := executedQty
		for _, lot := range lots {
			if left <= 0 {
				break
			}
			take := lot.Remaining
			if take > left {
				take = left
			}
			if err := createLine(take, lot.LotID); err != nil {
				return err
			}
			left -= take
		}
		// 尾差：历史无批次库存（未跟踪），不阻断业务。
		if left > 0 {
			return createLine(left, 0)
		}
		return nil
	}

	// 调拨/其余腿：单行；指派批次存在则挂上。
	lotID := uint32(0)
	if assignment != nil {
		if id, err := s.stockLotRepo.FindIdTx(ctx, tx,
			move.GetProductCode(), assignment.GetLotName()); err == nil {
			lotID = id
		}
	}
	return createLine(executedQty, lotID)
}

// Cancel 取消拣货单：将所有非终态 moves 迁至 CANCELLED（借鉴 Odoo
// action_cancel）。在事务内批量迁移。
func (s *StockPickingService) Cancel(ctx context.Context, req *inventoryV1.CancelStockPickingRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, inventoryV1.ErrorBadRequest("invalid request")
	}

	picking, err := s.stockPickingRepo.Get(ctx, &inventoryV1.GetStockPickingRequest{
		QueryBy: &inventoryV1.GetStockPickingRequest_Id{Id: req.GetId()},
	})
	if err != nil {
		return nil, err
	}

	// 终态不可取消。
	if picking.GetDerivedState() == inventoryV1.StockPicking_DONE ||
		picking.GetDerivedState() == inventoryV1.StockPicking_CANCELLED {
		return nil, inventoryV1.ErrorConflict("picking is already in a terminal state")
	}

	// 在事务内将所有非终态 moves 迁至 CANCELLED。
	tx, terr := s.stockPickingRepo.BeginTx(ctx)
	if terr != nil {
		return nil, terr
	}
	defer func() { s.stockPickingRepo.FinishTx(tx, terr) }()

	if err := s.stockPickingRepo.CancelMovesTx(ctx, tx, req.GetId()); err != nil {
		terr = err
		return nil, err
	}

	terr = nil
	return &emptypb.Empty{}, nil
}

func (s *StockPickingService) Delete(ctx context.Context, req *inventoryV1.DeleteStockPickingRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, inventoryV1.ErrorBadRequest("invalid request")
	}

	picking, err := s.stockPickingRepo.Get(ctx, &inventoryV1.GetStockPickingRequest{
		QueryBy: &inventoryV1.GetStockPickingRequest_Id{Id: req.GetId()},
	})
	if err != nil {
		return nil, err
	}

	// 仅 DRAFT/CANCELLED 可删（终态审计记录与在途单不可抹除）。
	if picking.GetDerivedState() != inventoryV1.StockPicking_DRAFT &&
		picking.GetDerivedState() != inventoryV1.StockPicking_CANCELLED {
		return nil, inventoryV1.ErrorConflict("only draft or cancelled pickings can be deleted")
	}

	if err := s.stockPickingRepo.Delete(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
