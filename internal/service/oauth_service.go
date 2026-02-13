package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/pkg/jwt"
	pkglogger "github.com/damoang/angple-backend/pkg/logger"
	"gorm.io/gorm"
)

// OAuthService handles OAuth2 social login flows
type OAuthService struct {
	db         *gorm.DB
	jwtManager *jwt.Manager
	providers  map[domain.OAuthProvider]*domain.OAuthConfig
}

// NewOAuthService creates a new OAuthService
func NewOAuthService(db *gorm.DB, jwtManager *jwt.Manager) *OAuthService {
	if db != nil {
		if err := db.AutoMigrate(&domain.OAuthAccount{}); err != nil {
			pkglogger.GetLogger().Warn().Err(err).Msg("failed to auto-migrate OAuthAccount")
		}
	}
	return &OAuthService{
		db:         db,
		jwtManager: jwtManager,
		providers:  make(map[domain.OAuthProvider]*domain.OAuthConfig),
	}
}

// RegisterProvider registers an OAuth provider configuration
func (s *OAuthService) RegisterProvider(provider domain.OAuthProvider, cfg *domain.OAuthConfig) {
	s.providers[provider] = cfg
}

// GetAuthURL returns the OAuth authorization URL for the given provider
func (s *OAuthService) GetAuthURL(provider domain.OAuthProvider, state string) (string, error) {
	cfg, ok := s.providers[provider]
	if !ok {
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}

	switch provider {
	case domain.OAuthProviderNaver:
		params := url.Values{
			"response_type": {"code"},
			"client_id":     {cfg.ClientID},
			"redirect_uri":  {cfg.RedirectURL},
			"state":         {state},
		}
		return "https://nid.naver.com/oauth2.0/authorize?" + params.Encode(), nil

	case domain.OAuthProviderKakao:
		params := url.Values{
			"response_type": {"code"},
			"client_id":     {cfg.ClientID},
			"redirect_uri":  {cfg.RedirectURL},
			"state":         {state},
		}
		return "https://kauth.kakao.com/oauth/authorize?" + params.Encode(), nil

	case domain.OAuthProviderGoogle:
		params := url.Values{
			"response_type": {"code"},
			"client_id":     {cfg.ClientID},
			"redirect_uri":  {cfg.RedirectURL},
			"scope":         {strings.Join(cfg.Scopes, " ")},
			"state":         {state},
			"access_type":   {"offline"},
		}
		return "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode(), nil

	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}
}

// HandleCallback exchanges the authorization code for tokens and user info
func (s *OAuthService) HandleCallback(ctx context.Context, provider domain.OAuthProvider, code string) (*domain.OAuthLoginResponse, error) {
	cfg, ok := s.providers[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	// Exchange code for access token
	tokenResp, err := s.exchangeCode(provider, cfg, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	// Get user info from provider
	accessTokenVal, ok := tokenResp["access_token"].(string)
	if !ok {
		return nil, fmt.Errorf("access_token not found or not a string in token response")
	}
	userInfo, err := s.getUserInfo(provider, accessTokenVal)
	if err != nil {
		return nil, fmt.Errorf("get user info failed: %w", err)
	}

	// Find or create OAuth account
	var oauthAccount domain.OAuthAccount
	result := s.db.WithContext(ctx).Where("provider = ? AND provider_uid = ?", provider, userInfo.ProviderUID).First(&oauthAccount)

	isNewUser := false
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// New OAuth user — create account
		isNewUser = true
		oauthAccount = domain.OAuthAccount{
			UserID:       fmt.Sprintf("oauth_%s_%s", provider, userInfo.ProviderUID),
			Provider:     provider,
			ProviderUID:  userInfo.ProviderUID,
			Email:        userInfo.Email,
			Name:         userInfo.Name,
			ProfileImage: userInfo.ProfileImage,
			AccessToken:  accessTokenVal,
		}
		if rt, ok := tokenResp["refresh_token"].(string); ok {
			oauthAccount.RefreshToken = rt
		}
		if err := s.db.WithContext(ctx).Create(&oauthAccount).Error; err != nil {
			return nil, fmt.Errorf("create oauth account failed: %w", err)
		}
	} else if result.Error != nil {
		return nil, result.Error
	} else {
		// Update tokens
		updates := map[string]interface{}{
			"access_token": tokenResp["access_token"],
			"name":         userInfo.Name,
			"email":        userInfo.Email,
		}
		if rt, ok := tokenResp["refresh_token"].(string); ok && rt != "" {
			updates["refresh_token"] = rt
		}
		if err := s.db.WithContext(ctx).Model(&oauthAccount).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("update oauth account failed: %w", err)
		}
	}

	// Generate JWT tokens
	accessToken, err := s.jwtManager.GenerateAccessToken(oauthAccount.UserID, oauthAccount.UserID, userInfo.Name, 1)
	if err != nil {
		return nil, fmt.Errorf("generate access token failed: %w", err)
	}
	refreshToken, err := s.jwtManager.GenerateRefreshToken(oauthAccount.UserID)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token failed: %w", err)
	}

	return &domain.OAuthLoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		IsNewUser:    isNewUser,
		UserID:       oauthAccount.UserID,
	}, nil
}

