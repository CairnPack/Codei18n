package langdetect

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// 本文件负责把归一化后的注释文本清洗为"只剩自然语言单词"的形式：
// 去掉代码片段、文档标签、链接、路径、像代码的标识符、数字与标点。
// 清洗只用于语言识别，绝不用于翻译或展示。

var (
	// 围栏代码块（归一化后换行已变为空格，未闭合时吃到结尾）。
	reFenced = regexp.MustCompile("(?s)```.*?(?:```|$)|~~~.*?(?:~~~|$)")
	// 行内反引号代码。
	reInlineCode = regexp.MustCompile("`[^`]*`")
	// Javadoc/KDoc/JSDoc 行内标签 {@link …}、{@code …} 等，以及 JSDoc 类型 {string} 等花括号片段。
	reBraces = regexp.MustCompile(`\{[^{}]*\}`)
	// 内容本身是代码的 XML/HTML 元素（Go 正则不支持反向引用，故逐一列出）。
	reCodeElems = regexp.MustCompile(`(?is)<c\b[^>]*>.*?</c\s*>|<code\b[^>]*>.*?</code\s*>|<pre\b[^>]*>.*?</pre\s*>|<tt\b[^>]*>.*?</tt\s*>|<kbd\b[^>]*>.*?</kbd\s*>|<samp\b[^>]*>.*?</samp\s*>|<var\b[^>]*>.*?</var\s*>`)
	// 其余 XML/HTML 标签（保留标签之间的文字，如 <summary>…</summary>）。
	reTags = regexp.MustCompile(`</?[A-Za-z][\w:.-]*(?:\s[^<>]*)?/?>`)
	// HTML 实体。
	reEntities = regexp.MustCompile(`&(?:#\d+|#x[0-9A-Fa-f]+|\w+);`)
	// Markdown 链接 [文字](地址)：保留文字。
	reMdLink = regexp.MustCompile(`\[([^\[\]]*)\]\([^()]*\)`)
	// KDoc/Dart/rustdoc 的 [ref] 引用。
	reRef = regexp.MustCompile(`\[[^\[\]]*\]`)
	// reST/Sphinx 字段 :param x: / :returns: 等。
	reRstField = regexp.MustCompile(`:(?:param|parameter|arg|argument|key|keyword|type|vartype|ivar|cvar|var|raises?|except|exception|returns?|rtype|yields?|ytype)\b[^:\s]*(?:\s+[^:\s]+)?:`)
	// URL 与邮箱。
	reURL = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://\S*|\bwww\.\S+|\b[\w.+-]+@[\w-]+\.[\w.-]+`)
)

// 需要连同下一个记号（参数名、异常类型、引用目标等）一起删除的块标签。
var nameTakingTags = map[string]bool{
	"param": true, "arg": true, "argument": true, "tparam": true, "typeparam": true,
	"throws": true, "throw": true, "exception": true, "raises": true, "raise": true,
	"property": true, "prop": true, "field": true, "member": true, "see": true,
	"sample": true, "since": true, "version": true, "author": true, "typedef": true,
	"callback": true, "template": true, "link": true, "linkplain": true, "ref": true,
	"p": true, "c": true, "a": true, "e": true, "em": true, "b": true, "class": true,
	"fn": true, "var": true, "def": true, "struct": true, "file": true, "retval": true,
	"exports": true, "module": true, "name": true, "alias": true, "memberof": true,
}

// 跨语言通用的任务标记，本身不代表任何语言。
var neutralMarkers = map[string]bool{
	"TODO": true, "FIXME": true, "XXX": true, "HACK": true, "NB": true,
}

// cleanComment 返回只含自然语言单词（以空格分隔）的文本。
// norm 应为 index.Normalize 的结果（已去掉注释符号并压缩空白）。
func cleanComment(norm string) string {
	s := norm
	s = reFenced.ReplaceAllString(s, " ")
	s = reInlineCode.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "`", " ")
	s = reCodeElems.ReplaceAllString(s, " ")
	s = reTags.ReplaceAllString(s, " ")
	s = reEntities.ReplaceAllString(s, " ")
	// 花括号可能嵌套（如 {@link Map<K, {V}>}），由内向外反复剥离。
	for i := 0; i < 4 && strings.ContainsRune(s, '{'); i++ {
		s = reBraces.ReplaceAllString(s, " ")
	}
	s = reMdLink.ReplaceAllString(s, " $1 ")
	s = reRef.ReplaceAllString(s, " ")
	s = reRstField.ReplaceAllString(s, " ")
	s = reURL.ReplaceAllString(s, " ")
	s = splitCJK(s)

	fields := strings.Fields(s)
	var b strings.Builder
	b.Grow(len(s))
	skipNext := false
	for _, tok := range fields {
		if skipNext {
			skipNext = false
			// 块标签后紧跟的参数名/类型（Doxygen 的 [in] 方向说明已被 reRef 删除）。
			continue
		}
		if r, _ := utf8.DecodeRuneInString(tok); r == '@' || r == '\\' {
			cmd := strings.ToLower(strings.TrimRightFunc(tok[1:], func(r rune) bool { return !isASCIILetter(r) }))
			if nameTakingTags[cmd] {
				skipNext = true
			}
			continue
		}
		w := keepWord(tok)
		if w == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(w)
	}
	return b.String()
}

