package docfmt

import (
	"errors"
	"math/rand/v2"
	"strings"
	"testing"
)

// TestFor 校验语言标识与注释类型到格式的映射。
func TestFor(t *testing.T) {
	cases := []struct {
		lang, typ string
		want      Format
	}{
		{"java", "doc", Javadoc},
		{"kotlin", "doc", KDoc},
		{"typescript", "doc", JSDoc},
		{"tsx", "doc", JSDoc},
		{"javascript", "doc", JSDoc},
		{"jsx", "doc", JSDoc},
		{"dart", "doc", Dartdoc},
		{"csharp", "doc", CSharpXML},
		{"go", "doc", None},
		{"rust", "doc", None},
		{"java", "line", None},
		{"java", "block", None},
		{"csharp", "line", None},
	}
	for _, c := range cases {
		if got := For(c.lang, c.typ); got != c.want {
			t.Errorf("For(%q, %q) = %q，期望 %q", c.lang, c.typ, got, c.want)
		}
	}
}

// TestPlaceholder 校验占位符的字面形式。
func TestPlaceholder(t *testing.T) {
	if got := Placeholder(0); got != "⟦0⟧" {
		t.Errorf("Placeholder(0) = %q", got)
	}
	if got := Placeholder(12); got != "⟦12⟧" {
		t.Errorf("Placeholder(12) = %q", got)
	}
}

// bodyCase 描述一条注释预期的正文与受保护片段。
type bodyCase struct {
	name      string
	f         Format
	comment   string
	body      string
	fragments []string
}

