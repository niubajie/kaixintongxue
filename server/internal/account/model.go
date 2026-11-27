// Package account handles WeChat mini-program login, phone binding,
// session management, and IP registration limits.
package account

import "time"

// ---- Entities (DB rows) ----

type User struct {
	ID        int64
	UUID      string
	Nickname  string
	AvatarURL string
	Status    string // active | blocked | pending_delete
	RiskLevel string // normal | low | medium | high
	CreatedAt time.Time
}

type WeChatAccount struct {
	ID             int64
	UserID         int64
	OpenID         string
	UnionID        string
	AppType        string // mini | web
	LastSessionKey string
	CreatedAt      time.Time
}

// PrivatePhone carries encrypted phone + HMAC + DEK wrapping metadata.
// The service layer never returns the plaintext to callers; only the vault
// module (not yet in S1) can decrypt.
type PrivatePhone struct {
	ID          int64
	UserID      int64
	PhoneCipher []byte
	PhoneHMAC   string
	DEKWrapped  []byte
	KeyID       string
	AESGCMNonce []byte
	UpdatedAt   time.Time
}

type Session struct {
	ID          int64
	UserID      int64
	TokenHash   string
	RefreshHash string
	DeviceHash  string
	CreatedIP   string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}

// ---- DTOs (HTTP request / response bodies) ----

type WxLoginRequest struct {
	Code       string `json:"code" binding:"required"`
	DeviceHash string `json:"deviceHash"`
	ClientApp  string `json:"clientApp"` // "mini" | "android" | "web"
}

type WxLoginResponse struct {
	UserID       int64  `json:"userId"`
	NeedPhone    bool   `json:"needPhone"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt"`
}

type BindPhoneRequest struct {
	Code string `json:"code" binding:"required"` // wx.getPhoneNumber code
}

type BindPhoneResponse struct {
	OK bool `json:"ok"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
	DeviceHash   string `json:"deviceHash"`
}

type RefreshResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt"`
}
