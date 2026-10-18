package kotlin

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/utils"
)

// parse 用新的 adapter 解析源码。
func parse(t *testing.T, file, src string) []*domain.Comment {
	t.Helper()
	got, err := NewAdapter().Parse(file, []byte(src))
	require.NoError(t, err)
	return got
}

// texts 返回所有注释记录的原文，便于断言"提取了哪些注释"。
func texts(cs []*domain.Comment) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.SourceText
	}
	return out
}

// find 按原文查找注释记录。
func find(t *testing.T, cs []*domain.Comment, text string) *domain.Comment {
	t.Helper()
	for _, c := range cs {
		if c.SourceText == text {
			return c
		}
	}
	t.Fatalf("未找到注释 %q，实际: %q", text, texts(cs))
	return nil
}

func TestLanguage(t *testing.T) {
	assert.Equal(t, "kotlin", NewAdapter().Language())
}

// TestCommentKinds 覆盖行注释、块注释、KDoc、连续行注释合并、行尾注释不合并。
func TestCommentKinds(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		text   string
		typ    domain.CommentType
		parts  int
		symbol string
		rng    domain.TextRange
	}{
		{
			name:   "行注释",
			src:    "package a\n\n// 你好\nfun f() {}\n",
			text:   "// 你好",
			typ:    domain.CommentTypeLine,
			parts:  1,
			symbol: "a#f",
			rng:    domain.TextRange{StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 10},
		},
		{
			name:   "块注释",
			src:    "class A {\n    /* 块注释 */\n    val x = 1\n}\n",
			text:   "/* 块注释 */",
			typ:    domain.CommentTypeBlock,
			parts:  1,
			symbol: "A#x",
			rng:    domain.TextRange{StartLine: 2, StartCol: 5, EndLine: 2, EndCol: 20},
		},
		{
			name:   "KDoc 文档注释",
			src:    "/**\n * 计算器\n */\nclass Calc\n",
			text:   "/**\n * 计算器\n */",
			typ:    domain.CommentTypeDoc,
			parts:  1,
			symbol: "Calc",
			rng:    domain.TextRange{StartLine: 1, StartCol: 1, EndLine: 3, EndCol: 4},
		},
		{
			name:   "空块注释 /**/ 不是文档注释",
			src:    "/**/ /* x */\nclass A\n",
			text:   "/* x */",
			typ:    domain.CommentTypeBlock,
			parts:  1,
			symbol: "A",
			rng:    domain.TextRange{StartLine: 1, StartCol: 6, EndLine: 1, EndCol: 13},
		},
		{
			name:   "连续行注释合并为一条",
			src:    "object O {\n    // 第一行\n    // 第二行\n    //\n    // 第三行\n    fun g() = 1\n}\n",
			text:   "// 第一行\n// 第二行\n//\n// 第三行",
			typ:    domain.CommentTypeLine,
			parts:  4,
			symbol: "O#g",
			rng:    domain.TextRange{StartLine: 2, StartCol: 5, EndLine: 5, EndCol: 17},
		},
		{
			name:   "CRLF 换行不进入原文",
			src:    "// 一\r\n// 二\r\nval v = 1\r\n",
			text:   "// 一\n// 二",
			typ:    domain.CommentTypeLine,
			parts:  2,
			symbol: "v",
			rng:    domain.TextRange{StartLine: 1, StartCol: 1, EndLine: 2, EndCol: 7},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := find(t, parse(t, "a.kt", tt.src), tt.text)
			assert.Equal(t, tt.typ, c.Type)
			assert.Equal(t, tt.parts, c.Parts)
			assert.Equal(t, tt.symbol, c.Symbol)
			assert.Equal(t, tt.rng, c.Range)
			assert.Equal(t, "kotlin", c.Language)
			assert.Equal(t, "a.kt", c.File)
			assert.Empty(t, c.ID, "ID 由扫描器统一生成")
		})
	}
}

// TestTrailingCommentNotMerged 行尾注释永远单独成条，不与下一行的注释合并。
func TestTrailingCommentNotMerged(t *testing.T) {
	src := "class A {\n    val x = 1 // 行尾\n    // 下一行\n    val y = 2\n}\n"
	cs := parse(t, "a.kt", src)
	assert.Equal(t, []string{"// 行尾", "// 下一行"}, texts(cs))
	for _, c := range cs {
		assert.Equal(t, 1, c.Parts)
	}
	assert.Equal(t, "A#y", find(t, cs, "// 下一行").Symbol)
}

