package oauth2

import (
	"context"
	"testing"

	"gosdk/cfg"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MockProvider struct {
	name string
}

func (m *MockProvider) GetName() string {
	return m.name
}

func (m *MockProvider) GetAuthURL(state, nonce, codeChallenge string) string {
	return "https://example.com/auth?state=" + state
}

func (m *MockProvider) HandleCallback(ctx context.Context, code, state, nonce, codeVerifier string) (*UserInfo, *TokenSet, error) {
	return &UserInfo{}, &TokenSet{}, nil
}

func (m *MockProvider) RefreshToken(ctx context.Context, refreshToken string) (*TokenSet, error) {
	return &TokenSet{}, nil
}

func TestNewManager_Success(t *testing.T) {
	ctx := context.Background()
	config := &cfg.Oauth2Config{}

	manager, err := NewManager(ctx, config)
	require.NoError(t, err)
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.providers)
	assert.NotNil(t, manager.stateStorage)
}

func TestManager_RegisterProvider(t *testing.T) {
	ctx := context.Background()
	config := &cfg.Oauth2Config{}

	manager, err := NewManager(ctx, config)
	require.NoError(t, err)

	mockProvider := &MockProvider{name: "test-provider"}
	manager.RegisterProvider(mockProvider)

	provider, exists := manager.providers["test-provider"]
	assert.True(t, exists)
	assert.Equal(t, mockProvider, provider)
}

func TestManager_GetProvider_Success(t *testing.T) {
	ctx := context.Background()
	config := &cfg.Oauth2Config{}

	manager, err := NewManager(ctx, config)
	require.NoError(t, err)

	mockProvider := &MockProvider{name: "test-provider"}
	manager.RegisterProvider(mockProvider)

	provider, err := manager.GetProvider("test-provider")
	assert.NoError(t, err)
	assert.Equal(t, mockProvider, provider)
}

func TestManager_GetProvider_NotFound(t *testing.T) {
	ctx := context.Background()
	config := &cfg.Oauth2Config{}

	manager, err := NewManager(ctx, config)
	require.NoError(t, err)

	_, err = manager.GetProvider("nonexistent")
	assert.Error(t, err)
	assert.Equal(t, ErrProviderNotFound, err)
}

func TestManager_Cleanup(t *testing.T) {
	ctx := context.Background()
	config := &cfg.Oauth2Config{}

	manager, err := NewManager(ctx, config)
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		manager.Cleanup()
	})
}
