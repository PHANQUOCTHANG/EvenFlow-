package testx

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	gooseUpMarker   = "-- +goose Up"
	gooseDownMarker = "-- +goose Down"
)

// gooseUp tra ve phan SQL nam giua "-- +goose Up" va "-- +goose Down" cua mot file migration.
//
// Container test chi can phan Up cua file SQL thuan, nen khong keo them thu vien goose.
// File thieu marker Up bi tu choi thay vi chay ca file: phan Down cua migration la
// DROP TABLE, chay nham no tren schema moi la mat sach du lieu seed.
func gooseUp(sql string) (string, error) {
	sql = strings.ReplaceAll(sql, "\r\n", "\n")

	_, afterUp, ok := strings.Cut(sql, gooseUpMarker)
	if !ok {
		return "", fmt.Errorf("thieu dong %q", gooseUpMarker)
	}
	up, _, _ := strings.Cut(afterUp, gooseDownMarker)
	if strings.TrimSpace(up) == "" {
		return "", fmt.Errorf("phan Up rong")
	}
	return up, nil
}

// migrationFiles liet ke file *.sql trong dir, sap theo ten (goose chay theo thu tu so).
func migrationFiles(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, fmt.Errorf("liet ke migration %q: %w", dir, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("khong co file *.sql nao trong %q", dir)
	}
	sort.Strings(files)
	return files, nil
}

// writeUpScripts doc moi migration trong migrationsDir va ghi phan Up cua chung vao outDir
// voi cung ten file; tra ve duong dan theo thu tu chay.
func writeUpScripts(migrationsDir, outDir string) ([]string, error) {
	files, err := migrationFiles(migrationsDir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("doc migration %q: %w", f, err)
		}
		up, err := gooseUp(string(raw))
		if err != nil {
			return nil, fmt.Errorf("migration %q: %w", filepath.Base(f), err)
		}
		dst := filepath.Join(outDir, filepath.Base(f))
		if err := os.WriteFile(dst, []byte(up), 0o600); err != nil {
			return nil, fmt.Errorf("ghi %q: %w", dst, err)
		}
		out = append(out, dst)
	}
	return out, nil
}
