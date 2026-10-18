package langdetect

import (
	"fmt"
	"slices"
	"strings"

	"github.com/CairnPack/codei18n/core/index"
	"github.com/CairnPack/codei18n/core/langcode"
	"github.com/CairnPack/codei18n/third_party/lingua"
)

// 识别流程（需求文档 §A5）：
//
//  1. 用 index.Normalize 去掉注释符号，再用 cleanComment 去掉代码片段、文档标签、链接、
//     路径、像代码的标识符、数字与标点，只留下自然语言单词。
//  2. 剩余文本过短（非中日文少于 minWords 个单词，且中日文字符少于 minCJKChars 个）时，
//     按文字系统判断：有假名 → ja；有韩文 → ko；有汉字 → 中文（再用简繁字表区分）；
//     有西里尔字母 → ru；只有拉丁字母 → 无法区分 en/de/fr/es，返回 unknown。
//     判断成功时 Source 为 heuristic，否则为 fallback。
//  3. 否则交给 lingua（仅限候选语言，要求第一、第二名置信度之差不低于 minRelativeDistance），
//     不够确定时返回 unknown/fallback；结果为中文时用简繁字表多数决定 zh-Hans / zh-Hant。
//     Source 为 detected。
//
// 已知局限：
//   - lingua 只有一个"中文"模型，简繁由字表区分。
//   - 简繁判定基于单字表：只含简繁共用字的文本（如"返回值"）无法区分，
//     此时取候选中的中文变体（两者都在时取 zh-Hans）。
//   - 判定出的中文变体不在候选中、但另一变体在时，返回候选中的那个变体
//     （仍是中文；候选集通常表示项目实际使用的变体）。
//   - 只含汉字的日文会被判为中文；日文新字体常与繁体字形重合（如"東"），因此可能落到 zh-Hant。
//   - 识别严格限定在候选集内：候选集代表项目声明会出现的语言。若同一文字系统只有一个候选
//     （如 en 与 zh-Hans），该文字的较长文本（德文等）也会归为那个候选。
//   - 短拉丁文本（少于 3 个单词）一律视为 unknown。

const (
	// minWords 是交给统计模型的最少非中日文单词数。
	minWords = 3
	// minCJKChars 是交给统计模型的最少中日文字符数。
	minCJKChars = 4
	// minRelativeDistance 是 lingua 第一、第二名置信度之差的下限，低于此值视为不确定。
	// 在 141 条 3~10 词的英/德/法/西注释语料上调参：唯一的误判（"handle it here" → de）差值为 0.08，
	// 取 0.10 时零误判、1 条被拒；0.20 时零误判、4 条被拒。取 0.15 在两者间留出余量。
	minRelativeDistance = 0.15
)

// supportedCodes 是本构建可识别的规范化语言码，顺序即 Supported 的返回顺序与默认候选集。
var supportedCodes = []string{"en", "zh-Hans", "zh-Hant", "ja", "ko", "de", "fr", "es", "ru"}

// codeToLingua 把规范化语言码映射到 lingua 语言；zh-Hans/zh-Hant 共用 lingua.Chinese。
var codeToLingua = map[string]lingua.Language{
	"en":      lingua.English,
	"zh-Hans": lingua.Chinese,
	"zh-Hant": lingua.Chinese,
	"ja":      lingua.Japanese,
	"ko":      lingua.Korean,
	"de":      lingua.German,
	"fr":      lingua.French,
	"es":      lingua.Spanish,
	"ru":      lingua.Russian,
}

// latinLanguages 是以拉丁字母书写的受支持语言。
var latinLanguages = []lingua.Language{lingua.English, lingua.German, lingua.French, lingua.Spanish}

var linguaToCode = map[lingua.Language]string{
	lingua.English:  "en",
	lingua.Japanese: "ja",
	lingua.Korean:   "ko",
	lingua.German:   "de",
	lingua.French:   "fr",
	lingua.Spanish:  "es",
	lingua.Russian:  "ru",
}

// Supported 返回本构建可识别的规范化语言码。
func Supported() []string {
	return slices.Clone(supportedCodes)
}

// detector 是基于 lingua 与文字系统启发式的 Detector 实现，创建后只读，可并发使用。
type detector struct {
	// cands 为候选语言码集合（规范化形式）。
	cands map[string]bool
	// lingua 检测器；其语言集合 = 候选语言对应的 lingua 语言（只有 1 种时补入一种"拒识"语言）。
	lingua lingua.LanguageDetector
}

