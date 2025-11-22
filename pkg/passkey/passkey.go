package passkey

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/webauthn"
)

type User struct {
	ID          []byte
	Username    string
	Credentials []webauthn.Credential
}

func (u *User) WebAuthnID() []byte {
	return u.ID
}

func (u *User) WebAuthnName() string {
	return u.Username
}

func (u *User) WebAuthnDisplayName() string {
	return u.Username
}

func (u *User) WebAuthnIcon() string {
	return ""
}

func (u *User) WebAuthnCredentials() []webauthn.Credential {
	return u.Credentials
}

func (u *User) AddCredential(cred webauthn.Credential) {
	u.Credentials = append(u.Credentials, cred)
}

func (u *User) UpdateCredential(cred webauthn.Credential) {
	for i, c := range u.Credentials {
		if string(c.ID) == string(cred.ID) {
			u.Credentials[i] = cred
			return
		}
	}
}

// Storage interface for user and session data
type Storage interface {
	GetUser(username string) (*User, error)
	GetUserByID(id []byte) (*User, error)
	CreateUser(username string) (*User, error)
	UpdateUser(user *User) error
	SaveSession(sessionID string, data *webauthn.SessionData) error
	GetSession(sessionID string) (*webauthn.SessionData, error)
	DeleteSession(sessionID string) error
}

type InMemoryStorage struct {
	users    map[string]*User
	userByID map[string]*User
	sessions map[string]*webauthn.SessionData
	mu       sync.RWMutex
}

func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		users:    make(map[string]*User),
		userByID: make(map[string]*User),
		sessions: make(map[string]*webauthn.SessionData),
	}
}

func (s *InMemoryStorage) GetUser(username string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, exists := s.users[username]
	if !exists {
		return nil, errors.New("user not found")
	}
	return user, nil
}

func (s *InMemoryStorage) GetUserByID(id []byte) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, exists := s.userByID[string(id)]
	if !exists {
		return nil, errors.New("user not found")
	}
	return user, nil
}

func (s *InMemoryStorage) CreateUser(username string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[username]; exists {
		return nil, errors.New("user already exists")
	}

	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}

	user := &User{
		ID:          id,
		Username:    username,
		Credentials: []webauthn.Credential{},
	}

	s.users[username] = user
	s.userByID[string(id)] = user
	return user, nil
}

func (s *InMemoryStorage) UpdateUser(user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[user.Username] = user
	s.userByID[string(user.ID)] = user
	return nil
}

func (s *InMemoryStorage) SaveSession(sessionID string, data *webauthn.SessionData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = data
	return nil
}

func (s *InMemoryStorage) GetSession(sessionID string) (*webauthn.SessionData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, errors.New("session not found")
	}
	return session, nil
}

func (s *InMemoryStorage) DeleteSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	return nil
}

// PasskeyHandler handles passkey registration and authentication
type PasskeyHandler struct {
	webAuthn *webauthn.WebAuthn
	storage  Storage
}

// Config holds configuration for passkey handler
type Config struct {
	RPDisplayName string   // "Example Corp"
	RPID          string   // "example.com"
	RPOrigins     []string // ["https://example.com"]
}

// NewPasskeyHandler creates a new passkey handler
func NewPasskeyHandler(config Config, storage Storage) (*PasskeyHandler, error) {
	wconfig := &webauthn.Config{
		RPDisplayName: config.RPDisplayName,
		RPID:          config.RPID,
		RPOrigins:     config.RPOrigins,
	}

	web, err := webauthn.New(wconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create webauthn: %w", err)
	}

	return &PasskeyHandler{
		webAuthn: web,
		storage:  storage,
	}, nil
}

// BeginRegistration starts the registration process
func (h *PasskeyHandler) BeginRegistration(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error1": err.Error()})
		return
	}

	// Try to get existing user or create new one
	user, err := h.storage.GetUser(req.Username)
	if err != nil {
		user, err = h.storage.CreateUser(req.Username)
		if err != nil {
			c.JSON(500, gin.H{"error2": err.Error()})
			return
		}
	}

	options, session, err := h.webAuthn.BeginRegistration(user)
	if err != nil {
		c.JSON(500, gin.H{"error3": err.Error()})
		return
	}

	// Generate session ID and store session
	sessionID := generateSessionID()
	if err := h.storage.SaveSession(sessionID, session); err != nil {
		c.JSON(500, gin.H{"error4": err.Error()})
		return
	}

	c.SetCookie("registration_session", sessionID, 300, "/", "", false, true)
	c.JSON(200, options)
}

