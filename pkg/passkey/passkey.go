package passkey

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// PasskeyHandler handles passkey registration and authentication
type PasskeyHandler struct {
	webAuthn *webauthn.WebAuthn
	storage  Storage
	config   Config
}

// Config holds configuration for passkey handler
type Config struct {
	RPDisplayName         string   // "Example Corp"
	RPID                  string   // "example.com"
	RPOrigins             []string // ["https://example.com"]
	Timeout               int      // Timeout in milliseconds (default: 60000)
	RequireResidentKey    bool     // Enable discoverable credentials (username-less login)
	UserVerification      string   // "required", "preferred", "discouraged"
	AttestationPreference string   // "none", "indirect", "direct", "enterprise"
	AuthenticatorType     string   // "platform", "cross-platform", "" (both)
}

// DefaultConfig returns a secure default configuration
func DefaultConfig() Config {
	return Config{
		RPDisplayName:         "Passkey App",
		RPID:                  "localhost",
		RPOrigins:             []string{"http://localhost:8080"},
		Timeout:               60000,
		RequireResidentKey:    false,
		UserVerification:      "required",
		AttestationPreference: "direct",
		AuthenticatorType:     "", // Allow both platform and cross-platform
	}
}

// NewPasskeyHandler creates a new passkey handler
func NewPasskeyHandler(config Config, storage Storage) (*PasskeyHandler, error) {
	wconfig := &webauthn.Config{
		RPDisplayName: config.RPDisplayName,
		RPID:          config.RPID,
		RPOrigins:     config.RPOrigins,
		Timeouts: webauthn.TimeoutsConfig{
			Login: webauthn.TimeoutConfig{
				Enforce:    true,
				Timeout:    time.Duration(config.Timeout) * time.Millisecond,
				TimeoutUVD: time.Duration(config.Timeout) * time.Millisecond,
			},
			Registration: webauthn.TimeoutConfig{
				Enforce:    true,
				Timeout:    time.Duration(config.Timeout) * time.Millisecond,
				TimeoutUVD: time.Duration(config.Timeout) * time.Millisecond,
			},
		},
	}

	web, err := webauthn.New(wconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create webauthn: %w", err)
	}

	return &PasskeyHandler{
		webAuthn: web,
		storage:  storage,
		config:   config,
	}, nil
}

// BeginRegistration starts the registration process
func (h *PasskeyHandler) BeginRegistration(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Nickname string `json:"nickname"` // Optional nickname for the credential
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	// Validate username
	if len(req.Username) < 3 || len(req.Username) > 64 {
		c.JSON(400, gin.H{"error": "Username must be between 3 and 64 characters"})
		return
	}

	// Try to get existing user or create new one
	user, err := h.storage.GetUser(req.Username)
	if err != nil {
		user, err = h.storage.CreateUser(req.Username)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to create user"})
			return
		}
	}

	// Build authenticator selection criteria
	authenticatorSelection := protocol.AuthenticatorSelection{
		UserVerification: protocol.UserVerificationRequirement(h.config.UserVerification),
	}

	if h.config.RequireResidentKey {
		residentKey := protocol.ResidentKeyRequirementRequired
		authenticatorSelection.ResidentKey = residentKey
		authenticatorSelection.RequireResidentKey = protocol.ResidentKeyRequired()
	} else {
		residentKey := protocol.ResidentKeyRequirementDiscouraged
		authenticatorSelection.ResidentKey = residentKey
		authenticatorSelection.RequireResidentKey = protocol.ResidentKeyNotRequired()
	}

	if h.config.AuthenticatorType != "" {
		attachment := protocol.AuthenticatorAttachment(h.config.AuthenticatorType)
		authenticatorSelection.AuthenticatorAttachment = attachment
	}

	// Convert credentials to descriptors for exclusion list
	exclusionList := make([]protocol.CredentialDescriptor, len(user.Credentials))
	for i, cred := range user.Credentials {
		exclusionList[i] = cred.Credential.Descriptor()
	}

	options, session, err := h.webAuthn.BeginRegistration(
		user,
		webauthn.WithAuthenticatorSelection(authenticatorSelection),
		webauthn.WithConveyancePreference(protocol.ConveyancePreference(h.config.AttestationPreference)),
		webauthn.WithExclusions(exclusionList),
	)

	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to begin registration"})
		return
	}

	sessionID := generateSessionID()
	if err := h.storage.SaveSession(sessionID, session); err != nil {
		c.JSON(500, gin.H{"error": "Failed to save session"})
		return
	}

	c.SetCookie("registration_session", sessionID, 300, "/", "", false, true)
	if req.Nickname != "" {
		c.SetCookie("credential_nickname", req.Nickname, 300, "/", "", false, true)
	}

	c.JSON(200, options)
}

