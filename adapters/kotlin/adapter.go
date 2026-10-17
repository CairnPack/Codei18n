// Package kotlin 实现 Kotlin（.kt / .kts）的注释提取 adapter。
//
// adapter 只负责用 tree-sitter 找出每一条物理注释（line_comment / multiline_comment）
// 并解析其符号，注释合并与类型判定统一交给 commentgroup.Build。
package kotlin

import (
	"context"
	"os"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/kotlin"

	"github.com/CairnPack/codei18n/adapters/commentgroup"
	"github.com/CairnPack/codei18n/core/domain"
)

// languageID 是写入 domain.Comment.Language 的语言标识。
const languageID = "kotlin"

// commentQuery 匹配 Kotlin 的两类物理注释：// 行注释与 /* */（含 KDoc /** */）块注释。
const commentQuery = `
(line_comment) @comment
(multiline_comment) @comment
`

// options 是 Kotlin 的注释类型规则（docs/index-format.md §6.3）：/** */ 为 KDoc，没有文档行注释。
var options = commentgroup.Options{
	Language:         languageID,
	DocBlockPrefixes: []string{"/**"},
}

// 查询在包级只编译一次并在所有 Adapter 之间复用（TSQuery 编译后只读，可并发使用）。
var (
	queryOnce sync.Once
	query     *sitter.Query
	queryErr  error
)

func commentQueryInstance() (*sitter.Query, error) {
	queryOnce.Do(func() {
		query, queryErr = sitter.NewQuery([]byte(commentQuery), kotlin.GetLanguage())
	})
	return query, queryErr
}

// Adapter 是 Kotlin 语言 adapter。每个 Adapter 持有独立的 parser，
// parser 不是并发安全的，因此同一个 Adapter 不能在多个 goroutine 间共享。
type Adapter struct {
	parser *sitter.Parser
}

// NewAdapter 创建 Kotlin adapter，.kt 与 .kts（脚本）共用同一个 adapter。
func NewAdapter() *Adapter {
	p := sitter.NewParser()
	p.SetLanguage(kotlin.GetLanguage())
	return &Adapter{parser: p}
}

// Language 返回语言标识 "kotlin"。
func (a *Adapter) Language() string {
	return languageID
}

// Parse 解析 Kotlin 源码并返回注释记录（未设置 ID，由扫描器统一生成）。
// src 为 nil 时从 file 读取源码。
func (a *Adapter) Parse(file string, src []byte) ([]*domain.Comment, error) {
	if src == nil {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		src = data
	}
	q, err := commentQueryInstance()
	if err != nil {
		return nil, err
	}

	tree, err := a.parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()
	root := tree.RootNode()
	pkg := packageName(root, src)

	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(q, root)

	var raws []commentgroup.Raw
	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}
		for _, c := range m.Captures {
			n := c.Node
			raws = append(raws, commentgroup.TrimLineEnd(commentgroup.Raw{
				Text:      n.Content(src),
				StartLine: int(n.StartPoint().Row) + 1,
				StartCol:  int(n.StartPoint().Column) + 1,
				EndLine:   int(n.EndPoint().Row) + 1,
				EndCol:    int(n.EndPoint().Column) + 1,
				Symbol:    resolveSymbol(n, src, pkg),
			}))
		}
	}
	return commentgroup.Build(src, file, raws, options), nil
}
