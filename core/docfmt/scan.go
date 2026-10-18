package docfmt

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// span 是正文中一段受保护片段的字节区间 [start, end)。
type span struct{ start, end int }

// matcher 尝试在 s[i:] 处识别一个受保护片段，成功时返回片段结束位置（> i）。
type matcher func(s string, i int) (int, bool)

// matchers 返回各格式按优先级排列的识别器。先命中者生效，因此"整段保护"的结构（代码块、<pre>）排在前面，
// 其内部的 {@code}、标签等不会再被单独切分。
func matchers(f Format) []matcher {
	switch f {
	case Javadoc:
		return []matcher{matchPlaceholderChar, matchFence, javadocTag, matchHTMLComment,
			htmlContent("pre", "code", "tt", "samp", "kbd"), matchMarkupTag, matchInlineTag,
			matchBacktick, matchURL, matchEntity}
	case KDoc, Dartdoc:
		return []matcher{matchPlaceholderChar, matchFence, kdocTag, matchBacktick, matchInlineTag,
			matchBracketLink, matchHTMLComment, htmlContent("pre", "code"), matchMarkupTag,
			matchURL, matchEntity}
	case JSDoc:
		return []matcher{matchPlaceholderChar, matchFence, jsdocTag, matchBacktick, matchBraces,
			matchHTMLComment, htmlContent("pre", "code"), matchMarkupTag, matchURL, matchEntity}
	case CSharpXML:
		return []matcher{matchPlaceholderChar, matchHTMLComment, matchCDATA, htmlContent("c", "code"),
			matchMarkupTag, matchEntity, matchURL}
	default:
		return []matcher{matchPlaceholderChar}
	}
}

// protect 扫描正文，返回按出现顺序排列、互不重叠且已合并的受保护片段。
//
// 合并规则：两个片段之间只隔着空格/制表符（或紧挨着）时合并为一个片段，从而减少占位符数量，
// 例如 "@param {string} name"、"{@link A} {@link B}"。跨行的片段不合并，以保留行结构。
func protect(f Format, s string) []span {
	ms := matchers(f)
	var spans []span
	for i := 0; i < len(s); {
		matched := false
		for _, m := range ms {
			if end, ok := m(s, i); ok && end > i {
				spans = append(spans, span{i, end})
				i = end
				matched = true
				break
			}
		}
		if !matched {
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
	}
	var merged []span
	for _, sp := range spans {
		if n := len(merged); n > 0 && onlyHSpace(s[merged[n-1].end:sp.start]) {
			merged[n-1].end = sp.end
			continue
		}
		merged = append(merged, sp)
	}
	return merged
}

func onlyHSpace(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isHSpace(s[i]) {
			return false
		}
	}
	return true
}

// atLineStart 判断 i 之前（本行内）是否只有空白。
func atLineStart(s string, i int) bool {
	j := i
	for j > 0 && isHSpace(s[j-1]) {
		j--
	}
	return j == 0 || s[j-1] == '\n'
}

// lineEnd 返回 i 所在行的结束位置（换行符处或串尾）。
func lineEnd(s string, i int) int {
	if k := strings.IndexByte(s[i:], '\n'); k >= 0 {
		return i + k
	}
	return len(s)
}

func skipHSpace(s string, i int) int {
	for i < len(s) && isHSpace(s[i]) {
		i++
	}
	return i
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func isIdent(c byte) bool { return isLetter(c) || c >= '0' && c <= '9' || c == '_' }

// balanced 返回从 s[i]（必须是 open）开始到匹配的 close 之后的位置；不配对时返回 -1。
func balanced(s string, i int, open, close byte) int {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return j + 1
			}
		}
	}
	return -1
}

// matchPlaceholderChar 保护原文中出现的 ⟦ ⟧ 字符，避免与占位符混淆。
func matchPlaceholderChar(s string, i int) (int, bool) {
	for _, p := range []string{"⟦", "⟧"} {
		if strings.HasPrefix(s[i:], p) {
			return i + len(p), true
		}
	}
	return 0, false
}