var bodyCases = []bodyCase{
	{
		name:      "Javadoc 简单",
		f:         Javadoc,
		comment:   "/**\n     * Returns the sum.\n     * @param a first {@code int}\n     */",
		body:      "Returns the sum.\n⟦0⟧ first ⟦1⟧",
		fragments: []string{"@param a", "{@code int}"},
	},
	{
		name: "Javadoc 完整",
		f:    Javadoc,
		comment: "/**\n" +
			" * Computes <b>stuff</b> fast. See {@link Foo#bar(int, String)}.\n" +
			" * <p>\n" +
			" * Example:\n" +
			" * <pre>{@code\n" +
			" *   int x = sum(1, 2);\n" +
			" * }</pre>\n" +
			" *\n" +
			" * @param <T> the type\n" +
			" * @param a the first value, e.g. <code>a &lt; b</code>\n" +
			" * @return the sum<br/>\n" +
			" * @throws IllegalArgumentException if a < 0\n" +
			" * @see Foo#bar(int, String) the bar\n" +
			" * @since 1.8\n" +
			" */",
		body: "Computes ⟦0⟧stuff⟦1⟧ fast. See ⟦2⟧.\n" +
			"⟦3⟧\n" +
			"Example:\n" +
			"⟦4⟧\n" +
			"\n" +
			"⟦5⟧ the type\n" +
			"⟦6⟧ the first value, e.g. ⟦7⟧\n" +
			"⟦8⟧ the sum⟦9⟧\n" +
			"⟦10⟧ if a < 0\n" +
			"⟦11⟧ the bar\n" +
			"⟦12⟧ 1.8",
		fragments: []string{"<b>", "</b>", "{@link Foo#bar(int, String)}", "<p>",
			"<pre>{@code\n  int x = sum(1, 2);\n}</pre>", "@param <T>", "@param a", "<code>a &lt; b</code>",
			"@return", "<br/>", "@throws IllegalArgumentException", "@see Foo#bar(int, String)", "@since"},
	},
	{
		name: "KDoc 链接/代码块/列表",
		f:    KDoc,
		comment: "/**\n" +
			" * Adds [a] and [b] using [Math.addExact][kotlin.math].\n" +
			" *\n" +
			" * ```kotlin\n" +
			" * val x = add(1, 2)\n" +
			" * ```\n" +
			" * - item `one`\n" +
			" * - item two\n" +
			" *\n" +
			" * @param a first\n" +
			" * @param[b] second\n" +
			" * @return the sum\n" +
			" * @throws ArithmeticException on overflow\n" +
			" * @sample samples.addSample\n" +
			" */",
		body: "Adds ⟦0⟧ and ⟦1⟧ using ⟦2⟧.\n\n⟦3⟧\n- item ⟦4⟧\n- item two\n\n⟦5⟧ first\n⟦6⟧ second\n⟦7⟧ the sum\n⟦8⟧ on overflow\n⟦9⟧",
		fragments: []string{"[a]", "[b]", "[Math.addExact][kotlin.math]", "```kotlin\nval x = add(1, 2)\n```",
			"`one`", "@param a", "@param[b]", "@return", "@throws ArithmeticException", "@sample samples.addSample"},
	},
	{
		name: "JSDoc 类型/可选参数/example",
		f:    JSDoc,
		comment: "/**\n" +
			" * Fetches a user. See {@link Api.get}.\n" +
			" * @param {string} id - the id\n" +
			" * @param {Object} [opts={}] options\n" +
			" * @param [retry=3] retries\n" +
			" * @returns {Promise<User>} the user\n" +
			" * @example\n" +
			" * const u = await fetchUser('1');\n" +
			" * console.log(u);\n" +
			" * @remarks Uses `fetch` internally.\n" +
			" */",
		body: "Fetches a user. See ⟦0⟧.\n⟦1⟧ - the id\n⟦2⟧ options\n⟦3⟧ retries\n⟦4⟧ the user\n⟦5⟧\n⟦6⟧ Uses ⟦7⟧ internally.",
		fragments: []string{"{@link Api.get}", "@param {string} id", "@param {Object} [opts={}]", "@param [retry=3]",
			"@returns {Promise<User>}", "@example\nconst u = await fetchUser('1');\nconsole.log(u);", "@remarks", "`fetch`"},
	},
	{
		name:      "JSDoc 单行类型",
		f:         JSDoc,
		comment:   "/** @type {Map<string, number>} cache of counts */",
		body:      "⟦0⟧ cache of counts",
		fragments: []string{"@type {Map<string, number>}"},
	},
	{
		name: "C# XML 文档注释",
		f:    CSharpXML,
		comment: "/// <summary>Adds two <see cref=\"int\"/> values.</summary>\n" +
			"/// <param name=\"a\">The first. Use <c>0</c> for none.</param>\n" +
			"/// <returns>The sum, see <paramref name=\"a\"/>.</returns>",
		body: "⟦0⟧Adds two ⟦1⟧ values.⟦2⟧\n⟦3⟧The first. Use ⟦4⟧ for none.⟦5⟧\n⟦6⟧The sum, see ⟦7⟧.⟦8⟧",
		fragments: []string{"<summary>", "<see cref=\"int\"/>", "</summary>", "<param name=\"a\">", "<c>0</c>",
			"</param>", "<returns>", "<paramref name=\"a\"/>", "</returns>"},
	},
	{
		name: "C# 多行 summary 与 code",
		f:    CSharpXML,
		comment: "/// <summary>\n" +
			"/// Parses the input.\n" +
			"/// </summary>\n" +
			"/// <example>\n" +
			"/// <code>\n" +
			"/// var x = Parse(\"1\");\n" +
			"/// </code>\n" +
			"/// </example>",
		// 跨行的相邻片段不合并，每个标签各占一个占位符。
		body:      "⟦0⟧\nParses the input.\n⟦1⟧\n⟦2⟧\n⟦3⟧\n⟦4⟧",
		fragments: []string{"<summary>", "</summary>", "<example>", "<code>\nvar x = Parse(\"1\");\n</code>", "</example>"},
	},
	{
		name:      "Dartdoc 链接",
		f:         Dartdoc,
		comment:   "/// Returns the [List] of items.\n///\n/// Throws a [StateError] if empty, see https://dart.dev/x.",
		body:      "Returns the ⟦0⟧ of items.\n\nThrows a ⟦1⟧ if empty, see ⟦2⟧.",
		fragments: []string{"[List]", "[StateError]", "https://dart.dev/x"},
	},
	{
		name:      "相邻片段合并",
		f:         Javadoc,
		comment:   "/** See {@link A} {@link B}. */",
		body:      "See ⟦0⟧.",
		fragments: []string{"{@link A} {@link B}"},
	},
}

