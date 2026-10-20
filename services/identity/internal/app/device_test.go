package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/services/identity/internal/app"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

// fakeDevices mo phong DeviceRegistrar: moi fingerprint mot id co dinh, toi da max thiet bi moi identity.
type fakeDevices struct {
	ids   map[string]uuid.UUID
	max   int
	calls []domain.DeviceInfo
	err   error
}

func newFakeDevices() *fakeDevices { return &fakeDevices{ids: map[string]uuid.UUID{}, max: 5} }

func (f *fakeDevices) Register(_ context.Context, identityID uuid.UUID, d domain.DeviceInfo) (uuid.UUID, error) {
	f.calls = append(f.calls, d)
	if f.err != nil {
		return uuid.Nil, f.err
	}
	key := identityID.String() + "|" + d.Fingerprint
	if id, ok := f.ids[key]; ok {
		return id, nil
	}
	if len(f.ids) >= f.max {
		return uuid.Nil, domain.ErrTooManyDevices
	}
	id := uuid.New()
	f.ids[key] = id
	return id, nil
}

type fakeDeviceRepo struct{ got []string }

func (f *fakeDeviceRepo) Register(_ context.Context, _ uuid.UUID, fp, _ string, max int) (uuid.UUID, error) {
	f.got = append(f.got, fp)
	if max != domain.MaxDevicesPerIdentity {
		return uuid.Nil, errors.New("sai gioi han")
	}
	return uuid.New(), nil
}

func TestDeviceService_KiemTraFingerprint(t *testing.T) {
	repo := &fakeDeviceRepo{}
	svc := app.NewDeviceService(repo)
	for name, fp := range map[string]string{"rong": "", "toan khoang trang": "        ", "qua ngan": "abc", "qua dai": strings.Repeat("x", 257)} {
		if _, err := svc.Register(context.Background(), uuid.New(), domain.DeviceInfo{Fingerprint: fp}); !errors.Is(err, domain.ErrInvalidFingerprint) {
			t.Fatalf("%s: muon ErrInvalidFingerprint, nhan %v", name, err)
		}
	}
	if len(repo.got) != 0 {
		t.Fatal("fingerprint khong hop le khong duoc toi repository")
	}
	if _, err := svc.Register(context.Background(), uuid.New(), domain.DeviceInfo{Fingerprint: "  fp-hop-le-12345  "}); err != nil {
		t.Fatal(err)
	}
	if repo.got[0] != "fp-hop-le-12345" {
		t.Fatalf("fingerprint phai duoc cat khoang trang, nhan %q", repo.got[0])
	}
}

func deviceClaim(t *testing.T, f *sessionFixture, p *domain.TokenPair) string {
	t.Helper()
	c, err := authx.VerifyAccessToken(sessionSecret, p.AccessToken, f.now)
	if err != nil {
		t.Fatal(err)
	}
	return c.DeviceID
}

func TestSession_DangNhapGanThietBiVaXoayGiuNguyen(t *testing.T) {
	f := newSessionFixture(t)
	devices := newFakeDevices()
	f.svc.WithDevices(devices)
	id := uuid.New()

	pair, _ := f.svc.GenerateTokens(domain.ContextWithDevice(context.Background(), id), f.identity)
	if got := deviceClaim(t, f, pair); got != id.String() {
		t.Fatalf("did = %q, muon %s", got, id)
	}

	// Refresh khong gui thiet bi: phien giu thiet bi cu.
	next, _, err := f.svc.Refresh(context.Background(), pair.RefreshToken, nil)
	if err != nil || deviceClaim(t, f, next) != id.String() {
		t.Fatalf("xoay phai giu thiet bi: %v", err)
	}
	if len(devices.calls) != 0 {
		t.Fatal("khong gui fingerprint thi khong duoc dang ky thiet bi")
	}
}

