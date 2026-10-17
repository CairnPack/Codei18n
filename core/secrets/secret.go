package secrets

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Secret 持有令牌字节。它的 String / Format / MarshalJSON 只输出脱敏形式（"••••abcd"），
// 因此误把 Secret 放进日志、错误或 JSON 也不会泄漏明文。只有 Reveal 返回明文。
type Secret struct {
	b []byte
}

// NewSecret 复制 b 并去掉首尾空白（例如从标准输入读取时的换行）。
func NewSecret(b []byte) Secret {
	s := strings.TrimSpace(string(b))
	return Secret{b: []byte(s)}
}

// Len 返回字节数。
func (s Secret) Len() int { return len(s.b) }

// IsZero 判断是否为空。
func (s Secret) IsZero() bool { return len(s.b) == 0 }

// Reveal 返回明文。只应在构造翻译后端的请求时调用，结果不得进入日志或错误信息。
func (s Secret) Reveal() string { return string(s.b) }

// Last4 返回末 4 个字符（不足 8 个字符的令牌不显示任何字符，避免泄漏过多）。
func (s Secret) Last4() string {
	str := string(s.b)
	if utf8.RuneCountInString(str) < 8 {
		return ""
	}
	r := []rune(str)
	return string(r[len(r)-4:])
}

// Masked 返回 "••••" + 末 4 位。
func (s Secret) Masked() string {
	if s.IsZero() {
		return ""
	}
	return "••••" + s.Last4()
}

func (s Secret) String() string                 { return s.Masked() }
func (s Secret) GoString() string               { return "secrets.Secret(" + s.Masked() + ")" }
func (s Secret) Format(f fmt.State, verb rune)  { _, _ = f.Write([]byte(s.Masked())) }
func (s Secret) MarshalJSON() ([]byte, error)   { return []byte(`"[REDACTED]"`), nil }
func (s Secret) MarshalText() ([]byte, error)   { return []byte("[REDACTED]"), nil }

// Redact 把 text 中出现的令牌明文及其较长片段替换为 [REDACTED]。
// 片段指长度 ≥ 8 的前缀或后缀（翻译后端的错误信息常带 "sk-proj-…wxyz" 这类截断形式）。
func (s Secret) Redact(text string) string {
	if s.Len() < 4 {
		return text
	}
	v := s.Reveal()
	text = strings.ReplaceAll(text, v, "[REDACTED]")
	for n := len(v) - 1; n >= 8; n-- {
		if p := v[:n]; strings.Contains(text, p) {
			text = strings.ReplaceAll(text, p, "[REDACTED]")
		}
		if q := v[len(v)-n:]; strings.Contains(text, q) {
			text = strings.ReplaceAll(text, q, "[REDACTED]")
		}
	}
	return text
}