// TestPrepareBody 校验各格式的正文与受保护片段，并确认正文中没有注释标记。
func TestPrepareBody(t *testing.T) {
	for _, c := range bodyCases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Prepare(c.f, c.comment)
			if err != nil {
				t.Fatal(err)
			}
			if p.Body() != c.body {
				t.Errorf("正文不符：\n得到 %q\n期望 %q", p.Body(), c.body)
			}
			if strings.Join(p.fragments, "|") != strings.Join(c.fragments, "|") {
				t.Errorf("片段不符：\n得到 %q\n期望 %q", p.fragments, c.fragments)
			}
			if p.Placeholders() != len(c.fragments) {
				t.Errorf("Placeholders() = %d，期望 %d", p.Placeholders(), len(c.fragments))
			}
			for _, d := range []string{"/**", "*/", "///", "\n *", "\n * "} {
				if strings.Contains(p.Body(), d) {
					t.Errorf("正文中残留注释标记 %q", d)
				}
			}
		})
	}
}

// TestPlaceholderCharMerged 校验原文中的 ⟦ ⟧ 字符各自被保护，且能原样往返。
func TestPlaceholderCharMerged(t *testing.T) {
	c := "/** a ⟦0⟧ b */"
	p, err := Prepare(None, c)
	if err != nil {
		t.Fatal(err)
	}
	if p.Placeholders() != 2 || p.Body() != "a ⟦0⟧0⟦1⟧ b" {
		t.Fatalf("正文 %q，片段 %q", p.Body(), p.fragments)
	}
	if got, err := p.Restore(p.Body()); err != nil || got != c {
		t.Fatalf("往返失败：%q %v", got, err)
	}
}

// roundTripCases 是原样往返用例：Restore(Body()) 必须逐字节还原原注释。
var roundTripCases = []struct {
	name    string
	f       Format
	comment string
}{
	{"Javadoc 方法注释", Javadoc, "/**\n     * Returns the sum.\n     * @param a first {@code int}\n     */"},
	{"Javadoc 单行", Javadoc, "/** Single line. */"},
	{"Javadoc 首行带文字", Javadoc, "/** Returns the sum.\n     *  Second line.\n     */"},
	{"Javadoc 末行带文字", Javadoc, "/**\n * First.\n * Last. */"},
	{"Javadoc 空行与行尾空白", Javadoc, "/**\n *\n * Text.   \n *\n * More.\n *\n */"},
	{"Javadoc 不规则装饰", Javadoc, "/**\n   * a\n  *  b\n *c\n   */"},
	{"Javadoc CRLF", Javadoc, "/**\r\n * Returns {@code x}.\r\n *\r\n * @return value\r\n */"},
	{"Javadoc 中文", Javadoc, "/**\n * 返回两数之和。\n * @param a 第一个数\n */"},
	{"Javadoc pre 块", Javadoc, "/**\n * Usage:\n * <pre>\n *   foo();\n *   bar();\n * </pre>\n */"},
	{"KDoc 无星号", KDoc, "/**\n    Adds two numbers.\n      Indented code\n    @param a first\n */"},
	{"KDoc 围栏代码块与列表", KDoc, "/**\n * Steps:\n * 1. first `a`\n * 2. second\n *\n * ```\n * val x = 1\n *\n * val y = 2\n * ```\n */"},
	{"JSDoc 多标签", JSDoc, "/**\n * Fetches.\n * @param {string} id the id\n * @returns {Promise<void>}\n * @example\n * ```ts\n * @decorator\n * fetch('x')\n * ```\n * @see other\n */"},
	{"JSDoc 单行 inheritDoc", JSDoc, "/** {@inheritDoc} */"},
	{"TSDoc remarks", JSDoc, "/**\n * Summary.\n *\n * @remarks\n * Details here.\n *\n * @typeParam T - element type\n */"},
	{"C# /// 组", CSharpXML, "/// <summary>Adds.</summary>\n/// <param name=\"a\">x</param>"},
	{"C# /// 含空行", CSharpXML, "/// <summary>\n/// First.\n///\n/// Second.\n/// </summary>"},
	{"C# /** */", CSharpXML, "/**\n * <summary>Adds.</summary>\n */"},
	{"Dartdoc ///", Dartdoc, "/// Returns [x].\n///\n/// ```dart\n/// var a = 1;\n/// ```"},
	{"Dartdoc CRLF", Dartdoc, "/// 第一行 [x]\r\n///\r\n/// 第二行"},
	{"Dartdoc 无空格", Dartdoc, "///Tight text\n/// normal"},
	{"空块", Javadoc, "/**/"},
	{"只有外框", Javadoc, "/**\n *\n */"},
	{"首行空 /// 组", Dartdoc, "///\n/// Text.\n///"},
}

