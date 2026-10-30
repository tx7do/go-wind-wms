package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"

	adminV1 "go-wind-wms/api/gen/go/admin/service/v1"
	"google.golang.org/protobuf/types/known/emptypb"

	inventoryV1 "go-wind-wms/api/gen/go/inventory/service/v1"
)

type StockQuantService struct {
	adminV1.StockQuantServiceHTTPServer

	log *log.Helper

	stockQuantServiceClient inventoryV1.StockQuantServiceClient
}

func NewStockQuantService(
	ctx *bootstrap.Context,
	stockQuantServiceClient inventoryV1.StockQuantServiceClient,
) *StockQuantService {
	l := log.NewHelper(log.With(ctx.GetLogger(), "module", "stock-quant/service/admin-service"))
	return &StockQuantService{
		log:                     l,
		stockQuantServiceClient: stockQuantServiceClient,
	}
}

// Freeze 冻结库存行（转发）。
func (s *StockQuantService) Freeze(ctx context.Context, req *inventoryV1.FreezeQuantRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	return s.stockQuantServiceClient.Freeze(ctx, req)
}

// Unfreeze 解冻库存行（转发）。
func (s *StockQuantService) Unfreeze(ctx context.Context, req *inventoryV1.UnfreezeQuantRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	return s.stockQuantServiceClient.Unfreeze(ctx, req)
}

// GetStockAging 库龄报表（转发）。
func (s *StockQuantService) GetStockAging(ctx context.Context, req *inventoryV1.GetStockAgingRequest) (*inventoryV1.StockAgingResponse, error) {
	return s.stockQuantServiceClient.GetStockAging(ctx, req)
}

func (s *StockQuantService) List(ctx context.Context, req *paginationV1.PagingRequest) (*inventoryV1.ListStockQuantResponse, error) {
	return s.stockQuantServiceClient.List(ctx, req)
}

func (s *StockQuantService) Get(ctx context.Context, req *inventoryV1.GetStockQuantRequest) (*inventoryV1.StockQuant, error) {
	return s.stockQuantServiceClient.Get(ctx, req)
}

func (s *StockQuantService) GetOverview(ctx context.Context, req *inventoryV1.GetStockQuantOverviewRequest) (*inventoryV1.StockQuantOverview, error) {
	return s.stockQuantServiceClient.GetOverview(ctx, req)
}

func (s *StockQuantService) GetMovementTrend(ctx context.Context, req *inventoryV1.GetMovementTrendRequest) (*inventoryV1.MovementTrendResponse, error) {
	return s.stockQuantServiceClient.GetMovementTrend(ctx, req)
}
