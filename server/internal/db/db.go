// Package db owns the PostgreSQL connection pool and Redis client.
// Repository packages receive the pool via dependency injection —
// they never construct it themselves.
package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"gocircle/server/internal/config"
)

type Pools struct {
	PG    *pgxpool.Pool
	Redis *redis.Client
}

func New(cfg *config.Config) (*Pools, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pgCfg, err := pgxpool.ParseConfig(cfg.DBConnStr)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	pgCfg.MaxConns = cfg.DBMaxConns
	pgCfg.MinConns = cfg.DBMinConns

	pgPool, err := pgxpool.NewWithConfig(ctx, pgCfg)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pgPool.Ping(ctx); err != nil {
		pgPool.Close()
		return nil, fmt.Errorf("pg ping: %w", err)
	}
	log.Println("[db] postgres pool ready")

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		pgPool.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	log.Println("[db] redis client ready")

	return &Pools{PG: pgPool, Redis: rdb}, nil
}

func (p *Pools) Close() {
	if p.PG != nil {
		p.PG.Close()
	}
	if p.Redis != nil {
		_ = p.Redis.Close()
	}
}
