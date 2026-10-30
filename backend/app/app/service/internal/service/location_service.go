package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"

	appV1 "go-wind-wms/api/gen/go/app/service/v1"
	inventoryV1 "go-wind-wms/api/gen/go/inventory/service/v1"
)

// LocationService 库存位置（PDA 只读转发）：上架时选择目标库位。
type LocationService struct {
	appV1.LocationServiceHTTPServer

	log *log.Helper

	locationServiceClient inventoryV1.LocationServiceClient
}

func NewLocationService(
	ctx *bootstrap.Context,
	locationServiceClient inventoryV1.LocationServiceClient,
) *LocationService {
	l := log.NewHelper(log.With(ctx.GetLogger(), "module", "location/service/app-service"))
	return &LocationService{
		log:                   l,
		locationServiceClient: locationServiceClient,
	}
}

func (s *LocationService) List(ctx context.Context, req *paginationV1.PagingRequest) (*inventoryV1.ListLocationResponse, error) {
	return s.locationServiceClient.List(ctx, req)
}

func (s *LocationService) Get(ctx context.Context, req *inventoryV1.GetLocationRequest) (*inventoryV1.StockLocation, error) {
	return s.locationServiceClient.Get(ctx, req)
}
