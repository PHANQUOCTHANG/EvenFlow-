package authx_test

import (
	"net/http"
	"testing"

	"github.com/eventflow/eventflow/libs/go/authx"
)

const someID = "7f6c2a8e-0000-4000-8000-000000000001"

func TestPrincipal_StripXoaMoiHeaderGiaDanh(t *testing.T) {
	h := http.Header{}
	for _, k := range []string{"X-Identity-Id", "X-Identity-Role", "X-User-Id", "X-Role", "X-Organizer-Id", "x-identity-id"} {
		h.Add(k, "attacker")
	}
	h.Set("Authorization", "Bearer abc")

	authx.StripIdentityHeaders(h)

	for k := range h {
		if k != "Authorization" {
			t.Fatalf("con header %q sau khi strip", k)
		}
	}
}

func TestPrincipal_SetGhiDeGiaTriCuaClient(t *testing.T) {
	h := http.Header{}
	h.Add("X-Identity-Id", "attacker-1")
	h.Add("X-Identity-Id", "attacker-2")

	authx.SetIdentityHeaders(h, authx.Principal{ID: someID, Role: "buyer"})

	if got := h.Values("X-Identity-Id"); len(got) != 1 || got[0] != someID {
		t.Fatalf("X-Identity-Id = %v, muon dung mot gia tri %s", got, someID)
	}
	p, ok := authx.PrincipalFromHeaders(h)
	if !ok || p.ID != someID || p.Role != "buyer" {
		t.Fatalf("doc lai sai: %+v ok=%v", p, ok)
	}
}

func TestPrincipal_VerifiedChiLaTrueKhiGatewayGan(t *testing.T) {
	for _, verified := range []bool{true, false} {
		h := http.Header{}
		authx.SetIdentityHeaders(h, authx.Principal{ID: someID, Role: "buyer", Verified: verified})
		p, ok := authx.PrincipalFromHeaders(h)
		if !ok || p.Verified != verified {
			t.Fatalf("verified=%v: doc lai %+v ok=%v", verified, p, ok)
		}
	}
	// Client gui true, gateway gan false: gia tri cua client khong con.
	h := http.Header{}
	h.Set("X-Identity-Verified", "true")
	authx.SetIdentityHeaders(h, authx.Principal{ID: someID, Role: "buyer", Verified: false})
	if p, _ := authx.PrincipalFromHeaders(h); p.Verified {
		t.Fatal("header verified do client gui khong duoc song sot")
	}
	// Thieu header: khong duoc coi la da xac thuc.
	h = http.Header{"X-Identity-Id": {someID}, "X-Identity-Role": {"buyer"}}
	if p, _ := authx.PrincipalFromHeaders(h); p.Verified {
		t.Fatal("thieu header verified phai la chua xac thuc")
	}
}

func TestPrincipal_ThietBiChiTuGatewayVaPhaiLaUUID(t *testing.T) {
	const did = "7f6c2a8e-0000-4000-8000-0000000000d1"
	h := http.Header{}
	authx.SetIdentityHeaders(h, authx.Principal{ID: someID, Role: "buyer", DeviceID: did})
	if p, ok := authx.PrincipalFromHeaders(h); !ok || p.DeviceID != did {
		t.Fatalf("doc lai %+v ok=%v", p, ok)
	}

	// Client gui X-Device-Id, token khong gan thiet bi: header cua client bi xoa.
	h = http.Header{}
	h.Set("X-Device-Id", did)
	authx.SetIdentityHeaders(h, authx.Principal{ID: someID, Role: "buyer"})
	if p, _ := authx.PrincipalFromHeaders(h); p.DeviceID != "" {
		t.Fatal("X-Device-Id do client gui khong duoc song sot")
	}

	// Gia tri khong phai UUID: tu choi ca principal.
	h = http.Header{"X-Identity-Id": {someID}, "X-Identity-Role": {"buyer"}, "X-Device-Id": {"khong-phai-uuid"}}
	if _, ok := authx.PrincipalFromHeaders(h); ok {
		t.Fatal("X-Device-Id hong phai bi tu choi")
	}
}

func TestPrincipal_FromHeadersTuChoiGiaTriSai(t *testing.T) {
	cases := map[string][2]string{
		"thieu id":      {"", "buyer"},
		"id khong uuid": {"123", "buyer"},
		"role la":       {someID, "admin"},
		"thieu role":    {someID, ""},
		"role viet hoa": {someID, "Ops"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := http.Header{}
			h.Set("X-Identity-Id", c[0])
			h.Set("X-Identity-Role", c[1])
			if _, ok := authx.PrincipalFromHeaders(h); ok {
				t.Fatal("phai tu choi")
			}
		})
	}
}
