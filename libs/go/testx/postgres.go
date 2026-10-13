package testx

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

var pg started

type postgresConfig struct {
	migrationsDir string
}

// PostgresOption tuy chinh Postgres. Chi co hieu luc o lan goi DAU TIEN cua tien trinh.
type PostgresOption func(*postgresConfig)

// WithMigrations doi thu muc migration (mac dinh: <goc repo>/migrations).
func WithMigrations(dir string) PostgresOption {
	return func(c *postgresConfig) { c.migrationsDir = dir }
}

// Postgres khoi dong (mot lan cho ca tien trinh) postgres:16-alpine da chay moi migration va tra
// ve DSN dang URL. Chi chay phan "-- +goose Up" cua tung file.
func Postgres(t testing.TB, opts ...PostgresOption) string {
	t.Helper()
	return pg.get(t, PostgresImage, func() (string, error) { return startPostgres(opts) })
}

func startPostgres(opts []PostgresOption) (string, error) {
	cfg := postgresConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.migrationsDir == "" {
		root, err := repoRoot()
		if err != nil {
			return "", err
		}
		cfg.migrationsDir = filepath.Join(root, "migrations")
	}

	// Script khoi tao chay theo thu tu ten file trong /docker-entrypoint-initdb.d, truoc khi
	// Postgres nhan ket noi: khong co cua so nao de test thay schema dang xay do.
	tmp, err := os.MkdirTemp("", "testx-migrations-")
	if err != nil {
		return "", fmt.Errorf("tao thu muc tam: %w", err)
	}
	onShutdown(func() error { return os.RemoveAll(tmp) })

	scripts, err := writeUpScripts(cfg.migrationsDir, tmp)
	if err != nil {
		return "", err
	}

	ctx, cancel := startCtx()
	defer cancel()

	ctr, err := postgres.Run(ctx, PostgresImage,
		postgres.WithDatabase("eventflow"),
		postgres.WithUsername("eventflow"),
		postgres.WithPassword("eventflow"),
		postgres.WithInitScripts(scripts...),
		postgres.BasicWaitStrategies(),
	)
	if ctr != nil {
		terminate(ctr)
	}
	if err != nil {
		return "", err
	}

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return "", fmt.Errorf("lay DSN: %w", err)
	}
	return dsn, nil
}
