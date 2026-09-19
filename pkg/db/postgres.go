// Package db 提供 PostgreSQL 连接池与迁移辅助。
package db

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"zhonghuawenhua_backend/internal/config"
)

// NewPostgres 创建 pgxpool 连接池。
func NewPostgres(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode)

	pcfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pgx config: %w", err)
	}

	maxConns := cfg.MaxConns
	if maxConns <= 0 {
		maxConns = 10
	}
	pcfg.MaxConns = int32(maxConns)

	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}

// DSNForMigrations 构造 golang-migrate 可用的 PostgreSQL URL。
func DSNForMigrations(cfg config.DatabaseConfig) string {
	return "postgres://" + cfg.User + ":" + cfg.Password +
		"@" + cfg.Host + ":" + strconv.Itoa(cfg.Port) + "/" + cfg.Name +
		"?sslmode=" + cfg.SSLMode
}
