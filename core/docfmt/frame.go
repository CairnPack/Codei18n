package docfmt

import (
	"errors"
	"strings"
)

// frameKind 区分两种注释外框。
type frameKind int

const (
	// frameBlock 是 /** ... */、/*! ... */、/* ... */ 块注释。
	frameBlock frameKind = iota
	// frameLines 是一组以 ///、//!、// 等标记开头的行注释（合并组）。
	frameLines
)

// frame 记录原注释的外框，用于 Restore 时按原样式重建。
//
// 原注释被切成若干物理行，每行分解为 prefix + text + suffix：
//   - 块注释首行的 prefix 含开头标记（"/**" 及其后空白），末行的 suffix 含结尾标记（空白 + "*/"）；
//   - 中间行的 prefix 是缩进 + 可选的 "*" + 一个可选空格（行装饰）；
//   - 行注释组每行的 prefix 是缩进 + 标记 + 一个可选空格；
//   - 其余行的 suffix 是行尾空白。
//
// text 为空的首尾若干行只属于外框（head/tail），原样保留；中间的 text 用 "\n" 连接即为正文。
type frame struct {
	kind  frameKind
	eol   string // 原注释的换行符："\n" 或 "\r\n"
	lines []string
	pre   []string
	text  []string
	suf   []string
	h     int // 正文起始行号（之前的行属于 head）
	e     int // 正文结束行号（含）；正文为空时 e < h

	opener     string // 块注释的开头标记，例如 "/**"
	defPrefix  string // 行数变化时新行使用的装饰前缀
	defEmpty   string // 行数变化时空行使用的装饰前缀
	singleLine bool   // 原注释是单行块注释 /** x */
}

var errFrame = errors.New("docfmt: 无法识别的注释外框")

// isHSpace 判断是否为行内空白。
func isHSpace(c byte) bool { return c == ' ' || c == '\t' }

// leadingWS 返回 s 开头的行内空白长度。
func leadingWS(s string) int {
	i := 0
	for i < len(s) && isHSpace(s[i]) {
		i++
	}
	return i
}

// trailingWS 返回 s 结尾的行内空白长度。
func trailingWS(s string) int {
	i := len(s)
	for i > 0 && isHSpace(s[i-1]) {
		i--
	}
	return len(s) - i
}

// parseFrame 剥离注释外框。
func parseFrame(comment string) (*frame, error) {
	fr := &frame{eol: "\n"}
	if strings.Contains(comment, "\r\n") {
		fr.eol = "\r\n"
		comment = strings.ReplaceAll(comment, "\r\n", "\n")
	}
	trimmed := comment[leadingWS(comment):]
	switch {
	case strings.HasPrefix(trimmed, "/*"):
		fr.kind = frameBlock
		if !strings.HasSuffix(strings.TrimRight(comment, " \t"), "*/") || len(trimmed) < 4 {
			return nil, errFrame
		}
	case strings.HasPrefix(trimmed, "//"):
		fr.kind = frameLines
	default:
		return nil, errFrame
	}
	fr.lines = strings.Split(comment, "\n")
	m := len(fr.lines)
	fr.pre = make([]string, m)
	fr.text = make([]string, m)
	fr.suf = make([]string, m)
	var err error
	if fr.kind == frameBlock {
		err = fr.splitBlock()
	} else {
		err = fr.splitLines()
	}
	if err != nil {
		return nil, err
	}
	fr.h = 0
	for fr.h < m && fr.text[fr.h] == "" {
		fr.h++
	}
	fr.e = m - 1
	for fr.e >= fr.h && fr.text[fr.e] == "" {
		fr.e--
	}
	fr.computeDefaults()
	return fr, nil
}

// splitBlock 分解块注释的每一行。
func (fr *frame) splitBlock() error {
	m := len(fr.lines)
	ws := leadingWS(fr.lines[0])
	rest := fr.lines[0][ws:]
	switch {
	case strings.HasPrefix(rest, "/**/"):
		fr.opener = "/*"
	case strings.HasPrefix(rest, "/**"):
		fr.opener = "/**"
	case strings.HasPrefix(rest, "/*!"):
		fr.opener = "/*!"
	default:
		fr.opener = "/*"
	}
	// 先切出每行的 suffix（末行含 "*/"），剩余部分再切 prefix。
	body := make([]string, m)
	for i, line := range fr.lines {
		if i == m-1 {
			// 末行：core + 空白 + "*/" + 行尾空白。"/**/" 的开头标记取 "/*"，不会与 "*/" 重叠。
			t := strings.TrimRight(line, " \t")
			core := strings.TrimSuffix(t, "*/")
			k := trailingWS(core)
			fr.suf[i] = line[len(core)-k:]
			body[i] = core[:len(core)-k]
		} else {
			k := trailingWS(line)
			fr.suf[i] = line[len(line)-k:]
			body[i] = line[:len(line)-k]
		}
	}
	// 首行：缩进 + 开头标记 + 空白。
	p := ws + len(fr.opener)
	if len(body[0]) < p {
		return errFrame // 开头与结尾标记重叠
	}
	p += leadingWS(body[0][p:])
	fr.pre[0], fr.text[0] = body[0][:p], body[0][p:]
	// 无星号的中间行取公共最小缩进作为 prefix，保留其后的相对缩进（Markdown 缩进代码等）。
	minIndent := -1
	for i := 1; i < m; i++ {
		b := body[i]
		w := leadingWS(b)
		r := b[w:]
		if r == "" || strings.HasPrefix(r, "*") {
			continue
		}
		if minIndent < 0 || w < minIndent {
			minIndent = w
		}
	}
	for i := 1; i < m; i++ {
		b := body[i]
		w := leadingWS(b)
		r := b[w:]
		var p int
		switch {
		case r == "":
			p = len(b)
		case strings.HasPrefix(r, "*"):
			p = w + 1
			if p < len(b) && isHSpace(b[p]) {
				p++
			}
		default:
			p = minIndent
		}
		fr.pre[i], fr.text[i] = b[:p], b[p:]
	}
	fr.singleLine = m == 1
	return nil
}

