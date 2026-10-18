// Package dart 实现 Dart 语言的注释提取 adapter。
//
// 语法来自 adapters/dart/grammar（tree-sitter-dart 的 cgo 绑定）。adapter 只负责找出每条
// 物理注释并解析其符号，合并与类型判定统一交给 commentgroup.Build。
package dart

import (
	"context"
	"os"
	"strings"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/CairnPack/codei18n/adapters/commentgroup"
	"github.com/CairnPack/codei18n/adapters/dart/grammar"
	"github.com/CairnPack/codei18n/core/domain"
)

// options 是 Dart 的注释类型规则（docs/index-format.md §6.3）：///、/** */ 为文档注释。
var options = commentgroup.Options{
	Language:         "dart",
	DocLineMarkers:   []string{"///"},
	DocBlockPrefixes: []string{"/**"},
}

// 注释查询在包级只编译一次。TSQuery 编译后只读，可被多个 QueryCursor 并发使用。
var (
	queryOnce sync.Once
	query     *sitter.Query
	queryErr  error
)

func commentQuery() (*sitter.Query, error) {
	queryOnce.Do(func() {
		query, queryErr = sitter.NewQuery([]byte(dartCommentQuery), grammar.GetLanguage())
	})
	return query, queryErr
}

// Adapter 是 Dart 语言的 LanguageAdapter 实现。
// 每个实例持有一个 tree-sitter Parser；Parser 本身不能并发使用，因此用互斥锁保护，
// 需要并行解析时请创建多个实例。
type Adapter struct {
	mu     sync.Mutex
	parser *sitter.Parser
}

// NewAdapter 创建 Dart adapter 并加载 Dart 语法。
func NewAdapter() *Adapter {
	p := sitter.NewParser()
	p.SetLanguage(grammar.GetLanguage())
	return &Adapter{parser: p}
}

// Language 返回语言标识 "dart"。
func (a *Adapter) Language() string {
	return "dart"
}

// Parse 解析 Dart 源码并提取注释。src 为 nil 时从 file 读取。
// 返回的注释已按 commentgroup 规则合并，ID 未设置。
func (a *Adapter) Parse(file string, src []byte) ([]*domain.Comment, error) {
	if src == nil {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		src = data
	}
	q, err := commentQuery()
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	tree, err := a.parser.ParseCtx(context.Background(), nil, src)
	a.mu.Unlock()
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	lib := libraryName(root, src)

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
			raws = append(raws, rawOf(c.Node, src, lib))
		}
	}
	return commentgroup.Build(src, file, raws, options), nil
}

// rawOf 把一个注释节点转换为物理注释。注释节点带有匿名子节点（"//"、"/*" 等），
// 这里只使用节点自身的字节范围。
func rawOf(n *sitter.Node, src []byte, lib string) commentgroup.Raw {
	sp, ep := n.StartPoint(), n.EndPoint()
	r := commentgroup.Raw{
		Text:      n.Content(src),
		StartLine: int(sp.Row) + 1,
		StartCol:  int(sp.Column) + 1,
		EndLine:   int(ep.Row) + 1,
		EndCol:    int(ep.Column) + 1,
		Symbol:    resolveSymbol(n, src, lib),
	}
	if strings.HasPrefix(r.Text, "//") {
		// CRLF 文件中行注释节点会带上行尾的 \r
		r = commentgroup.TrimLineEnd(r)
	}
	return r
}
