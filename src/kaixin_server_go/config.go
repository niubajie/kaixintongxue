package main

import (
	"os"
	"strings"
)

// Config 全部来自环境变量，默认值与 Python 版保持一致，便于直接复用现有数据库。
type Config struct {
	DatabaseURL string
	Port        string
	DataRoot    string
	AdminDist   string
	Scanner     string
}

func env(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func loadConfig() Config {
	databaseURL := env("DATABASE_URL", "postgres://kaixin:kaixin_dev_pwd@localhost:5432/kaixin")
	databaseURL = strings.Replace(databaseURL, "postgresql+asyncpg://", "postgres://", 1)
	databaseURL = strings.Replace(databaseURL, "postgresql://", "postgres://", 1)

	return Config{
		DatabaseURL: databaseURL,
		Port:        env("PORT", "5080"),
		DataRoot:    env("KAIXIN_SERVER_DATA", "server-data"),
		AdminDist:   env("ADMIN_DIST", "../kaixin_admin/dist"),
		Scanner:     strings.ToLower(env("KAIXIN_SCANNER", "auto")),
	}
}
