package rust

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/CairnPack/codei18n/core/domain"
)

func parseRust(t *testing.T, src string) []*domain.Comment {
	t.Helper()
	cs, err := NewRustAdapter().Parse("lib.rs", []byte(src))
	require.NoError(t, err)
	return cs
}

func TestGrouping_Types(t *testing.T) {
	src := `//! 模块文档

/// 外部文档
fn a() {}

/** 块文档 */
fn b() {}

/*! 内部块文档 */

/* 普通块 */
fn c() {}

// 普通行
fn d() {}
`
	cs := parseRust(t, src)
	require.Len(t, cs, 6)
	want := []domain.CommentType{
		domain.CommentTypeDoc, domain.CommentTypeDoc, domain.CommentTypeDoc,
		domain.CommentTypeDoc, domain.CommentTypeBlock, domain.CommentTypeLine,
	}
	for i, c := range cs {
		assert.Equal(t, want[i], c.Type, c.SourceText)
		assert.Equal(t, "rust", c.Language)
		assert.NotEmpty(t, c.ID)
	}
	assert.Equal(t, "a", cs[1].Symbol)
	assert.Equal(t, "b", cs[2].Symbol)
}

func TestGrouping_ConsecutiveDocCommentsMerged(t *testing.T) {
	src := `/// 第一行
/// 第二行
///
/// 第三段
#[derive(Debug)]
pub struct S;
`
	cs := parseRust(t, src)
	require.Len(t, cs, 1)
	c := cs[0]
	assert.Equal(t, 4, c.Parts)
	assert.Equal(t, "/// 第一行\n/// 第二行\n///\n/// 第三段", c.SourceText)
	assert.Equal(t, domain.CommentTypeDoc, c.Type)
	assert.Equal(t, "S", c.Symbol)
	// 行注释节点包含的换行已去掉，结束坐标在最后一行
	assert.Equal(t, domain.TextRange{StartLine: 1, StartCol: 1, EndLine: 4, EndCol: 1 + len("/// 第三段")}, c.Range)
}

func TestGrouping_DifferentMarkersNotMerged(t *testing.T) {
	src := "mod m {\n    //! 内部文档\n    /// 外部文档\n    // 普通\n    fn f() {}\n}\n"
	cs := parseRust(t, src)
	require.Len(t, cs, 3)
	for _, c := range cs {
		assert.Equal(t, 1, c.PartCount())
	}
}

func TestGrouping_LineCommentsMergedAndTrailingNot(t *testing.T) {
	src := `fn main() {
    // 甲
    // 乙
    let x = 1; // 行尾一
    let y = 2; // 行尾二
    /* 块一 */
    /* 块二 */
}
`
	cs := parseRust(t, src)
	require.Len(t, cs, 5)
	assert.Equal(t, "// 甲\n// 乙", cs[0].SourceText)
	assert.Equal(t, 2, cs[0].Parts)
	assert.Equal(t, "main", cs[0].Symbol)
	for _, c := range cs[1:] {
		assert.Equal(t, 1, c.PartCount(), c.SourceText)
	}
}

func TestGrouping_EmptyCommentsDropped(t *testing.T) {
	src := "//\n\n///\n\n/* */\nfn f() {}\n"
	assert.Empty(t, parseRust(t, src))
}

func TestGrouping_StringsIgnored(t *testing.T) {
	src := "fn main() {\n    let a = \"/// 不是\";\n    let b = r\"//! 也不是 /* */\";\n    // 真注释\n}\n"
	cs := parseRust(t, src)
	require.Len(t, cs, 1)
	assert.Equal(t, "// 真注释", cs[0].SourceText)
}

func TestGrouping_SymbolStability(t *testing.T) {
	body := `/// 单条
fn a() {}

/// 多行一
/// 多行二
struct S;

impl S {
    // 方法内
    fn m(&self) {}
}
`
	before := parseRust(t, body)
	after := parseRust(t, "use std::fmt;\n\n\nconst X: u8 = 1;\n\n"+body)
	require.Len(t, before, 3)
	require.Len(t, after, 3)
	for i := range before {
		assert.Equal(t, before[i].ID, after[i].ID, before[i].SourceText)
		assert.Greater(t, after[i].Range.StartLine, before[i].Range.StartLine)
	}
}

func TestGrouping_CRLF(t *testing.T) {
	cs := parseRust(t, "/// 甲\r\n/// 乙\r\nfn f() {}\r\n")
	require.Len(t, cs, 1)
	assert.Equal(t, "/// 甲\n/// 乙", cs[0].SourceText)
	assert.Equal(t, 2, cs[0].Range.EndLine)
}

func TestQueryCompiledOnce(t *testing.T) {
	q1, err := commentQuery()
	require.NoError(t, err)
	q2, err := commentQuery()
	require.NoError(t, err)
	assert.Same(t, q1, q2)
}

func TestConcurrentAdaptersShareQuery(t *testing.T) {
	// 每个 goroutine 使用独立的 Adapter（独立 Parser），共享包级查询；配合 -race 检查
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := NewRustAdapter()
			for j := 0; j < 20; j++ {
				cs, err := a.Parse("a.rs", []byte("/// 甲\n/// 乙\nfn f() {}\n"))
				if err != nil || len(cs) != 1 || cs[0].Parts != 2 {
					t.Errorf("并发解析结果错误: %v %v", err, cs)
					return
				}
			}
		}()
	}
	wg.Wait()
}
