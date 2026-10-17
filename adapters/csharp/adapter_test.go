package csharp

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
func parse(t *testing.T, src string) []*domain.Comment {
	t.Helper()
	got, err := NewAdapter().Parse("a.cs", []byte(src))
	require.NoError(t, err)
	return got
}

// texts 返回所有注释记录的原文。
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
	assert.Equal(t, "csharp", NewAdapter().Language())
}

// TestCommentKinds 覆盖行注释、块注释、/// XML 文档注释（多行合并）、/** */ 文档块、连续行注释合并。
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
			src:    "namespace N {\n    // 你好\n    class A {}\n}\n",
			text:   "// 你好",
			typ:    domain.CommentTypeLine,
			parts:  1,
			symbol: "N.A",
			rng:    domain.TextRange{StartLine: 2, StartCol: 5, EndLine: 2, EndCol: 14},
		},
		{
			name:   "块注释",
			src:    "class A {\n    /* 块注释 */\n    int x;\n}\n",
			text:   "/* 块注释 */",
			typ:    domain.CommentTypeBlock,
			parts:  1,
			symbol: "A#x",
			rng:    domain.TextRange{StartLine: 2, StartCol: 5, EndLine: 2, EndCol: 20},
		},
		{
			name:   "多行 /// XML 文档注释合并为一条",
			src:    "class A {\n    /// <summary>\n    /// 计算和\n    /// </summary>\n    public int Add(int a, int b) => a + b;\n}\n",
			text:   "/// <summary>\n/// 计算和\n/// </summary>",
			typ:    domain.CommentTypeDoc,
			parts:  3,
			symbol: "A#Add",
			rng:    domain.TextRange{StartLine: 2, StartCol: 5, EndLine: 4, EndCol: 19},
		},
		{
			name:   "/** */ 文档块",
			src:    "/** 文档块 */\nstruct S {}\n",
			text:   "/** 文档块 */",
			typ:    domain.CommentTypeDoc,
			parts:  1,
			symbol: "S",
			rng:    domain.TextRange{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 17},
		},
		{
			name:   "连续 // 行注释合并为一条",
			src:    "class A {\n    // 第一行\n    // 第二行\n    //\n    // 第三行\n    void M() {}\n}\n",
			text:   "// 第一行\n// 第二行\n//\n// 第三行",
			typ:    domain.CommentTypeLine,
			parts:  4,
			symbol: "A#M",
			rng:    domain.TextRange{StartLine: 2, StartCol: 5, EndLine: 5, EndCol: 17},
		},
		{
			name:   "CRLF 换行不进入原文",
			src:    "/// 一\r\n/// 二\r\nclass A {}\r\n",
			text:   "/// 一\n/// 二",
			typ:    domain.CommentTypeDoc,
			parts:  2,
			symbol: "A",
			rng:    domain.TextRange{StartLine: 1, StartCol: 1, EndLine: 2, EndCol: 8},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := find(t, parse(t, tt.src), tt.text)
			assert.Equal(t, tt.typ, c.Type)
			assert.Equal(t, tt.parts, c.Parts)
			assert.Equal(t, tt.symbol, c.Symbol)
			assert.Equal(t, tt.rng, c.Range)
			assert.Equal(t, "csharp", c.Language)
			assert.Equal(t, "a.cs", c.File)
			assert.Empty(t, c.ID, "ID 由扫描器统一生成")
		})
	}
}

// TestDifferentMarkersNotMerged // 与 /// 标记不同，不合并。
func TestDifferentMarkersNotMerged(t *testing.T) {
	cs := parse(t, "class A {\n    // 普通\n    /// <summary>文档</summary>\n    void M() {}\n}\n")
	require.Equal(t, []string{"// 普通", "/// <summary>文档</summary>"}, texts(cs))
	assert.Equal(t, domain.CommentTypeLine, cs[0].Type)
	assert.Equal(t, domain.CommentTypeDoc, cs[1].Type)
}

// TestTrailingCommentNotMerged 行尾注释永远单独成条，不与下一行的注释合并。
func TestTrailingCommentNotMerged(t *testing.T) {
	src := "class A {\n    int x = 1; // 行尾\n    // 下一行\n    int y = 2;\n}\n"
	cs := parse(t, src)
	assert.Equal(t, []string{"// 行尾", "// 下一行"}, texts(cs))
	for _, c := range cs {
		assert.Equal(t, 1, c.Parts)
	}
	assert.Equal(t, "A#y", find(t, cs, "// 下一行").Symbol)
}

// TestStringLiteralsNotExtracted 普通、逐字、原始、插值字符串中的类注释文本不应被提取。
func TestStringLiteralsNotExtracted(t *testing.T) {
	tests := []struct {
		name string
		decl string
	}{
		{"普通字符串", `string a = "// 不是注释 /* 也不是 */";`},
		{"逐字字符串", `string b = @"// 不是 "" /* 注释 */";`},
		{"多行逐字字符串", "string c = @\"\n// 第一行\n/* 第二行 */\";"},
		{"原始字符串", `string d = """ // raw /* x */ """;`},
		{"多行原始字符串", "string e = \"\"\"\n    // 第一行\n    /// 第二行\n    \"\"\";"},
		{"插值字符串", `string f = $"{x} // no /* no */";`},
		{"逐字插值字符串", `string g = $@"{x} // no";`},
		{"字符字面量", `char h = '/';`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "class A {\n    int x;\n    " + tt.decl + "\n    // 真注释\n    int z;\n}\n"
			assert.Equal(t, []string{"// 真注释"}, texts(parse(t, src)))
		})
	}
}

