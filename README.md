# g79-regex

English | [简体中文](./README.zh-CN.md)

A content-moderation library for Go, backed by [PCRE2](https://github.com/PCRE2Project/pcre2) with JIT enabled. The rule bundle is extracted from **NetEase Minecraft BE**.

## Requirements

- Go 1.26+
- A C compiler (clang/gcc) on `PATH` with **cgo enabled**

## Install

```bash
go get github.com/CoozillaX/g79-regex
```

## Usage

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/CoozillaX/g79-regex/review"
)

func main() {
	r, err := review.New(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	text := "text to check"

	// Default: all groups except nickname; subject = level/channel/content
	for _, m := range r.ReviewWord(text, nil) {
		fmt.Printf(
			"matched: [%s, %d] %s (%d - %d)\n",
			m.Group, m.Index, m.Text, m.Start, m.End,
		)
	}

	// Nickname: true uses the nickname group only and matches raw content
	for _, m := range r.ReviewWord(text, &review.Options{Nickname: true}) {
		fmt.Printf(
			"nickname: [%s, %d] %s (%d - %d)\n",
			m.Group, m.Index, m.Text, m.Start, m.End,
		)
	}

	if hits := r.ReviewWord(text, &review.Options{FirstOnly: true}); len(hits) > 0 {
		hit := hits[0]
		fmt.Printf(
			"first hit: [%s, %d] %s (%d - %d)\n",
			hit.Group, hit.Index, hit.Text, hit.Start, hit.End,
		)
	}
}
```

## API

| Method | Description |
| --- | --- |
| `review.New(ctx)` | Download, decrypt, and load rules; returns a ready `*Reviewer`. |
| `(*Reviewer) ReviewWord(content, opts)` | Check text. `opts` may be `nil` (general review, all matches); returns `nil` on no hit. |
| `(*Reviewer) Reload(ctx)` | Re-fetch and atomically swap the rule set; returns `false` when unchanged. |
| `(*Reviewer) Close()` | Release underlying resources. |

### `Options` parameters

The second argument to `ReviewWord` is `*Options`. Pass `nil` to use the defaults.

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `Level` | `string` | `"0"` | Expression level; a few rules are gated on this field. Ignored when `Nickname` is true. |
| `Channel` | `string` | `"item_comment"` | Source scenario; different channels activate different rules. Ignored when `Nickname` is true. |
| `Nickname` | `bool` | `false` | When `true`, use the nickname group only and match raw content (no level/channel prefix). |
| `FirstOnly` | `bool` | `false` | When `true`, stop at the first match; still returns a slice. |

#### Common `Channel` values

| Value | Description |
| --- | --- |
| `item_comment` | Default; use for general text checks. |
| `check_long_numbers` | Also check whether the text contains long numbers. |
| `World` | World chat; the game sets `level` to `1` when this channel is used. |

## License

Released under the [MIT License](./LICENSE); free for anyone to use this project as long as
the copyright notice and license text are retained.

The vendored PCRE2 (under `internal/pcre2/cpcre2/`) is distributed under the BSD
license; see [`internal/pcre2/cpcre2/LICENCE`](./internal/pcre2/cpcre2/LICENCE).