// TestStringLiteralsNotExtracted 字符串、原始字符串、字符串模板中的类注释文本不应被提取。
func TestStringLiteralsNotExtracted(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"普通字符串", `val a = "// 不是注释 /* 也不是 */"`},
		{"原始字符串", "val b = \"\"\" // not a comment \"\"\""},
		{"多行原始字符串", "val c = \"\"\"\n    // 第一行\n    /* 第二行 */\n\"\"\""},
		{"字符串模板", `val d = "a ${"/* no */"} // b $x"`},
		{"原始字符串中的模板", "val e = \"\"\"\n    // raw ${this} /* x */\n\"\"\""},
		{"字符字面量", `val f = '/'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := tt.src + "\n// 真注释\nval z = 0\n"
			for _, ext := range []string{"a.kt", "a.kts"} {
				assert.Equal(t, []string{"// 真注释"}, texts(parse(t, ext, src)), ext)
			}
		})
	}
}

// TestSymbolResolution 覆盖代表性声明的符号解析。
func TestSymbolResolution(t *testing.T) {
	src := `package com.example.app

import kotlin.math.*

// 文件头
/** KDoc 类 */
@Suppress("x")
public data class Foo(
    // 主构造器属性
    val id: Int,
) : Bar {
    // 属性
    private var y: String = ""
    /* 带注解的方法 */
    @JvmStatic
    override fun bar(p: Int): Int {
        // 方法体内
        fun local() {}
        return p
    }
    companion object {
        // 常量
        const val K = 1
    }
    class Inner {
        // 内部方法
        fun baz() {}
    }
    init {
        // 初始化块
    }
    // 次构造器
    constructor(s: String) : this(1)
    // 扩展函数
    fun String.ext(): String = this
    // 解构
    val (m, n) = Pair(1, 2)
    // 类末尾
}
// 单例
object Single {}
// 接口
interface Api {
    // 接口方法
    fun f()
}
// 枚举
enum class Color {
    // 红色
    RED,
    GREEN
}
// 类型别名
typealias Id = Int
// 顶层属性
val topVal = 3
// 文件末尾
`
	tests := []struct {
		text   string
		symbol string
	}{
		{"// 文件头", "com.example.app.Foo"},
		{"/** KDoc 类 */", "com.example.app.Foo"},
		{"// 主构造器属性", "com.example.app.Foo#id"},
		{"// 属性", "com.example.app.Foo#y"},
		{"/* 带注解的方法 */", "com.example.app.Foo#bar"},
		{"// 方法体内", "com.example.app.Foo#bar"},
		{"// 常量", "com.example.app.Foo.Companion#K"},
		{"// 内部方法", "com.example.app.Foo.Inner#baz"},
		{"// 初始化块", "com.example.app.Foo"},
		{"// 次构造器", "com.example.app.Foo#constructor"},
		{"// 扩展函数", "com.example.app.Foo#ext"},
		{"// 解构", "com.example.app.Foo#m,n"},
		{"// 类末尾", "com.example.app.Foo"},
		{"// 单例", "com.example.app.Single"},
		{"// 接口", "com.example.app.Api"},
		{"// 接口方法", "com.example.app.Api#f"},
		{"// 枚举", "com.example.app.Color"},
		{"// 红色", "com.example.app.Color#RED"},
		{"// 类型别名", "com.example.app.Id"},
		{"// 顶层属性", "com.example.app#topVal"},
		{"// 文件末尾", ""},
	}
	cs := parse(t, "Foo.kt", src)
	require.Len(t, cs, len(tests), "实际: %q", texts(cs))
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			assert.Equal(t, tt.symbol, find(t, cs, tt.text).Symbol)
		})
	}
}

// TestScript .kts 脚本：顶层语句、函数与类。
func TestScript(t *testing.T) {
	src := "#!/usr/bin/env kotlin\n// 脚本说明\nval x = 1\n// 函数\nfun hello() = println(\"hi // no\")\n// 调用\nprintln(x)\nclass K {\n    // 脚本内类成员\n    fun m() {}\n}\n"
	cs := parse(t, "build.gradle.kts", src)
	tests := []struct {
		text   string
		symbol string
	}{
		{"// 脚本说明", "x"},
		{"// 函数", "hello"},
		{"// 调用", ""},
		{"// 脚本内类成员", "K#m"},
	}
	require.Len(t, cs, len(tests), "实际: %q", texts(cs))
	for _, tt := range tests {
		assert.Equal(t, tt.symbol, find(t, cs, tt.text).Symbol, tt.text)
	}
}

// ids 计算全部注释的 ID（按原文索引）。
func ids(t *testing.T, file, src string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, c := range parse(t, file, src) {
		out[c.SourceText] = utils.GenerateCommentID(c)
	}
	return out
}

// TestSymbolAndIDStability 在上方插入空行或代码不应改变符号与 ID。
func TestSymbolAndIDStability(t *testing.T) {
	body := "class A {\n    // 方法说明\n    fun m() {\n        // 内部\n    }\n    /** 属性 */\n    val p = 1\n}\n// 顶层\nfun top() {}\n"
	head := "package demo\n\n"
	tests := []struct {
		name   string
		insert string
	}{
		{"插入空行", "\n\n\n"},
		{"插入代码", "val inserted = 42\nfun other() {}\n\n"},
		{"插入注释与类", "// 新增注释\nclass New { fun x() {} }\n"},
	}
	before := ids(t, "a.kt", head+body)
	require.Len(t, before, 4)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			after := ids(t, "a.kt", head+tt.insert+body)
			for text, id := range before {
				assert.Equal(t, id, after[text], text)
			}
		})
	}
}

// TestParseFromFile src 为 nil 时从文件读取；文件不存在时返回错误。
func TestParseFromFile(t *testing.T) {
	_, err := NewAdapter().Parse("不存在.kt", nil)
	assert.Error(t, err)
}

// TestConcurrentAdapters 多个独立 Adapter 并发解析是安全的（配合 -race 运行）。
func TestConcurrentAdapters(t *testing.T) {
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			a := NewAdapter()
			for i := 0; i < 20; i++ {
				src := fmt.Sprintf("package p%d\n%s// 注释 %d\nclass C%d { /** 文档 */ fun f() {} }\n", g, strings.Repeat("\n", i), i, g)
				cs, err := a.Parse("c.kt", []byte(src))
				if err != nil {
					errs <- err
					return
				}
				if len(cs) != 2 || cs[0].Symbol != fmt.Sprintf("p%d.C%d", g, g) || cs[1].Symbol != fmt.Sprintf("p%d.C%d#f", g, g) {
					errs <- fmt.Errorf("结果异常: %+v", cs)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
