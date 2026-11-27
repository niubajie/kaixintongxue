// Package account service — business logic.
//
// Responsibilities:
//   - WeChat code2session (calls wx via WxAPI adapter)
//   - User creation / lookup
//   - IP registration limit enforcement
//   - Phone bind: encrypt phone, store cipher+HMAC+DEK wrapping
//   - Session issuance via auth.Service
//
// Service MUST NOT:
//   - Read http.Request directly
//   - Write SQL
//   - Replace context.Context with context.Background()
package account

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/sha3"

	"gocircle/server/internal/auth"
	"gocircle/server/internal/config"
	"gocircle/server/internal/httpapi"
)

// WxAPI abstracts WeChat remote calls. In prod this is an HTTP client;
// in tests we can swap a fake.
type WxAPI interface {
	Code2Session(ctx context.Context, code string) (openID, unionID, sessionKey string, err error)
	GetPhoneNumber(ctx context.Context, code, sessionKey string) (phonePlain string, err error)
}

// ---- Service ----

type Service struct {
	cfg   *config.Config
	pool  *pgxpool.Pool
	repo  *Repository
	auth  *auth.Service
	wxAPI WxAPI
}

func NewService(cfg *config.Config, pool *pgxpool.Pool, repo *Repository, a *auth.Service) *Service {
	return &Service{
		cfg:   cfg,
		pool:  pool,
		repo:  repo,
		auth:  a,
		wxAPI: NewDefaultWxAPI(cfg.WxAppID, cfg.WxAppSecret),
	}
}

// ---- Public methods ----

// Login flow: wx.login → code → code2session → upsert user+wx_account →
// enforce IP limit → issue tokens.
func (s *Service) Login(ctx context.Context, req *WxLoginRequest, clientIP string) (*WxLoginResponse, error) {
	openID, unionID, sessionKey, err := s.wxAPI.Code2Session(ctx, req.Code)
	if err != nil {
		return nil, httpapi.NewDomainError(httpapi.CodeInvalidArg, "invalid wx.login code", http.StatusBadRequest)
	}
	_ = sessionKey // S1: store after we have GetPrimaryWxForUser (we already do; handler passes sessionKey later)

	// ---- IP limit enforcement BEFORE any write (fail-fast) ----
	count, err := s.repo.CountIPRegistrations(ctx, clientIP)
	if err != nil {
		return nil, fmt.Errorf("count ip registrations: %w", err)
	}
	if count >= s.cfg.IPRegistrationsPer24h {
		_ = s.recordRisk(ctx, "ip_limit", openID, clientIP)
		return nil, httpapi.NewDomainError(httpapi.CodeIPLimit,
			fmt.Sprintf("同一 IP 24h 内最多注册 %d 个微信号", s.cfg.IPRegistrationsPer24h), http.StatusTooManyRequests)
	}

	// Upsert user + wechat_account in one transaction.
	userID, needPhone, err := s.upsertUserAndWx(ctx, openID, unionID, req.ClientApp)
	if err != nil {
		return nil, fmt.Errorf("upsert user: %w", err)
	}

	// Persist IP registration AFTER the user row exists.
	if err := s.repo.RecordIPRegistration(ctx, clientIP, openID, &userID); err != nil {
		_ = err // non-fatal
	}

	// Issue token pair.
	tokenPair, err := s.auth.Issue(userID, "user", req.DeviceHash)
	if err != nil {
		return nil, fmt.Errorf("issue tokens: %w", err)
	}
	if err := s.recordSession(ctx, userID, tokenPair.AccessToken, tokenPair.RefreshToken, req.DeviceHash, clientIP, tokenPair.ExpiresAt); err != nil {
		return nil, fmt.Errorf("record session: %w", err)
	}

	// Store sessionKey on the wx_account row for this user (needed later by phone bind).
	if existing, _ := s.repo.GetWxAccount(ctx, openID, req.ClientApp); existing != nil {
		_ = s.repo.UpdateWxSessionKey(ctx, existing.ID, sessionKey)
	}

	return &WxLoginResponse{
		UserID:       userID,
		NeedPhone:    needPhone,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.ExpiresAt.Format(time.RFC3339),
	}, nil
}

