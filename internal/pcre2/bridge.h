#pragma once

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct RegexCode RegexCode;
typedef struct RegexLibrary RegexLibrary;
typedef struct RegexSet RegexSet;

typedef struct
{
    int group;
    int index;
    int start;
    int end;
} SetMatchResult;

/*
 * Deserialize a serialized PCRE2 bundle into a library (one group). The library
 * is owned by C; add it to a set with bridge_set_add (then freed via
 * bridge_set_free) or release it directly with bridge_library_free.
 */
RegexLibrary *bridge_deserialize(
    const unsigned char *blob,
    size_t len);

void bridge_library_free(
    RegexLibrary *lib);

/* Set: several pattern groups (one library per group) held together. */

RegexSet *bridge_set_new(
    size_t capacity);

/*
 * Add a library to the set; the set takes ownership of it
 * (later freed together by bridge_set_free).
 * Returns the group index on success, or -1 on failure.
 */
int bridge_set_add(
    RegexSet *set,
    RegexLibrary *lib);

/* Total pattern count across all groups (used to size the result buffer). */
size_t bridge_set_total_codes(
    RegexSet *set);

void bridge_set_free(
    RegexSet *set);

/*
 * Match the subject against every pattern of every group in a
 * single call. The subject is normalized once and the whole loop
 * runs inside C, so no matter how many groups or patterns there
 * are, the Go side only pays for one cgo crossing.
 *
 * Each SetMatchResult carries the index of the group it matched.
 * This function does not mutate the set and is safe to call from
 * multiple threads concurrently.
 *
 * results is allocated by the caller; its capacity must be >=
 * bridge_set_total_codes().
 * Returns the number of matches, or -1 on error.
 */
int bridge_set_find_all(
    RegexSet *set,
    const char *subject,
    SetMatchResult *results,
    size_t capacity);

/*
 * Like bridge_set_find_all but stops at the first match. The subject
 * is normalized once and matching short-circuits as soon as any
 * pattern in any group hits, which is the common case for a pass/fail
 * content check.
 *
 * On a hit, fills *result and returns 1. Returns 0 if nothing matched,
 * or -1 on error. Safe to call from multiple threads concurrently.
 */
int bridge_set_find_first(
    RegexSet *set,
    const char *subject,
    SetMatchResult *result);

#ifdef __cplusplus
}
#endif