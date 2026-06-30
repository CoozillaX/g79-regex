#define PCRE2_CODE_UNIT_WIDTH 8

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <pcre2.h>

#include "normalize.h"
#include "bridge.h"

struct RegexCode {
    pcre2_code *code;
};

struct RegexLibrary {
    size_t count;
    RegexCode **codes;
};

struct RegexSet {
    size_t count;
    size_t capacity;
    RegexLibrary **groups;
};

/* PCRE2 serialize header */
typedef struct {
    uint32_t magic;
    uint32_t version;
    uint32_t config;
    int32_t number_of_codes;
} SerializedHeader;

RegexLibrary *bridge_deserialize(
    const unsigned char *blob,
    size_t len)
{
    (void)len;

    if (!blob)
        return NULL;

    const SerializedHeader *hdr =
        (const SerializedHeader *)blob;

    if (hdr->number_of_codes <= 0)
        return NULL;

    size_t count = (size_t)hdr->number_of_codes;

    pcre2_code **decoded =
        malloc(sizeof(pcre2_code *) * count);

    if (!decoded)
        return NULL;

    int rc =
        pcre2_serialize_decode(
            decoded,
            (int32_t)count,
            blob,
            NULL);

    if (rc < 0) {
        free(decoded);
        return NULL;
    }

    RegexLibrary *lib =
        calloc(1, sizeof(*lib));

    lib->count = count;

    lib->codes =
        calloc(count, sizeof(RegexCode*));

    for (size_t i = 0; i < count; i++) {

        lib->codes[i] =
            calloc(1, sizeof(RegexCode));

        lib->codes[i]->code =
            decoded[i];

        /*
         * Enable JIT. A failure here is harmless: pcre2_match
         * automatically falls back to the interpreter.
         */
        pcre2_jit_compile(
            decoded[i],
            PCRE2_JIT_COMPLETE);
    }

    /*
     * decoded is just the array of pointers returned by
     * the decoder, not the regex objects themselves.
     */
    free(decoded);

    return lib;
}

void bridge_library_free(
    RegexLibrary *lib)
{
    if (!lib)
        return;

    for (size_t i = 0; i < lib->count; i++) {

        if (!lib->codes[i])
            continue;

        if (lib->codes[i]->code)
            pcre2_code_free(
                lib->codes[i]->code);

        free(lib->codes[i]);
    }

    free(lib->codes);

    free(lib);
}

RegexSet *bridge_set_new(
    size_t capacity)
{
    if (capacity == 0)
        capacity = 1;

    RegexSet *set =
        calloc(1, sizeof(*set));

    if (!set)
        return NULL;

    set->groups =
        calloc(capacity, sizeof(RegexLibrary *));

    if (!set->groups) {
        free(set);
        return NULL;
    }

    set->capacity = capacity;
    set->count = 0;

    return set;
}

int bridge_set_add(
    RegexSet *set,
    RegexLibrary *lib)
{
    if (!set || !lib)
        return -1;

    if (set->count >= set->capacity)
        return -1;

    int index = (int)set->count;

    set->groups[set->count] = lib;
    set->count++;

    return index;
}

size_t bridge_set_total_codes(
    RegexSet *set)
{
    if (!set)
        return 0;

    size_t total = 0;

    for (size_t g = 0; g < set->count; g++) {

        if (set->groups[g])
            total += set->groups[g]->count;
    }

    return total;
}

void bridge_set_free(
    RegexSet *set)
{
    if (!set)
        return;

    for (size_t g = 0; g < set->count; g++)
        bridge_library_free(set->groups[g]);

    free(set->groups);
    free(set);
}

int bridge_set_find_range(
    RegexSet *set,
    const char *subject,
    size_t code_start,
    size_t code_count,
    SetMatchResult *results,
    size_t capacity)
{
    if (!set || !subject || !results)
        return -1;

    /* Normalize the subject once; shared by every pattern in this range. */
    NormalizeResult norm =
        normalize_subject(subject);

    if (!norm.text)
        return -1;

    size_t subjectLen = strlen(norm.text);

    pcre2_match_data *match =
        pcre2_match_data_create(1, NULL);

    if (!match) {
        normalize_free(&norm);
        return -1;
    }

    size_t code_end = code_start + code_count;
    size_t globalIdx = 0;
    size_t found = 0;

    for (size_t g = 0;
         g < set->count && globalIdx < code_end && found < capacity;
         g++) {

        RegexLibrary *lib = set->groups[g];

        if (!lib)
            continue;

        /* Skip whole groups that fall entirely before the range. */
        if (globalIdx + lib->count <= code_start) {
            globalIdx += lib->count;
            continue;
        }

        for (size_t i = 0;
             i < lib->count && globalIdx < code_end && found < capacity;
             i++, globalIdx++) {

            if (globalIdx < code_start)
                continue;

            RegexCode *rc = lib->codes[i];

            if (!rc || !rc->code)
                continue;

            int n =
                pcre2_match(
                    rc->code,
                    (PCRE2_SPTR)norm.text,
                    subjectLen,
                    0,
                    0,
                    match,
                    NULL);

            if (n < 0)
                continue;

            PCRE2_SIZE *ov =
                pcre2_get_ovector_pointer(match);

            results[found].group = (int)g;
            results[found].index = (int)i;

            results[found].start =
                normalize_to_original(
                    &norm,
                    (int)ov[0]);

            results[found].end =
                normalize_to_original(
                    &norm,
                    (int)ov[1]);

            found++;
        }
    }

    pcre2_match_data_free(match);
    normalize_free(&norm);

    return (int)found;
}

int bridge_set_find_first(
    RegexSet *set,
    const char *subject,
    SetMatchResult *result)
{
    if (!set || !subject || !result)
        return -1;

    /* Normalize the subject once; shared by every group. */
    NormalizeResult norm =
        normalize_subject(subject);

    if (!norm.text)
        return -1;

    size_t subjectLen = strlen(norm.text);

    pcre2_match_data *match =
        pcre2_match_data_create(1, NULL);

    if (!match) {
        normalize_free(&norm);
        return -1;
    }

    int found = 0;

    for (size_t g = 0; g < set->count && !found; g++) {

        RegexLibrary *lib = set->groups[g];

        if (!lib)
            continue;

        for (size_t i = 0; i < lib->count; i++) {

            RegexCode *rc = lib->codes[i];

            if (!rc || !rc->code)
                continue;

            int n =
                pcre2_match(
                    rc->code,
                    (PCRE2_SPTR)norm.text,
                    subjectLen,
                    0,
                    0,
                    match,
                    NULL);

            if (n < 0)
                continue;

            PCRE2_SIZE *ov =
                pcre2_get_ovector_pointer(match);

            result->group = (int)g;
            result->index = (int)i;

            result->start =
                normalize_to_original(
                    &norm,
                    (int)ov[0]);

            result->end =
                normalize_to_original(
                    &norm,
                    (int)ov[1]);

            found = 1;
            break;
        }
    }

    pcre2_match_data_free(match);
    normalize_free(&norm);

    return found;
}