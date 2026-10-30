package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"google.golang.org/protobuf/types/known/emptypb"

	paginationV1 "github.com/tx7do/go-crud/api/gen/go/pagination/v1"

	adminV1 "go-wind-wms/api/gen/go/admin/service/v1"
	asnV1 "go-wind-wms/api/gen/go/asn/service/v1"
)

type AsnService struct {
	adminV1.AsnServiceHTTPServer

	log *log.Helper

	asnServiceClient asnV1.AsnServiceClient
}

func NewAsnService(
	ctx *bootstrap.Context,
	asnServiceClient asnV1.AsnServiceClient,
) *AsnService {
	l := log.NewHelper(log.With(ctx.GetLogger(), "module", "asn/service/admin-service"))
	return &AsnService{
		log:              l,
		asnServiceClient: asnServiceClient,
	}
}

func (s *AsnService) List(ctx context.Context, req *paginationV1.PagingRequest) (*asnV1.ListAsnResponse, error) {
	return s.asnServiceClient.List(ctx, req)
}

func (s *AsnService) Count(ctx context.Context, req *paginationV1.PagingRequest) (*asnV1.CountAsnResponse, error) {
	return s.asnServiceClient.Count(ctx, req)
}

func (s *AsnService) Get(ctx context.Context, req *asnV1.GetAsnRequest) (*asnV1.Asn, error) {
	return s.asnServiceClient.Get(ctx, req)
}

func (s *AsnService) Create(ctx context.Context, req *asnV1.CreateAsnRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	return s.asnServiceClient.Create(ctx, req)
}

func (s *AsnService) Update(ctx context.Context, req *asnV1.UpdateAsnRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	return s.asnServiceClient.Update(ctx, req)
}

func (s *AsnService) Delete(ctx context.Context, req *asnV1.DeleteAsnRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	return s.asnServiceClient.Delete(ctx, req)
}

func (s *AsnService) Announce(ctx context.Context, req *asnV1.AnnounceAsnRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	return s.asnServiceClient.Announce(ctx, req)
}

func (s *AsnService) Cancel(ctx context.Context, req *asnV1.CancelAsnRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	return s.asnServiceClient.Cancel(ctx, req)
}

func (s *AsnService) Receive(ctx context.Context, req *asnV1.ReceiveAsnRequest) (*asnV1.ReceiveAsnResponse, error) {
	if req == nil || req.GetAsnId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	return s.asnServiceClient.Receive(ctx, req)
}

func (s *AsnService) Putaway(ctx context.Context, req *asnV1.PutawayAsnRequest) (*asnV1.PutawayAsnResponse, error) {
	if req == nil || req.GetAsnId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	return s.asnServiceClient.Putaway(ctx, req)
}

func (s *AsnService) Close(ctx context.Context, req *asnV1.CloseAsnRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	return s.asnServiceClient.Close(ctx, req)
}
