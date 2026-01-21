package bootstrap

import (
	"context"

	"gosdk/cfg"
	"gosdk/pkg/oauth2"
)

func InitOAuth2(ctx context.Context, config *cfg.Oauth2Config) (*oauth2.Manager, error) {
	return oauth2.NewManager(ctx, config)
}
