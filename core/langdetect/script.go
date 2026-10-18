package langdetect

import (
	"slices"
	"unicode"
)

// 简繁字表：由 zhtables.go 中的常量在包初始化时展开为按码点升序的切片，查表用二分查找。
var (
	simpOnly = []rune(simpOnlyChars)
	tradOnly = []rune(tradOnlyChars)
)

// scriptStats 统计清洗后文本中各文字系统的字符数与单词数。
type scriptStats struct {
	han, kana, hangul, cyrillic, latin int
	// words 为非中日文的单词数（以空格分隔、至少含一个字母的记号）。
	words int
	// simp / trad 为命中"仅简体"/"仅繁体"字表的汉字个数。
	simp, trad int
}

// cjkChars 返回不以空格分词的中日文字符数（汉字 + 假名）。韩文按单词计。
func (s scriptStats) cjkChars() int { return s.han + s.kana }

func scanScripts(text string) scriptStats {
	var st scriptStats
	inWord, wordHasLetter := false, false
	flush := func() {
		if inWord && wordHasLetter {
			st.words++
		}
		inWord, wordHasLetter = false, false
	}
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			flush()
			st.han++
			if _, ok := slices.BinarySearch(simpOnly, r); ok {
				st.simp++
			} else if _, ok := slices.BinarySearch(tradOnly, r); ok {
				st.trad++
			}
			continue
		case isKana(r):
			flush()
			st.kana++
			continue
		case r == ' ':
			flush()
			continue
		}
		inWord = true
		if unicode.IsLetter(r) {
			wordHasLetter = true
			switch {
			case unicode.Is(unicode.Hangul, r):
				st.hangul++
			case unicode.Is(unicode.Cyrillic, r):
				st.cyrillic++
			case unicode.Is(unicode.Latin, r):
				st.latin++
			}
		}
	}
	flush()
	return st
}

// chineseVariant 按简繁字表多数决定 zh-Hans / zh-Hant；无命中或持平时返回空串。
func (s scriptStats) chineseVariant() string {
	switch {
	case s.simp > s.trad:
		return "zh-Hans"
	case s.trad > s.simp:
		return "zh-Hant"
	default:
		return ""
	}
}
