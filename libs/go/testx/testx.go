// Package testx dung ha tang THAT (Postgres, Redis, RabbitMQ) cho test tich hop qua testcontainers.
//
// Moi loai container khoi dong MOT lan cho ca tien trinh test va dung chung; cac test tu co lap
// bang du lieu rieng (event_id, tien to queue moi), khong truncate. Khoi dong lai container moi
// lan goi setup la hang chuc phut khi chay -count=200.
//
// Khong co Docker thi test FAIL, khong skip: `go test` coi skip la thanh cong, va mot cong chat
// luong xanh ma khong kiem gi la thu nguy hiem nhat CI co the in ra (xem
// .github/scripts/run-and-report-tests.sh).
//
// Package nay chi duoc import tu file `//go:build integration` de khong lot vao binary.
// Moi package co dung container phai goi Shutdown trong TestMain sau m.Run().
package testx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
)

// startTimeout la thoi gian toi da de keo anh va doi mot container san sang.
const startTimeout = 3 * time.Minute

// Anh khop deploy/compose/docker-compose.yml: test chay tren cung phien ban voi moi truong that.
const (
	PostgresImage = "postgres:16-alpine"
	RedisImage    = "redis:7-alpine"
)

// started nho ket qua khoi dong cua mot loai container, ke ca loi: mot lan khoi dong that bai
// thi moi test sau fail ngay thay vi doi them 3 phut moi test.
type started struct {
	once sync.Once
	val  string
	err  error
}

func (s *started) get(t testing.TB, what string, start func() (string, error)) string {
	t.Helper()
	s.once.Do(func() { s.val, s.err = start() })
	if s.err != nil {
		t.Fatalf("khong khoi dong duoc %s (can Docker): %v", what, s.err)
	}
	return s.val
}

var (
	cleanupMu sync.Mutex
	cleanups  []func() error
)

// onShutdown dang ky viec don dep chay trong Shutdown.
func onShutdown(f func() error) {
	cleanupMu.Lock()
	defer cleanupMu.Unlock()
	cleanups = append(cleanups, f)
}

// terminate dang ky go bo mot container khi Shutdown.
func terminate(c testcontainers.Container) {
	onShutdown(func() error { return testcontainers.TerminateContainer(c) })
}

// Shutdown go moi container da khoi dong. Goi trong TestMain:
//
//	func TestMain(m *testing.M) {
//		code := m.Run()
//		testx.Shutdown()
//		os.Exit(code)
//	}
func Shutdown() {
	cleanupMu.Lock()
	fs := cleanups
	cleanups = nil
	cleanupMu.Unlock()

	// Go theo thu tu nguoc: thu muc tam sau container da dung no.
	for i := len(fs) - 1; i >= 0; i-- {
		if err := fs[i](); err != nil {
			fmt.Fprintf(os.Stderr, "testx: don dep: %v\n", err)
		}
	}
}

// RepoRoot tra ve thu muc goc repo (noi co go.work).
func RepoRoot(t testing.TB) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("testx: %v", err)
	}
	return root
}

func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("khong xac dinh duoc vi tri source cua testx")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("khong thay go.work di nguoc tu %q", filepath.Dir(file))
		}
		dir = parent
	}
}

func startCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), startTimeout)
}
