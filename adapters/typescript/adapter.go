package typescript

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/CairnPack/codei18n/adapters/commentgroup"
	"github.com/CairnPack/codei18n/core/domain"
)

// commentLanguage 是写入 Comment.Language 的语言标识。
//
// 注意：所有扩展名（.ts/.tsx/.mts/.cts/.js/.jsx/.mjs/.cjs）一律使用 "typescript"，与改造前完全一致。
// 注释 ID 的计算包含 Comment.Language（见 core/utils/id.go），如果改成 "javascript"/"tsx" 等精确标识，
// 已有 JS/TSX 文件中所有注释的 ID 都会变化，mappings.json 里的译文将全部失联。
// index.json 需要的精确语言标识（typescript/tsx/javascript/jsx）由调用方根据文件扩展名推导，
// 不依赖 Comment.Language。
const commentLanguage = "typescript"

// tsOptions 是 TypeScript/JavaScript 的注释类型规则（docs/index-format.md §6.3）：
// `/** */`（JSDoc/TSDoc）为 doc，`/* */` 为 block，`//` 与 `///` 为 line
// （TS 的 `///` 三斜线指令不是文档注释，因此不设置 DocLineMarkers）。
var tsOptions = commentgroup.Options{
	Language:         commentLanguage,
	DocBlockPrefixes: []string{"/**"},
}

// grammar 描述一种 tree-sitter 语法及其注释查询。
// 查询按语法在包级只编译一次（sync.Once），所有 Adapter 与所有 Parse 调用共享；
// tree-sitter 的 Query 编译后只读，可以被多个 QueryCursor 并发使用。
type grammar struct {
	name     string
	language func() *sitter.Language

	once  sync.Once
	query *sitter.Query
	err   error
}

// commentQuery 返回该语法编译好的注释查询。
func (g *grammar) commentQuery() (*sitter.Query, error) {
	g.once.Do(func() {
		g.query, g.err = sitter.NewQuery([]byte(queryTS), g.language())
		if g.err != nil {
			g.err = fmt.Errorf("编译 %s 注释查询失败: %w", g.name, g.err)
		}
	})
	return g.query, g.err
}

var (
	grammarTypeScript = &grammar{name: "typescript", language: typescript.GetLanguage}
	grammarTSX        = &grammar{name: "tsx", language: tsx.GetLanguage}
	// smacker 的 javascript 语法本身支持 JSX，因此 .jsx 也用它解析。
	grammarJavaScript = &grammar{name: "javascript", language: javascript.GetLanguage}
)

// Adapter 实现 TypeScript/JavaScript 的 LanguageAdapter。
// 每个 Adapter 按语法懒加载并持有自己的 *sitter.Parser，跨 Parse 调用复用；
// Adapter 不是并发安全的，不要在多个 goroutine 间共享。
type Adapter struct {
	parsers map[*grammar]*sitter.Parser
}

// NewAdapter 创建 TypeScript/JavaScript adapter。
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Language 返回语言标识 "typescript"。
func (a *Adapter) Language() string {
	return commentLanguage
}

// Parse 解析源码并提取注释，按扩展名选择语法；
// 连续的独占行 // 注释按 commentgroup 的统一规则合并为一条记录。
func (a *Adapter) Parse(file string, src []byte) ([]*domain.Comment, error) {
	g, err := getGrammar(file)
	if err != nil {
		return nil, err
	}

	tree, err := a.parser(g).ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}
	defer tree.Close()

	raws, err := collectRaws(tree.RootNode(), src, g)
	if err != nil {
		return nil, err
	}
	return commentgroup.Build(src, file, raws, tsOptions), nil
}

// parser 返回本 Adapter 用于语法 g 的 Parser，首次使用时创建。
func (a *Adapter) parser(g *grammar) *sitter.Parser {
	if p, ok := a.parsers[g]; ok {
		return p
	}
	if a.parsers == nil {
		a.parsers = make(map[*grammar]*sitter.Parser)
	}
	p := sitter.NewParser()
	p.SetLanguage(g.language())
	a.parsers[g] = p
	return p
}

// getGrammar 根据扩展名选择语法：
// .ts/.mts/.cts 用 typescript 语法，.tsx 用 tsx 语法，.js/.mjs/.cjs/.jsx 用 javascript 语法。
func getGrammar(filename string) (*grammar, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".ts", ".mts", ".cts":
		return grammarTypeScript, nil
	case ".tsx":
		return grammarTSX, nil
	case ".js", ".mjs", ".cjs", ".jsx":
		return grammarJavaScript, nil
	default:
		return nil, fmt.Errorf("unsupported file extension for typescript adapter: %s", ext)
	}
}

// getLanguage 返回文件扩展名对应的 tree-sitter 语法。
func getLanguage(filename string) (*sitter.Language, error) {
	g, err := getGrammar(filename)
	if err != nil {
		return nil, err
	}
	return g.language(), nil
}

// collectRaws 从语法树中收集每一条物理注释及其符号（符号规则与合并改造前一致）。
func collectRaws(root *sitter.Node, src []byte, g *grammar) ([]commentgroup.Raw, error) {
	q, err := g.commentQuery()
	if err != nil {
		return nil, err
	}

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
				Symbol:    resolveSymbol(node, src),
			}))
		}
	}
	return raws, nil
}
