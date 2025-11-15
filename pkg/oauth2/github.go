package oauth2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	githubAuthURL      = "https://github.com/login/oauth/authorize"
	githubTokenURL     = "https://github.com/login/oauth/access_token"
	githubUserURL      = "https://api.github.com/user"
	githubUserEmailURL = "https://api.github.com/user/emails"
)

// GitHubProvider implements Provider interface for GitHub OAuth2
type GitHubProvider struct {
	clientID     string
	clientSecret string
	redirectURL  string
	scopes       []string
}

type githubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type githubEmail struct {
	Email      string `json:"email"`
	Primary    bool   `json:"primary"`
	Verified   bool   `json:"verified"`
	Visibility string `json:"visibility"`
}

func NewGitHubProvider(clientID, clientSecret, redirectURL string, scopes []string) *GitHubProvider {
	if len(scopes) == 0 {
		scopes = []string{"user:email"}
	}

	return &GitHubProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		scopes:       scopes,
	}
}

func (gh *GitHubProvider) GetName() string {
	return "github"
}

func (gh *GitHubProvider) GetAuthURL(state, codeChallenge string) string {
	params := url.Values{}
	params.Add("client_id", gh.clientID)
	params.Add("redirect_uri", gh.redirectURL)
	params.Add("scope", strings.Join(gh.scopes, " "))
	params.Add("state", state)

	return githubAuthURL + "?" + params.Encode()
}

func (gh *GitHubProvider) Exchange(ctx context.Context, code string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", gh.clientID)
	data.Set("client_secret", gh.clientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", gh.redirectURL)

	req, err := http.NewRequestWithContext(ctx, "POST", githubTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

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

func (gh *GitHubProvider) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", githubUserURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("user info request failed: %s", string(body))
	}

	var ghUser githubUser
	if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	email := ghUser.Email
	emailVerified := false

	// If email is not public, fetch from emails endpoint
	if email == "" {
		email, emailVerified, err = gh.getPrimaryEmail(ctx, accessToken)
		if err != nil {
			return nil, fmt.Errorf("failed to get email: %w", err)
		}
	}

	name := ghUser.Name
	if name == "" {
		name = ghUser.Login
	}

	return &UserInfo{
		ID:            fmt.Sprintf("%d", ghUser.ID),
		Email:         email,
		EmailVerified: emailVerified,
		Name:          name,
		Picture:       ghUser.AvatarURL,
		Provider:      "github",
		CreatedAt:     time.Now(),
	}, nil
}

func (gh *GitHubProvider) getPrimaryEmail(ctx context.Context, accessToken string) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", githubUserEmailURL, nil)
	if err != nil {
		return "", false, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false, errors.New("failed to get emails")
	}

	var emails []githubEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", false, err
	}

	for _, e := range emails {
		if e.Primary {
			return e.Email, e.Verified, nil
		}
	}

	if len(emails) > 0 {
		return emails[0].Email, emails[0].Verified, nil
	}

	return "", false, errors.New("no email found")
}
