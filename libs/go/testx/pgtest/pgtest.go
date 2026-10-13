//go:build integration

// Package pgtest dung Postgres that (testcontainers, anh postgres:16-alpine khop compose) cho test tich hop
// va ap migration goose bang SQL truc tiep. CHI duoc import tu file test co build tag `integration`.
//
// Mot container cho moi tien trinh test (package); moi lan goi New/NewEmpty tao mot DATABASE moi
// de cac test doc lap. Khong co Docker thi test FAIL (khong skip) theo hop dong K1 cua CI.
//
// Cach dung:
//
//	func TestMain(m *testing.M) { code := m.Run(); pgtest.Terminate(); os.Exit(code) }
//	pool := pgtest.New(t)               // database moi da ap Up cua moi file migrations/*.sql
//	pool := pgtest.NewEmpty(t)          // database moi chua ap migration nao
//	pgtest.ApplyUp(t, pool, file)       // ap phan "-- +goose Up" cua mot file trong transaction tuong minh
//	pgtest.ApplyDown(t, pool, file)     // ap phan "-- +goose Down"
package pgtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Image la anh Postgres dung cho test (khop deploy/compose).
const Image = "postgres:16-alpine"

const (
	user     = "evtest"
	password = "evtest"
	adminDB  = "postgres"
)

var (
	once    sync.Once
	ctr     testcontainers.Container
	hostEP  string // host:port
	initErr error
	dbSeq   atomic.Int64
)

// start khoi dong container (mot lan) va tra ve host:port.
func start(t testing.TB) string {
	t.Helper()
	once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		req := testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        Image,
				ExposedPorts: []string{"5432/tcp"},
				Env: map[string]string{
					"POSTGRES_USER":     user,
					"POSTGRES_PASSWORD": password,
					"POSTGRES_DB":       adminDB,
				},
				// Anh postgres khoi dong hai lan (initdb roi chay that) nen dong "ready" xuat hien 2 lan.
				WaitingFor: wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).WithStartupTimeout(2 * time.Minute),
			},
			Started: true,
		}
		ctr, initErr = testcontainers.GenericContainer(ctx, req)
		if initErr != nil {
			return
		}
		dc, ok := ctr.(*testcontainers.DockerContainer)
		if !ok {
			initErr = fmt.Errorf("container kieu %T, muon *DockerContainer", ctr)
			return
		}
		hostEP, initErr = dc.PortEndpoint(ctx, "5432/tcp", "")
	})
	if initErr != nil {
		t.Fatalf("khong khoi dong duoc Postgres (%s) — can Docker: %v", Image, initErr)
	}
	return hostEP
}

// Terminate dung container (goi trong TestMain sau m.Run). An toan khi chua khoi dong.
func Terminate() {
	if ctr != nil {
		_ = testcontainers.TerminateContainer(ctr)
	}
}

func dsn(host, db string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", user, password, host, db)
}

// NewEmpty tao mot database moi (chua co bang nao) va tra ve pool toi no. Pool dong khi test xong.
func NewEmpty(t testing.TB) *pgxpool.Pool {
	t.Helper()
	host := start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := fmt.Sprintf("t%d_%d", os.Getpid(), dbSeq.Add(1))
	admin, err := pgx.Connect(ctx, dsn(host, adminDB))
	if err != nil {
		t.Fatalf("ket noi database quan tri: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("tao database %s: %v", name, err)
	}

	pool, err := pgxpool.New(ctx, dsn(host, name))
	if err != nil {
		t.Fatalf("tao pool %s: %v", name, err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping %s: %v", name, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// New tao database moi va ap phan Up cua moi file migrations/*.sql theo thu tu ten file.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool := NewEmpty(t)
	for _, f := range MigrationFiles(t) {
		ApplyUp(t, pool, f)
	}
	return pool
}

// MigrationsDir tim thu muc migrations/ cua repo: bien moi truong MIGRATIONS_DIR neu co, neu khong thi
// di nguoc tu thu muc hien tai toi khi gap migrations/0001_core.sql.
func MigrationsDir(t testing.TB) string {
	t.Helper()
	if d := os.Getenv("MIGRATIONS_DIR"); d != "" {
		return d
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		cand := filepath.Join(dir, "migrations")
		if _, err := os.Stat(filepath.Join(cand, "0001_core.sql")); err == nil {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("khong tim thay thu muc migrations/ (dat MIGRATIONS_DIR)")
		}
		dir = parent
	}
}

// MigrationFiles tra ve duong dan tuyet doi cac file migrations/*.sql, sap theo ten.
func MigrationFiles(t testing.TB) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(MigrationsDir(t), "*.sql"))
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("thu muc migrations/ rong")
	}
	return files
}

// MigrationFile tra ve duong dan file migration co ten bat dau bang prefix (vd. "0002").
func MigrationFile(t testing.TB, prefix string) string {
	t.Helper()
	for _, f := range MigrationFiles(t) {
		if strings.HasPrefix(filepath.Base(f), prefix) {
			return f
		}
	}
	t.Fatalf("khong co migration %s*", prefix)
	return ""
}

// Sections tach noi dung file goose thanh phan Up va Down. Dong chu thich "-- +goose StatementBegin/End"
// bi bo (ca khoi duoc gui mot lan bang simple protocol nen khong can tach cau).
func Sections(content string) (up, down string, err error) {
	var cur *strings.Builder
	var upB, downB strings.Builder
	seenUp := false
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		trim := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trim, "-- +goose Up"):
			cur, seenUp = &upB, true
			continue
		case strings.HasPrefix(trim, "-- +goose Down"):
			cur = &downB
			continue
		case strings.HasPrefix(trim, "-- +goose "):
			continue
		}
		if cur != nil {
			cur.WriteString(line)
			cur.WriteByte('\n')
		}
	}
	if !seenUp {
		return "", "", errors.New("thieu dong -- +goose Up")
	}
	return upB.String(), downB.String(), nil
}

// ApplyUp ap phan Up cua file trong MOT transaction tuong minh (nhu goose) de SET LOCAL co hieu luc.
func ApplyUp(t testing.TB, pool *pgxpool.Pool, file string) {
	t.Helper()
	if err := Apply(pool, file, true); err != nil {
		t.Fatalf("ap Up %s: %v", filepath.Base(file), err)
	}
}

// ApplyDown ap phan Down cua file trong mot transaction tuong minh.
func ApplyDown(t testing.TB, pool *pgxpool.Pool, file string) {
	t.Helper()
	if err := Apply(pool, file, false); err != nil {
		t.Fatalf("ap Down %s: %v", filepath.Base(file), err)
	}
}

// Apply nhu ApplyUp/ApplyDown nhung tra loi thay vi fail (de test truong hop migration phai bo cuoc).
func Apply(pool *pgxpool.Pool, file string, up bool) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	upSQL, downSQL, err := Sections(string(b))
	if err != nil {
		return err
	}
	sql := downSQL
	if up {
		sql = upSQL
	}
	if strings.TrimSpace(sql) == "" {
		return errors.New("phan migration rong")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	// Khong co tham so -> pgx dung simple protocol, chay duoc nhieu cau trong mot lan gui.
	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
