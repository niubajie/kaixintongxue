// Package auth implements JWT access/refresh token issuance, verification,
// and gin middleware. Tokens are short-lived (access 2h, refresh 30d) and
// bind to a device hash to reduce the blast radius of theft.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"gocircle/server/internal/config"
)

// Role constants — shared by user and admin tokens.
const (
	RoleUser          = "user"
	RoleSuperAdmin    = "super_admin"
	RoleAdminOperator = "admin_operator"
	RoleReviewer      = "reviewer"
	RoleFinance       = "finance"
	RoleAuditor       = "auditor"
)

// Payload is what we put inside both access and refresh tokens.
type Payload struct {
	UserID     int64  `json:"uid"`
	Role       string `json:"role,omitempty"`
	DeviceHash string `json:"dev,omitempty"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time // access expiry
}

type Service struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

// Issue produces a fresh access/refresh pair.
func (s *Service) Issue(userID int64, role, deviceHash string) (*TokenPair, error) {
	now := time.Now()

	access := jwt.NewWithClaims(jwt.SigningMethodHS256, Payload{
		UserID:     userID,
		Role:       role,
		DeviceHash: deviceHash,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.AccessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "gocircle",
		},
	})
	accessStr, err := access.SignedString([]byte(s.cfg.AccessTokenSecret))
	if err != nil {
		return nil, err
	}

	// Refresh is just a random opaque token stored server-side (Redis),
	// not a JWT — easier to revoke. For S1 we still embed it in JWT claims
	// but rotate the signature secrets on revoke.
	refreshRaw := make([]byte, 32)
	if _, err := rand.Read(refreshRaw); err != nil {
		return nil, err
	}
	refresh := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   hex.EncodeToString(refreshRaw),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.RefreshTokenTTL)),
		IssuedAt:  jwt.NewNumericDate(now),
		Issuer:    "gocircle",
	})
	refreshStr, err := refresh.SignedString([]byte(s.cfg.RefreshTokenSecret))
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessStr,
		RefreshToken: refreshStr,
		ExpiresAt:    now.Add(s.cfg.AccessTokenTTL),
	}, nil
}

// VerifyAccess validates an access token and returns its payload.
func (s *Service) VerifyAccess(tokenStr string) (*Payload, error) {
	claims := &Payload{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.AccessTokenSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}
	if !tok.Valid {
		return nil, errors.New("token invalid")
	}
	return claims, nil
}

// VerifyRefresh extracts the opaque refresh subject for server-side lookup.
func (s *Service) VerifyRefresh(tokenStr string) (string, time.Time, error) {
	claims := &jwt.RegisteredClaims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.RefreshTokenSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return "", time.Time{}, err
	}
	if !tok.Valid || claims.Subject == "" || claims.ExpiresAt == nil {
		return "", time.Time{}, errors.New("refresh token invalid")
	}
	return claims.Subject, claims.ExpiresAt.Time, nil
}

// ---- Gin middleware ----

type ctxUserKey struct{}

// User returns the authenticated Payload from the context, or nil.
func User(ctx context.Context) *Payload {
	p, _ := ctx.Value(ctxUserKey{}).(*Payload)
	return p
}

// RequireAuth extracts Bearer token, verifies it, and injects the payload.
func (s *Service) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if h == "" || len(h) < 8 || h[:7] != "Bearer " {
			c.AbortWithStatusJSON(401, gin.H{"code": 10002, "message": "missing token", "traceId": c.GetString("traceId")})
			return
		}
		payload, err := s.VerifyAccess(h[7:])
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"code": 10002, "message": "token invalid", "traceId": c.GetString("traceId")})
			return
		}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxUserKey{}, payload))
		c.Set("user", payload)
		c.Next()
	}
}
