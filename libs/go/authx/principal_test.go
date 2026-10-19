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
