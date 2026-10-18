// Package langdetect 识别注释文本的原语言。
package langdetect

import (
	"unicode"

	"github.com/CairnPack/codei18n/core/index"
	"github.com/CairnPack/codei18n/core/langcode"
)

// Result 是一次识别的结果。Lang 为规范化 BCP 47 语言码或 "unknown"；
// Source 取 index.LangSourceDetected / LangSourceHeuristic / LangSourceFallback。
type Result struct {
	Lang   string
	Source string
}

// Detector 识别一段注释原文（含注释符号）的语言。实现必须是并发安全的。
type Detector interface {
	Detect(commentText string) Result
}

// Legacy 复现旧版"含汉字即中文，否则视为源语言"的规则：含汉字返回 zh-Hans，其余返回 unknown。
type Legacy struct{}

func (Legacy) Detect(text string) Result {
	for _, r := range index.Normalize(text) {
		if unicode.Is(unicode.Han, r) {
			return Result{Lang: "zh-Hans", Source: index.LangSourceHeuristic}
		}
	}
	return Result{Lang: langcode.Unknown, Source: index.LangSourceFallback}
}
