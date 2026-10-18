// Package benchgen 生成用于性能基准的中型样例项目（确定性：相同参数生成相同内容）。
package benchgen

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
)

// Languages 是生成的文件类型。只包含 Legacy 也支持的语言的子集见 LegacyLanguages。
var Languages = []string{"go", "java", "kt", "ts", "js", "rs", "cs", "dart"}

// LegacyLanguages 是旧版本（重构前的 main）也能解析的语言，用于公平对比。
var LegacyLanguages = []string{"go", "java", "kt", "ts", "js", "rs"}

var words = strings.Fields(`compute value result buffer cache index parse token stream request response
handle error retry timeout config option builder factory service client server queue worker batch
item record entry field method class module package interface default number string list map`)

// Generate 在 dir 下生成 files 个源文件，平均每个文件约 12 条注释、约 120 行。
func Generate(dir string, files int, seed int64, langs []string) error {
	r := rand.New(rand.NewSource(seed))
	for i := 0; i < files; i++ {
		lang := langs[i%len(langs)]
		sub := filepath.Join(dir, fmt.Sprintf("module%02d", i%40), fmt.Sprintf("pkg%02d", (i/40)%25))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			return err
		}
		name := filepath.Join(sub, fmt.Sprintf("file%04d.%s", i, lang))
		if err := os.WriteFile(name, []byte(source(r, lang, i)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func sentence(r *rand.Rand) string {
	n := 4 + r.Intn(8)
	parts := make([]string, n)
	for i := range parts {
		parts[i] = words[r.Intn(len(words))]
	}
	s := strings.Join(parts, " ")
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

func source(r *rand.Rand, lang string, n int) string {
	var b strings.Builder
	switch lang {
	case "go":
		fmt.Fprintf(&b, "package pkg%d\n\n", n)
	case "java":
		fmt.Fprintf(&b, "package com.example.p%d;\n\n", n)
	case "kt":
		fmt.Fprintf(&b, "package com.example.p%d\n\n", n)
	case "cs":
		fmt.Fprintf(&b, "namespace Example.P%d;\n\n", n)
	case "dart":
		fmt.Fprintf(&b, "library p%d;\n\n", n)
	}
	b.WriteString("// " + sentence(r) + "\n\n")
	for f := 0; f < 6; f++ {
		// 文档注释
		switch lang {
		case "cs", "dart", "rs":
			b.WriteString("/// " + sentence(r) + "\n/// " + sentence(r) + "\n")
		case "go":
			b.WriteString("// " + sentence(r) + "\n// " + sentence(r) + "\n")
		default:
			b.WriteString("/**\n * " + sentence(r) + "\n * @param a " + sentence(r) + "\n */\n")
		}
		writeFunc(&b, r, lang, n, f)
	}
	return b.String()
}

func writeFunc(b *strings.Builder, r *rand.Rand, lang string, n, f int) {
	body := func(indent string) {
		for k := 0; k < 12; k++ {
			if k%6 == 0 {
				b.WriteString(indent + "// " + sentence(r) + "\n")
			}
			switch lang {
			case "go":
				fmt.Fprintf(b, "%sx%d := a + %d // %s\n", indent, k, k, words[r.Intn(len(words))])
			case "rs":
				fmt.Fprintf(b, "%slet x%d = a + %d;\n", indent, k, k)
			case "kt":
				fmt.Fprintf(b, "%sval x%d = a + %d\n", indent, k, k)
			case "ts", "js", "dart":
				fmt.Fprintf(b, "%sconst x%d = a + %d;\n", indent, k, k)
			default:
				fmt.Fprintf(b, "%sint x%d = a + %d;\n", indent, k, k)
			}
		}
	}
	switch lang {
	case "go":
		fmt.Fprintf(b, "func F%d_%d(a int) int {\n", n, f)
		body("\t")
		b.WriteString("\t_ = []int{x0, x1, x2, x3, x4, x5, x6, x7, x8, x9, x10, x11}\n\treturn a\n}\n\n")
	case "rs":
		fmt.Fprintf(b, "pub fn f%d_%d(a: i32) -> i32 {\n", n, f)
		body("    ")
		b.WriteString("    a\n}\n\n")
	case "kt":
		fmt.Fprintf(b, "fun f%d_%d(a: Int): Int {\n", n, f)
		body("    ")
		b.WriteString("    return a\n}\n\n")
	case "ts":
		fmt.Fprintf(b, "export function f%d_%d(a: number): number {\n", n, f)
		body("  ")
		b.WriteString("  return a;\n}\n\n")
	case "js":
		fmt.Fprintf(b, "export function f%d_%d(a) {\n", n, f)
		body("  ")
		b.WriteString("  return a;\n}\n\n")
	case "dart":
		fmt.Fprintf(b, "int f%d_%d(int a) {\n", n, f)
		body("  ")
		b.WriteString("  return a;\n}\n\n")
	case "cs":
		fmt.Fprintf(b, "public static class C%d_%d {\n  public static int F(int a) {\n", n, f)
		body("    ")
		b.WriteString("    return a;\n  }\n}\n\n")
	default: // java
		fmt.Fprintf(b, "class C%d_%d {\n  int f(int a) {\n", n, f)
		body("    ")
		b.WriteString("    return a;\n  }\n}\n\n")
	}
}