// FinishRegistration completes the registration process
func (h *PasskeyHandler) FinishRegistration(c *gin.Context) {
	sessionID, err := c.Cookie("registration_session")
	if err != nil {
		c.JSON(400, gin.H{"error": "Session not found"})
		return
	}

	session, err := h.storage.GetSession(sessionID)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid or expired session"})
		return
	}

	user, err := h.storage.GetUserByID(session.UserID)
	if err != nil {
		c.JSON(400, gin.H{"error": "User not found"})
		return
	}

	credential, err := h.webAuthn.FinishRegistration(user, *session, c.Request)
	if err != nil {
		c.JSON(400, gin.H{"error": "Registration verification failed"})
		return
	}

	// Get nickname from cookie if available
	nickname, _ := c.Cookie("credential_nickname")
	if nickname == "" {
		nickname = fmt.Sprintf("Passkey %d", len(user.Credentials)+1)
	}

	// Extract transports if available
	var transports []string
	if credential.Transport != nil {
		transports = make([]string, len(credential.Transport))
		for i, t := range credential.Transport {
			transports[i] = string(t)
		}
	}

	user.AddCredential(*credential, nickname, transports)
	if err := h.storage.UpdateUser(user); err != nil {
		c.JSON(500, gin.H{"error": "Failed to save credential"})
		return
	}

	if err := h.storage.DeleteSession(sessionID); err != nil {
		log.Printf("Warning: failed to delete registration session: %v", err)
	}
	c.SetCookie("registration_session", "", -1, "/", "", false, true)
	c.SetCookie("credential_nickname", "", -1, "/", "", false, true)

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Registration completed",
		"credential": gin.H{
			"id":       base64.RawURLEncoding.EncodeToString(credential.ID),
			"nickname": nickname,
		},
	})
}

// BeginLogin starts the authentication process
func (h *PasskeyHandler) BeginLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
	}

	// Username is optional for discoverable credentials
	if err := c.ShouldBindJSON(&req); err != nil {
		// For discoverable credentials, username is optional, so we ignore binding errors
		// The req.Username will remain empty which is valid for username-less login
		req.Username = ""
	}

	var options *protocol.CredentialAssertion
	var session *webauthn.SessionData
	var err error

	if req.Username != "" {
		user, err := h.storage.GetUser(req.Username)
		if err != nil {
			c.JSON(404, gin.H{"error": "Authentication failed"})
			return
		}

		options, session, err = h.webAuthn.BeginLogin(user)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to begin login"})
			return
		}
	} else {
		options, session, err = h.webAuthn.BeginDiscoverableLogin()
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to begin login"})
			return
		}
	}

	sessionID := generateSessionID()
	if err := h.storage.SaveSession(sessionID, session); err != nil {
		c.JSON(500, gin.H{"error": "Failed to save session"})
		return
	}

	c.SetCookie("login_session", sessionID, 300, "/", "", false, true)
	c.JSON(200, options)
}

// FinishLogin completes the authentication process
func (h *PasskeyHandler) FinishLogin(c *gin.Context) {
	sessionID, err := c.Cookie("login_session")
	if err != nil {
		c.JSON(400, gin.H{"error": "Session not found"})
		return
	}

	session, err := h.storage.GetSession(sessionID)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid or expired session"})
		return
	}

	user, err := h.storage.GetUserByID(session.UserID)
	if err != nil {
		c.JSON(400, gin.H{"error": "Authentication failed"})
		return
	}

	credential, err := h.webAuthn.FinishLogin(user, *session, c.Request)
	if err != nil {
		c.JSON(400, gin.H{"error": "Authentication verification failed"})
		return
	}

	// Check for credential cloning (counter should always increment)
	existingCred := user.GetCredential(credential.ID)
	if existingCred != nil && credential.Authenticator.SignCount > 0 {
		if credential.Authenticator.SignCount <= existingCred.Authenticator.SignCount {
			c.JSON(400, gin.H{"error": "Credential cloning detected"})
			return
		}
	}

	user.UpdateCredential(*credential)
	if err := h.storage.UpdateUser(user); err != nil {
		c.JSON(500, gin.H{"error": "Failed to update credential"})
		return
	}

	if err := h.storage.DeleteSession(sessionID); err != nil {
		log.Printf("Warning: failed to delete login session: %v", err)
	}
	c.SetCookie("login_session", "", -1, "/", "", false, true)

	c.JSON(200, gin.H{
		"status":   "success",
		"message":  "Login completed",
		"username": user.Username,
		"credential": gin.H{
			"id":       base64.RawURLEncoding.EncodeToString(credential.ID),
			"nickname": existingCred.Nickname,
		},
	})
}