// exchangeCode exchanges authorization code for access token
func (s *OAuthService) exchangeCode(provider domain.OAuthProvider, cfg *domain.OAuthConfig, code string) (map[string]interface{}, error) {
	var tokenURL string
	params := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {cfg.RedirectURL},
	}

	switch provider {
	case domain.OAuthProviderNaver:
		tokenURL = "https://nid.naver.com/oauth2.0/token" //nolint:gosec // credential variable name, not actual credentials
		params.Set("client_id", cfg.ClientID)
		params.Set("client_secret", cfg.ClientSecret) //nolint:gosec // credential variable name, not actual credentials

	case domain.OAuthProviderKakao:
		tokenURL = "https://kauth.kakao.com/oauth/token" //nolint:gosec // credential variable name, not actual credentials
		params.Set("client_id", cfg.ClientID)
		params.Set("client_secret", cfg.ClientSecret) //nolint:gosec // credential variable name, not actual credentials

	case domain.OAuthProviderGoogle:
		tokenURL = "https://oauth2.googleapis.com/token" //nolint:gosec // credential variable name, not actual credentials
		params.Set("client_id", cfg.ClientID)
		params.Set("client_secret", cfg.ClientSecret) //nolint:gosec // credential variable name, not actual credentials

	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, tokenURL, strings.NewReader(params.Encode())) //nolint:gosec // URL is from OAuth provider config, not user input
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response body failed: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse token response failed: %w", err)
	}

	if errMsg, ok := result["error"]; ok {
		return nil, fmt.Errorf("oauth error: %v", errMsg)
	}

	return result, nil
}

// getUserInfo fetches user profile from the OAuth provider
func (s *OAuthService) getUserInfo(provider domain.OAuthProvider, accessToken string) (*domain.OAuthUserInfo, error) {
	var apiURL string

	switch provider {
	case domain.OAuthProviderNaver:
		apiURL = "https://openapi.naver.com/v1/nid/me"
	case domain.OAuthProviderKakao:
		apiURL = "https://kapi.kakao.com/v2/user/me"
	case domain.OAuthProviderGoogle:
		apiURL = "https://www.googleapis.com/oauth2/v2/userinfo"
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, apiURL, nil) //nolint:gosec // URL is from OAuth provider config, not user input
	if err != nil {
		return nil, fmt.Errorf("create user info request failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read user info response body failed: %w", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	return s.parseUserInfo(provider, raw)
}

// parseUserInfo parses provider-specific user info into a common struct
func (s *OAuthService) parseUserInfo(provider domain.OAuthProvider, raw map[string]interface{}) (*domain.OAuthUserInfo, error) {
	info := &domain.OAuthUserInfo{Provider: provider}

	switch provider {
	case domain.OAuthProviderNaver:
		response, ok := raw["response"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid naver response")
		}
		info.ProviderUID = fmt.Sprintf("%v", response["id"])
		info.Email, _ = response["email"].(string)                //nolint:errcheck // type assertion, not error
		info.Name, _ = response["name"].(string)                  //nolint:errcheck // type assertion, not error
		info.Nickname, _ = response["nickname"].(string)          //nolint:errcheck // type assertion, not error
		info.ProfileImage, _ = response["profile_image"].(string) //nolint:errcheck // type assertion, not error

	case domain.OAuthProviderKakao:
		info.ProviderUID = fmt.Sprintf("%v", raw["id"])
		if account, ok := raw["kakao_account"].(map[string]interface{}); ok {
			info.Email, _ = account["email"].(string) //nolint:errcheck // type assertion, not error
			if profile, ok := account["profile"].(map[string]interface{}); ok {
				info.Nickname, _ = profile["nickname"].(string)              //nolint:errcheck // type assertion, not error
				info.ProfileImage, _ = profile["profile_image_url"].(string) //nolint:errcheck // type assertion, not error
			}
		}
		info.Name = info.Nickname

	case domain.OAuthProviderGoogle:
		info.ProviderUID = fmt.Sprintf("%v", raw["id"])
		info.Email, _ = raw["email"].(string)          //nolint:errcheck // type assertion, not error
		info.Name, _ = raw["name"].(string)            //nolint:errcheck // type assertion, not error
		info.ProfileImage, _ = raw["picture"].(string) //nolint:errcheck // type assertion, not error
	}

	if info.ProviderUID == "" {
		return nil, fmt.Errorf("could not extract provider UID")
	}

	return info, nil
}

// --- API Key Management ---

// GenerateAPIKey creates a new API key for a user
func (s *OAuthService) GenerateAPIKey(ctx context.Context, userID, name, scopes string) (*domain.APIKey, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	if err := s.db.AutoMigrate(&domain.APIKey{}); err != nil {
		pkglogger.GetLogger().Warn().Err(err).Msg("failed to auto-migrate APIKey")
	}

	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, err
	}
	key := "ak_" + hex.EncodeToString(keyBytes)

	apiKey := &domain.APIKey{
		Key:    key,
		Name:   name,
		UserID: userID,
		Scopes: scopes,
		Active: true,
	}

	if err := s.db.WithContext(ctx).Create(apiKey).Error; err != nil {
		return nil, err
	}

	pkglogger.GetLogger().Info().
		Str("user_id", userID).
		Str("key_name", name).
		Msg("API key generated")

	return apiKey, nil
}

// ValidateAPIKey checks if a key is valid and returns the associated record
func (s *OAuthService) ValidateAPIKey(ctx context.Context, key string) (*domain.APIKey, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	var apiKey domain.APIKey
	err := s.db.WithContext(ctx).Where("api_key = ? AND active = ?", key, true).First(&apiKey).Error
	if err != nil {
		return nil, fmt.Errorf("invalid API key")
	}

	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("API key expired")
	}

	// Update last used
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&apiKey).Update("last_used_at", now).Error; err != nil {
		return nil, fmt.Errorf("update last used: %w", err)
	}

	return &apiKey, nil
}
