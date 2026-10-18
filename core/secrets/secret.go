package secrets

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Secret 持有令牌字节。它的 String / Format / MarshalJSON 只输出脱敏形式（"••••abcd"），
// 因此误把 Secret 放进日志、错误或 JSON 也不会泄漏明文。只有 Reveal 返回明文。
//
// 字节放在指针后面：Secret 作为未导出字段被 %+v / %#v 打印（fmt 无法调用其 Format）时，只会输出地址。
type Secret struct {
	p *secretBytes
}

type secretBytes struct{ b []byte }

// NewSecret 复制 b 并去掉首尾空白（例如从标准输入读取时的换行）。
func NewSecret(b []byte) Secret {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return Secret{}
	}
	return Secret{p: &secretBytes{b: []byte(s)}}
}

func (s Secret) bytes() []byte {
	if s.p == nil {
		return nil
	}
	return s.p.b
}

// Len 返回字节数。
func (s Secret) Len() int { return len(s.bytes()) }

// IsZero 判断是否为空。
func (s Secret) IsZero() bool { return s.Len() == 0 }

// Reveal 返回明文。只应在构造翻译后端的请求时调用，结果不得进入日志或错误信息。
func (s Secret) Reveal() string { return string(s.bytes()) }

// Last4 返回末 4 个字符；不足 12 个字符的令牌不显示任何字符，避免泄漏过多。
func (s Secret) Last4() string {
	str := s.Reveal()
	if utf8.RuneCountInString(str) < 12 {
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

func (s Secret) String() string                { return s.Masked() }
func (s Secret) GoString() string              { return "secrets.Secret(" + s.Masked() + ")" }
func (s Secret) Format(f fmt.State, verb rune) { _, _ = f.Write([]byte(s.Masked())) }
func (s Secret) MarshalJSON() ([]byte, error)  { return []byte(`"[REDACTED]"`), nil }
func (s Secret) MarshalText() ([]byte, error)  { return []byte("[REDACTED]"), nil }

// minFragment 是会被当作令牌片段抹去的最短子串长度。
const minFragment = 8

// Redact 把 text 中出现的令牌抹为 [REDACTED]：
//   - 令牌本身，以及它的 URL 编码、JSON 转义、Base64 形式（后端常把凭据编码后回显）；
//   - 上述各形式中任意长度 ≥ 8 的子串（截断显示如 "sk-proj-…wxyz"，或去掉首尾若干字符的回显）；
//   - 不足 8 字节的令牌只做精确匹配（更短的片段无法与普通文本区分）。
func (s Secret) Redact(text string) string {
	v := s.Reveal()
	if v == "" {
		return text
	}
	forms := []string{v, url.QueryEscape(v), url.PathEscape(v), base64.StdEncoding.EncodeToString([]byte(v)),
		base64.RawURLEncoding.EncodeToString([]byte(v))}
	if j, err := json.Marshal(v); err == nil {
		forms = append(forms, strings.Trim(string(j), `"`))
	}
	seen := map[string]bool{}
	for _, f := range forms {
		if seen[f] {
			continue
		}
		seen[f] = true
		text = redactForm(text, f)
	}
	return text
}

// redactForm 先抹去完整形式，再从长到短抹去长度 ≥ minFragment 的子串。
func redactForm(text, form string) string {
	text = strings.ReplaceAll(text, form, "[REDACTED]")
	if len(form) < minFragment {
		return text
	}
	for n := len(form) - 1; n >= minFragment; n-- {
		for i := 0; i+n <= len(form); i++ {
			if frag := form[i : i+n]; strings.Contains(text, frag) {
				text = strings.ReplaceAll(text, frag, "[REDACTED]")
			}
		}
	}
	return text
}
