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
	maxTextLen = 30
)

// TestReviewFromCloud fetches the live rule bundle and exercises both review
// paths against the sample text. It is network-backed, so it is skipped under
// -short.
func TestReviewFromCloud(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cloud-backed test in -short mode")
	}

	content := loadSample(t)
	r := newReviewer(t)
	defer r.Close()

	t.Run("All", func(t *testing.T) {
		reviewAll(t, r, content)
	})

	t.Run("FirstOnly", func(t *testing.T) {
		start := time.Now()
		hits := r.ReviewWord(content, &review.Options{FirstOnly: true})
		t.Logf("first-match scan took %v", time.Since(start))
		if len(hits) == 0 {
			t.Fatal("expected at least one match, got none")
		}
		hit := hits[0]
		t.Logf("first hit: [%s, %d] %s (%d - %d)", hit.Group, hit.Index, truncate(hit.Text, maxTextLen), hit.Start, hit.End)
	})
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

// reviewAll runs a full scan, logs timing plus the truncated matches, and
// returns the match count.
func reviewAll(t *testing.T, r *review.Reviewer, content string) int {
	t.Helper()

	start := time.Now()
	matches := r.ReviewWord(content, nil)
	t.Logf("scan took %v; matches=%d", time.Since(start), len(matches))
	for _, m := range matches {
		t.Logf("  - [%s, %d] %s (%d - %d)", m.Group, m.Index, truncate(m.Text, maxTextLen), m.Start, m.End)
	}
	return len(matches)
}

// truncate shortens s to at most n runes, appending an ellipsis when cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
