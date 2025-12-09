# Oauth2 & OIDC
Handling oauth2, oidc and session management (tied to oidc data)

## Features
- GetAuthURL
- CallbackHandler
- RefreshToken for access token
- InMemory Session handler
- AuthMiddleware: this functions put on the http middleware (intercepted requeste before proceed the original request)
