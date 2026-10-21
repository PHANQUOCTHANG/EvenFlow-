package activeevents

import (
	"slices"
	"testing"
)

func TestUnion_BoTrungBoRongSapXep(t *testing.T) {
	got := Union([]string{"b", "a", ""}, nil, []string{"a", "c"})
	if want := []string{"a", "b", "c"}; !slices.Equal(got, want) {
		t.Fatalf("Union = %v, muon %v", got, want)
	}
	if got := Union(); len(got) != 0 {
		t.Fatalf("Union() = %v, muon rong", got)
	}
}
