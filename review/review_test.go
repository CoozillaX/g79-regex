package review_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/CoozillaX/g79-regex/review"
)

const (
	// sampleFile is the text to match, read from a file in this directory.
	sampleFile = "test_data.txt"
	// maxTextLen caps how much matched text we print so long hits stay readable.
	maxTextLen = 10
	// nicknameGroup is the group name reserved for nickname-only review.
	nicknameGroup = "nickname"
)

// mode describes one review path and how to validate its results: the general
// path must never hit the nickname group, and the nickname path must only hit
// it.
type mode struct {
	name     string
	nickname bool
}

var modes = []mode{
	{name: "General", nickname: false},
	{name: "Nickname", nickname: true},
}

// validGroup reports whether a matched group is allowed for this mode.
func (m mode) validGroup(group string) bool {
	if m.nickname {
		return group == nicknameGroup
	}
	return group != nicknameGroup
}

// TestReviewFromCloud fetches the live rule bundle and exercises both the
// general and nickname review paths against the sample text, in both full-scan
// and first-only modes. It is network-backed, so it is skipped under -short.
func TestReviewFromCloud(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cloud-backed test in -short mode")
	}

	content := loadSample(t)
	r := newReviewer(t)
	defer r.Close()

	for _, m := range modes {
		t.Run(m.name+"/All", func(t *testing.T) {
			if hits := reviewWord(t, r, content, m, false); len(hits) == 0 {
				t.Fatalf("%s: expected matches, got none", m.name)
			}
		})

		t.Run(m.name+"/FirstOnly", func(t *testing.T) {
			if hits := reviewWord(t, r, content, m, true); len(hits) > 1 {
				t.Fatalf("%s: FirstOnly returned %d matches, want at most 1", m.name, len(hits))
			}
		})
	}
}

// reviewWord runs one ReviewWord call, asserts every hit belongs to an allowed
// group, logs the (truncated) results, and returns the matches.
func reviewWord(t *testing.T, r *review.Reviewer, content string, m mode, firstOnly bool) []*review.GroupMatch {
	t.Helper()

	opts := &review.Options{Nickname: m.nickname, FirstOnly: firstOnly}

	start := time.Now()
	hits := r.ReviewWord(content, opts)
	t.Logf("[%s firstOnly=%v] scan took %v; matches=%d", m.name, firstOnly, time.Since(start), len(hits))

	for _, h := range hits {
		if !m.validGroup(h.Group) {
			t.Fatalf("%s: unexpected group %q", m.name, h.Group)
		}
		t.Logf("  - [%s, %d] %s (%d - %d)", h.Group, h.Index, truncate(h.Text, maxTextLen), h.Start, h.End)
	}

	return hits
}

// loadSample reads the local sample text used as the review subject.
func loadSample(t *testing.T) string {
	t.Helper()

	b, err := os.ReadFile(sampleFile)
	if err != nil {
		t.Fatalf("read %s: %v", sampleFile, err)
	}
	return string(b)
}

// newReviewer builds a Reviewer from the cloud bundle and logs init timing.
func newReviewer(t *testing.T) *review.Reviewer {
	t.Helper()

	start := time.Now()
	r, err := review.New(context.Background())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Logf("init (download + decrypt + load) took %v", time.Since(start))
	return r
}

// truncate shortens s to at most n runes, appending an ellipsis when cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
