package rust

import (
	"fmt"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/rust"

	"github.com/CairnPack/codei18n/adapters/commentgroup"
	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/utils"
)

// rustOptions 是 Rust 的注释类型规则（docs/index-format.md §6.3）：
// `///`、`//!`、`/** */`、`/*! */` 为 doc，`/* */` 为 block，`//` 为 line。
var rustOptions = commentgroup.Options{
	Language:         "rust",
	DocLineMarkers:   []string{"///", "//!"},
	DocBlockPrefixes: []string{"/**", "/*!"},
}

// 注释查询在包级只编译一次，所有 RustAdapter 与所有 Parse 调用共享。
// tree-sitter 的 Query 编译后只读，可以被多个 QueryCursor 并发使用。
var (
	queryOnce sync.Once
	query     *sitter.Query
	queryErr  error
)

// commentQuery 返回编译好的 Rust 注释查询。
func commentQuery() (*sitter.Query, error) {
	queryOnce.Do(func() {
		query, queryErr = sitter.NewQuery([]byte(rustCommentQuery), rust.GetLanguage())
		if queryErr != nil {
			queryErr = fmt.Errorf("编译 Rust 注释查询失败: %w", queryErr)
		}
	})
	return query, queryErr
}

// extractComments 收集所有物理注释，再交给 commentgroup.Build 按统一规则合并、判定类型。
// 只由标记和空白组成的注释（例如单独的 `//`）不再在这里跳过：它们可以作为段落分隔出现在合并组中间，
// 整组归一化后为空的记录由 Build 丢弃。
func (a *RustAdapter) extractComments(root *sitter.Node, src []byte, file string) ([]*domain.Comment, error) {
	raws, err := collectRaws(root, src)
	if err != nil {
		return nil, err
	}

	comments := commentgroup.Build(src, file, raws, rustOptions)
	// 保持既有行为：Rust adapter 自行填充 ID（扫描器之后会用同一函数重新计算，结果一致）
	for _, c := range comments {
		c.ID = utils.GenerateCommentID(c)
	}
	return comments, nil
}

// collectRaws 从语法树中收集每一条物理注释及其符号（符号规则与合并改造前一致）。
func collectRaws(root *sitter.Node, src []byte) ([]commentgroup.Raw, error) {
	q, err := commentQuery()
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
			owner := FindOwnerNode(node, src)
			// Rust 语法会把行注释末尾的换行算进节点，TrimLineEnd 去掉换行并修正结束坐标
			raws = append(raws, commentgroup.TrimLineEnd(commentgroup.Raw{
				Text:      node.Content(src),
				StartLine: int(node.StartPoint().Row) + 1,
				StartCol:  int(node.StartPoint().Column) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
				EndCol:    int(node.EndPoint().Column) + 1,
				Symbol:    ResolveSymbolPath(owner, src),
			}))
		}
	}
	return raws, nil
}