// New 构建限定于候选语言的并发安全 Detector。
//
// candidates 为 BCP 47 语言码，经 langcode.Canonical 规范化；"zh-Hans"/"zh-Hant"
// 都对应 lingua 的中文模型，再用简繁字表区分。包含不支持的语言码时返回错误并列出支持的语言码。
// candidates 为空时使用默认集合 en、zh-Hans、zh-Hant、ja、ko、de、fr、es、ru。
//
// 创建时会预加载所需语言模型（全部 8 种约 30 ms、约 28 MB 堆内存；模型进程内全局共享，
// 重复创建不会重复加载），应在一次运行中只创建一次并复用。
func New(candidates []string) (Detector, error) {
	if len(candidates) == 0 {
		candidates = supportedCodes
	}
	cands := make(map[string]bool, len(candidates))
	var bad []string
	for _, c := range candidates {
		code := langcode.Canonical(c)
		if _, ok := codeToLingua[code]; !ok {
			bad = append(bad, c)
			continue
		}
		cands[code] = true
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("langdetect: 不支持的候选语言 %q，支持的语言码为：%s",
			bad, strings.Join(supportedCodes, ", "))
	}

	var langs []lingua.Language
	for _, code := range supportedCodes {
		if l := codeToLingua[code]; cands[code] && !slices.Contains(langs, l) {
			langs = append(langs, l)
		}
	}
	// lingua 至少需要 2 种语言。只有 1 种时补入一种不同文字系统的语言作为"拒识"项
	// （拉丁文字候选补俄语，其余补英语）：落在拒识项上的结果返回 unknown。
	// 这样与多候选时的语义一致——同一文字系统内只有一个候选时，该文字的文本都归为它。
	if len(langs) == 1 {
		pad := lingua.English
		if slices.Contains(latinLanguages, langs[0]) {
			pad = lingua.Russian
		}
		langs = append(langs, pad)
	}

	ld := lingua.NewLanguageDetectorBuilder().
		FromLanguages(langs...).
		WithMinimumRelativeDistance(minRelativeDistance).
		WithPreloadedLanguageModels().
		Build()
	return &detector{cands: cands, lingua: ld}, nil
}

// Detect 实现 Detector。
func (d *detector) Detect(commentText string) Result {
	text := cleanComment(index.Normalize(commentText))
	st := scanScripts(text)
	if st.cjkChars() < minCJKChars && st.words < minWords {
		return d.heuristic(st)
	}

	lang, ok := d.lingua.DetectLanguageOf(text)
	if !ok {
		return unknown()
	}
	if lang == lingua.Chinese {
		if code := d.chinese(st); code != "" {
			return Result{Lang: code, Source: index.LangSourceDetected}
		}
		return unknown()
	}
	if code := linguaToCode[lang]; d.cands[code] {
		return Result{Lang: code, Source: index.LangSourceDetected}
	}
	return unknown()
}

// heuristic 对过短文本按文字系统判断。
func (d *detector) heuristic(st scriptStats) Result {
	code := ""
	switch {
	case st.kana > 0:
		code = d.only("ja")
	case st.hangul > 0:
		code = d.only("ko")
	case st.han > 0:
		code = d.chinese(st)
	case st.cyrillic > 0 && st.latin == 0:
		code = d.only("ru")
	}
	if code == "" {
		return unknown()
	}
	return Result{Lang: code, Source: index.LangSourceHeuristic}
}

// chinese 在候选范围内决定中文变体；候选中没有任何中文变体时返回空串。
func (d *detector) chinese(st scriptStats) string {
	hans, hant := d.cands["zh-Hans"], d.cands["zh-Hant"]
	switch v := st.chineseVariant(); {
	case v != "" && d.cands[v]:
		return v
	case hans:
		return "zh-Hans"
	case hant:
		return "zh-Hant"
	default:
		return ""
	}
}

// only 在 code 属于候选集时返回 code，否则返回空串。
func (d *detector) only(code string) string {
	if d.cands[code] {
		return code
	}
	return ""
}

func unknown() Result {
	return Result{Lang: langcode.Unknown, Source: index.LangSourceFallback}
}
