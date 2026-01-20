package app

import (
	"context"
	"fmt"
	"net"

	"gosdk/gen/proto/user"
	"gosdk/internal/service/user"
	"gosdk/pkg/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
)

func (s *Server) SetupGRPCServer() error {
	lis, err := net.Listen("tcp", ":"+s.config.GRPCServer.Port)
	if err != nil {
		return fmt.Errorf("grpc listener: %w", err)
	}

	authInterceptor := NewAuthInterceptor(NewAuthValidator(s.authService)).Unary()
	loggingInterceptor := NewLoggingInterceptor(s.logger).Unary()

	s.grpcServer = grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			loggingInterceptor,
			authInterceptor,
		),
	)

	userService := user.NewGRPCService(s.db)
	userpb.RegisterUserServiceServer(s.grpcServer, userService)

	s.healthSrv = grpc_health_v1.NewServer()
	s.healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(s.grpcServer, s.healthSrv)

	grpc_reflection_v1.Register(s.grpcServer)

	s.grpcShutdown = func(ctx context.Context) error {
		s.logger.Info(ctx, "Shutting down gRPC server")
		s.grpcServer.GracefulStop()
		return nil
	}

	go func() {
		s.logger.Info(context.Background(), "gRPC server listening", logger.Field{Key: "addr", Value: ":" + s.config.GRPCServer.Port})
		if err := s.grpcServer.Serve(lis); err != nil {
			s.logger.Error(context.Background(), "gRPC server error", logger.Field{Key: "error", Value: err.Error()})
		}
	}()

	return nil
}

func (s *Server) UpdateGRPCHealthStatus(isHealthy bool) {
	if s.healthSrv == nil {
		return
	}

	status := grpc_health_v1.HealthCheckResponse_NOT_SERVING
	if isHealthy {
		status = grpc_health_v1.HealthCheckResponse_SERVING
	}
	s.healthSrv.SetServingStatus("", status)
}
