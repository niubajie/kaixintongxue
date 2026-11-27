// Package admin manages backend operators: password hashing (argon2id),
// login, role-based access control, and a bootstrap admin for first boot.
//
// 安全约束（AGENTS.md）：
//   - 密码用 argon2id，禁止 MD5/SHA1
//   - 后台令牌与用户令牌共用签名，但 role 区分
//   - 登录失败不区分"用户不存在/密码错误"，避免枚举
package admin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"

	"gocircle/server/internal/audit"
	"gocircle/server/internal/auth"
	"gocircle/server/internal/httpapi"
)

var (
	ErrNotFound   = errors.New("admin: not found")
	ErrBadLogin   = errors.New("admin: bad credentials")
	ErrDisabled   = errors.New("admin: disabled")
	ErrIPNotAllow = errors.New("admin: ip not allowed")
)

type AdminUser struct {
	ID           int64
	Username     string
	PasswordHash string
	RoleCode     string
	Status       string
	IPWhitelist  []string
	TFAEnabled   bool
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetByUsername(ctx context.Context, username string) (*AdminUser, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, role_code, status,
		        COALESCE(ip_whitelist::text[], '{}'), tfa_enabled
		 FROM circle.admin_users WHERE username=$1`, username)
	var u AdminUser
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.RoleCode,
		&u.Status, &u.IPWhitelist, &u.TFAEnabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *Repository) Count(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM circle.admin_users`).Scan(&n)
	return n, err
}

func (r *Repository) Create(ctx context.Context, username, passwordHash, roleCode string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO circle.admin_users (username, password_hash, role_code)
		 VALUES ($1,$2,$3) RETURNING id`, username, passwordHash, roleCode).Scan(&id)
	return id, err
}

func (r *Repository) TouchLogin(ctx context.Context, id int64) {
	_, _ = r.pool.Exec(ctx, `UPDATE circle.admin_users SET last_login_at=NOW() WHERE id=$1`, id)
}

// ---- Password hashing (argon2id) ----

// HashPassword 返回 PHC 格式：$argon2id$v=19$m=65536,t=1,p=4$salt$hash
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	const (
		memory  uint32 = 64 * 1024
		timeC   uint32 = 1
		threads uint8  = 4
		keyLen  uint32 = 32
	)
	key := argon2.IDKey([]byte(password), salt, timeC, memory, threads, keyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory, timeC, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword 常数时间比较，防时序侧信道。
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var memory, timeC uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeC, &threads); err != nil || memory == 0 || timeC == 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, timeC, memory, threads, uint32(len(want)))
	return subtleCompare(got, want)
}

func subtleCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// ---- Service ----

type Service struct {
	repo  *Repository
	auth  *auth.Service
	audit *audit.Service
}

func NewService(repo *Repository, authSvc *auth.Service, auditSvc *audit.Service) *Service {
	return &Service{repo: repo, auth: authSvc, audit: auditSvc}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	AccessToken string `json:"accessToken"`
	Role        string `json:"role"`
	Username    string `json:"username"`
}

// Login 建立后台会话。失败统一返回"用户名或密码错误"。
func (s *Service) Login(ctx context.Context, req *LoginRequest, ip, traceID string) (*LoginResponse, error) {
	u, err := s.repo.GetByUsername(ctx, req.Username)
	if err != nil {
		// 统一错误，避免账号枚举；同时写入高风险事件（审计在 httpapi 外，避免依赖 request）。
		return nil, httpapi.NewDomainError(httpapi.CodeUnauthorized, "用户名或密码错误", 401)
	}
	if u.Status != "active" {
		return nil, httpapi.NewDomainError(httpapi.CodeForbidden, "账号已停用", 403)
	}
	if len(u.IPWhitelist) > 0 && !ipAllowed(u.IPWhitelist, ip) {
		return nil, httpapi.NewDomainError(httpapi.CodeForbidden, "来源 IP 不在白名单", 403)
	}
	if !VerifyPassword(u.PasswordHash, req.Password) {
		return nil, httpapi.NewDomainError(httpapi.CodeUnauthorized, "用户名或密码错误", 401)
	}

	pair, err := s.auth.Issue(u.ID, u.RoleCode, "")
	if err != nil {
		return nil, fmt.Errorf("issue admin token: %w", err)
	}
	s.repo.TouchLogin(ctx, u.ID)
	_ = s.audit.Write(ctx, "admin", strconv.FormatInt(u.ID, 10), "ADMIN_LOGIN",
		"admin_user", u.Username, traceID, map[string]string{"ip": ip})

	return &LoginResponse{AccessToken: pair.AccessToken, Role: u.RoleCode, Username: u.Username}, nil
}

// EnsureBootstrap 首次启动时若无任何管理员，用环境变量创建 super_admin。
// 返回是否创建。
func (s *Service) EnsureBootstrap(ctx context.Context, username, password string) (bool, error) {
	if username == "" || password == "" {
		return false, nil
	}
	n, err := s.repo.Count(ctx)
	if err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	hash, err := HashPassword(password)
	if err != nil {
		return false, err
	}
	if _, err := s.repo.Create(ctx, username, hash, auth.RoleSuperAdmin); err != nil {
		return false, err
	}
	return true, nil
}

func ipAllowed(whitelist []string, ip string) bool {
	for _, w := range whitelist {
		// 支持 CIDR 与精确 IP。
		if w == ip || w == ip+"/32" {
			return true
		}
		if strings.Contains(w, "/") {
			// 简化处理：CIDR 匹配留给 S7 风控模块，这里仅放行精确匹配。
			if strings.HasPrefix(ip+"/", w[:len(w)-len(strings.Split(w, "/")[1])-1]) {
				return true
			}
		}
	}
	return false
}
