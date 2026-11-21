package httpadapter_test

// ops-warroom fix round 1: R-3 (generated_at keeps sub-second precision).

import (
	"net/http"
	"testing"
	"time"
)

func TestOpsFix_R3_GeneratedAtKeepsSubSecondPrecision(t *testing.T) {
	r := baseReader()
	r.sales.OrganizerID = organizerA
	for i := 0; i < 3; i++ {
		for _, c := range []struct{ path, id, role string }{
			{invPath(opsEv), opsUser, "ops"},
			{salesPath(opsEv), organizerA, "organizer"},
		} {
			before := time.Now()
			rec := opsGet(newOpsMux(r), c.path, c.id, c.role)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: %d", c.path, rec.Code)
			}
			raw, _ := jsonOf(t, rec)["generated_at"].(string)
			at, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				t.Fatalf("%s generated_at %q: %v", c.path, raw, err)
			}
			if at.Before(before.Add(-time.Microsecond)) {
				t.Fatalf("%s generated_at %s is truncated (request started %s)", c.path, raw, before.UTC().Format(time.RFC3339Nano))
			}
		}
		time.Sleep(3 * time.Millisecond)
	}
}
