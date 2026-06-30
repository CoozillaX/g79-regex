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
	"sort"
	"sync"
	"unsafe"

	// Compiles and links the vendored PCRE2 C library; no Go API is used.
	_ "github.com/CoozillaX/g79-regex/internal/pcre2/cpcre2"
)

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

// FindAll matches the subject against every pattern of every group in a single
// cgo call. Each result carries the name of the group it matched. Safe for
// concurrent use.
func (s *Set) FindAll(subject string) []*GroupMatch {

	if s.ptr == nil {
		return nil
	}

	capacity := int(
		C.bridge_set_total_codes(s.ptr),
	)

	if capacity == 0 {
		return nil
	}

	cs := C.CString(subject)
	defer C.free(
		unsafe.Pointer(cs),
	)

	bufp := setBufPool.Get().(*[]C.SetMatchResult)

	buf := *bufp
	if cap(buf) < capacity {
		buf = make([]C.SetMatchResult, capacity)
	} else {
		buf = buf[:capacity]
	}

	cnt := int(
		C.bridge_set_find_all(
			s.ptr,
			cs,
			&buf[0],
			C.size_t(capacity),
		),
	)

	var out []*GroupMatch

	if cnt > 0 {

		out = make([]*GroupMatch, 0, cnt)

		for i := range cnt {

			r := buf[i]

			start := int(r.start)
			end := int(r.end)

			name := ""
			if gi := int(r.group); gi >= 0 && gi < len(s.names) {
				name = s.names[gi]
			}

			out = append(out, &GroupMatch{
				Group: name,
				Index: int(r.index),
				Start: start,
				End:   end,
				Text:  subject[start:end],
			})
		}
	}

	*bufp = buf
	setBufPool.Put(bufp)

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
