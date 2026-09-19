package db

import (
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// RunMigrations 执行指定目录下的 SQL 迁移文件。
func RunMigrations(dbURL, migrationsPath string) error {
	// 使用 iofs 源（os.DirFS）而非 file:// URL，避免 Windows 盘符被误解析为端口。
	d, err := iofs.New(os.DirFS(migrationsPath), ".")
	if err != nil {
		return fmt.Errorf("create migrate source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, dbURL)
	if err != nil {
		return fmt.Errorf("create migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
