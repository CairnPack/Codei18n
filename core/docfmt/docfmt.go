// Package docfmt 实现文档注释的结构化翻译预处理与还原。
//
// 翻译前，Prepare 剥离注释外框（开头标记 /** 或 ///、每行装饰 " * " 或 "/// "、结尾标记 */），
// 并把不应翻译的片段（代码、链接、块标签及其参数名、HTML/XML 标签等）按出现顺序替换为占位符
// ⟦0⟧、⟦1⟧……，得到只含自然语言、占位符、Markdown 结构与换行的正文交给翻译器。
// 翻译后，Restore 校验占位符完整且顺序不变，代回原片段，再按原注释的样式重新加上外框。
//
// 占位符合并规则：相邻的两个受保护片段之间只隔空格/制表符（或紧挨着）时合并为一个占位符，
// 例如 "@param {string} name" 或 "{@link A} {@link B}" 各只占一个占位符；跨行的片段从不合并。
package docfmt

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Format 是文档注释的格式。
type Format string

const (
	None      Format = ""
	Javadoc   Format = "javadoc"
	KDoc      Format = "kdoc"
	JSDoc     Format = "jsdoc" // JSDoc + TSDoc
	Dartdoc   Format = "dartdoc"
	CSharpXML Format = "csharp-xml"
)

// For 把 index.json 的编程语言标识与注释类型（"doc"/"line"/"block"）映射为文档注释格式；
// commentType 不是 "doc" 或语言不支持时返回 None。
func For(language, commentType string) Format {
	if commentType != "doc" {
		return None
	}
	switch strings.ToLower(language) {
	case "java":
		return Javadoc
	case "kotlin":
		return KDoc
	case "typescript", "tsx", "javascript", "jsx":
		return JSDoc
	case "dart":
		return Dartdoc
	case "csharp":
		return CSharpXML
	}
	return None
}

// Placeholder 返回第 i 个受保护片段的占位符："⟦" + i + "⟧"（U+27E6 / U+27E7）。
func Placeholder(i int) string { return "⟦" + strconv.Itoa(i) + "⟧" }

// ErrPlaceholderMismatch 表示译文中的占位符缺失、多余、重复或顺序被打乱。
var ErrPlaceholderMismatch = errors.New("docfmt: 译文占位符与原文不一致")

var rePlaceholder = regexp.MustCompile(`⟦([0-9]+)⟧`)

// Prepared 是 Prepare 的结果：剥离外框、替换受保护片段后的正文，以及还原所需的信息。
type Prepared struct {
	fr        *frame
	body      string
	fragments []string
}

// Prepare 剥离注释外框，并把受保护片段按出现顺序替换为占位符 ⟦0⟧、⟦1⟧……
// f 为 None 时只剥离外框、不保护任何片段。注释不是以 /* 或 // 开头（或块注释未闭合）时返回错误。
func Prepare(f Format, comment string) (*Prepared, error) {
	switch f {
	case None, Javadoc, KDoc, JSDoc, Dartdoc, CSharpXML:
	default:
		return nil, fmt.Errorf("docfmt: 未知的注释格式 %q", string(f))
	}
	fr, err := parseFrame(comment)
	if err != nil {
		return nil, err
	}
	raw := fr.body()
	p := &Prepared{fr: fr}
	var b strings.Builder
	last := 0
	for _, sp := range protect(f, raw) {
		b.WriteString(raw[last:sp.start])
		b.WriteString(Placeholder(len(p.fragments)))
		p.fragments = append(p.fragments, raw[sp.start:sp.end])
		last = sp.end
	}
	b.WriteString(raw[last:])
	p.body = b.String()
	return p, nil
}

// Body 返回交给翻译器的正文：自然语言 + 占位符 + Markdown 结构与换行，不含注释标记。
func (p *Prepared) Body() string { return p.body }

// Placeholders 返回占位符个数。
func (p *Prepared) Placeholders() int { return len(p.fragments) }

// HasProse 报告正文去掉占位符后是否还含有文字（任意 Unicode 字母）。
// 返回 false 时正文全部受保护（或只有标点、数字），调用方可跳过翻译。
func (p *Prepared) HasProse() bool {
	rest := rePlaceholder.ReplaceAllString(p.body, "")
	for _, r := range rest {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// Restore 校验译文中每个占位符恰好出现一次且按 0..n-1 递增排列（否则返回包装了
// ErrPlaceholderMismatch 的错误），代回原片段，再按原注释的样式加上外框：开头/结尾标记、
// 每行装饰与缩进与原注释一致；单行 /** x */ 在译文无换行时保持单行；/// 组每个正文行输出一行。
// 译文行数可以与原文不同；行数相同时逐行套用原行的装饰，保证原样往返。
func (p *Prepared) Restore(translated string) (string, error) {
	translated = strings.ReplaceAll(translated, "\r\n", "\n")
	if err := p.check(translated); err != nil {
		return "", err
	}
	// 代回片段：按占位符切分，片段内容不再参与替换。
	var b strings.Builder
	last := 0
	for _, loc := range rePlaceholder.FindAllStringSubmatchIndex(translated, -1) {
		b.WriteString(translated[last:loc[0]])
		k, _ := strconv.Atoi(translated[loc[2]:loc[3]])
		b.WriteString(p.fragments[k])
		last = loc[1]
	}
	b.WriteString(translated[last:])
	lines := cleanLines(b.String())
	if p.fr.e < p.fr.h && len(lines) > 0 {
		return "", errors.New("docfmt: 原注释没有正文，无法放入译文")
	}
	return p.fr.rebuild(lines), nil
}

// check 校验占位符。
func (p *Prepared) check(s string) error {
	n := len(p.fragments)
	ms := rePlaceholder.FindAllStringSubmatch(s, -1)
	if open, close := strings.Count(s, "⟦"), strings.Count(s, "⟧"); open != len(ms) || close != len(ms) {
		return fmt.Errorf("%w: 存在残缺的占位符括号", ErrPlaceholderMismatch)
	}
	seen := make(map[int]bool, len(ms))
	for j, m := range ms {
		k, err := strconv.Atoi(m[1])
		if err != nil || k >= n {
			return fmt.Errorf("%w: 多余的占位符 %s（共 %d 个）", ErrPlaceholderMismatch, m[0], n)
		}
		if seen[k] {
			return fmt.Errorf("%w: 占位符 %s 重复", ErrPlaceholderMismatch, m[0])
		}
		seen[k] = true
		if k != j {
			return fmt.Errorf("%w: 第 %d 个占位符应为 %s，实际为 %s", ErrPlaceholderMismatch, j+1, Placeholder(j), m[0])
		}
	}
	if len(ms) != n {
		return fmt.Errorf("%w: 缺少占位符 %s（应有 %d 个，实际 %d 个）", ErrPlaceholderMismatch, Placeholder(len(ms)), n, len(ms))
	}
	return nil
}

// cleanLines 把正文切成行，去掉每行行尾空白以及首尾的空行。
func cleanLines(s string) []string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
