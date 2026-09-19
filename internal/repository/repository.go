// Package repository 提供数据访问基础设施。
package repository

import (
	"context"
	"fmt"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Base 封装 *pgxpool.Pool，提供通用查询辅助方法，供各业务 repository 嵌入使用。
type Base struct {
	Pool *pgxpool.Pool
}

// NewBase 创建 Base。
func NewBase(pool *pgxpool.Pool) *Base {
	return &Base{Pool: pool}
}

// QueryRow 执行返回单行的查询，结果扫描到 dest。
func (b *Base) QueryRow(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	rows, err := b.Pool.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	return pgxscan.ScanOne(dest, rows)
}

// Query 执行返回多行的查询，结果扫描到 dest（切片指针）。
func (b *Base) Query(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return pgxscan.Select(ctx, b.Pool, dest, query, args...)
}

// Exec 执行不返回行的语句（INSERT/UPDATE/DELETE）。
func (b *Base) Exec(ctx context.Context, query string, args ...interface{}) error {
	_, err := b.Pool.Exec(ctx, query, args...)
	return err
}

// RunInTx 在数据库事务中执行 fn；fn 返回错误时回滚，否则提交。
// 供需要跨表一致性的业务（如点赞计数 + 明细）使用。
func (b *Base) RunInTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := b.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) // 已提交时为 no-op
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