func TestSession_RefreshGanHoacDoiThietBi(t *testing.T) {
	f := newSessionFixture(t)
	devices := newFakeDevices()
	f.svc.WithDevices(devices)

	// Phien chua co thiet bi (Google/VNeID): refresh kem fingerprint de gan.
	pair, _ := f.svc.GenerateTokens(context.Background(), f.identity)
	if deviceClaim(t, f, pair) != "" {
		t.Fatal("phien chua gan thiet bi khong duoc co did")
	}
	bound, _, err := f.svc.Refresh(context.Background(), pair.RefreshToken, &domain.DeviceInfo{Fingerprint: "may-A-fingerprint"})
	if err != nil {
		t.Fatal(err)
	}
	a := deviceClaim(t, f, bound)
	if a == "" {
		t.Fatal("refresh kem fingerprint phai gan thiet bi")
	}

	// Lan sau khong gui: van thiet bi A (da luu vao refresh token moi).
	again, _, _ := f.svc.Refresh(context.Background(), bound.RefreshToken, nil)
	if deviceClaim(t, f, again) != a {
		t.Fatal("thiet bi phai duoc luu theo phien")
	}
	// Gui fingerprint khac: doi sang thiet bi B.
	moved, _, _ := f.svc.Refresh(context.Background(), again.RefreshToken, &domain.DeviceInfo{Fingerprint: "may-B-fingerprint"})
	if b := deviceClaim(t, f, moved); b == "" || b == a {
		t.Fatalf("phai doi sang thiet bi moi, did=%q cu=%q", b, a)
	}
}

func TestSession_RefreshLoiThietBiKhongTieuTokenMoi(t *testing.T) {
	f := newSessionFixture(t)
	devices := newFakeDevices()
	devices.err = domain.ErrTooManyDevices
	f.svc.WithDevices(devices)
	pair, _ := f.svc.GenerateTokens(context.Background(), f.identity)

	if _, _, err := f.svc.Refresh(context.Background(), pair.RefreshToken, &domain.DeviceInfo{Fingerprint: "may-moi-12345"}); !errors.Is(err, domain.ErrTooManyDevices) {
		t.Fatalf("muon ErrTooManyDevices, nhan %v", err)
	}
}

func TestLogin_GanThietBiTruocKhiCapToken(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	email := "buyer@example.com"
	repo.identities[email] = &domain.Identity{ID: uuid.New(), Email: &email, Role: domain.RoleBuyer}
	devices := newFakeDevices()
	uc := app.NewLoginUseCase(repo, &fakePasswordHasher{match: true}, &fakeTokenGen{token: "t"}, app.WithDeviceRegistrar(devices))

	out, err := uc.Execute(ctx, app.LoginInput{Identifier: email, Password: "x", Device: &domain.DeviceInfo{Fingerprint: "may-A-fingerprint", IP: "1.2.3.4"}})
	if err != nil || out.Tokens == nil || len(devices.calls) != 1 || devices.calls[0].IP != "1.2.3.4" {
		t.Fatalf("out=%+v err=%v calls=%+v", out, err, devices.calls)
	}

	// Vuot gioi han thiet bi: khong cap token.
	devices.max = 0
	if out, err := uc.Execute(ctx, app.LoginInput{Identifier: email, Password: "x", Device: &domain.DeviceInfo{Fingerprint: "may-B-fingerprint"}}); !errors.Is(err, domain.ErrTooManyDevices) || out != nil {
		t.Fatalf("muon ErrTooManyDevices va khong co token, nhan %+v %v", out, err)
	}
	// Khong gui thiet bi: dang nhap nhu cu.
	if _, err := uc.Execute(ctx, app.LoginInput{Identifier: email, Password: "x"}); err != nil {
		t.Fatal(err)
	}
	// Gui thiet bi nhung server chua cau hinh: loi ro rang thay vi am tham bo qua.
	bare := app.NewLoginUseCase(repo, &fakePasswordHasher{match: true}, &fakeTokenGen{token: "t"})
	if _, err := bare.Execute(ctx, app.LoginInput{Identifier: email, Password: "x", Device: &domain.DeviceInfo{Fingerprint: "may-A-fingerprint"}}); err == nil {
		t.Fatal("thieu registrar phai bao loi")
	}
}
