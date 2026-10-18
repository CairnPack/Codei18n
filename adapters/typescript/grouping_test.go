package typescript

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/CairnPack/codei18n/core/domain"
	"github.com/CairnPack/codei18n/core/utils"
)

func parseFile(t *testing.T, a *Adapter, file, src string) []*domain.Comment {
	t.Helper()
	cs, err := a.Parse(file, []byte(src))
	require.NoError(t, err)
	for _, c := range cs {
		c.ID = utils.GenerateCommentID(c)
	}
	return cs
}

func TestGrouping_Types(t *testing.T) {
	src := `/// <reference path="./types.d.ts" />
/** 文档 */
function a() {}
/* 块 */
function b() {}
// 行
function c() {}
`
	cs := parseFile(t, NewAdapter(), "a.ts", src)
	require.Len(t, cs, 4)
	// TS 的 /// 三斜线指令不是文档注释
	assert.Equal(t, domain.CommentTypeLine, cs[0].Type)
	assert.Equal(t, domain.CommentTypeDoc, cs[1].Type)
	assert.Equal(t, "a", cs[1].Symbol)
	assert.Equal(t, domain.CommentTypeBlock, cs[2].Type)
	assert.Equal(t, domain.CommentTypeLine, cs[3].Type)
}

func TestGrouping_ConsecutiveLineCommentsMerged(t *testing.T) {
	src := `class Calc {
  // 计算两数之和
  // 支持负数
  //
  // 不检查溢出
  add(a: number, b: number): number { return a + b; }
}
`
	cs := parseFile(t, NewAdapter(), "calc.ts", src)
	require.Len(t, cs, 1)
	c := cs[0]
	assert.Equal(t, 4, c.Parts)
	assert.Equal(t, "// 计算两数之和\n// 支持负数\n//\n// 不检查溢出", c.SourceText)
	assert.Equal(t, "Calc.add", c.Symbol)
	assert.Equal(t, domain.TextRange{StartLine: 2, StartCol: 3, EndLine: 5, EndCol: 3 + len("// 不检查溢出")}, c.Range)
}

func TestGrouping_TrailingAndBlockNotMerged(t *testing.T) {
	src := `const x = 1; // 行尾一
const y = 2; // 行尾二
/* 块一 */
/* 块二 */
// 独占行
const z = 3;
`
	cs := parseFile(t, NewAdapter(), "a.ts", src)
	require.Len(t, cs, 5)
	for _, c := range cs {
		assert.Equal(t, 1, c.PartCount(), c.SourceText)
	}
}

func TestGrouping_StringsIgnored(t *testing.T) {
	src := "const a = \"// 不是注释\";\nconst b = '/* 也不是 */';\nconst c = `/** 更不是 */`;\n// 真注释\nconst d = 1;\n"
	cs := parseFile(t, NewAdapter(), "a.ts", src)
	require.Len(t, cs, 1)
	assert.Equal(t, "// 真注释", cs[0].SourceText)
}

func TestGrouping_SymbolStability(t *testing.T) {
	body := `// 单条
function a() {}

// 多行一
// 多行二
class C {
  /** 方法文档 */
  m() {}
}
`
	before := parseFile(t, NewAdapter(), "a.ts", body)
	after := parseFile(t, NewAdapter(), "a.ts", "import { x } from './x';\n\n\nconst y = x;\n\n"+body)
	require.Len(t, before, 3)
	require.Len(t, after, 3)
	for i := range before {
		assert.Equal(t, before[i].ID, after[i].ID, before[i].SourceText)
		assert.Greater(t, after[i].Range.StartLine, before[i].Range.StartLine)
	}
}

// TestExtensions_GrammarAndLanguage 检查各扩展名使用的语法，以及 Comment.Language 一律为 "typescript"
// （保证已有注释 ID 不变；index.json 的精确语言标识由调用方按扩展名推导）。
func TestExtensions_GrammarAndLanguage(t *testing.T) {
	cases := map[string]*grammar{
		"a.ts": grammarTypeScript, "a.mts": grammarTypeScript, "a.cts": grammarTypeScript,
		"a.tsx": grammarTSX,
		"a.js":  grammarJavaScript, "a.mjs": grammarJavaScript, "a.cjs": grammarJavaScript, "a.jsx": grammarJavaScript,
		"A.TS": grammarTypeScript,
	}
	for file, want := range cases {
		g, err := getGrammar(file)
		require.NoError(t, err, file)
		assert.Same(t, want, g, file)
		lang, err := getLanguage(file)
		require.NoError(t, err, file)
		assert.NotNil(t, lang, file)

		cs := parseFile(t, NewAdapter(), file, "// 注释一\n// 注释二\nfunction f() {}\n")
		require.Len(t, cs, 1, file)
		assert.Equal(t, "typescript", cs[0].Language, file)
		assert.Equal(t, 2, cs[0].Parts, file)
		assert.Equal(t, "f", cs[0].Symbol, file)
	}
	_, err := getGrammar("a.py")
	assert.Error(t, err)
}

func TestExtensions_SyntaxSpecificGrammars(t *testing.T) {
	a := NewAdapter()
	// .mts 需要 TypeScript 语法（类型注解）
	cs := parseFile(t, a, "a.mts", "// 加法\nexport function add(a: number, b: number): number { return a + b; }\n")
	require.Len(t, cs, 1)
	assert.Equal(t, "add", cs[0].Symbol)
	// .tsx 需要 TSX 语法
	cs = parseFile(t, a, "a.tsx", "// 组件\nconst App = (p: Props) => <div>{p.x}</div>;\n")
	require.Len(t, cs, 1)
	assert.Equal(t, "App", cs[0].Symbol)
	// .cjs 使用 JavaScript 语法
	cs = parseFile(t, a, "a.cjs", "// 导出\nmodule.exports = function f() {};\n")
	require.Len(t, cs, 1)
}

func TestParserPerAdapterAndQueryOnce(t *testing.T) {
	a := NewAdapter()
	parseFile(t, a, "a.ts", "// x\nconst a = 1;\n")
	p := a.parsers[grammarTypeScript]
	require.NotNil(t, p)
	parseFile(t, a, "b.js", "// x\nconst a = 1;\n")
	parseFile(t, a, "c.ts", "// x\nconst a = 1;\n")
	assert.Same(t, p, a.parsers[grammarTypeScript], "同一 Adapter 应复用同一语法的 Parser")
	assert.Len(t, a.parsers, 2, "Parser 按语法懒加载")

	q1, err := grammarTypeScript.commentQuery()
	require.NoError(t, err)
	q2, err := grammarTypeScript.commentQuery()
	require.NoError(t, err)
	assert.Same(t, q1, q2)
}

func TestConcurrentAdaptersShareQuery(t *testing.T) {
	// 每个 goroutine 使用独立的 Adapter（独立 Parser），共享包级查询；配合 -race 检查
	files := []string{"a.ts", "a.tsx", "a.js", "a.mjs"}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := NewAdapter()
			for j := 0; j < 20; j++ {
				cs, err := a.Parse(files[(i+j)%len(files)], []byte("// 甲\n// 乙\nfunction f() {}\n"))
				if err != nil || len(cs) != 1 || cs[0].Parts != 2 {
					t.Errorf("并发解析结果错误: %v %v", err, cs)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}
