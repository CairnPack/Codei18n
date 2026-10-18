package commentgroup

import (
	"strings"
	"testing"

	"github.com/CairnPack/codei18n/core/domain"
)

// raws 以简单的逐行扫描构造测试输入：每个 // 开始到行尾为一条行注释，/* */ 在同一行内。
func raws(src string) []Raw {
	var out []Raw
	for i, line := range strings.Split(src, "\n") {
		if k := strings.Index(line, "//"); k >= 0 {
			out = append(out, Raw{Text: line[k:], StartLine: i + 1, StartCol: k + 1, EndLine: i + 1, EndCol: len(line) + 1, Symbol: "s" + string(rune('0'+i))})
		} else if k := strings.Index(line, "/*"); k >= 0 {
			e := strings.Index(line, "*/") + 2
			out = append(out, Raw{Text: line[k:e], StartLine: i + 1, StartCol: k + 1, EndLine: i + 1, EndCol: e + 1})
		}
	}
	return out
}

var rust = Options{Language: "rust", DocLineMarkers: []string{"///", "//!"}, DocBlockPrefixes: []string{"/**", "/*!"}}

func TestMergeConsecutiveOwnLineComments(t *testing.T) {
	src := "// a\n// b\n//\n// c\nfn x() {}\n"
	got := Build([]byte(src), "f.rs", raws(src), rust)
	if len(got) != 1 {
		t.Fatalf("应合并为 1 条: %d", len(got))
	}
	c := got[0]
	if c.SourceText != "// a\n// b\n//\n// c" || c.Parts != 4 || c.Type != domain.CommentTypeLine {
		t.Fatalf("%+v", c)
	}
	if c.Range != (domain.TextRange{StartLine: 1, StartCol: 1, EndLine: 4, EndCol: 5}) {
		t.Fatalf("%+v", c.Range)
	}
	if c.Symbol != "s3" || len(c.PartSymbols) != 4 || c.PartTexts[1] != "// b" {
		t.Fatalf("符号应取最后一条: %+v", c)
	}
}

func TestDoNotMergeAcrossMarkersBlankLinesColumnsOrTrailing(t *testing.T) {
	cases := map[string]int{
		"// a\n/// b\n":            2, // 标记不同
		"/// a\n//! b\n":           2,
		"// a\n\n// b\n":           2, // 空行
		"// a\n  // b\n":           2, // 起始列不同
		"x(); // a\n// b\n":        2, // 行尾注释不合并
		"// a\ny(); // b\n":        2,
		"/* a */\n/* b */\n":       2, // 块注释不合并
		"// a\n// b\n/* c */\n":    2,
		"    // a\n    // b\n":     1,
		"/// a\n/// b\n/// c\n":    1,
		"//// sep\n//// sep2\n":    1,
		"// a\n//// b\n":           2,
	}
	for src, want := range cases {
		if got := Build([]byte(src), "f", raws(src), rust); len(got) != want {
			t.Errorf("%q: got %d, want %d", src, len(got), want)
		}
	}
}

func TestTypes(t *testing.T) {
	cs := Options{Language: "csharp", DocLineMarkers: []string{"///"}, DocBlockPrefixes: []string{"/**"}}
	ts := Options{Language: "typescript", DocBlockPrefixes: []string{"/**"}}
	check := func(opts Options, src string, want domain.CommentType) {
		t.Helper()
		got := Build([]byte(src), "f", raws(src), opts)
		if len(got) != 1 || got[0].Type != want {
			t.Errorf("%s %q: %+v", opts.Language, src, got)
		}
	}
	check(cs, "/// <summary>x</summary>", domain.CommentTypeDoc)
	check(ts, "/// <reference path=\"a\" />", domain.CommentTypeLine) // 三斜线指令不是文档注释
	check(ts, "/** doc */", domain.CommentTypeDoc)
	check(ts, "/* blk */", domain.CommentTypeBlock)
	check(ts, "/**/ x", domain.CommentTypeBlock)
	check(rust, "//! inner", domain.CommentTypeDoc)
	check(rust, "/*! inner */", domain.CommentTypeDoc)
}

func TestEmptyGroupsDropped(t *testing.T) {
	src := "//\n\n///\n\n/* */\n// real\n"
	got := Build([]byte(src), "f", raws(src), rust)
	if len(got) != 1 || got[0].SourceText != "// real" {
		t.Fatalf("%+v", got)
	}
}

func TestTrimLineEnd(t *testing.T) {
	r := TrimLineEnd(Raw{Text: "// x\r\n", StartLine: 2, StartCol: 3, EndLine: 3, EndCol: 1})
	if r.Text != "// x" || r.EndLine != 2 || r.EndCol != 7 {
		t.Fatalf("%+v", r)
	}
}

func TestMarker(t *testing.T) {
	for in, want := range map[string]string{"// a": "//", "///a": "///", "//! a": "//!", "////": "////", "/* x */": "", "//!!": "//!"} {
		if got := Marker(in); got != want {
			t.Errorf("Marker(%q)=%q want %q", in, got, want)
		}
	}
}
