package java

import (
	"context"
	"fmt"
	"os"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"

	"github.com/CairnPack/codei18n/adapters/commentgroup"
	"github.com/CairnPack/codei18n/core/domain"
)

// 注释查询在包级只编译一次，所有 Adapter 与所有 Parse 调用共享。
// tree-sitter 的 Query 编译后只读，可以被多个 QueryCursor 并发使用；
// 每次 Parse 仍然创建自己的 QueryCursor。
var (
	queryOnce sync.Once
	query     *sitter.Query
	queryErr  error
)

// commentQuery 返回编译好的 Java 注释查询。
func commentQuery() (*sitter.Query, error) {
	queryOnce.Do(func() {
		query, queryErr = sitter.NewQuery([]byte(javaCommentQuery), java.GetLanguage())
		if queryErr != nil {
			queryErr = fmt.Errorf("编译 Java 注释查询失败: %w", queryErr)
		}
	})
	return query, queryErr
}

// Adapter 实现 Java（以及沿用 Java 语法解析的 kotlin/groovy/scala）的 LanguageAdapter。
// 使用 Tree-sitter 解析源码，提取注释及其所属符号。
// 每个 Adapter 独占一个 *sitter.Parser，不是并发安全的，不要在多个 goroutine 间共享。
type Adapter struct {
	parser   *sitter.Parser
	language string
}

// NewAdapter 创建 Java adapter。
// 可选参数 lang 指定上报的语言标识（写入 Language() 与 Comment.Language），默认 "java"；
// 无论 lang 为何，都使用 Java 语法解析。
func NewAdapter(lang ...string) *Adapter {
	p := sitter.NewParser()
	p.SetLanguage(java.GetLanguage())
	l := "java"
	if len(lang) > 0 {
		l = lang[0]
	}
	return &Adapter{parser: p, language: l}
}

// Language 返回语言标识（默认 "java"）。
func (a *Adapter) Language() string {
	return a.language
}

// options 返回本 adapter 的注释类型规则（docs/index-format.md §6.3）：
// `/** */`（Javadoc）为 doc，`/* */` 为 block，`//` 为 line。
func (a *Adapter) options() commentgroup.Options {
	return commentgroup.Options{Language: a.language, DocBlockPrefixes: []string{"/**"}}
}

// Parse 解析 Java 源码并提取注释。src 为 nil 时从 file 读取。
// 连续的独占行 // 注释按 commentgroup 的统一规则合并为一条记录。
func (a *Adapter) Parse(file string, src []byte) ([]*domain.Comment, error) {
	if src == nil {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		src = data
	}

	tree, err := a.parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	raws, err := collectRaws(tree.RootNode(), src)
	if err != nil {
		return nil, err
	}
	return commentgroup.Build(src, file, raws, a.options()), nil
}

// collectRaws 从语法树中收集每一条物理注释及其符号（符号规则与合并改造前一致）。
func collectRaws(root *sitter.Node, src []byte) ([]commentgroup.Raw, error) {
	q, err := commentQuery()
	if err != nil {
		return nil, err
	}

	packageName := extractPackageName(root, src)

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
			node := c.Node
			raws = append(raws, commentgroup.TrimLineEnd(commentgroup.Raw{
				Text:      node.Content(src),
				StartLine: int(node.StartPoint().Row) + 1,
				StartCol:  int(node.StartPoint().Column) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
				EndCol:    int(node.EndPoint().Column) + 1,
				Symbol:    resolveSymbol(node, src, packageName),
			}))
		}
	}
	return raws, nil
}
