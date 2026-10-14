package testx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGooseUp_CatPhanUp(t *testing.T) {
	in := "-- +goose Up\nCREATE TABLE a (id INT);\n\n-- +goose Down\nDROP TABLE a;\n"
	got, err := gooseUp(in)
	if err != nil {
		t.Fatalf("gooseUp: %v", err)
	}
	if !strings.Contains(got, "CREATE TABLE a") {
		t.Errorf("thieu phan Up: %q", got)
	}
	if strings.Contains(got, "DROP TABLE") {
		t.Errorf("lot phan Down vao ket qua: %q", got)
	}
}

func TestGooseUp_KhongCoDown(t *testing.T) {
	got, err := gooseUp("-- +goose Up\nSELECT 1;\n")
	if err != nil {
		t.Fatalf("gooseUp: %v", err)
	}
	if !strings.Contains(got, "SELECT 1") {
		t.Errorf("mat noi dung: %q", got)
	}
}

func TestGooseUp_CRLF(t *testing.T) {
	got, err := gooseUp("-- +goose Up\r\nSELECT 1;\r\n-- +goose Down\r\nDROP TABLE a;\r\n")
	if err != nil {
		t.Fatalf("gooseUp: %v", err)
	}
	if strings.Contains(got, "DROP") || strings.Contains(got, "\r") {
		t.Errorf("khong chuan hoa xuong dong / van lot Down: %q", got)
	}
}

func TestGooseUp_TuChoiFileKhongHopLe(t *testing.T) {
	cases := map[string]string{
		"thieu marker Up": "CREATE TABLE a (id INT);\n-- +goose Down\nDROP TABLE a;\n",
		"phan Up rong":    "-- +goose Up\n   \n-- +goose Down\nDROP TABLE a;\n",
		"rong":            "",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if got, err := gooseUp(in); err == nil {
				t.Errorf("phai tu choi, nhan %q", got)
			}
		})
	}
}

func TestMigrationFiles_SapTheoTen(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"0010_b.sql", "0002_a.sql", "0001_core.sql", "README.md"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("-- +goose Up\nSELECT 1;"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := migrationFiles(dir)
	if err != nil {
		t.Fatalf("migrationFiles: %v", err)
	}
	var names []string
	for _, f := range got {
		names = append(names, filepath.Base(f))
	}
	want := "0001_core.sql,0002_a.sql,0010_b.sql"
	if strings.Join(names, ",") != want {
		t.Errorf("thu tu = %v, ky vong %s", names, want)
	}
}

func TestMigrationFiles_ThuMucRongLaLoi(t *testing.T) {
	if _, err := migrationFiles(t.TempDir()); err == nil {
		t.Error("thu muc khong co *.sql phai la loi")
	}
}

// Migration THAT cua repo phai tach duoc: neu ai do doi dinh dang file, container Postgres
// se khoi dong voi schema rong va moi test tich hop do theo mot cach kho hieu.
func TestWriteUpScripts_MigrationThatCuaRepo(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repoRoot: %v", err)
	}
	out := t.TempDir()
	files, err := writeUpScripts(filepath.Join(root, "migrations"), out)
	if err != nil {
		t.Fatalf("writeUpScripts: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("khong co migration nao")
	}
	var all strings.Builder
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	s := all.String()
	for _, want := range []string{"CREATE TABLE ticket_inventory", "CREATE TABLE seat_holds", "available >= 0"} {
		if !strings.Contains(s, want) {
			t.Errorf("schema thieu %q", want)
		}
	}
	if strings.Contains(s, "DROP TABLE") {
		t.Error("phan Down cua migration bi lot vao script khoi tao")
	}
}
