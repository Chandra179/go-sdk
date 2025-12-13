# Oauth2 & OIDC
Handling oauth2, oidc

## Features
- GetAuthURL
- CallbackHandler: register callback function to be called when we got the oauth2 callback (ex: from google) 
- RefreshToken for access token

## Callback
If internal service need some information related to user or token they can create callback functiton

```go
package oauth2

// CallbackHandler is invoked after successful OAuth2 authentication
// It receives the provider name, user info, and token set
// Returns custom data that can be used by the caller (e.g., session data)
type CallbackInfo struct {
	SessionCookieName string
	UserID            string
	SessionID         string
	CookieMaxAge      int
}
type UserInfo struct {
	ID        string    
	Email     string    
	Name      string    
	Provider  string   
	CreatedAt time.Time 
}
type TokenSet struct {
	AccessToken  string    
	TokenType    string   
	RefreshToken string    
	IDToken      string   
	ExpiresAt    time.Time
}

type CallbackHandler func(ctx context.Context, provider string, userInfo *UserInfo, tokenSet *TokenSet) (*CallbackInfo, error)
```