// FinishRegistration completes the registration process
func (h *PasskeyHandler) FinishRegistration(c *gin.Context) {
	sessionID, err := c.Cookie("registration_session")
	if err != nil {
		c.JSON(400, gin.H{"error": "session not found"})
		return
	}

	session, err := h.storage.GetSession(sessionID)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid session"})
		return
	}

	user, err := h.storage.GetUserByID(session.UserID)
	if err != nil {
		c.JSON(400, gin.H{"error": "user not found"})
		return
	}

	credential, err := h.webAuthn.FinishRegistration(user, *session, c.Request)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	user.AddCredential(*credential)
	if err := h.storage.UpdateUser(user); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	h.storage.DeleteSession(sessionID)
	c.SetCookie("registration_session", "", -1, "/", "", false, true)

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "registration completed",
	})
}

// BeginLogin starts the authentication process
func (h *PasskeyHandler) BeginLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	user, err := h.storage.GetUser(req.Username)
	if err != nil {
		c.JSON(404, gin.H{"error": "user not found"})
		return
	}

	options, session, err := h.webAuthn.BeginLogin(user)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	sessionID := generateSessionID()
	if err := h.storage.SaveSession(sessionID, session); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.SetCookie("login_session", sessionID, 300, "/", "", false, true)
	c.JSON(200, options)
}

// FinishLogin completes the authentication process
func (h *PasskeyHandler) FinishLogin(c *gin.Context) {
	sessionID, err := c.Cookie("login_session")
	if err != nil {
		c.JSON(400, gin.H{"error": "session not found"})
		return
	}

	session, err := h.storage.GetSession(sessionID)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid session"})
		return
	}

	user, err := h.storage.GetUserByID(session.UserID)
	if err != nil {
		c.JSON(400, gin.H{"error": "user not found"})
		return
	}

	credential, err := h.webAuthn.FinishLogin(user, *session, c.Request)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Update credential sign count
	user.UpdateCredential(*credential)
	if err := h.storage.UpdateUser(user); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	h.storage.DeleteSession(sessionID)
	c.SetCookie("login_session", "", -1, "/", "", false, true)

	c.JSON(200, gin.H{
		"status":   "success",
		"message":  "login completed",
		"username": user.Username,
	})
}

// RegisterRoutes registers all passkey routes to a Gin router
func (h *PasskeyHandler) RegisterRoutes(r *gin.Engine) {
	r.POST("/passkey/register/begin", h.BeginRegistration)
	r.POST("/passkey/register/finish", h.FinishRegistration)
	r.POST("/passkey/login/begin", h.BeginLogin)
	r.POST("/passkey/login/finish", h.FinishLogin)
}

func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// ServeHTML serves the test HTML page
func (h *PasskeyHandler) ServeHTML(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(200, htmlContent)
}