// TestSymbolResolution 覆盖代表性声明的符号解析（块级命名空间）。
func TestSymbolResolution(t *testing.T) {
	src := `using System;

// 文件头
namespace Foo.Bar
{
    /// <summary>类说明</summary>
    [Serializable]
    public partial class Baz<T> : IBaz
    {
        // 字段
        private int x = 1, x2;
        // 构造器
        public Baz() { }
        // 析构器
        ~Baz() { }
        // 属性
        public int P { get; set; }
        // 带特性的方法
        [Obsolete]
        public void M(int a)
        {
            // 方法体内
            void Local() { }
        }
        // 事件字段
        public event EventHandler E;
        // 运算符
        public static Baz operator +(Baz a, Baz b) => a;
        // 索引器
        public int this[int i] => i;
        // 嵌套结构体
        public struct S
        {
            // 嵌套字段
            int f;
        }
        // 类末尾
    }
    // 接口
    interface I
    {
        // 接口方法
        void N();
    }
    // 记录
    public record R(int A);
    // 枚举
    enum Color
    {
        // 红色
        Red,
        Green
    }
    // 委托
    delegate void D();
    namespace Inner
    {
        // 嵌套命名空间中的类
        class C { }
    }
    // 命名空间末尾
}
`
	tests := []struct {
		text   string
		symbol string
	}{
		{"// 文件头", "Foo.Bar"},
		{"/// <summary>类说明</summary>", "Foo.Bar.Baz"},
		{"// 字段", "Foo.Bar.Baz#x"},
		{"// 构造器", "Foo.Bar.Baz#Baz"},
		{"// 析构器", "Foo.Bar.Baz#~Baz"},
		{"// 属性", "Foo.Bar.Baz#P"},
		{"// 带特性的方法", "Foo.Bar.Baz#M"},
		{"// 方法体内", "Foo.Bar.Baz#M"},
		{"// 事件字段", "Foo.Bar.Baz#E"},
		{"// 运算符", "Foo.Bar.Baz#operator+"},
		{"// 索引器", "Foo.Bar.Baz#this"},
		{"// 嵌套结构体", "Foo.Bar.Baz.S"},
		{"// 嵌套字段", "Foo.Bar.Baz.S#f"},
		{"// 类末尾", "Foo.Bar.Baz"},
		{"// 接口", "Foo.Bar.I"},
		{"// 接口方法", "Foo.Bar.I#N"},
		{"// 记录", "Foo.Bar.R"},
		{"// 枚举", "Foo.Bar.Color"},
		{"// 红色", "Foo.Bar.Color#Red"},
		{"// 委托", "Foo.Bar.D"},
		{"// 嵌套命名空间中的类", "Foo.Bar.Inner.C"},
		{"// 命名空间末尾", "Foo.Bar"},
	}
	cs := parse(t, src)
	require.Len(t, cs, len(tests), "实际: %q", texts(cs))
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			assert.Equal(t, tt.symbol, find(t, cs, tt.text).Symbol)
		})
	}
}

// TestFileScopedNamespace 文件级命名空间 namespace X; 之后的声明都带上命名空间前缀。
func TestFileScopedNamespace(t *testing.T) {
	src := "// 头部\nusing System;\n\nnamespace A.B;\n\n// 类\npublic class C\n{\n    // 方法\n    void M() { }\n}\n// 文件末尾\n"
	tests := []struct {
		text   string
		symbol string
	}{
		{"// 头部", ""},
		{"// 类", "A.B.C"},
		{"// 方法", "A.B.C#M"},
		{"// 文件末尾", "A.B"},
	}
	cs := parse(t, src)
	require.Len(t, cs, len(tests), "实际: %q", texts(cs))
	for _, tt := range tests {
		assert.Equal(t, tt.symbol, find(t, cs, tt.text).Symbol, tt.text)
	}
}

// ids 计算全部注释的 ID（按原文索引）。
func ids(t *testing.T, src string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, c := range parse(t, src) {
		out[c.SourceText] = utils.GenerateCommentID(c)
	}
	return out
}

// TestSymbolAndIDStability 在上方插入空行或代码不应改变符号与 ID。
func TestSymbolAndIDStability(t *testing.T) {
	head := "using System;\n\nnamespace Demo;\n\n"
	body := "/// <summary>\n/// 类说明\n/// </summary>\nclass A\n{\n    // 方法说明\n    void M()\n    {\n        // 内部\n    }\n    /** 属性 */\n    int P { get; }\n}\n"
	tests := []struct {
		name   string
		insert string
	}{
		{"插入空行", "\n\n\n"},
		{"插入代码", "class Other { void X() { } }\n\n"},
		{"插入注释与类型", "// 新增注释\nenum E { One }\n"},
	}
	before := ids(t, head+body)
	require.Len(t, before, 4)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			after := ids(t, head+tt.insert+body)
			for text, id := range before {
				assert.Equal(t, id, after[text], text)
			}
		})
	}
}

// TestParseFromFile src 为 nil 时从文件读取；文件不存在时返回错误。
func TestParseFromFile(t *testing.T) {
	_, err := NewAdapter().Parse("不存在.cs", nil)
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
				src := fmt.Sprintf("namespace N%d;\n%s// 注释 %d\nclass C%d {\n    /// 文档\n    void F() { }\n}\n", g, strings.Repeat("\n", i), i, g)
				cs, err := a.Parse("c.cs", []byte(src))
				if err != nil {
					errs <- err
					return
				}
				if len(cs) != 2 || cs[0].Symbol != fmt.Sprintf("N%d.C%d", g, g) || cs[1].Symbol != fmt.Sprintf("N%d.C%d#F", g, g) {
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
