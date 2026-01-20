package app

import "context"

func (s *Server) SetupGateway(ctx context.Context) error {
	// Gateway removed - no longer needed without gRPC services
	return nil
}
