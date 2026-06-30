#pragma once

#ifdef __cplusplus
extern "C" {
#endif

typedef struct
{
    int normalized;
    int original;
} OffsetEntry;

typedef struct
{
    char *text;

    OffsetEntry *offsets;

    int offsetCount;

} NormalizeResult;

/* Produce the normalized text used for PCRE2 matching. */
NormalizeResult normalize_subject(
    const char *subject);

/* Free the data returned by normalize_subject(). */
void normalize_free(
    NormalizeResult *result);

/* Map a normalized byte offset back to the original string. */
int normalize_to_original(
    const NormalizeResult *result,
    int normalizedOffset);

#ifdef __cplusplus
}
#endif