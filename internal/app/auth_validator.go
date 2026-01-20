package app

import (
	"context"

	"gosdk/internal/service/auth"
)

type AuthValidator struct {
	authService *auth.Service
}

func NewAuthValidator(authService *auth.Service) *AuthValidator {
	return &AuthValidator{
		authService: authService,
	}
}

func (v *AuthValidator) ValidateSession(ctx context.Context, sessionID string) (*auth.SessionData, error) {
	sessionData, err := v.authService.ValidateAndRefreshSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return sessionData, nil
}
