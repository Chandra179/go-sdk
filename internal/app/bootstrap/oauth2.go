package bootstrap

import (
	"context"
	"fmt"

	"gosdk/internal/cfg"
	"gosdk/pkg/oauth2"
)

// InitOAuth2 initializes the OAuth2 manager with Google provider
// The callbackHandler must be provided and will be called after successful OAuth2 authentication
func InitOAuth2(ctx context.Context, config *cfg.Oauth2Config, callbackHandler oauth2.CallbackHandler) (*oauth2.Manager, error) {
	if callbackHandler == nil {
		return nil, fmt.Errorf("callback handler is required")
	}

	// Create manager configuration from app config
	managerConfig := &oauth2.ManagerConfig{
		CallbackHandler: callbackHandler,
		StateTimeout:    config.StateTimeout,
	}

	// Create manager with Google provider
	manager, err := oauth2.NewManagerWithGoogle(
		ctx,
		managerConfig,
		config.GoogleClientID,
		config.GoogleClientSecret,
		config.GoogleRedirectUrl,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OAuth2 manager: %w", err)
	}

	return manager, nil
}
