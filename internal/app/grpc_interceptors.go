package app

import (
	"context"

	"gosdk/pkg/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type contextKey string

const UserIDKey contextKey = "user_id"

type AuthInterceptor struct {
	validator *AuthValidator
}

func NewAuthInterceptor(validator *AuthValidator) *AuthInterceptor {
	return &AuthInterceptor{
		validator: validator,
	}
}

func (i *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "metadata not found")
		}

		sessionIDs := md.Get("session-id")
		if len(sessionIDs) == 0 {
			return nil, status.Error(codes.Unauthenticated, "session-id not found")
		}

		sessionData, err := i.validator.ValidateSession(ctx, sessionIDs[0])
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid session")
		}

		ctx = context.WithValue(ctx, UserIDKey, sessionData.UserID)
		return handler(ctx, req)
	}
}

type LoggingInterceptor struct {
	logger logger.Logger
}

func NewLoggingInterceptor(logger logger.Logger) *LoggingInterceptor {
	return &LoggingInterceptor{logger: logger}
}

func (i *LoggingInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		i.logger.Info(ctx, "gRPC request", logger.Field{Key: "method", Value: info.FullMethod})
		resp, err := handler(ctx, req)
		if err != nil {
			i.logger.Error(ctx, "gRPC error", logger.Field{Key: "error", Value: err.Error()}, logger.Field{Key: "method", Value: info.FullMethod})
		}
		return resp, err
	}
}