// splitLines 分解行注释组的每一行。
func (fr *frame) splitLines() error {
	for i, line := range fr.lines {
		k := trailingWS(line)
		fr.suf[i] = line[len(line)-k:]
		b := line[:len(line)-k]
		w := leadingWS(b)
		if !strings.HasPrefix(b[w:], "//") {
			return errFrame
		}
		p := w
		for p < len(b) && b[p] == '/' {
			p++
		}
		if p < len(b) && b[p] == '!' {
			p++
		}
		if p < len(b) && isHSpace(b[p]) {
			p++
		}
		fr.pre[i], fr.text[i] = b[:p], b[p:]
	}
	return nil
}

// computeDefaults 计算行数变化时使用的装饰前缀：取非空正文行中出现最多的 prefix。
func (fr *frame) computeDefaults() {
	m := len(fr.lines)
	count := map[string]int{}
	best, bestN := "", 0
	start := 0
	if fr.kind == frameBlock {
		start = 1 // 块注释首行的 prefix 含开头标记，不参与统计
	}
	for i := start; i < m; i++ {
		if fr.text[i] == "" {
			continue
		}
		count[fr.pre[i]]++
		if n := count[fr.pre[i]]; n > bestN {
			best, bestN = fr.pre[i], n
		}
	}
	if bestN == 0 {
		switch {
		case fr.kind == frameLines:
			w := leadingWS(fr.lines[0])
			mk := strings.TrimRight(fr.pre[0][w:], " \t")
			best = fr.pre[0][:w] + mk + " "
		case m == 1:
			best = " * "
		default:
			// 没有中间正文行：用结尾行的缩进 + "* "。
			best = fr.lines[m-1][:leadingWS(fr.lines[m-1])] + "* "
		}
	}
	fr.defPrefix = best
	fr.defEmpty = strings.TrimRight(best, " \t")
}

// body 返回剥离外框后的正文。
func (fr *frame) body() string {
	if fr.e < fr.h {
		return ""
	}
	return strings.Join(fr.text[fr.h:fr.e+1], "\n")
}

// rebuild 用新的正文行按原外框重建注释。lines 已去掉首尾空行和行尾空白。
func (fr *frame) rebuild(lines []string) string {
	m := len(fr.lines)
	n := fr.e - fr.h + 1
	if n <= 0 {
		// 原注释没有正文：只能原样返回（调用方已保证 lines 为空）。
		return strings.Join(fr.lines, fr.eol)
	}
	var out []string
	if len(lines) == n {
		// 行数不变：逐行套用原 prefix/suffix，保证原样往返。
		out = append(out, fr.lines[:fr.h]...)
		for i, t := range lines {
			j := fr.h + i
			out = append(out, fr.pre[j]+t+fr.suf[j])
		}
		out = append(out, fr.lines[fr.e+1:]...)
		return strings.Join(out, fr.eol)
	}
	if fr.kind == frameBlock && fr.singleLine {
		// 单行块注释变成多行：采用规范形式 "/**\n * a\n * b\n */"。
		out = append(out, fr.opener)
		for _, t := range lines {
			out = append(out, fr.linePrefix(t)+t)
		}
		out = append(out, " */")
		return strings.Join(out, fr.eol)
	}
	out = append(out, fr.lines[:fr.h]...)
	for i, t := range lines {
		var pre, suf string
		if i == 0 && fr.h == 0 && fr.kind == frameBlock {
			pre = fr.pre[0] // 正文首行与开头标记同行
		} else {
			pre = fr.linePrefix(t)
		}
		if i == len(lines)-1 && fr.e == m-1 && fr.kind == frameBlock {
			suf = fr.suf[m-1] // 正文末行与结尾标记同行
			if i > 0 && fr.e > 0 {
				pre = fr.pre[m-1]
			}
		}
		out = append(out, pre+t+suf)
	}
	out = append(out, fr.lines[fr.e+1:]...)
	return strings.Join(out, fr.eol)
}

// linePrefix 返回新行使用的装饰前缀。
func (fr *frame) linePrefix(t string) string {
	if t == "" {
		return fr.defEmpty
	}
	return fr.defPrefix
}
