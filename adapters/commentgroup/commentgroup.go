// Package commentgroup 实现所有语言统一的"物理注释 → 注释记录"规则：
// 连续行注释合并、空注释丢弃、注释类型判定。规则见 docs/index-format.md §6.1 与 §6.3。
//
// 各语言 adapter 只负责从语法树中找出每一条物理注释（Raw）并解析其符号，然后调用 Build。
package commentgroup

import (
	"sort"
	"strings"

	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/index"
)

// Raw 是一条物理注释：一个 //... 行注释，或一个 /* ... */ 块注释。
type Raw struct {
	// Text 是注释原文，不含行尾换行符。
	Text string
	// StartLine/StartCol/EndLine/EndCol：1 起始，列为字节偏移，EndCol 左闭右开。
	StartLine, StartCol, EndLine, EndCol int
	// Symbol 是该物理注释所属的语义符号路径（按 adapter 原有规则解析）。
	Symbol string
}

// Options 描述一种语言的注释类型规则。
type Options struct {
	// Language 写入 domain.Comment.Language。
	Language string
	// DocLineMarkers 是该语言视为文档注释的行注释标记，例如 Rust 的 "///"、"//!"，C#/Dart 的 "///"。
	DocLineMarkers []string
	// DocBlockPrefixes 是该语言视为文档注释的块注释前缀，例如 "/**"、"/*!"。"/**/" 永远不是文档注释。
	DocBlockPrefixes []string
}

// Marker 返回行注释的标记：开头连续的 '/' 加紧随其后的一个可选 '!'。块注释返回 ""。
func Marker(text string) string {
	if !strings.HasPrefix(text, "//") {
		return ""
	}
	i := 0
	for i < len(text) && text[i] == '/' {
		i++
	}
	if i < len(text) && text[i] == '!' {
		i++
	}
	return text[:i]
}

// Build 按统一规则把物理注释合并为注释记录。src 用于判断注释是否独占一行。
// 返回的注释按位置排序，ID 未设置（由扫描器统一生成）。
func Build(src []byte, file string, raws []Raw, opts Options) []*domain.Comment {
	sorted := append([]Raw(nil), raws...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].StartLine != sorted[j].StartLine {
			return sorted[i].StartLine < sorted[j].StartLine
		}
		return sorted[i].StartCol < sorted[j].StartCol
	})
	lines := index.Lines(src)
	ownLine := func(r Raw) bool {
		if r.StartLine < 1 || r.StartLine > len(lines) {
			return false
		}
		line := lines[r.StartLine-1]
		end := r.StartCol - 1
		if end > len(line) {
			end = len(line)
		}
		return strings.TrimSpace(string(line[:end])) == ""
	}

	var out []*domain.Comment
	for i := 0; i < len(sorted); {
		group := []Raw{sorted[i]}
		marker := Marker(sorted[i].Text)
		if marker != "" && ownLine(sorted[i]) {
			for j := i + 1; j < len(sorted); j++ {
				prev, next := group[len(group)-1], sorted[j]
				if Marker(next.Text) != marker || next.StartLine != prev.EndLine+1 ||
					next.StartCol != group[0].StartCol || !ownLine(next) {
					break
				}
				group = append(group, next)
			}
		}
		i += len(group)
		if c := build(file, group, marker, opts); c != nil {
			out = append(out, c)
		}
	}
	return out
}

func build(file string, group []Raw, marker string, opts Options) *domain.Comment {
	texts := make([]string, len(group))
	symbols := make([]string, len(group))
	for k, r := range group {
		texts[k] = r.Text
		symbols[k] = r.Symbol
	}
	text := strings.Join(texts, "\n")
	if index.Normalize(text) == "" {
		return nil
	}
	first, last := group[0], group[len(group)-1]
	c := &domain.Comment{
		File:     file,
		Language: opts.Language,
		Symbol:   last.Symbol, // 紧邻声明的最后一条决定符号
		Range: domain.TextRange{
			StartLine: first.StartLine, StartCol: first.StartCol,
			EndLine: last.EndLine, EndCol: last.EndCol,
		},
		SourceText:  text,
		Type:        commentType(first.Text, marker, opts),
		Parts:       len(group),
		PartSymbols: symbols,
		PartTexts:   texts,
	}
	return c
}

func commentType(text, marker string, opts Options) domain.CommentType {
	if marker != "" {
		for _, m := range opts.DocLineMarkers {
			if marker == m {
				return domain.CommentTypeDoc
			}
		}
		return domain.CommentTypeLine
	}
	if strings.HasPrefix(text, "/**/") {
		return domain.CommentTypeBlock
	}
	for _, p := range opts.DocBlockPrefixes {
		if strings.HasPrefix(text, p) {
			return domain.CommentTypeDoc
		}
	}
	return domain.CommentTypeBlock
}

// TrimLineEnd 去掉行注释原文末尾的 \r\n / \n（部分 tree-sitter 语法会把换行算进注释节点），
// 并相应修正结束坐标。
func TrimLineEnd(r Raw) Raw {
	trimmed := strings.TrimRight(r.Text, "\r\n")
	if trimmed == r.Text {
		return r
	}
	r.Text = trimmed
	if !strings.Contains(trimmed, "\n") {
		r.EndLine = r.StartLine
		r.EndCol = r.StartCol + len(trimmed)
	}
	return r
}
