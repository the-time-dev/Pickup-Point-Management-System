package grpc_api

import (
	pb "avito_intr/internal/grpc_api/pvz_v1"
	"avito_intr/internal/storage"
	"context"
	"go.uber.org/zap"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

type GrpcServer struct {
	pb.UnimplementedPVZServiceServer
	storage storage.Storage
	logger  *zap.Logger
}

func NewGrpcServer(storage storage.Storage, logger *zap.Logger) *GrpcServer {
	return &GrpcServer{storage: storage, logger: logger}
}

func (s GrpcServer) GetPVZList(ctx context.Context, request *pb.GetPVZListRequest) (*pb.GetPVZListResponse, error) {
	method := "getPVZList"

	var ip string
	p, ok := peer.FromContext(ctx)
	if ok {
		ip = p.Addr.String()
	} else {
		ip = "unknown"
	}

	t := time.Now()

	info, err := s.storage.GetOnlyPvzList()
	if err != nil {
		s.logger.Error("GRPC Request",
			zap.String("method", request.String()),
			zap.String("client_ip", ip),
			zap.Duration("duration", time.Since(t)),
			zap.Error(err),
		)
		return nil, err
	}

	var ans []*pb.PVZ

	for _, v := range info {
		ans = append(ans, &pb.PVZ{Id: *v.PvzId, RegistrationDate: timestamppb.New(*v.RegistrationDate), City: string(v.City)})
	}

	s.logger.Info("GRPC Request",
		zap.String("method", method),
		zap.String("client_ip", ip),
		zap.Duration("duration", time.Since(t)),
	)

	return &pb.GetPVZListResponse{Pvzs: ans}, nil
}