// matchFence 识别 Markdown 围栏代码块（``` 或 ~~~，至少 3 个），整块保护；未闭合时保护到正文结尾。
func matchFence(s string, i int) (int, bool) {
	if !atLineStart(s, i) || i >= len(s) || (s[i] != '`' && s[i] != '~') {
		return 0, false
	}
	c := s[i]
	n := 0
	for i+n < len(s) && s[i+n] == c {
		n++
	}
	if n < 3 {
		return 0, false
	}
	le := lineEnd(s, i)
	if c == '`' && strings.IndexByte(s[i+n:le], '`') >= 0 {
		return 0, false
	}
	for pos := le; pos < len(s); {
		ls := pos + 1
		le2 := lineEnd(s, ls)
		line := s[ls:le2]
		w := leadingWS(line)
		k := 0
		for w+k < len(line) && line[w+k] == c {
			k++
		}
		if k >= n && strings.TrimSpace(line[w+k:]) == "" {
			return ls + w + k, true
		}
		pos = le2
	}
	return len(s), true
}

// matchBacktick 识别行内代码 `x` / “x“。找不到配对时把整串反引号当普通文本跳过。
func matchBacktick(s string, i int) (int, bool) {
	if s[i] != '`' {
		return 0, false
	}
	n := 0
	for i+n < len(s) && s[i+n] == '`' {
		n++
	}
	for j := i + n; j < len(s); {
		if s[j] == '`' {
			k := 0
			for j+k < len(s) && s[j+k] == '`' {
				k++
			}
			if k == n {
				return j + k, true
			}
			j += k
			continue
		}
		if strings.HasPrefix(s[j:], "\n\n") {
			break // 行内代码不跨越空行
		}
		j++
	}
	return 0, false
}

// matchInlineTag 识别 {@link …}、{@code …}、{@inheritDoc} 等行内标签（花括号配对）。
func matchInlineTag(s string, i int) (int, bool) {
	if !strings.HasPrefix(s[i:], "{@") {
		return 0, false
	}
	if e := balanced(s, i, '{', '}'); e > 0 {
		return e, true
	}
	return 0, false
}

// matchBraces 识别 JSDoc 的任意配对花括号表达式：{type}、{@link …}、{@inheritDoc}。
func matchBraces(s string, i int) (int, bool) {
	if s[i] != '{' {
		return 0, false
	}
	if e := balanced(s, i, '{', '}'); e > 0 {
		return e, true
	}
	return 0, false
}

// matchBracketLink 识别 KDoc/Dartdoc 链接：[ref]、[label][ref]、[text](url)，整体保护。
func matchBracketLink(s string, i int) (int, bool) {
	e := bracketGroup(s, i)
	if e < 0 {
		return 0, false
	}
	if e2 := bracketGroup(s, e); e2 > 0 {
		return e2, true
	}
	if e < len(s) && s[e] == '(' {
		if e2 := balanced(s, e, '(', ')'); e2 > 0 && !strings.Contains(s[e:e2], "\n") {
			return e2, true
		}
	}
	return e, true
}

// bracketGroup 识别同一行内非空的 [ … ]（不含嵌套），返回结束位置或 -1。
func bracketGroup(s string, i int) int {
	if i >= len(s) || s[i] != '[' || (i > 0 && s[i-1] == '\\') {
		return -1
	}
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case ']':
			if j == i+1 {
				return -1
			}
			return j + 1
		case '[', '\n':
			return -1
		}
	}
	return -1
}

func matchHTMLComment(s string, i int) (int, bool) {
	if !strings.HasPrefix(s[i:], "<!--") {
		return 0, false
	}
	if k := strings.Index(s[i+4:], "-->"); k >= 0 {
		return i + 4 + k + 3, true
	}
	return 0, false
}

func matchCDATA(s string, i int) (int, bool) {
	if !strings.HasPrefix(s[i:], "<![CDATA[") {
		return 0, false
	}
	if k := strings.Index(s[i:], "]]>"); k >= 0 {
		return i + k + 3, true
	}
	return 0, false
}

