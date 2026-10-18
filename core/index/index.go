// Package index 读写位置索引 .codei18n/index.json。格式契约见 docs/index-format.md。
package index

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/CairnPack/codei18n/internal/atomicfile"
)

// Version 是当前格式版本。
const Version = "1"

// FileName 是索引在 .codei18n 目录下的文件名。
const FileName = "index.json"

// 原语言来源。
const (
	LangSourceDetected  = "detected"
	LangSourceHeuristic = "heuristic"
	LangSourceOverride  = "override"
	LangSourceFallback  = "fallback"
)

// Index 是 index.json 的顶层结构。
type Index struct {
	Version        string           `json:"version"`
	GeneratedAt    string           `json:"generatedAt"`
	Generator      string           `json:"generator,omitempty"`
	SourceLanguage string           `json:"sourceLanguage,omitempty"`
	LocalLanguage  string           `json:"localLanguage,omitempty"`
	Files          map[string]*File `json:"files"`
}

// File 是单个源文件的条目。
type File struct {
	ContentHash string    `json:"contentHash"`
	Language    string    `json:"language"`
	Comments    []Comment `json:"comments"`
}

// Range 使用 1 起始的行号与 UTF-16 码元列号，endCol 为左闭右开。
type Range struct {
	StartLine int `json:"startLine"`
	StartCol  int `json:"startCol"`
	EndLine   int `json:"endLine"`
	EndCol    int `json:"endCol"`
}

// Comment 是单条注释（可能由多条物理注释合并而来）的条目。
type Comment struct {
	ID         string `json:"id"`
	Range      Range  `json:"range"`
	Type       string `json:"type"`
	Lang       string `json:"lang"`
	LangSource string `json:"langSource"`
	SourceKey  string `json:"sourceKey"`
	TextHash   string `json:"textHash"`
	Parts      int    `json:"parts"`
	Symbol     string `json:"symbol"`
}

// New 创建空索引。
func New() *Index {
	return &Index{Version: Version, Files: map[string]*File{}}
}

// ErrUnsupportedVersion 表示索引主版本不被支持。
var ErrUnsupportedVersion = errors.New("不支持的 index.json 版本")

// Read 读取索引；文件不存在时返回 (nil, nil)。
func Read(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	if major(idx.Version) != Version {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedVersion, idx.Version)
	}
	if idx.Files == nil {
		idx.Files = map[string]*File{}
	}
	return &idx, nil
}

// Write 对条目排序后原子写出索引。
func Write(path string, idx *Index) error {
	for _, f := range idx.Files {
		sort.SliceStable(f.Comments, func(i, j int) bool {
			a, b := f.Comments[i].Range, f.Comments[j].Range
			if a.StartLine != b.StartLine {
				return a.StartLine < b.StartLine
			}
			return a.StartCol < b.StartCol
		})
	}
	return atomicfile.WriteJSON(path, idx)
}

func major(v string) string {
	if i := strings.IndexByte(v, '.'); i >= 0 {
		return v[:i]
	}
	return v
}

// Find 返回 id 对应的条目及其所在文件路径。
func (idx *Index) Find(id string) (string, *Comment) {
	for path, f := range idx.Files {
		for i := range f.Comments {
			if f.Comments[i].ID == id {
				return path, &f.Comments[i]
			}
		}
	}
	return "", nil
}

// ContentHash 计算文件原始字节的 SHA-1。
func ContentHash(content []byte) string {
	sum := sha1.Sum(content)
	return hex.EncodeToString(sum[:])
}

// TextHash 计算归一化文本（见 Normalize）的 SHA-1。
func TextHash(text string) string {
	sum := sha1.Sum([]byte(Normalize(text)))
	return hex.EncodeToString(sum[:])
}

var openMarkers = []string{"/**", "/*!", "/*", "///", "//!", "//"}

// Normalize 实现 docs/index-format.md §6.2 的归一化（版本 1）。
func Normalize(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		line = strings.TrimFunc(line, unicode.IsSpace)
		stripped := false
		for _, m := range openMarkers {
			if strings.HasPrefix(line, m) {
				line = line[len(m):]
				stripped = true
				break
			}
		}
		if !stripped && strings.HasPrefix(line, "*") && !strings.HasPrefix(line, "*/") {
			line = line[1:]
		}
		line = strings.TrimSuffix(line, "*/")
		lines[i] = line
	}
	return strings.Join(strings.FieldsFunc(strings.Join(lines, " "), unicode.IsSpace), " ")
}

// UTF16Col 把某一行内的字节列（1 起始）换算为 UTF-16 码元列（1 起始）。
// line 为该行内容（不含换行符）；byteCol 超出行长时按行尾计算。
func UTF16Col(line []byte, byteCol int) int {
	if byteCol <= 1 {
		return 1
	}
	limit := byteCol - 1
	if limit > len(line) {
		limit = len(line)
	}
	col := 1
	for i := 0; i < limit; {
		r, size := utf8.DecodeRune(line[i:])
		if r == utf8.RuneError && size <= 1 {
			col++
			i++
			continue
		}
		col += utf16.RuneLen(r)
		i += size
	}
	return col
}

// Lines 把内容切分为行（\n 分隔，去掉行尾 \r），供列换算使用。
func Lines(content []byte) [][]byte {
	raw := strings.Split(string(content), "\n")
	out := make([][]byte, len(raw))
	for i, l := range raw {
		out[i] = []byte(strings.TrimSuffix(l, "\r"))
	}
	return out
}

// RangeFromBytes 把字节列坐标（1 起始，endCol 左闭右开）换算为索引坐标。
func RangeFromBytes(lines [][]byte, startLine, startCol, endLine, endCol int) Range {
	line := func(n int) []byte {
		if n >= 1 && n <= len(lines) {
			return lines[n-1]
		}
		return nil
	}
	return Range{
		StartLine: startLine,
		StartCol:  UTF16Col(line(startLine), startCol),
		EndLine:   endLine,
		EndCol:    UTF16Col(line(endLine), endCol),
	}
}
