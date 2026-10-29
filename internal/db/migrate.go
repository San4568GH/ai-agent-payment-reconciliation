package db

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Migrate applies migrations from the embedded filesystem to the database.
func Migrate(pool *pgxpool.Pool, fs embed.FS, dir string) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	goose.SetBaseFS(fs)

	if err := goose.Up(sqlDB, dir); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}

	return nil
}

// Version returns the current migration version.
func Version(pool *pgxpool.Pool) (int64, error) {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return 0, fmt.Errorf("set dialect: %w", err)
	}

	ver, err := goose.GetDBVersion(sqlDB)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("get db version: %w", err)
	}
	return ver, nil
}