// TestRoundTrip 校验 Restore(Body()) 原样还原。
func TestRoundTrip(t *testing.T) {
	for _, c := range roundTripCases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Prepare(c.f, c.comment)
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.Restore(p.Body())
			if err != nil {
				t.Fatal(err)
			}
			if got != c.comment {
				t.Errorf("往返不一致：\n得到 %q\n期望 %q\n正文 %q", got, c.comment, p.Body())
			}
		})
	}
	for _, c := range bodyCases {
		t.Run("正文用例/"+c.name, func(t *testing.T) {
			p, err := Prepare(c.f, c.comment)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := p.Restore(p.Body()); err != nil || got != c.comment {
				t.Errorf("往返不一致：%v\n得到 %q\n期望 %q", err, got, c.comment)
			}
		})
	}
}

// TestRestoreTranslated 用模拟译文（替换文字、保留占位符、改变行数）校验外框与缩进。
func TestRestoreTranslated(t *testing.T) {
	cases := []struct {
		name       string
		f          Format
		comment    string
		translated string
		want       string
	}{
		{
			name:       "Javadoc 行数不变",
			f:          Javadoc,
			comment:    "/**\n     * Returns the sum.\n     * @param a first {@code int}\n     */",
			translated: "返回两数之和。\n⟦0⟧ 第一个 ⟦1⟧",
			want:       "/**\n     * 返回两数之和。\n     * @param a 第一个 {@code int}\n     */",
		},
		{
			name:       "Javadoc 行数增加",
			f:          Javadoc,
			comment:    "/**\n     * Returns the sum.\n     * @param a first {@code int}\n     */",
			translated: "返回\n两数之和。\n\n⟦0⟧ 第一个 ⟦1⟧",
			want:       "/**\n     * 返回\n     * 两数之和。\n     *\n     * @param a 第一个 {@code int}\n     */",
		},
		{
			name:       "Javadoc 行数减少",
			f:          Javadoc,
			comment:    "/**\n   * Line one.\n   * Line two.\n   * Line three.\n   */",
			translated: "合并后的一行。",
			want:       "/**\n   * 合并后的一行。\n   */",
		},
		{
			name:       "单行保持单行",
			f:          Javadoc,
			comment:    "/** Returns {@code x}. */",
			translated: "返回 ⟦0⟧。",
			want:       "/** 返回 {@code x}。 */",
		},
		{
			name:       "单行变多行",
			f:          JSDoc,
			comment:    "/** Returns a value. */",
			translated: "返回\n一个值。",
			want:       "/**\n * 返回\n * 一个值。\n */",
		},
		{
			name:       "首行与末行带文字",
			f:          Javadoc,
			comment:    "/** First line.\n  * Last line. */",
			translated: "第一行。\n中间。\n最后一行。",
			want:       "/** 第一行。\n  * 中间。\n  * 最后一行。 */",
		},
		{
			name:       "/// 组行数增加且含空行",
			f:          CSharpXML,
			comment:    "/// <summary>Adds.</summary>\n/// <param name=\"a\">x</param>",
			translated: "⟦0⟧相加。\n\n更多说明。⟦1⟧\n⟦2⟧甲⟦3⟧",
			want:       "/// <summary>相加。\n///\n/// 更多说明。</summary>\n/// <param name=\"a\">甲</param>",
		},
		{
			name:       "/// 组行数减少",
			f:          Dartdoc,
			comment:    "/// Returns the [List] of items.\n///\n/// Throws if empty.",
			translated: "返回 ⟦0⟧，为空时抛出异常。",
			want:       "/// 返回 [List]，为空时抛出异常。",
		},
		{
			name:       "CRLF 保留",
			f:          Javadoc,
			comment:    "/**\r\n * Returns x.\r\n */",
			translated: "返回 x。\r\n第二行。",
			want:       "/**\r\n * 返回 x。\r\n * 第二行。\r\n */",
		},
		{
			name: "KDoc 围栏代码块在行数变化后保持",
			f:    KDoc,
			comment: "/**\n" +
				"     * Example:\n" +
				"     * ```kotlin\n" +
				"     * val x = 1\n" +
				"     *     .plus(2)\n" +
				"     * ```\n" +
				"     * - first item\n" +
				"     *   continued\n" +
				"     */",
			translated: "示例\n（说明）：\n⟦0⟧\n- 第一项\n  续行",
			want: "/**\n" +
				"     * 示例\n" +
				"     * （说明）：\n" +
				"     * ```kotlin\n" +
				"     * val x = 1\n" +
				"     *     .plus(2)\n" +
				"     * ```\n" +
				"     * - 第一项\n" +
				"     *   续行\n" +
				"     */",
		},
		{
			name:       "译文首尾多余空行与行尾空白被清理",
			f:          Javadoc,
			comment:    "/**\n * Hello.\n */",
			translated: "\n\n你好。  \n\n",
			want:       "/**\n * 你好。\n */",
		},
		{
			name:       "片段内含占位符样文本不被二次替换",
			f:          Javadoc,
			comment:    "/**\n * Use {@code ⟦1⟧} and {@code y}.\n */",
			translated: "使用 ⟦0⟧ 和 ⟦1⟧。",
			want:       "/**\n * 使用 {@code ⟦1⟧} 和 {@code y}。\n */",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Prepare(c.f, c.comment)
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.Restore(c.translated)
			if err != nil {
				t.Fatalf("Restore 失败：%v（正文 %q）", err, p.Body())
			}
			if got != c.want {
				t.Errorf("结果不符：\n得到 %q\n期望 %q", got, c.want)
			}
		})
	}
}