var (
	reMarkupTag = regexp.MustCompile(`^</?[A-Za-z][A-Za-z0-9_:.\-]*(?:\s[^<>]*)?/?>`)
	reEntity    = regexp.MustCompile(`^&(?:[A-Za-z][A-Za-z0-9]*|#[0-9]+|#[xX][0-9A-Fa-f]+);`)
	reURL       = regexp.MustCompile("^https?://[^\\s<>()\\[\\]{}\"'`]+")
)

// matchMarkupTag 识别单个 HTML/XML 标签：<p>、</summary>、<br/>、<see cref="X"/>、<param name="a"> 等。
func matchMarkupTag(s string, i int) (int, bool) {
	if s[i] != '<' {
		return 0, false
	}
	if loc := reMarkupTag.FindStringIndex(s[i:]); loc != nil {
		return i + loc[1], true
	}
	return 0, false
}

// htmlContent 返回识别"连同内容一起保护"的元素的识别器，例如 <pre>…</pre>、<code>…</code>、<c>…</c>。
// 找不到闭合标签时不命中，开标签会由 matchMarkupTag 单独保护。
func htmlContent(names ...string) matcher {
	type pair struct{ open, close *regexp.Regexp }
	var ps []pair
	for _, n := range names {
		ps = append(ps, pair{
			regexp.MustCompile(`^(?i)<` + n + `(?:\s[^<>]*)?>`),
			regexp.MustCompile(`(?i)</` + n + `\s*>`),
		})
	}
	return func(s string, i int) (int, bool) {
		if s[i] != '<' {
			return 0, false
		}
		for _, p := range ps {
			loc := p.open.FindStringIndex(s[i:])
			if loc == nil {
				continue
			}
			if strings.HasSuffix(s[i:i+loc[1]], "/>") {
				return i + loc[1], true // 自闭合
			}
			if c := p.close.FindStringIndex(s[i+loc[1]:]); c != nil {
				return i + loc[1] + c[1], true
			}
		}
		return 0, false
	}
}

func matchEntity(s string, i int) (int, bool) {
	if s[i] != '&' {
		return 0, false
	}
	if loc := reEntity.FindStringIndex(s[i:]); loc != nil {
		return i + loc[1], true
	}
	return 0, false
}

// matchURL 识别裸 URL，去掉结尾的句读符号。
func matchURL(s string, i int) (int, bool) {
	if s[i] != 'h' || (i > 0 && isIdent(s[i-1])) {
		return 0, false
	}
	loc := reURL.FindStringIndex(s[i:])
	if loc == nil {
		return 0, false
	}
	e := i + loc[1]
	for e > i && strings.IndexByte(".,;:!?", s[e-1]) >= 0 {
		e--
	}
	return e, true
}

// blockTagName 识别行首的块标签 @name，返回标签名与其后位置。
func blockTagName(s string, i int) (string, int, bool) {
	if s[i] != '@' || i+1 >= len(s) || !isLetter(s[i+1]) || !atLineStart(s, i) {
		return "", 0, false
	}
	j := i + 1
	for j < len(s) && isIdent(s[j]) {
		j++
	}
	return s[i+1 : j], j, true
}

// readToken 读取同一行内、跳过前导空白后的一个非空白记号；没有记号时返回 i 本身。
func readToken(s string, i int) int {
	j := skipHSpace(s, i)
	k := j
	for k < len(s) && !isHSpace(s[k]) && s[k] != '\n' {
		k++
	}
	if k == j {
		return i
	}
	return k
}

// readRef 读取一个引用记号，如 Foo#bar(int, String)：括号内允许空白。
// 以 '"' 或 '<' 开头的（字符串标签、HTML 链接）不算引用。
func readRef(s string, i int) int {
	j := skipHSpace(s, i)
	if j >= len(s) || s[j] == '"' || s[j] == '<' || s[j] == '{' {
		return i
	}
	k := j
	for k < len(s) && !isHSpace(s[k]) && s[k] != '\n' {
		if s[k] == '(' {
			if e := balanced(s, k, '(', ')'); e > 0 {
				k = e
				continue
			}
		}
		k++
	}
	if k == j {
		return i
	}
	return k
}

