// Package config loads application configuration from environment variables.
// All secrets (DB password, JWT secret, WxAppSecret, etc.) MUST come from
// environment variables — never from code, Git, or database.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// Database
	DBConnStr  string
	DBMaxConns int32
	DBMinConns int32

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// JWT
	AccessTokenSecret  string
	RefreshTokenSecret string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration

	// WeChat Mini-Program
	WxAppID     string
	WxAppSecret string

	// Phone (Wx getPhoneNumber)
	WxPhoneSecret string

	// KMS — DEK wrapping key reference (not the key itself).
	// Real KMS integration comes later in vault/; for S1 we accept an env-based master
	// key only for dev, and force KMS in staging/prod (see EnforceKMS).
	MasterKeyEnv string // env var name, e.g. "MASTER_KEY"

	// IP registration limit per openid per 24h
	IPRegistrationsPer24h int

	// Resource library
	DedupSecret               string
	ResourceDefaultExpireDays int
	SimhashThreshold          int
	QualityThreshold          int

	// Admin bootstrap（仅首次启动、无任何管理员时生效）
	AdminBootstrapUsername string
	AdminBootstrapPassword string
}

func Load() (*Config, error) {
	// .env is optional — staging/prod inject via the platform.
	_ = godotenv.Load()

	addr := getenv("ADDR", ":8080")

	dbConnStr := os.Getenv("DATABASE_URL")
	if dbConnStr == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		return nil, fmt.Errorf("REDIS_ADDR is required")
	}

	accessSecret := os.Getenv("JWT_ACCESS_SECRET")
	refreshSecret := os.Getenv("JWT_REFRESH_SECRET")
	if accessSecret == "" || refreshSecret == "" {
		return nil, fmt.Errorf("JWT_ACCESS_SECRET and JWT_REFRESH_SECRET are required")
	}

	return &Config{
		Addr:         addr,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,

		DBConnStr:  dbConnStr,
		DBMaxConns: getenvInt32("DB_MAX_CONNS", 20),
		DBMinConns: getenvInt32("DB_MIN_CONNS", 5),

		RedisAddr:     redisAddr,
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       getenvInt("REDIS_DB", 0),

		AccessTokenSecret:  accessSecret,
		RefreshTokenSecret: refreshSecret,
		AccessTokenTTL:     2 * time.Hour,
		RefreshTokenTTL:    30 * 24 * time.Hour,

		WxAppID:       os.Getenv("WX_APPID"),
		WxAppSecret:   os.Getenv("WX_SECRET"),
		WxPhoneSecret: os.Getenv("WX_PHONE_SECRET"),

		MasterKeyEnv: os.Getenv("MASTER_KEY_ENV"),

		IPRegistrationsPer24h: getenvInt("IP_REGISTRATIONS_PER_24H", 10),

		DedupSecret:               os.Getenv("DEDUP_SECRET"),
		ResourceDefaultExpireDays: getenvInt("RESOURCE_DEFAULT_EXPIRE_DAYS", 90),
		SimhashThreshold:          getenvInt("SIMHASH_HAMMING_THRESHOLD", 3),
		QualityThreshold:          getenvInt("RESOURCE_QUALITY_THRESHOLD", 50),

		AdminBootstrapUsername: os.Getenv("ADMIN_BOOTSTRAP_USERNAME"),
		AdminBootstrapPassword: os.Getenv("ADMIN_BOOTSTRAP_PASSWORD"),
	}, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func getenvInt32(key string, def int32) int32 {
	return int32(getenvInt(key, int(def)))
}