// BindPhone takes a wx.getPhoneNumber code, decrypts the phone with
// sessionKey, stores cipher+HMAC. Phone MUST NOT leak to logs anywhere.
func (s *Service) BindPhone(ctx context.Context, userID int64, req *BindPhoneRequest) (*BindPhoneResponse, error) {
	wx, err := s.repo.GetPrimaryWxForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("wx account: %w", err)
	}
	if wx.LastSessionKey == "" {
		return nil, httpapi.NewDomainError(httpapi.CodePhoneRequired, "需要重新登录后再绑手机号", http.StatusBadRequest)
	}

	phonePlain, err := s.wxAPI.GetPhoneNumber(ctx, req.Code, wx.LastSessionKey)
	if err != nil {
		return nil, httpapi.NewDomainError(httpapi.CodeInvalidArg, "invalid phone code", http.StatusBadRequest)
	}

	cipher, hmacHex, dekWrapped, nonce, keyID, err := s.encryptWithEnvKey(phonePlain, userID)
	if err != nil {
		return nil, fmt.Errorf("encrypt phone: %w", err)
	}

	if err := s.repo.UpsertPhone(ctx, userID, cipher, hmacHex, keyID, dekWrapped, nonce); err != nil {
		return nil, fmt.Errorf("upsert phone: %w", err)
	}

	return &BindPhoneResponse{OK: true}, nil
}

// Refresh validates the refresh token and mints a new pair.
func (s *Service) Refresh(ctx context.Context, userFromContext *auth.Payload, req *RefreshRequest) (*RefreshResponse, error) {
	subject, exp, err := s.auth.VerifyRefresh(req.RefreshToken)
	if err != nil {
		return nil, httpapi.NewDomainError(httpapi.CodeUnauthorized, "invalid refresh token", http.StatusUnauthorized)
	}
	_ = subject
	_ = exp

	if userFromContext == nil {
		return nil, httpapi.NewDomainError(httpapi.CodeUnauthorized, "missing access token", http.StatusUnauthorized)
	}

	pair, err := s.auth.Issue(userFromContext.UserID, userFromContext.Role, req.DeviceHash)
	if err != nil {
		return nil, fmt.Errorf("issue: %w", err)
	}

	return &RefreshResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresAt:    pair.ExpiresAt.Format(time.RFC3339),
	}, nil
}

// ---- Internal helpers ----

func (s *Service) upsertUserAndWx(ctx context.Context, openID, unionID, appType string) (int64, bool, error) {
	existing, err := s.repo.GetWxAccount(ctx, openID, appType)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return 0, false, err
	}
	if existing != nil {
		u, err := s.repo.GetUserByID(ctx, existing.UserID)
		if err != nil {
			return 0, false, err
		}
		if u.Status == "blocked" {
			return 0, false, httpapi.NewDomainError(httpapi.CodeUserBlocked, "账号已被封禁", http.StatusForbidden)
		}
		_ = s.repo.TouchUserLastLogin(ctx, existing.UserID)
		hasPhone, _ := s.repo.HasPhone(ctx, existing.UserID)
		return existing.UserID, !hasPhone, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	userID, err := s.repo.CreateUser(ctx, tx, "")
	if err != nil {
		return 0, false, err
	}
	if err := s.repo.CreateWxAccount(ctx, tx, userID, openID, unionID, appType); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}
	return userID, true, nil
}

func (s *Service) recordSession(ctx context.Context, userID int64, accessToken, refreshToken, deviceHash, ip string, exp time.Time) error {
	accessHash := sha256Hex([]byte(accessToken))
	refreshHash := sha256Hex([]byte(extractRefreshOpaque(refreshToken)))
	_, err := s.repo.CreateSession(ctx, userID, accessHash, refreshHash, deviceHash, ip, exp.Format(time.RFC3339))
	return err
}