const htmlContent = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Passkey Test</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 600px;
            margin: 50px auto;
            padding: 20px;
            background: #f5f5f5;
        }
        .container {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        h1 {
            color: #333;
            margin-top: 0;
        }
        .section {
            margin-bottom: 30px;
            padding-bottom: 30px;
            border-bottom: 1px solid #eee;
        }
        .section:last-child {
            border-bottom: none;
        }
        input {
            width: 100%;
            padding: 10px;
            margin: 10px 0;
            border: 1px solid #ddd;
            border-radius: 4px;
            box-sizing: border-box;
        }
        button {
            background: #007bff;
            color: white;
            padding: 12px 24px;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            font-size: 16px;
            width: 100%;
        }
        button:hover {
            background: #0056b3;
        }
        button:disabled {
            background: #ccc;
            cursor: not-allowed;
        }
        .message {
            margin-top: 15px;
            padding: 10px;
            border-radius: 4px;
        }
        .success {
            background: #d4edda;
            color: #155724;
            border: 1px solid #c3e6cb;
        }
        .error {
            background: #f8d7da;
            color: #721c24;
            border: 1px solid #f5c6cb;
        }
        .info {
            background: #d1ecf1;
            color: #0c5460;
            border: 1px solid #bee5eb;
            margin-bottom: 20px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔐 Passkey Test</h1>
        
        <div class="info message">
            <strong>Ready!</strong> Server running on <code>http://localhost:8080</code>
        </div>

        <div class="section">
            <h2>Register New Passkey</h2>
            <input type="text" id="registerUsername" placeholder="Enter username" />
            <button onclick="register()">Register with Passkey</button>
            <div id="registerMessage"></div>
        </div>

        <div class="section">
            <h2>Login with Passkey</h2>
            <input type="text" id="loginUsername" placeholder="Enter username" />
            <button onclick="login()">Login with Passkey</button>
            <div id="loginMessage"></div>
        </div>
    </div>

    <script>
        const API_BASE = window.location.origin;

        function showMessage(elementId, message, type) {
            const el = document.getElementById(elementId);
            el.innerHTML = '<div class="message ' + type + '">' + message + '</div>';
        }

        function base64urlToBuffer(base64url) {
            const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/');
            const padLen = (4 - (base64.length % 4)) % 4;
            const padded = base64 + '='.repeat(padLen);
            const binary = atob(padded);
            const bytes = new Uint8Array(binary.length);
            for (let i = 0; i < binary.length; i++) {
                bytes[i] = binary.charCodeAt(i);
            }
            return bytes.buffer;
        }

        function bufferToBase64url(buffer) {
            const bytes = new Uint8Array(buffer);
            let binary = '';
            for (let i = 0; i < bytes.length; i++) {
                binary += String.fromCharCode(bytes[i]);
            }
            return btoa(binary)
                .replace(/\+/g, '-')
                .replace(/\//g, '_')
                .replace(/=/g, '');
        }

        async function register() {
            const username = document.getElementById('registerUsername').value;
            if (!username) {
                showMessage('registerMessage', 'Please enter a username', 'error');
                return;
            }

            try {
                showMessage('registerMessage', 'Starting registration...', 'info');

                const beginResp = await fetch(API_BASE + '/passkey/register/begin', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    credentials: 'include',
                    body: JSON.stringify({ username: username })
                });

                if (!beginResp.ok) {
                    throw new Error('Server error: ' + beginResp.status);
                }

                const options = await beginResp.json();

                options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge);
                options.publicKey.user.id = base64urlToBuffer(options.publicKey.user.id);

                if (options.publicKey.excludeCredentials) {
                    options.publicKey.excludeCredentials = options.publicKey.excludeCredentials.map(function(cred) {
                        return {
                            id: base64urlToBuffer(cred.id),
                            type: cred.type,
                            transports: cred.transports
                        };
                    });
                }

                showMessage('registerMessage', '👆 Please use your biometric or security key...', 'info');

                const credential = await navigator.credentials.create(options);

                const credentialJSON = {
                    id: credential.id,
                    rawId: bufferToBase64url(credential.rawId),
                    type: credential.type,
                    response: {
                        attestationObject: bufferToBase64url(credential.response.attestationObject),
                        clientDataJSON: bufferToBase64url(credential.response.clientDataJSON)
                    }
                };

                const finishResp = await fetch(API_BASE + '/passkey/register/finish', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    credentials: 'include',
                    body: JSON.stringify(credentialJSON)
                });

                if (!finishResp.ok) {
                    const error = await finishResp.json();
                    throw new Error(error.error || 'Registration failed');
                }

                const result = await finishResp.json();
                showMessage('registerMessage', '✅ ' + result.message + '! You can now login.', 'success');

            } catch (error) {
                console.error('Registration error:', error);
                showMessage('registerMessage', '❌ Error: ' + error.message, 'error');
            }
        }

        async function login() {
            const username = document.getElementById('loginUsername').value;
            if (!username) {
                showMessage('loginMessage', 'Please enter a username', 'error');
                return;
            }

            try {
                showMessage('loginMessage', 'Starting login...', 'info');

                const beginResp = await fetch(API_BASE + '/passkey/login/begin', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    credentials: 'include',
                    body: JSON.stringify({ username: username })
                });

                if (!beginResp.ok) {
                    throw new Error('Server error: ' + beginResp.status);
                }

                const options = await beginResp.json();

                options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge);

                if (options.publicKey.allowCredentials) {
                    options.publicKey.allowCredentials = options.publicKey.allowCredentials.map(function(cred) {
                        return {
                            id: base64urlToBuffer(cred.id),
                            type: cred.type,
                            transports: cred.transports
                        };
                    });
                }

                showMessage('loginMessage', '👆 Please use your biometric or security key...', 'info');

                const credential = await navigator.credentials.get(options);

                const assertionJSON = {
                    id: credential.id,
                    rawId: bufferToBase64url(credential.rawId),
                    type: credential.type,
                    response: {
                        authenticatorData: bufferToBase64url(credential.response.authenticatorData),
                        clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
                        signature: bufferToBase64url(credential.response.signature),
                        userHandle: credential.response.userHandle ? bufferToBase64url(credential.response.userHandle) : null
                    }
                };

                const finishResp = await fetch(API_BASE + '/passkey/login/finish', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    credentials: 'include',
                    body: JSON.stringify(assertionJSON)
                });

                if (!finishResp.ok) {
                    const error = await finishResp.json();
                    throw new Error(error.error || 'Login failed');
                }

                const result = await finishResp.json();
                showMessage('loginMessage', '✅ ' + result.message + '! Welcome, ' + result.username + '!', 'success');

            } catch (error) {
                console.error('Login error:', error);
                showMessage('loginMessage', '❌ Error: ' + error.message, 'error');
            }
        }
    </script>
</body>
</html>`
