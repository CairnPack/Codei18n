package golang

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/utils"
)

func parse(t *testing.T, src string) []*domain.Comment {
	t.Helper()
	cs, err := NewAdapter().Parse("sample.go", []byte(src))
	require.NoError(t, err)
	for _, c := range cs {
		c.ID = utils.GenerateCommentID(c)
	}
	return cs
}

func TestParse_LineComment(t *testing.T) {
	cs := parse(t, "package p\n\n// 计算余额\nfunc Balance() {}\n")
	require.Len(t, cs, 1)
	c := cs[0]
	assert.Equal(t, "// 计算余额", c.SourceText)
	assert.Equal(t, domain.CommentTypeLine, c.Type)
	assert.Equal(t, "func.Balance", c.Symbol)
	assert.Equal(t, "go", c.Language)
	assert.Equal(t, 1, c.PartCount())
	// 列为 1 起始的字节偏移，EndCol 左闭右开
	assert.Equal(t, domain.TextRange{StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 1 + len("// 计算余额")}, c.Range)
}

func TestParse_BlockComment(t *testing.T) {
	cs := parse(t, "package p\n\n/* 块注释\n   第二行 */\nvar V = 1\n")
	require.Len(t, cs, 1)
	assert.Equal(t, domain.CommentTypeBlock, cs[0].Type)
	assert.Equal(t, 3, cs[0].Range.StartLine)
	assert.Equal(t, 4, cs[0].Range.EndLine)
}

func TestParse_NoDocTypeInGo(t *testing.T) {
	// Go 没有文档注释标记：/// 与 /** */ 也分别是 line 与 block
	cs := parse(t, "package p\n\n/// 三斜线\nfunc A() {}\n\n/** 星号 */\nfunc B() {}\n")
	require.Len(t, cs, 2)
	assert.Equal(t, domain.CommentTypeLine, cs[0].Type)
	assert.Equal(t, domain.CommentTypeBlock, cs[1].Type)
}

func TestParse_CommentLikeTextInStringsIgnored(t *testing.T) {
	src := "package p\n\nvar a = \"// 不是注释\"\nvar b = `/* 也不是 */`\n\n// 真注释\nvar c = 1\n"
	cs := parse(t, src)
	require.Len(t, cs, 1)
	assert.Equal(t, "// 真注释", cs[0].SourceText)
}

func TestParse_ConsecutiveLineCommentsMerged(t *testing.T) {
	src := "package p\n\n// 第一行\n// 第二行\n//\n// 第三段\nfunc F() {}\n"
	cs := parse(t, src)
	require.Len(t, cs, 1)
	c := cs[0]
	assert.Equal(t, 4, c.Parts)
	assert.Equal(t, "// 第一行\n// 第二行\n//\n// 第三段", c.SourceText)
	assert.Equal(t, "func.F", c.Symbol)
	assert.Equal(t, domain.TextRange{StartLine: 3, StartCol: 1, EndLine: 6, EndCol: 1 + len("// 第三段")}, c.Range)
	assert.Len(t, c.PartTexts, 4)
	assert.Len(t, c.PartSymbols, 4)
}

func TestParse_BlockCommentsStaySeparate(t *testing.T) {
	cs := parse(t, "package p\n\n/* 甲 */\n/* 乙 */\n// 丙\nfunc F() {}\n")
	require.Len(t, cs, 3)
	for _, c := range cs {
		assert.Equal(t, 1, c.PartCount())
	}
}

func TestParse_TrailingCommentsNotMerged(t *testing.T) {
	src := "package p\n\nfunc F() {\n\tx := 1 // 行尾一\n\ty := 2 // 行尾二\n\t// 独占行\n\t_ = x + y\n}\n"
	cs := parse(t, src)
	require.Len(t, cs, 3)
	for _, c := range cs {
		assert.Equal(t, 1, c.PartCount(), c.SourceText)
	}
}

func TestParse_IndentedGroupMerged(t *testing.T) {
	src := "package p\n\nfunc F() {\n\t// 甲\n\t// 乙\n\tprintln()\n}\n"
	cs := parse(t, src)
	require.Len(t, cs, 1)
	assert.Equal(t, 2, cs[0].Parts)
	assert.Equal(t, "// 甲\n// 乙", cs[0].SourceText)
	assert.Equal(t, 2, cs[0].Range.StartCol)
}

func TestParse_SymbolStability(t *testing.T) {
	body := "// 单条注释\nfunc F() {}\n\n// 多行注释一\n// 多行注释二\ntype T struct{}\n\n/* 块 */\nvar V = 1\n"
	before := parse(t, "package p\n\n"+body)
	after := parse(t, "package p\n\nimport \"fmt\"\n\nvar _ = fmt.Sprint\n\n\n"+body)
	require.Len(t, before, 3)
	require.Len(t, after, 3)
	for i := range before {
		assert.Equal(t, before[i].ID, after[i].ID, before[i].SourceText)
		assert.Equal(t, before[i].Symbol, after[i].Symbol)
		assert.Greater(t, after[i].Range.StartLine, before[i].Range.StartLine)
	}
}

func TestParse_SingleCommentSymbolUnchanged(t *testing.T) {
	// 单条注释的符号规则与合并改造前一致：包级注释 package.<名>，声明前注释取声明符号
	cs := parse(t, "// 包注释\npackage p\n\n// 类型\ntype T int\n\nconst (\n\t// 常量\n\tA = 1\n)\n")
	require.Len(t, cs, 3)
	got := make([]string, len(cs))
	for i, c := range cs {
		got[i] = c.Symbol
	}
	assert.Equal(t, "package.p|decl.type|var.A", strings.Join(got, "|"))
}
