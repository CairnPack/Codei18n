// Package langcode 提供全项目统一的 BCP 47 语言码规范化与等价判断。
// 规则见 docs/index-format.md §8；IDEA 插件的 LanguageCodes 必须与本包保持一致（共享测试向量）。
package langcode

import "strings"

// Unknown 表示无法识别的语言。
const Unknown = "unknown"

var aliases = map[string]string{
	"zho": "zh", "chi": "zh", "cmn": "zh",
	"eng": "en", "jpn": "ja", "kor": "ko",
	"deu": "de", "ger": "de", "fra": "fr", "fre": "fr",
	"spa": "es", "rus": "ru",
	"iw": "he", "in": "id", "ji": "yi",
}

// Canonical 返回规范化语言码：中文为 zh-Hans / zh-Hant，其他语言为小写主语言子标签。
// 空串返回空串；und、unknown 返回 "unknown"。
func Canonical(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	parts := strings.Split(strings.ReplaceAll(code, "_", "-"), "-")
	lang := strings.ToLower(parts[0])
	if lang == "und" || lang == Unknown {
		return Unknown
	}
	if a, ok := aliases[lang]; ok {
		lang = a
	}
	if lang != "zh" {
		return lang
	}
	for _, sub := range parts[1:] {
		switch strings.ToLower(sub) {
		case "hant", "tw", "hk", "mo":
			return "zh-Hant"
		}
	}
	return "zh-Hans"
}

// Equivalent 判断两个语言码规范化后是否相同（任一为空时为 false）。
func Equivalent(a, b string) bool {
	ca, cb := Canonical(a), Canonical(b)
	return ca != "" && ca == cb
}

// FindKey 在 keys 中查找与 code 对应的键：先找完全相同的写法，再找等价写法；找不到返回 ""。
func FindKey[V any](m map[string]V, code string) string {
	if _, ok := m[code]; ok {
		return code
	}
	for k := range m {
		if Equivalent(k, code) {
			return k
		}
	}
	return ""
}
