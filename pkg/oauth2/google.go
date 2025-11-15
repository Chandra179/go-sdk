package oauth2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
)

// GoogleProvider implements Provider interface for Google OAuth2
type GoogleProvider struct {
	clientID     string
	clientSecret string
	redirectURL  string
	scopes       []string
}

type googleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

func NewGoogleProvider(clientID, clientSecret, redirectURL string, scopes []string) *GoogleProvider {
	if len(scopes) == 0 {
		scopes = []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		}
	}

	return &GoogleProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		scopes:       scopes,
	}
}

func (g *GoogleProvider) GetName() string {
	return "google"
}

func (g *GoogleProvider) GetAuthURL(state, codeChallenge string) string {
	params := url.Values{}
	params.Add("client_id", g.clientID)
	params.Add("redirect_uri", g.redirectURL)
	params.Add("response_type", "code")
	params.Add("scope", strings.Join(g.scopes, " "))
	params.Add("state", state)
	params.Add("code_challenge", codeChallenge)
	params.Add("code_challenge_method", "S256")
	params.Add("access_type", "offline")
	params.Add("prompt", "consent")

	return googleAuthURL + "?" + params.Encode()
}

func (g *GoogleProvider) Exchange(ctx context.Context, code string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", g.clientID)
	data.Set("client_secret", g.clientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", g.redirectURL)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, "POST", googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s", string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

func (g *GoogleProvider) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", googleUserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("user info request failed: %s", string(body))
	}

	var googleUser googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&googleUser); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	return &UserInfo{
		ID:            googleUser.ID,
		Email:         googleUser.Email,
		EmailVerified: googleUser.VerifiedEmail,
		Name:          googleUser.Name,
		Picture:       googleUser.Picture,
		Provider:      "google",
		CreatedAt:     time.Now(),
	}, nil
}
