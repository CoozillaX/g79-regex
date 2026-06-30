// Package cpcre2 vendors the PCRE2 C library (pinned to upstream 10.40) and
// compiles it through cgo. The 27 pcre2_*.c files in this directory are built
// as separate translation units (matching the upstream CMake build); headers,
// the include-only JIT sources, and sljit live under include/ so cgo does not
// try to compile them standalone.
//
// This package exposes no Go API. It exists solely so the PCRE2 object code is
// produced and linked into the final binary; the parent pcre2 package calls the
// resulting C symbols. See LICENCE for the upstream license and README.md for
// the vendoring layout and update procedure.
package cpcre2

/*
#cgo CFLAGS: -I${SRCDIR}/include -DHAVE_CONFIG_H -DPCRE2_CODE_UNIT_WIDTH=8 -DPCRE2_STATIC
*/
import "C"
