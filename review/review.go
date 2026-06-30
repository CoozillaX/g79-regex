// Package review provides a content-moderation reviewer backed by the NetEase
// PCRE2 regex bundle. A Reviewer downloads and parses the bundle on creation
// and exposes detailed match information that can be hot-reloaded at runtime.
//
// ReviewWord covers both general text review (all groups except nickname, with a
// level/channel/content subject) and nickname-only review (opts.Nickname).
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
	nicknameGroup  = "nickname"
)

// GroupMatch is one rule hit returned by ReviewWord.
type GroupMatch = pcre2.GroupMatch

// Reviewer matches text against the loaded regex groups. It is safe for
// concurrent use; the underlying rule sets can be swapped via Reload without
// disturbing in-flight checks.
type Reviewer struct {
	mu          sync.RWMutex
	set         *pcre2.Set // all groups except nickname
	nicknameSet *pcre2.Set // nickname group only
	url         string     // bundle URL of the currently loaded set, to skip no-op reloads
}

// New downloads, decrypts, and parses the current regex bundle and returns a
// ready Reviewer. The caller owns the returned Reviewer and must Close it.
func New(ctx context.Context) (*Reviewer, error) {
	groups, url, err := source.Fetch(ctx)
	if err != nil {
		return nil, err
	}

	set, nicknameSet, err := loadSets(groups)
	if err != nil {
		return nil, err
	}

	return &Reviewer{set: set, nicknameSet: nicknameSet, url: url}, nil
}

// Reload re-fetches the bundle and atomically swaps in the new rule sets. When
// the bundle URL is unchanged since the last load it is a no-op returning
// false. In-flight checks keep using the previous sets until they complete.
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

	set, nicknameSet, err := loadSets(groups)
	if err != nil {
		return false, err
	}

	r.mu.Lock()
	oldSet, oldNickname := r.set, r.nicknameSet
	r.set = set
	r.nicknameSet = nicknameSet
	r.url = url
	r.mu.Unlock()

	if oldSet != nil {
		oldSet.Close()
	}
	if oldNickname != nil {
		oldNickname.Close()
	}

	return true, nil
}

// Options tunes a ReviewWord call. A nil *Options is valid and uses the
// defaults: general review, level "0", channel "item_comment", and a full scan
// that returns every match.
type Options struct {
	// Level is the expression level; a few rules are gated on it. Defaults to
	// "0". Ignored when Nickname is true.
	Level string
	// Channel is the scenario the text comes from; it changes which rules apply.
	// Defaults to "item_comment". Other known values include "check_long_numbers"
	// and "World" (world chat). Ignored when Nickname is true.
	Channel string
	// Nickname selects the nickname rule set instead of the general one. When
	// true, content is matched as-is (no level/channel prefix), mirroring the
	// in-game reviewNickname path. Defaults to false.
	Nickname bool
	// FirstOnly stops at the first matching rule instead of scanning every rule.
	FirstOnly bool
}

// ReviewWord checks content against the loaded rules and returns the matches,
// or nil when nothing matches. opts may be nil to use the defaults.
//
// By default it uses the general rule set (every group except nickname) with a
// level/channel/content subject. With opts.Nickname it instead uses the
// nickname-only rule set and matches the raw content directly.
func (r *Reviewer) ReviewWord(content string, opts *Options) []*GroupMatch {
	var o Options
	if opts != nil {
		o = *opts
	}

	if o.Nickname {
		return r.match(r.nicknameSet, content, o.FirstOnly)
	}

	return r.match(r.set, buildSubject(o.Level, o.Channel, content), o.FirstOnly)
}

// Close releases the underlying rule sets.
func (r *Reviewer) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.set != nil {
		r.set.Close()
		r.set = nil
	}
	if r.nicknameSet != nil {
		r.nicknameSet.Close()
		r.nicknameSet = nil
	}
}

func (r *Reviewer) match(set *pcre2.Set, subject string, firstOnly bool) []*GroupMatch {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if set == nil {
		return nil
	}

	if firstOnly {
		if m := set.FindFirst(subject); m != nil {
			return []*GroupMatch{m}
		}
		return nil
	}

	matches := set.FindAll(subject)
	if len(matches) == 0 {
		return nil
	}
	return matches
}

func loadSets(groups map[string][]byte) (set *pcre2.Set, nicknameSet *pcre2.Set, err error) {
	nicknameBlob, ok := groups[nicknameGroup]
	if !ok || len(nicknameBlob) == 0 {
		return nil, nil, fmt.Errorf("bundle missing %q group", nicknameGroup)
	}

	rest := make(map[string][]byte, len(groups)-1)
	for name, blob := range groups {
		if name == nicknameGroup {
			continue
		}
		if len(blob) == 0 {
			return nil, nil, fmt.Errorf("group %q: empty blob", name)
		}
		rest[name] = blob
	}
	if len(rest) == 0 {
		return nil, nil, fmt.Errorf("bundle has no groups besides %q", nicknameGroup)
	}

	set, err = pcre2.LoadGroups(rest)
	if err != nil {
		return nil, nil, err
	}

	nicknameSet, err = pcre2.LoadGroups(map[string][]byte{nicknameGroup: nicknameBlob})
	if err != nil {
		set.Close()
		return nil, nil, err
	}

	return set, nicknameSet, nil
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
