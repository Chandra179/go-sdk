package user

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/emptypb"
	"gosdk/gen/proto/user"
	"gosdk/pkg/db"
)

type GRPCService struct {
	UnimplementedUserServiceServer
	db db.DB
}

func NewGRPCService(db db.DB) *GRPCService {
	return &GRPCService{db: db}
}

func (s *GRPCService) GetUser(ctx context.Context, req *GetUserRequest) (*GetUserResponse, error) {
	query := `SELECT id, name, email FROM users WHERE id = $1`
	var pbUser User
	err := s.db.QueryRowContext(ctx, query, req.Id).Scan(&pbUser.Id, &pbUser.Name, &pbUser.Email)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &GetUserResponse{User: &pbUser}, nil
}

func (s *GRPCService) CreateUser(ctx context.Context, req *CreateUserRequest) (*emptypb.Empty, error) {
	query := `INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id`
	var id string
	err := s.db.QueryRowContext(ctx, query, req.Name, req.Email).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &emptypb.Empty{}, nil
}
