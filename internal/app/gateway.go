package app

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func (s *Server) SetupGateway(ctx context.Context) error {
	conn, err := grpc.NewClient(
		fmt.Sprintf(":%s", s.config.GRPCServer.Port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("gateway grpc connection: %w", err)
	}

	gwmux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
			return key, true
		}),
	)

	if err := userpb.RegisterUserServiceHandler(ctx, gwmux, conn); err != nil {
		return fmt.Errorf("gateway registration: %w", err)
	}

	s.router.Any("/v1/*filepath", func(c *gin.Context) {
		gwmux.ServeHTTP(c.Writer, c.Request)
	})

	s.logger.Info(ctx, "Gateway initialized on /v1/*")
	return nil
}