// TestRestoreMismatch 校验占位符缺失、多余、重复、乱序、残缺时返回 ErrPlaceholderMismatch。
func TestRestoreMismatch(t *testing.T) {
	comment := "/**\n * See {@link A}, {@code b} and <i>c</i>.\n */"
	p, err := Prepare(Javadoc, comment)
	if err != nil {
		t.Fatal(err)
	}
	if p.Body() != "See ⟦0⟧, ⟦1⟧ and ⟦2⟧c⟦3⟧." {
		t.Fatalf("正文 %q", p.Body())
	}
	bad := []struct{ name, s string }{
		{"缺少末尾", "参见 ⟦0⟧、⟦1⟧ 和 ⟦2⟧c。"},
		{"缺少中间", "参见 ⟦0⟧、⟦2⟧c⟦3⟧。"},
		{"全部缺失", "参见。"},
		{"多余", "参见 ⟦0⟧、⟦1⟧ 和 ⟦2⟧c⟦3⟧⟦4⟧。"},
		{"重复", "参见 ⟦0⟧、⟦1⟧ 和 ⟦1⟧ ⟦2⟧c⟦3⟧。"},
		{"重复首个", "⟦0⟧ 参见 ⟦0⟧、⟦1⟧ 和 ⟦2⟧c⟦3⟧。"},
		{"乱序", "参见 ⟦1⟧、⟦0⟧ 和 ⟦2⟧c⟦3⟧。"},
		{"残缺括号", "参见 ⟦0⟧、⟦1 和 ⟦2⟧c⟦3⟧。"},
		{"占位符内含空格", "参见 ⟦0⟧、⟦ 1 ⟧ 和 ⟦2⟧c⟦3⟧。"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			_, err := p.Restore(c.s)
			if !errors.Is(err, ErrPlaceholderMismatch) {
				t.Errorf("期望 ErrPlaceholderMismatch，得到 %v", err)
			}
		})
	}
	if _, err := p.Restore("参见 ⟦0⟧、⟦1⟧ 和 ⟦2⟧c⟦3⟧。"); err != nil {
		t.Errorf("合法译文不应报错：%v", err)
	}
}