// readBracketOrToken 读取 [name]、[name=default]（方括号配对）或普通记号。
func readBracketOrToken(s string, i int, allowSpace bool) int {
	j := i
	if allowSpace {
		j = skipHSpace(s, i)
	}
	if j < len(s) && s[j] == '[' {
		if e := balanced(s, j, '[', ']'); e > 0 && !strings.Contains(s[j:e], "\n") {
			return e
		}
	}
	if !allowSpace {
		return i
	}
	return readToken(s, i)
}

func inSet(set []string, name string) bool {
	for _, x := range set {
		if x == name {
			return true
		}
	}
	return false
}

// javadocTag：@param name、@throws Type、@see ref 等保护标签与操作数，其余块标签只保护标签本身。
func javadocTag(s string, i int) (int, bool) {
	name, j, ok := blockTagName(s, i)
	if !ok {
		return 0, false
	}
	switch {
	case inSet([]string{"param", "serialField"}, name):
		k := skipHSpace(s, j)
		if k < len(s) && s[k] == '<' {
			if e := strings.IndexByte(s[k:], '>'); e >= 0 && !strings.Contains(s[k:k+e], "\n") {
				return k + e + 1, true
			}
		}
		return readToken(s, j), true
	case inSet([]string{"throws", "exception", "see", "uses", "provides"}, name):
		return readRef(s, j), true
	}
	return j, true
}

// kdocTag：KDoc/Dartdoc 的块标签，@param name / @param[name]、@property name、@throws Type、@sample ref、@see ref。
func kdocTag(s string, i int) (int, bool) {
	name, j, ok := blockTagName(s, i)
	if !ok {
		return 0, false
	}
	switch {
	case inSet([]string{"param", "property"}, name):
		if e := readBracketOrToken(s, j, false); e > j {
			return e, true
		}
		return readToken(s, j), true
	case inSet([]string{"throws", "exception", "sample", "see"}, name):
		if k := skipHSpace(s, j); k < len(s) && s[k] == '[' {
			if e := readBracketOrToken(s, j, true); e > j {
				return e, true
			}
		}
		return readRef(s, j), true
	}
	return j, true
}

// jsdocNameTags 是 JSDoc/TSDoc 中后跟名称操作数的块标签。
var jsdocNameTags = []string{
	"param", "arg", "argument", "prop", "property", "typedef", "callback", "template",
	"typeParam", "typeparam", "module", "namespace", "memberof", "memberOf", "name", "function",
	"func", "method", "event", "fires", "emits", "listens", "mixes", "alias", "augments", "extends",
	"lends", "requires",
}

// jsdocTag：@param {T} name / [name] / [name=default]、@returns {T}、@example 整块、@see ref 等。
func jsdocTag(s string, i int) (int, bool) {
	name, j, ok := blockTagName(s, i)
	if !ok {
		return 0, false
	}
	if name == "example" {
		return exampleEnd(s, i), true
	}
	// 可选的 {type}。
	if k := skipHSpace(s, j); k < len(s) && s[k] == '{' {
		if e := balanced(s, k, '{', '}'); e > 0 {
			j = e
		}
	}
	switch {
	case inSet(jsdocNameTags, name):
		return readBracketOrToken(s, j, true), true
	case name == "see":
		return readRef(s, j), true
	}
	return j, true
}

// exampleEnd 返回 @example 块的结束位置：到下一个行首块标签之前，或正文结尾；不含结尾空行。
// 块内的围栏代码块整体跳过，其中以 @ 开头的行不会被当作标签。
func exampleEnd(s string, i int) int {
	last := lineEnd(s, i)
	for pos := last; pos < len(s); {
		ls := pos + 1
		w := ls + leadingWS(s[ls:lineEnd(s, ls)])
		if w < len(s) && s[w] == '@' && w+1 < len(s) && isLetter(s[w+1]) {
			break
		}
		if e, ok := matchFence(s, w); ok {
			pos = lineEnd(s, e)
			last = pos
			continue
		}
		pos = lineEnd(s, ls)
		if strings.TrimSpace(s[ls:pos]) != "" {
			last = pos
		}
	}
	return last
}
