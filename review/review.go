// Package review provides a content-moderation reviewer backed by the NetEase
// PCRE2 regex bundle. A Reviewer downloads and parses the bundle on creation
// and exposes detailed match information that can be hot-reloaded at runtime.
package review

import (
	"context"
	"fmt"
	"sync"

	"github.com/CoozillaX/g79-regex/internal/pcre2"
	"github.com/CoozillaX/g79-regex/internal/source"
)

const (
	defaultLevel   = "0"
	defaultChannel = "item_comment"
)

// Reviewer matches text against the loaded regex groups. It is safe for
// concurrent use; the underlying rule set can be swapped via Reload without
// disturbing in-flight checks.
type Reviewer struct {
	mu  sync.RWMutex
	set *pcre2.Set
	url string // bundle URL of the currently loaded set, to skip no-op reloads
}

// New downloads, decrypts, and parses the current regex bundle and returns a
// ready Reviewer. The caller owns the returned Reviewer and must Close it.
func New(ctx context.Context) (*Reviewer, error) {
	groups, url, err := source.Fetch(ctx)
	if err != nil {
		return nil, err
	}

	set, err := pcre2.LoadGroups(groups)
	if err != nil {
		return nil, err
	}

	return &Reviewer{set: set, url: url}, nil
}

// Reload re-fetches the bundle and atomically swaps in the new rule set. When
// the bundle URL is unchanged since the last load it is a no-op returning
// false. In-flight checks keep using the previous set until they complete.
func (r *Reviewer) Reload(ctx context.Context) (changed bool, err error) {
	url, err := source.ResolveURL(ctx)
	if err != nil {
		return false, err
	}

	r.mu.RLock()
	unchanged := url == r.url
	r.mu.RUnlock()
	if unchanged {
		return false, nil
	}

	packed, err := source.Download(ctx, url)
	if err != nil {
		return false, err
	}

	groups, err := source.ParseGroups(packed)
	if err != nil {
		return false, err
	}

	set, err := pcre2.LoadGroups(groups)
	if err != nil {
		return false, err
	}

	r.mu.Lock()
	old := r.set
	r.set = set
	r.url = url
	r.mu.Unlock()

	// Safe: the write lock above drained all readers, and new readers now see
	// the new set, so nothing can still be using old.
	if old != nil {
		old.Close()
	}

	return true, nil
}

// Options tunes a ReviewWord call. A nil *Options is valid and uses the
// defaults: level "0", channel "item_comment", and a full scan that returns
// every match.
type Options struct {
	// Level is the expression level; a few rules are gated on it. Defaults to "0".
	Level string
	// Channel is the scenario the text comes from; it changes which rules apply.
	// Defaults to "item_comment". Other known values include "check_long_numbers"
	// and "World" (world chat).
	Channel string
	// FirstOnly stops at the first matching rule instead of scanning every rule.
	FirstOnly bool
}

// ReviewWord checks content against the loaded rules and returns the matches,
// or nil when nothing matches. With opts.FirstOnly it stops at the first hit
// (still returned as a slice). opts may be nil to use the defaults.
func (r *Reviewer) ReviewWord(content string, opts *Options) []*pcre2.GroupMatch {
	var level, channel string
	var firstOnly bool
	if opts != nil {
		level, channel, firstOnly = opts.Level, opts.Channel, opts.FirstOnly
	}
	subject := buildSubject(level, channel, content)

	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.set == nil {
		return nil
	}

	if firstOnly {
		if m := r.set.FindFirst(subject); m != nil {
			return []*pcre2.GroupMatch{m}
		}
		return nil
	}

	matches := r.set.FindAll(subject)
	if len(matches) == 0 {
		return nil
	}
	return matches
}

// Close releases the underlying rule set.
func (r *Reviewer) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.set != nil {
		r.set.Close()
		r.set = nil
	}
}

func buildSubject(level, channel, content string) string {
	if level == "" {
		level = defaultLevel
	}
	if channel == "" {
		channel = defaultChannel
	}

	return fmt.Sprintf("level=%s_channel=%s_content=%s", level, channel, content)
}
