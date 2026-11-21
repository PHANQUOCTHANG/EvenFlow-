package httpadapter_test

// ops-warroom fix round 1: X-8 (ownership compare is case-insensitive on UUIDs).

import (
	"net/http"
	"strings"
	"testing"
)

func TestOpsSalesFix_X8_OwnerUUIDCaseInsensitive(t *testing.T) {
	for _, owner := range []string{organizerA, strings.ToUpper(organizerA)} {
		for _, caller := range []string{organizerA, strings.ToUpper(organizerA)} {
			r := baseReader()
			r.owners[opsEv] = owner
			r.sales.OrganizerID = owner
			if rec := opsGet(newOpsMux(r), salesPath(opsEv), caller, "organizer"); rec.Code != http.StatusOK {
				t.Errorf("owner %s caller %s: %d %s", owner, caller, rec.Code, rec.Body)
			}
		}
	}
	r := baseReader()
	r.sales.OrganizerID = organizerA
	if rec := opsGet(newOpsMux(r), salesPath(opsEv), strings.ToUpper(organizerB), "organizer"); rec.Code != http.StatusNotFound {
		t.Errorf("other organizer must stay 404, got %d", rec.Code)
	}
}