// TestRandomReorder 对随机打乱/丢弃/重复占位符的译文都必须报错。
func TestRandomReorder(t *testing.T) {
	comment := "/**\n * A {@link X} b {@code y} c <i>d</i> e `f` g {@link Z} h.\n * @param q value\n * @throws E bad\n */"
	p, err := Prepare(Javadoc, comment)
	if err != nil {
		t.Fatal(err)
	}
	n := p.Placeholders()
	if n < 6 {
		t.Fatalf("占位符太少：%d（正文 %q）", n, p.Body())
	}
	// 把正文切成 文字/占位符 交替的序列，按给定顺序重新拼接占位符。
	texts := rePlaceholder.Split(p.Body(), -1)
	build := func(order []int) string {
		var b strings.Builder
		for i, k := range order {
			if i < len(texts) {
				b.WriteString(texts[i])
			}
			b.WriteString(Placeholder(k))
		}
		if len(order) < len(texts) {
			b.WriteString(strings.Join(texts[len(order):], ""))
		}
		return b.String()
	}
	identity := make([]int, n)
	for i := range identity {
		identity[i] = i
	}
	if got, err := p.Restore(build(identity)); err != nil || got != comment {
		t.Fatalf("恒等顺序应往返成功：%v %q", err, got)
	}
	r := rand.New(rand.NewPCG(1, 2))
	for iter := 0; iter < 500; iter++ {
		order := append([]int(nil), identity...)
		switch iter % 3 {
		case 0: // 打乱
			for same := true; same; {
				r.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
				same = equalInts(order, identity)
			}
		case 1: // 丢弃一个
			k := r.IntN(n)
			order = append(order[:k], order[k+1:]...)
		case 2: // 重复一个
			k := r.IntN(n)
			order = append(order[:k+1], order[k:]...)
		}
		if _, err := p.Restore(build(order)); !errors.Is(err, ErrPlaceholderMismatch) {
			t.Fatalf("顺序 %v 应报错，得到 %v", order, err)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestHasProse 校验"正文全部受保护"的判定。
func TestHasProse(t *testing.T) {
	cases := []struct {
		f       Format
		comment string
		want    bool
	}{
		{JSDoc, "/** {@inheritDoc} */", false},
		{Javadoc, "/**\n * {@inheritDoc}\n * @since 1.8\n */", false},
		{CSharpXML, "/// <inheritdoc/>", false},
		{CSharpXML, "/// <summary><see cref=\"X\"/>.</summary>", false},
		{Javadoc, "/** Hello. */", true},
		{Dartdoc, "/// 你好 [x]", true},
		{Javadoc, "/**/", false},
	}
	for _, c := range cases {
		p, err := Prepare(c.f, c.comment)
		if err != nil {
			t.Fatal(err)
		}
		if p.HasProse() != c.want {
			t.Errorf("%q: HasProse() = %v，期望 %v（正文 %q）", c.comment, p.HasProse(), c.want, p.Body())
		}
		if !c.want {
			if rest := rePlaceholder.ReplaceAllString(p.Body(), ""); strings.ContainsAny(rest, "abcdefghijklmnopqrstuvwxyz") {
				t.Errorf("%q: 无文字的正文中残留字母：%q", c.comment, p.Body())
			}
		}
	}
}

// TestPrepareErrors 校验无法识别的外框与未知格式。
func TestPrepareErrors(t *testing.T) {
	for _, c := range []string{"plain text", "/* unterminated", "/*/", "/*/ ", "# hash"} {
		if _, err := Prepare(Javadoc, c); err == nil {
			t.Errorf("%q 应返回错误", c)
		}
	}
	if _, err := Prepare(Format("rst"), "/** x */"); err == nil {
		t.Error("未知格式应返回错误")
	}
	// 原注释没有正文时不能放入译文。
	p, err := Prepare(Javadoc, "/**\n *\n */")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Restore("多出来的文字"); err == nil {
		t.Error("空注释放入译文应返回错误")
	}
}
