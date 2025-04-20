package main

import (
	"avito_intr/internal/auth/jwt_auth"
	"avito_intr/internal/grpc_api"
	pb "avito_intr/internal/grpc_api/pvz_v1"
	"avito_intr/internal/http_api"
	"avito_intr/internal/storage/pg_storage"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"net"
	"os"
)

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	defer func(logger *zap.Logger) {
		err := logger.Sync()
		if err != nil {
			logger.Error("failed to sync logger", zap.Error(err))
		}
	}(logger)

	logger.Info("starting application")

	pgConn, ok := os.LookupEnv("PG_CONN")
	if !ok {
		logger.Fatal("PG_CONN environment variable not set")
	}

	jwtKey, ok := os.LookupEnv("JWT_SECRET_KEY")
	if !ok {
		jwtKey = "secret_key"
		logger.Warn("JWT_SECRET_KEY not set, using default")
	}

	port, ok := os.LookupEnv("PORT")
	if !ok {
		port = "8080"
		logger.Warn("PORT not set, using default :8080")
	}

	metrics_port, ok := os.LookupEnv("METRICS_PORT")
	if !ok {
		metrics_port = "9000"
		logger.Warn("METRICS_PORT not set, using default :9000")
	}

	grpc_port, ok := os.LookupEnv("GRPC_PORT")
	if !ok {
		metrics_port = "3000"
		logger.Warn("GRPC_PORT not set, using default :9000")
	}

	pg, err := pg_storage.NewPgStorage(pgConn)
	if err != nil {
		logger.Fatal("failed to connect to Postgres", zap.Error(err))
	}

	logger.Info("running database migrations")
	if err := pg.Migrate(); err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}

	auth := jwt_auth.NewJwtAuth(jwtKey)
	h := http_api.NewServer(pg, auth, logger)

	lis, err := net.Listen("tcp", ":"+grpc_port)
	if err != nil {
		logger.Fatal("failed to listen on gRPC port", zap.Error(err))
	}

	logger.Info("starting gRPC server", zap.String("grpc-port", grpc_port))
	s := grpc.NewServer()
	pb.RegisterPVZServiceServer(s, grpc_api.NewGrpcServer(pg, logger))

	go func() {
		if err := s.Serve(lis); err != nil {
			logger.Fatal("failed to start gRPC server", zap.Error(err))
		}
	}()

	logger.Info("starting HTTP server", zap.String("port", port), zap.String("metrics-port", metrics_port))
	if err := h.ListenAndServe(port, metrics_port); err != nil {
		logger.Fatal("HTTP server failed", zap.Error(err))
	}
}
