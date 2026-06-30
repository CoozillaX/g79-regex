#include "normalize.h"
#include "chinese_map.h"

#include <stdint.h>
#include <stdlib.h>
#include <string.h>

static uint32_t utf8_decode(
    const char *s,
    size_t *len);

static size_t utf8_encode(
    uint32_t cp,
    char *dst);

NormalizeResult normalize_subject(
    const char *subject)
{
    NormalizeResult r;
    memset(&r, 0, sizeof(r));

    if (!subject)
        return r;

    size_t srcLen = strlen(subject);

    r.text = malloc(srcLen + 1);
    r.offsets = malloc(sizeof(OffsetEntry) * (srcLen + 1));

    if (!r.text || !r.offsets) {
        normalize_free(&r);
        return r;
    }

    const char *src = subject;
    char *dst = r.text;
    OffsetEntry *off = r.offsets;
    size_t offCount = 0;

    size_t originalByte = 0;
    size_t normalizedByte = 0;

    while (*src) {

        size_t inLen;
        uint32_t cp = utf8_decode(src, &inLen);

        if (cp == 0x3000)
            cp = 0x20;
        else if ((cp - 0xFF01u) <= 93)
            cp -= 0xFEE0;

        cp = chinese_simplify(cp);

        if ((cp - 0x4E00u) <= 0x51FFu)
            cp++;

        size_t outLen = utf8_encode(cp, dst);

        for (size_t i = 0; i < outLen; i++) {
            off[offCount].normalized = normalizedByte + i;
            off[offCount].original = originalByte;
            offCount++;
        }

        dst += outLen;
        src += inLen;

        normalizedByte += outLen;
        originalByte += inLen;
    }

    *dst = '\0';

    off[offCount].normalized = normalizedByte;
    off[offCount].original = originalByte;
    offCount++;

    r.offsetCount = offCount;

    return r;
}

void normalize_free(
    NormalizeResult *r)
{
    if (!r)
        return;

    free(r->text);
    free(r->offsets);

    memset(r, 0, sizeof(*r));
}

int normalize_to_original(
    const NormalizeResult *r,
    int normalized)
{
    if (!r)
        return normalized;

    if (normalized < 0)
        return 0;

    if (normalized >= r->offsetCount)
        return r->offsets[r->offsetCount - 1].original;

    return r->offsets[normalized].original;
}

static inline uint32_t utf8_decode(
    const char *s,
    size_t *len)
{
    unsigned char c = (unsigned char)s[0];

    if (c < 0x80) {
        *len = 1;
        return c;
    }

    if ((c & 0xE0) == 0xC0) {
        *len = 2;

        return ((uint32_t)(c & 0x1F) << 6) |
               (uint32_t)(s[1] & 0x3F);
    }

    if ((c & 0xF0) == 0xE0) {
        *len = 3;

        return ((uint32_t)(c & 0x0F) << 12) |
               ((uint32_t)(s[1] & 0x3F) << 6) |
               (uint32_t)(s[2] & 0x3F);
    }

    *len = 4;

    return ((uint32_t)(c & 0x07) << 18) |
           ((uint32_t)(s[1] & 0x3F) << 12) |
           ((uint32_t)(s[2] & 0x3F) << 6) |
           (uint32_t)(s[3] & 0x3F);
}

static inline size_t utf8_encode(
    uint32_t cp,
    char *dst)
{
    if (cp < 0x80) {
        dst[0] = (char)cp;
        return 1;
    }

    if (cp < 0x800) {
        dst[0] = (char)(0xC0 | (cp >> 6));
        dst[1] = (char)(0x80 | (cp & 0x3F));
        return 2;
    }

    if (cp < 0x10000) {
        dst[0] = (char)(0xE0 | (cp >> 12));
        dst[1] = (char)(0x80 | ((cp >> 6) & 0x3F));
        dst[2] = (char)(0x80 | (cp & 0x3F));
        return 3;
    }

    dst[0] = (char)(0xF0 | (cp >> 18));
    dst[1] = (char)(0x80 | ((cp >> 12) & 0x3F));
    dst[2] = (char)(0x80 | ((cp >> 6) & 0x3F));
    dst[3] = (char)(0x80 | (cp & 0x3F));

    return 4;
}
