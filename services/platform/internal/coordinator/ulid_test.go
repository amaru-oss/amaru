package coordinator

import (
	"regexp"
	"testing"
	"time"
)

var ulidPattern = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)

func TestULIDFormatOrderAndUniqueness(t *testing.T) {
	// The ULID specification example: time 1469918176385 encodes as 01ARYZ6S41.
	if got := ulidAt(time.UnixMilli(1469918176385))[:10]; got != "01ARYZ6S41" {
		t.Fatalf("time prefix: %s", got)
	}
	seen := map[string]bool{}
	for range 1000 {
		id := randomULID()
		if !ulidPattern.MatchString(id) {
			t.Fatalf("not a ULID: %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate %s", id)
		}
		seen[id] = true
	}
	base := time.UnixMilli(1_700_000_000_000)
	if a, b := ulidAt(base), ulidAt(base.Add(time.Millisecond)); a[:10] >= b[:10] {
		t.Fatalf("not ordered by time: %s %s", a, b)
	}
}

func TestDefaultApprovalIDsAreULIDs(t *testing.T) {
	c := New(nil, time.Now)
	if id := c.newApprovalID(); !ulidPattern.MatchString(id) {
		t.Fatalf("default approval id %q", id)
	}
}
