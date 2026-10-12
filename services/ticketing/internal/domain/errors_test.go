package domain_test

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
)

// Ma hold/don la 128 bit ngau nhien dang hex: du dai de khong doan duoc va
// khong trung. Trung ma thi viec tra kho theo ma hold (BR-O7) se tra nham hold.
func TestNewID_MaNgauNhien128BitKhongTrung(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := domain.NewID()
		b, err := hex.DecodeString(id)
		if err != nil || len(b) != 16 {
			t.Fatalf("ma %q phai la 16 byte dang hex (32 ky tu), err=%v", id, err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("ma bi trung: %s", id)
		}
		seen[id] = struct{}{}
	}
}

// Moi loi nghiep vu phai phan biet duoc voi nhau: tang HTTP dua vao chung de tra
// dung ma trang thai (het ve 409, vuot gioi han 422, chua admit 403), va
// ErrInventoryExhausted phai khac ErrSoldOut de biet Redis dang lech Postgres.
func TestLoiNghiepVu_PhanBietDuocVoiNhau(t *testing.T) {
	all := []struct {
		name string
		err  error
	}{
		{"ErrSoldOut", domain.ErrSoldOut},
		{"ErrInventoryExhausted", domain.ErrInventoryExhausted},
		{"ErrNotAdmitted", domain.ErrNotAdmitted},
		{"ErrQuantityNotAllowed", domain.ErrQuantityNotAllowed},
		{"ErrPerIdentityLimit", domain.ErrPerIdentityLimit},
		{"ErrHoldExpired", domain.ErrHoldExpired},
		{"ErrOrderNotFound", domain.ErrOrderNotFound},
	}
	for i, a := range all {
		if a.err == nil || a.err.Error() == "" {
			t.Fatalf("%s phai co thong diep", a.name)
		}
		for j, b := range all {
			if i != j && errors.Is(a.err, b.err) {
				t.Errorf("%s khong duoc trung voi %s", a.name, b.name)
			}
		}
	}
}