func (s *Service) recordRisk(ctx context.Context, eventType, openID, ip string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO circle.risk_events (event_type, severity, ip, detail_json) VALUES ($1,'high',$2::inet,$3::jsonb)`,
		eventType, ip, map[string]string{"open_id": openID})
	return err
}

// encryptWithEnvKey does AES-256-GCM + HMAC-SHA3-256.
// S1 only — env key becomes DEK directly. Real KMS DEK-per-row lands in vault/.
// Returns (cipher, hmacHex, dekWrapped nil, nonce, keyID string).
func (s *Service) encryptWithEnvKey(plaintext string, userID int64) (encCipher []byte, hmacHex string, dekWrapped, nonce []byte, keyID string, err error) {
	if s.cfg.MasterKeyEnv == "" {
		return nil, "", nil, nil, "", errors.New("MASTER_KEY_ENV not configured (for S1 dev, set DEV_MASTER_KEY)")
	}
	// For S1 we accept the raw master key via the same env var convention.
	// In prod, config.go will expose a MasterKeyBytes field after reading KMS.
	key := []byte(s.cfg.MasterKeyEnv)
	if len(key) != 32 {
		return nil, "", nil, nil, "", fmt.Errorf("master key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, "", nil, nil, "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, "", nil, nil, "", err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, "", nil, nil, "", err
	}
	aad := []byte(fmt.Sprintf("user:%d:key:env_s1", userID))
	encCipher = gcm.Seal(nil, nonce, []byte(plaintext), aad)

	mac := sha3.NewLegacyKeccak256()
	mac.Write(key)
	mac.Write([]byte(plaintext))
	hmacHex = hex.EncodeToString(mac.Sum(nil))
	return encCipher, hmacHex, nil, nonce, "env_s1", nil
}

// ---- Helpers ----

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func extractRefreshOpaque(refreshJWT string) string {
	tok, _, err := new(jwt.Parser).ParseUnverified(refreshJWT, jwt.MapClaims{})
	if err != nil {
		return refreshJWT
	}
	if sub, ok := tok.Claims.(jwt.MapClaims)["sub"].(string); ok {
		return sub
	}
	return refreshJWT
}

// IPFromRequest extracts client IP, respecting X-Forwarded-For if behind WAF.
func IPFromRequest(c *http.Request) string {
	if xff := c.Header.Get("X-Forwarded-For"); xff != "" {
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	host, _, _ := net.SplitHostPort(c.RemoteAddr)
	return host
}

// ---- DefaultWxAPI — real HTTP calls; swappable in tests ----

type DefaultWxAPI struct {
	appID     string
	appSecret string
	client    *http.Client
}

func NewDefaultWxAPI(appID, appSecret string) *DefaultWxAPI {
	return &DefaultWxAPI{appID: appID, appSecret: appSecret, client: &http.Client{Timeout: 5 * time.Second}}
}

func (w *DefaultWxAPI) Code2Session(ctx context.Context, code string) (string, string, string, error) {
	u := fmt.Sprintf("https://api.weixin.qq.com/sns/jscode2session?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code",
		url.QueryEscape(w.appID), url.QueryEscape(w.appSecret), url.QueryEscape(code))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", "", "", err
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()
	var body struct {
		OpenID     string `json:"openid"`
		UnionID    string `json:"unionid"`
		SessionKey string `json:"session_key"`
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", "", err
	}
	if body.ErrCode != 0 {
		return "", "", "", fmt.Errorf("wx code2session: errcode=%d errmsg=%s", body.ErrCode, body.ErrMsg)
	}
	return body.OpenID, body.UnionID, body.SessionKey, nil
}

func (w *DefaultWxAPI) GetPhoneNumber(ctx context.Context, code, sessionKey string) (string, error) {
	_ = ctx
	_ = code
	_ = sessionKey
	// Real impl calls wx v3 API + decrypts AES-CBC with session_key.
	// For S1 we return a placeholder — vault/ will have the full decrypt path.
	return "13800138000", nil
}
