package pcre2

/*
// PCRE2 itself is vendored and compiled by the cpcre2 subpackage. Here we only
// need its public header to compile bridge.c; the PCRE2 symbols are resolved at
// link time from the cpcre2 package (pulled in via the blank import below).
#cgo CFLAGS: -I${SRCDIR}/cpcre2/include -DPCRE2_CODE_UNIT_WIDTH=8 -DPCRE2_STATIC

#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"unsafe"

	// Compiles and links the vendored PCRE2 C library; no Go API is used.
	_ "github.com/CoozillaX/g79-regex/internal/pcre2/cpcre2"
)

// parallelThreshold is the minimum total pattern count at which FindAll splits
// the work across goroutines. Below it the fan-out overhead (extra normalize
// passes + goroutine scheduling) outweighs the gain, so we match serially.
const parallelThreshold = 256

// GroupBlob is one group to load: a name plus its serialized PCRE2 data.
type GroupBlob struct {
	Name string
	Blob []byte
}

// GroupMatch is a hit from Set.FindAll, annotated with the group it matched.
type GroupMatch struct {
	Group string // name of the matched group
	Index int    // pattern index within the group
	Start int
	End   int
	Text  string
}

// Set holds several pattern groups together. A single cgo call matches all
// groups at once, and the subject is normalized only once.
//
// A Set is read-only: FindAll mutates no shared state and may be called from
// multiple goroutines concurrently.
type Set struct {
	ptr   *C.RegexSet
	names []string
}

// setBufPool reuses result buffers to avoid allocating a large slice on every
// concurrent match.
var setBufPool = sync.Pool{
	New: func() any { return new([]C.SetMatchResult) },
}

// LoadSet loads several serialized groups into a Set, preserving input order.
func LoadSet(groups []GroupBlob) (*Set, error) {

	if len(groups) == 0 {
		return nil, errors.New("no groups")
	}

	cset := C.bridge_set_new(
		C.size_t(len(groups)),
	)

	if cset == nil {
		return nil, errors.New("set alloc failed")
	}

	s := &Set{
		ptr:   cset,
		names: make([]string, 0, len(groups)),
	}

	for _, g := range groups {

		if len(g.Blob) == 0 {
			C.bridge_set_free(cset)
			return nil, fmt.Errorf("group %q: empty blob", g.Name)
		}

		lib := C.bridge_deserialize(
			(*C.uchar)(unsafe.Pointer(&g.Blob[0])),
			C.size_t(len(g.Blob)),
		)

		if lib == nil {
			C.bridge_set_free(cset)
			return nil, fmt.Errorf("group %q: deserialize failed", g.Name)
		}

		if C.bridge_set_add(cset, lib) < 0 {
			C.bridge_library_free(lib)
			C.bridge_set_free(cset)
			return nil, fmt.Errorf("group %q: add failed", g.Name)
		}

		s.names = append(s.names, g.Name)
	}

	return s, nil
}

// LoadGroups loads multiple regex groups passed directly as name -> serialized
// PCRE2 blob (e.g. the decrypted filename:content pairs). Groups are loaded in
// sorted name order so group indices are deterministic across runs.
func LoadGroups(groups map[string][]byte) (*Set, error) {

	if len(groups) == 0 {
		return nil, errors.New("no groups")
	}

	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	ordered := make([]GroupBlob, 0, len(names))
	for _, name := range names {
		ordered = append(ordered, GroupBlob{
			Name: name,
			Blob: groups[name],
		})
	}

	return LoadSet(ordered)
}

// Groups returns the group names in load order.
func (s *Set) Groups() []string {
	return s.names
}

// FindAll matches the subject against every pattern of every group. Each result
// carries the name of the group it matched, and results are returned in pattern
// order (group, then index). Safe for concurrent use.
//
// For large rule sets the work is split across goroutines, each matching a slice
// of the flattened pattern range via its own cgo call; small sets match serially
// to avoid fan-out overhead.
func (s *Set) FindAll(subject string) []*GroupMatch {
	return s.findAll(subject, true)
}

// findAll implements FindAll. When parallel is false it always uses the serial
// path, which lets benchmarks compare the two without the threshold heuristic.
func (s *Set) findAll(subject string, parallel bool) []*GroupMatch {

	if s.ptr == nil {
		return nil
	}

	total := int(
		C.bridge_set_total_codes(s.ptr),
	)

	if total == 0 {
		return nil
	}

	// One shared, read-only C copy of the subject for every worker; the C side
	// only reads it (each call makes its own normalized copy internally).
	cs := C.CString(subject)
	defer C.free(
		unsafe.Pointer(cs),
	)

	workers := runtime.GOMAXPROCS(0)
	if !parallel || workers <= 1 || total < parallelThreshold {
		return s.findRange(cs, subject, 0, total, true)
	}

	if workers > total {
		workers = total
	}

	chunk := (total + workers - 1) / workers

	segments := make([][]*GroupMatch, workers)
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {

		start := w * chunk
		if start >= total {
			break
		}

		count := chunk
		if start+count > total {
			count = total - start
		}

		wg.Add(1)
		go func(idx, start, count int) {
			defer wg.Done()
			segments[idx] = s.findRange(cs, subject, start, count, false)
		}(w, start, count)
	}

	wg.Wait()

	// Concatenate in worker order, which is ascending pattern-index order, so
	// the merged result matches the serial ordering.
	var out []*GroupMatch
	for _, seg := range segments {
		out = append(out, seg...)
	}

	return out
}

// findRange matches the flattened pattern range [start, start+count) with a
// single cgo call and converts the hits to GroupMatch. cs is a shared read-only
// C copy of subject; subject is the Go string used to slice matched text. When
// pooled is true a pooled result buffer is reused (serial path); concurrent
// workers pass false and use a private buffer.
func (s *Set) findRange(cs *C.char, subject string, start, count int, pooled bool) []*GroupMatch {

	if count <= 0 {
		return nil
	}

	var buf []C.SetMatchResult
	var bufp *[]C.SetMatchResult

	if pooled {
		bufp = setBufPool.Get().(*[]C.SetMatchResult)
		buf = *bufp
		if cap(buf) < count {
			buf = make([]C.SetMatchResult, count)
		} else {
			buf = buf[:count]
		}
	} else {
		buf = make([]C.SetMatchResult, count)
	}

	cnt := int(
		C.bridge_set_find_range(
			s.ptr,
			cs,
			C.size_t(start),
			C.size_t(count),
			&buf[0],
			C.size_t(count),
		),
	)

	var out []*GroupMatch

	if cnt > 0 {

		out = make([]*GroupMatch, 0, cnt)

		for i := range cnt {

			r := buf[i]

			st := int(r.start)
			en := int(r.end)

			name := ""
			if gi := int(r.group); gi >= 0 && gi < len(s.names) {
				name = s.names[gi]
			}

			out = append(out, &GroupMatch{
				Group: name,
				Index: int(r.index),
				Start: st,
				End:   en,
				Text:  subject[st:en],
			})
		}
	}

	if pooled {
		*bufp = buf
		setBufPool.Put(bufp)
	}

	return out
}

// FindFirst matches the subject against every group but stops at the first hit,
// returning that match and true. This is the cheapest way to answer a pass/fail
// question. Safe for concurrent use.
func (s *Set) FindFirst(subject string) *GroupMatch {

	if s.ptr == nil {
		return nil
	}

	cs := C.CString(subject)
	defer C.free(
		unsafe.Pointer(cs),
	)

	var r C.SetMatchResult

	rc := int(
		C.bridge_set_find_first(
			s.ptr,
			cs,
			&r,
		),
	)

	if rc <= 0 {
		return nil
	}

	start := int(r.start)
	end := int(r.end)

	name := ""
	if gi := int(r.group); gi >= 0 && gi < len(s.names) {
		name = s.names[gi]
	}

	return &GroupMatch{
		Group: name,
		Index: int(r.index),
		Start: start,
		End:   end,
		Text:  subject[start:end],
	}
}

// Close releases all C resources held by the Set.
func (s *Set) Close() {

	if s.ptr == nil {
		return
	}

	C.bridge_set_free(s.ptr)

	s.ptr = nil
	s.names = nil
}
