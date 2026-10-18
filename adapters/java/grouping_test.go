package java

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/utils"
)

func parseJava(t *testing.T, a *Adapter, src string) []*domain.Comment {
	t.Helper()
	cs, err := a.Parse("Sample.java", []byte(src))
	require.NoError(t, err)
	for _, c := range cs {
		c.ID = utils.GenerateCommentID(c)
	}
	return cs
}

func TestGrouping_ConsecutiveLineCommentsMerged(t *testing.T) {
	src := `package com.example;

public class Calc {
    // 计算两数之和
    // 支持负数
    //
    // 不检查溢出
    public int add(int a, int b) { return a + b; }
}
`
	cs := parseJava(t, NewAdapter(), src)
	require.Len(t, cs, 1)
	c := cs[0]
	assert.Equal(t, 4, c.Parts)
	assert.Equal(t, "// 计算两数之和\n// 支持负数\n//\n// 不检查溢出", c.SourceText)
	assert.Equal(t, domain.CommentTypeLine, c.Type)
	// 符号取紧邻声明的最后一条
	assert.Equal(t, "com.example.Calc#add", c.Symbol)
	assert.Equal(t, domain.TextRange{StartLine: 4, StartCol: 5, EndLine: 7, EndCol: 5 + len("// 不检查溢出")}, c.Range)
	require.Len(t, c.PartSymbols, 4)
	assert.Equal(t, "com.example.Calc#add", c.PartSymbols[3])
}

func TestGrouping_TrailingAndBlockNotMerged(t *testing.T) {
	src := `package com.example;

public class A {
    private int x = 1; // 行尾一
    private int y = 2; // 行尾二
    /* 块一 */
    /* 块二 */
    // 独占行
    private int z;
}
`
	cs := parseJava(t, NewAdapter(), src)
	require.Len(t, cs, 5)
	for _, c := range cs {
		assert.Equal(t, 1, c.PartCount(), c.SourceText)
	}
	assert.Equal(t, domain.CommentTypeBlock, cs[2].Type)
}

func TestGrouping_CommentLikeTextInStringsIgnored(t *testing.T) {
	src := `package com.example;

public class A {
    String a = "// 不是注释";
    String b = "/* 也不是 */";
    String c = "/** 更不是 */";
    // 真注释
    void m() {}
}
`
	cs := parseJava(t, NewAdapter(), src)
	require.Len(t, cs, 1)
	assert.Equal(t, "// 真注释", cs[0].SourceText)
}

func TestGrouping_Types(t *testing.T) {
	src := `package com.example;

/** 类文档 */
public class A {
    /* 块 */
    int a;
    /**/
    int b;
    /* */
    int d;
    // 行
    int c;
}
`
	cs := parseJava(t, NewAdapter(), src)
	require.Len(t, cs, 4, "/* */ 归一化为空应被丢弃")
	assert.Equal(t, domain.CommentTypeDoc, cs[0].Type)
	assert.Equal(t, domain.CommentTypeBlock, cs[1].Type)
	// "/**/" 永远不是文档注释
	assert.Equal(t, "/**/", cs[2].SourceText)
	assert.Equal(t, domain.CommentTypeBlock, cs[2].Type)
	assert.Equal(t, domain.CommentTypeLine, cs[3].Type)
}

// TestJavadoc_SymbolResolution 是 Javadoc 符号解析的回归测试：
// 类、嵌套类、方法、构造器、字段、枚举、接口上的 /** */ 都应为 doc 类型并解析到正确符号。
func TestJavadoc_SymbolResolution(t *testing.T) {
	src := `package com.example.app;

/** 外部类 */
public class Outer {
    /** 字段 */
    private int count = 0;

    /** 构造器 */
    public Outer() {}

    /**
     * 方法
     * @return 数量
     */
    public int getCount() { return count; }

    /** 嵌套类 */
    public static class Inner {
        /** 嵌套类方法 */
        void work() {}
    }

    /** 嵌套枚举 */
    enum Color { RED, GREEN }
}

/** 接口 */
interface Service {
    /** 接口方法 */
    void serve();
}

/** 顶层枚举 */
enum Level { LOW, HIGH }
`
	cs := parseJava(t, NewAdapter(), src)
	want := []string{
		"com.example.app.Outer",
		"com.example.app.Outer#count",
		"com.example.app.Outer#Outer",
		"com.example.app.Outer#getCount",
		"com.example.app.Outer.Inner",
		"com.example.app.Outer.Inner#work",
		"com.example.app.Outer.Color",
		"com.example.app.Service",
		"com.example.app.Service#serve",
		"com.example.app.Level",
	}
	require.Len(t, cs, len(want))
	for i, c := range cs {
		assert.Equal(t, domain.CommentTypeDoc, c.Type, c.SourceText)
		assert.Equal(t, want[i], c.Symbol, c.SourceText)
		assert.Equal(t, 1, c.PartCount())
	}
}

func TestGrouping_SymbolStability(t *testing.T) {
	body := `public class A {
    // 单条
    int x;

    // 多行一
    // 多行二
    void m() {}

    /** 文档 */
    A() {}
}
`
	before := parseJava(t, NewAdapter(), "package p;\n\n"+body)
	after := parseJava(t, NewAdapter(), "package p;\n\nimport java.util.List;\nimport java.util.Map;\n\n\n"+body)
	require.Len(t, before, 3)
	require.Len(t, after, 3)
	for i := range before {
		assert.Equal(t, before[i].ID, after[i].ID, before[i].SourceText)
		assert.Greater(t, after[i].Range.StartLine, before[i].Range.StartLine)
	}
}

func TestGrouping_LanguageParameter(t *testing.T) {
	// NewAdapter("groovy") 仍用 Java 语法解析，但上报 groovy
	a := NewAdapter("groovy")
	assert.Equal(t, "groovy", a.Language())
	cs := parseJava(t, a, "class A {\n    // 甲\n    // 乙\n    void m() {}\n}\n")
	require.Len(t, cs, 1)
	assert.Equal(t, "groovy", cs[0].Language)
	assert.Equal(t, 2, cs[0].Parts)
}

func TestGrouping_CRLF(t *testing.T) {
	cs := parseJava(t, NewAdapter(), "class A {\r\n    // 甲\r\n    // 乙\r\n    void m() {}\r\n}\r\n")
	require.Len(t, cs, 1)
	assert.Equal(t, "// 甲\n// 乙", cs[0].SourceText)
}

func TestQueryCompiledOnceAndParserReused(t *testing.T) {
	q1, err := commentQuery()
	require.NoError(t, err)
	q2, err := commentQuery()
	require.NoError(t, err)
	assert.Same(t, q1, q2)

	// 同一 Adapter 连续解析多个文件，结果互不影响
	a := NewAdapter()
	for i := 0; i < 3; i++ {
		require.Len(t, parseJava(t, a, "class A {\n    // 注释\n    void m() {}\n}\n"), 1)
	}
}

func TestConcurrentAdaptersShareQuery(t *testing.T) {
	// 每个 goroutine 使用独立的 Adapter（独立 Parser），共享包级查询；配合 -race 检查
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := NewAdapter()
			for j := 0; j < 20; j++ {
				cs, err := a.Parse("A.java", []byte("class A {\n    // 甲\n    // 乙\n    void m() {}\n}\n"))
				if err != nil || len(cs) != 1 || cs[0].Parts != 2 {
					t.Errorf("并发解析结果错误: %v %v", err, cs)
					return
				}
			}
		}()
	}
	wg.Wait()
}
