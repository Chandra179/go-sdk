# App
do service initialization and service dependency injection here. 
function must be depends on the interface not the concrete implementation for some service

## server.go
```go
package app
import (
	"gosdk/internal/service/auth"
	"gosdk/pkg/cache"
	"gosdk/pkg/db"
	"gosdk/pkg/logger"
	"gosdk/pkg/oauth2"
	"gosdk/pkg/session"
)

type Server struct {
	config        *cfg.Config
	router        *gin.Engine
	logger        *logger.AppLogger
	db            *db.SQLClient
	cache         cache.Cache
	sessionStore  session.Store
	oauth2Manager *oauth2.Manager
	authService   *auth.Service
}

func NewServer(ctx context.Context, config *cfg.Config) (*Server, error) {
	s := &Server{
		config: config,
	}

    shutdown, err := setupObservability(ctx, &config.Observability)
	if err != nil {
		return nil, fmt.Errorf("observability setup: %w", err)
	}
	s.shutdown = shutdown

	s.logger = logger.NewLogger(config.AppEnv)

	if err := s.initDatabase(); err != nil {
		return nil, fmt.Errorf("database init: %w", err)
	}

	if err := s.initCache(); err != nil {
		return nil, fmt.Errorf("cache init: %w", err)
	}

	s.sessionStore = session.NewInMemoryStore()

	if err := s.initOAuth2(ctx); err != nil {
		return nil, fmt.Errorf("oauth2 init: %w", err)
	}

	s.initServices()
	s.setupRoutes()

	return s, nil
}

func (s *Server) initServices() {
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
    s.serviceA = a.NewService()
    s.serviceB = b.NewService()
}
```

## routes.go
```go
// register the internal service endpoint here.
func setupInfraRoutes(r *gin.Engine) {
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.GET("/docs", docsHandler)
}
func setupAbcRoutes(r *gin.Engine) {
}
func setupDefRoutes(r *gin.Engine) {
}
```