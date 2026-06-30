# g79-regex

[English](./README.md) | 简体中文

基于 Golang + [PCRE2](https://github.com/PCRE2Project/pcre2) 实现的屏蔽词检测依赖库，匹配规则提取自**网易我的世界BE**

## 环境要求

- Go 1.26+
- 配置 C 编译器（clang/gcc）且 **启用 cgo**

## 安装

```bash
go get https://github.com/CoozillaX/g79-regex
```

## 使用方法

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/CoozillaX/g79-regex/review"
)

func main() {
	// 从云端拉取、解密并加载规则包。
	r, err := review.New(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	text := "待检测文本"

	// 默认返回全部命中；无命中时返回 nil。
	for _, m := range r.ReviewWord(text, nil) {
		fmt.Printf(
			"matched: [%s, %d] %s (%d - %d)\n",
			m.Group, m.Index, m.Text, m.Start, m.End,
		)
	}

	// 命中第一条即停止，仍返回切片（最多一个元素）。
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

| 方法 | 说明 |
| --- | --- |
| `review.New(ctx)` | 从云端拉取、解密并加载规则，返回就绪的 `*Reviewer`。 |
| `(*Reviewer) ReviewWord(content, opts)` | 检测文本。`opts` 可为 `nil`（默认全部匹配）；无命中返回 `nil`。 |
| `(*Reviewer) Reload(ctx)` | 重新拉取并原子替换规则集；规则未变化时返回 `false`。 |
| `(*Reviewer) Close()` | 释放底层资源。 |

### `Options` 参数

`ReviewWord` 的第二个参数为 `*Options`，传 `nil` 时使用默认值。

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `Level` | `string` | `"0"` | 表达式分级，少部分规则会按此字段过滤。 |
| `Channel` | `string` | `"item_comment"` | 文本来源场景，不同场景下命中的规则不同。 |
| `FirstOnly` | `bool` | `false` | 为 `true` 时命中第一条即停止，仍返回切片。 |

#### `Channel` 常用取值

| 值 | 说明 |
| --- | --- |
| `item_comment` | 默认场景，一般文本检测使用此值。 |
| `check_long_numbers` | 需要额外检查文本是否包含长数字时使用。 |
| `World` | 文本来源为世界聊天；游戏内此 channel 会将 `level` 设为 `1`。 |

## 许可

采用 [MIT License](./LICENSE)，任何人在保留版权声明和许可文本的前提下可自由使用此项目。

其中 vendoring 的 PCRE2（位于 `internal/pcre2/cpcre2/`）以 BSD 许可分发，详见 [`internal/pcre2/cpcre2/LICENCE`](./internal/pcre2/cpcre2/LICENCE)。
