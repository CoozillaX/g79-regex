#include "chinese_map.h"

#include <stdint.h>
#include <stddef.h>

typedef struct
{
    uint32_t from;
    uint32_t to;
} ChineseMap;

#include "chinese_table.h"

uint32_t chinese_simplify(uint32_t cp)
{
    size_t left = 0;
    size_t right = gCount;

    while (left < right)
    {
        size_t mid = left + ((right - left) >> 1);

        if (gMap[mid].from < cp)
        {
            left = mid + 1;
        }
        else
        {
            right = mid;
        }
    }

    if (left < gCount && gMap[left].from == cp)
        return gMap[left].to;

    return cp;
}
