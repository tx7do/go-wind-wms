package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"google.golang.org/protobuf/types/known/emptypb"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"

	adminV1 "go-wind-wms/api/gen/go/admin/service/v1"
	waveV1 "go-wind-wms/api/gen/go/wave/service/v1"
)

type WaveService struct {
	adminV1.WaveServiceHTTPServer

	log *log.Helper

	waveServiceClient waveV1.WaveServiceClient
}

func NewWaveService(
	ctx *bootstrap.Context,
	waveServiceClient waveV1.WaveServiceClient,
) *WaveService {
	l := log.NewHelper(log.With(ctx.GetLogger(), "module", "wave/service/admin-service"))
	return &WaveService{
		log:               l,
		waveServiceClient: waveServiceClient,
	}
}

func (s *WaveService) List(ctx context.Context, req *paginationV1.PagingRequest) (*waveV1.ListWaveResponse, error) {
	return s.waveServiceClient.List(ctx, req)
}

func (s *WaveService) Count(ctx context.Context, req *paginationV1.PagingRequest) (*waveV1.CountWaveResponse, error) {
	return s.waveServiceClient.Count(ctx, req)
}

func (s *WaveService) Get(ctx context.Context, req *waveV1.GetWaveRequest) (*waveV1.Wave, error) {
	return s.waveServiceClient.Get(ctx, req)
}

func (s *WaveService) Create(ctx context.Context, req *waveV1.CreateWaveRequest) (*emptypb.Empty, error) {
	if req == nil || len(req.GetPickingIds()) == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	return s.waveServiceClient.Create(ctx, req)
}

func (s *WaveService) Run(ctx context.Context, req *waveV1.RunWaveRequest) (*waveV1.RunWaveResponse, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	return s.waveServiceClient.Run(ctx, req)
}

func (s *WaveService) Cancel(ctx context.Context, req *waveV1.CancelWaveRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	return s.waveServiceClient.Cancel(ctx, req)
}