// keepWord 判断一个记号是否是自然语言单词；是则返回去掉首尾标点后的单词，否则返回空串。
func keepWord(tok string) string {
	// 路径、链接残留、命名空间等。
	if strings.ContainsAny(tok, `/\`) {
		return ""
	}
	// 函数调用形式 foo()、foo(x)：字母或数字后紧跟左括号。
	if i := strings.IndexByte(tok, '('); i > 0 {
		if r, _ := utf8.DecodeLastRuneInString(tok[:i]); isWordRune(r) {
			return ""
		}
	}
	w := strings.TrimFunc(tok, func(r rune) bool { return !isWordRune(r) })
	if w == "" {
		return ""
	}
	if neutralMarkers[w] {
		return ""
	}
	if isCJKRun(w) {
		return w
	}
	return keepLatinLike(w)
}

// keepLatinLike 过滤空格分词文字（拉丁、西里尔、韩文等）中像代码的标识符。
func keepLatinLike(w string) string {
	var prev, prev2 rune
	letters := 0
	for i, r := range w {
		switch {
		case r == '_' || unicode.IsDigit(r):
			return "" // snake_case、SCREAMING_CASE、含数字的标识符或数字本身
		case r == '.':
			// 点分名 os.Path、util.go、v1.2（e.g. 等缩写也会被删除，影响不大）
			if i+1 < len(w) && isWordRune(prev) {
				return ""
			}
		case strings.ContainsRune("=<>{}[]#$%^&*|~+;:\"", r):
			return "" // a=b、Map<K,V>、foo::bar、x->y 等
		case unicode.IsUpper(r):
			if unicode.IsLower(prev) {
				return "" // camelCase / PascalCase 中的内部大写
			}
		case unicode.IsLower(r):
			if unicode.IsUpper(prev) && unicode.IsUpper(prev2) {
				return "" // HTTPServer 这类"缩写+单词"
			}
		}
		if unicode.IsLetter(r) {
			letters++
		}
		prev2, prev = prev, r
	}
	if letters == 0 {
		return ""
	}
	return w
}

// splitCJK 在中日韩文字与其他字符的边界插入空格，并把全角/中日韩标点替换为空格，
// 使 "调用fooBar()来初始化" 这类不分词的文本也能按记号过滤。
func splitCJK(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)
	prevCJK := false
	for i, r := range s {
		if isCJKPunct(r) {
			b.WriteByte(' ')
			prevCJK = false
			continue
		}
		cur := isCJK(r)
		if i > 0 && cur != prevCJK {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
		prevCJK = cur
	}
	return b.String()
}

// isCJK 判断是否为不以空格分词的中日文字（汉字、平假名、片假名）。
// 韩文以空格分词，按普通单词处理。
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || isKana(r)
}

func isKana(r rune) bool {
	return unicode.In(r, unicode.Hiragana, unicode.Katakana) || r == 'ー' || r == 'ｰ'
}

// isCJKPunct 判断是否为中日韩符号与标点（U+3000–U+303F）或全角标点。
func isCJKPunct(r rune) bool {
	if r >= 0x3000 && r <= 0x303F {
		return r != '々' && r != '〆' // 々、〆 属于文字
	}
	if r >= 0xFF00 && r <= 0xFFEF {
		return unicode.IsPunct(r) || unicode.IsSymbol(r) || r == 0xFF00
	}
	return false
}

func isCJKRun(w string) bool {
	for _, r := range w {
		if !isCJK(r) {
			return false
		}
	}
	return true
}

// isWordRune 判断是否属于单词内部可能出现的字符（字母、数字、下划线、撇号、连字符、点等由调用方另行判断）。
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_'
}

func isASCIILetter(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}
