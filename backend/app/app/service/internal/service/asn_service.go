package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"

	appV1 "go-wind-wms/api/gen/go/app/service/v1"
	asnV1 "go-wind-wms/api/gen/go/asn/service/v1"
)

// AsnService 到货通知单（PDA 手持端转发）：扫码查单 + 收货 + 上架。
type AsnService struct {
	appV1.AsnServiceHTTPServer

	log *log.Helper

	asnServiceClient asnV1.AsnServiceClient
}

func NewAsnService(
	ctx *bootstrap.Context,
	asnServiceClient asnV1.AsnServiceClient,
) *AsnService {
	l := log.NewHelper(log.With(ctx.GetLogger(), "module", "asn/service/app-service"))
	return &AsnService{
		log:              l,
		asnServiceClient: asnServiceClient,
	}
}

func (s *AsnService) List(ctx context.Context, req *paginationV1.PagingRequest) (*asnV1.ListAsnResponse, error) {
	return s.asnServiceClient.List(ctx, req)
}

func (s *AsnService) Get(ctx context.Context, req *asnV1.GetAsnRequest) (*asnV1.Asn, error) {
	return s.asnServiceClient.Get(ctx, req)
}

func (s *AsnService) GetByNumber(ctx context.Context, req *asnV1.GetAsnByNumberRequest) (*asnV1.Asn, error) {
	if req == nil || req.GetNumber() == "" {
		return nil, appV1.ErrorBadRequest("invalid parameter")
	}
	return s.asnServiceClient.GetByNumber(ctx, req)
}

func (s *AsnService) Receive(ctx context.Context, req *asnV1.ReceiveAsnRequest) (*asnV1.ReceiveAsnResponse, error) {
	if req == nil || req.GetAsnId() == 0 {
		return nil, appV1.ErrorBadRequest("invalid parameter")
	}
	return s.asnServiceClient.Receive(ctx, req)
}

func (s *AsnService) Putaway(ctx context.Context, req *asnV1.PutawayAsnRequest) (*asnV1.PutawayAsnResponse, error) {
	if req == nil || req.GetAsnId() == 0 {
		return nil, appV1.ErrorBadRequest("invalid parameter")
	}
	return s.asnServiceClient.Putaway(ctx, req)
}
