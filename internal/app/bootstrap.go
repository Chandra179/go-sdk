package app

import (
	"context"
	"fmt"

	"gosdk/internal/service/auth"
	"gosdk/internal/service/event"
	"gosdk/internal/service/session"
	"gosdk/pkg/cache"
	"gosdk/pkg/db"
	"gosdk/pkg/kafka"
	"gosdk/pkg/oauth2"
)

func (s *Server) initDatabase() error {
	pg := s.config.Postgres
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		pg.User, pg.Password, pg.Host, pg.Port, pg.DBName, pg.SSLMode,
	)

	dbClient, err := db.NewSQLClient("postgres", dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	s.db = dbClient

	if err := runMigrations(dsn); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	return nil
}

func (s *Server) initCache() error {
	addr := s.config.Redis.Host + ":" + s.config.Redis.Port
	s.cache = cache.NewRedisCache(addr)
	return nil
}

func (s *Server) initOAuth2(ctx context.Context) error {
	mgr, err := oauth2.NewManager(ctx, &s.config.OAuth2)
	if err != nil {
		return err
	}
	s.oauth2Manager = mgr
	return nil
}

func (s *Server) initEvent() error {
	s.kafkaClient = kafka.NewClient(s.config.Kafka.Brokers)
	s.messageBrokerSvc = event.NewService(s.kafkaClient)
	s.messageBrokerHandler = event.NewHandler(s.messageBrokerSvc)
	return nil
}

func (s *Server) initServices() {
	s.sessionStore = session.NewRedisStore(s.cache)

	s.authService = auth.NewService(
		s.oauth2Manager,
		s.sessionStore,
		s.db,
	)

	s.oauth2Manager.CallbackHandler = func(
		ctx context.Context,
		provider string,
		userInfo *oauth2.UserInfo,
		tokenSet *oauth2.TokenSet,
	) (*oauth2.CallbackInfo, error) {
		return s.authService.HandleCallback(ctx, provider, userInfo, tokenSet)
	}
}
