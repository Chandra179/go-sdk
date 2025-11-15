package oauth2

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
)

// JWTClaims represents JWT claims
type JWTClaims struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Picture  string `json:"picture"`
	Provider string `json:"provider"`
	Iat      int64  `json:"iat"`
	Exp      int64  `json:"exp"`
}

// GenerateJWT generates a JWT token for user
func GenerateJWT(userInfo *UserInfo, secret string, expiresIn time.Duration) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		Sub:      userInfo.ID,
		Email:    userInfo.Email,
		Name:     userInfo.Name,
		Picture:  userInfo.Picture,
		Provider: userInfo.Provider,
		Iat:      now.Unix(),
		Exp:      now.Add(expiresIn).Unix(),
	}

	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	headerEncoded := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsEncoded := base64.RawURLEncoding.EncodeToString(claimsJSON)

	message := headerEncoded + "." + claimsEncoded
	signature := generateSignature(message, secret)

	return message + "." + signature, nil
}

// ValidateJWT validates and parses a JWT token
func ValidateJWT(token, secret string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	message := parts[0] + "." + parts[1]
	signature := parts[2]

	expectedSignature := generateSignature(message, secret)
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return nil, ErrInvalidToken
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims JWTClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	if time.Now().Unix() > claims.Exp {
		return nil, ErrTokenExpired
	}

	return &claims, nil
}

func generateSignature(message, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