// ListCredentials returns all credentials for a user
func (h *PasskeyHandler) ListCredentials(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		c.JSON(400, gin.H{"error": "Username required"})
		return
	}

	user, err := h.storage.GetUser(username)
	if err != nil {
		c.JSON(404, gin.H{"error": "User not found"})
		return
	}

	credentials := make([]gin.H, len(user.Credentials))
	for i, cred := range user.Credentials {
		credentials[i] = gin.H{
			"id":           base64.RawURLEncoding.EncodeToString(cred.ID),
			"nickname":     cred.Nickname,
			"created_at":   cred.CreatedAt,
			"last_used_at": cred.LastUsedAt,
			"transports":   cred.Transports,
			"backup_state": cred.BackupState,
		}
	}

	c.JSON(200, gin.H{
		"username":    user.Username,
		"credentials": credentials,
	})
}

// DeleteCredential removes a credential
func (h *PasskeyHandler) DeleteCredential(c *gin.Context) {
	username := c.Param("username")
	credentialIDStr := c.Param("credentialId")

	if username == "" || credentialIDStr == "" {
		c.JSON(400, gin.H{"error": "Username and credential ID required"})
		return
	}

	credentialID, err := base64.RawURLEncoding.DecodeString(credentialIDStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid credential ID"})
		return
	}

	user, err := h.storage.GetUser(username)
	if err != nil {
		c.JSON(404, gin.H{"error": "User not found"})
		return
	}

	if !user.DeleteCredential(credentialID) {
		c.JSON(404, gin.H{"error": "Credential not found"})
		return
	}

	if err := h.storage.UpdateUser(user); err != nil {
		c.JSON(500, gin.H{"error": "Failed to delete credential"})
		return
	}

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Credential deleted",
	})
}

// RenameCredential updates a credential's nickname
func (h *PasskeyHandler) RenameCredential(c *gin.Context) {
	username := c.Param("username")
	credentialIDStr := c.Param("credentialId")

	var req struct {
		Nickname string `json:"nickname" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Nickname required"})
		return
	}

	credentialID, err := base64.RawURLEncoding.DecodeString(credentialIDStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid credential ID"})
		return
	}

	user, err := h.storage.GetUser(username)
	if err != nil {
		c.JSON(404, gin.H{"error": "User not found"})
		return
	}

	if !user.RenameCredential(credentialID, req.Nickname) {
		c.JSON(404, gin.H{"error": "Credential not found"})
		return
	}

	if err := h.storage.UpdateUser(user); err != nil {
		c.JSON(500, gin.H{"error": "Failed to rename credential"})
		return
	}

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Credential renamed",
	})
}

// RegisterRoutes registers all passkey routes to a Gin router
func (h *PasskeyHandler) RegisterRoutes(r *gin.Engine) {
	r.POST("/passkey/register/begin", h.BeginRegistration)
	r.POST("/passkey/register/finish", h.FinishRegistration)
	r.POST("/passkey/login/begin", h.BeginLogin)
	r.POST("/passkey/login/finish", h.FinishLogin)
	r.GET("/passkey/credentials/:username", h.ListCredentials)
	r.DELETE("/passkey/credentials/:username/:credentialId", h.DeleteCredential)
	r.PUT("/passkey/credentials/:username/:credentialId", h.RenameCredential)
}

func generateSessionID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("failed to generate random session ID: %v", err))
	}
	return base64.URLEncoding.EncodeToString(b)
}
