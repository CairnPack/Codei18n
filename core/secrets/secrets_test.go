package secrets

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestSecretNeverPrintsPlaintext(t *testing.T) {
	s := NewSecret([]byte("  sk-test-1234567890abcd\n"))
	if s.Reveal() != "sk-test-1234567890abcd" {
		t.Fatalf("应去掉首尾空白: %q", s.Reveal())
	}
	outputs := []string{
		s.String(), fmt.Sprint(s), fmt.Sprintf("%v %s %q %+v %#v", s, s, s, s, s),
		fmt.Sprintf("%v", struct{ K Secret }{s}),
	}
	data, _ := json.Marshal(map[string]any{"k": s})
	outputs = append(outputs, string(data))
	for _, o := range outputs {
		if strings.Contains(o, "1234567890") {
			t.Fatalf("泄漏明文: %s", o)
		}
	}
	if s.Last4() != "abcd" || s.Masked() != "••••abcd" {
		t.Fatal(s.Masked())
	}
	if NewSecret([]byte("short")).Last4() != "" {
		t.Fatal("过短的令牌不应显示末 4 位")
	}
}

func TestRedact(t *testing.T) {
	s := NewSecret([]byte("sk-proj-ABCDEFGHIJKLMNOP"))
	msg := "Incorrect API key provided: sk-proj-ABCDEFGH****. full=sk-proj-ABCDEFGHIJKLMNOP tail=IJKLMNOP"
	got := s.Redact(msg)
	for _, frag := range []string{"ABCDEFGHIJKLMNOP", "sk-proj-ABCDEFGH", "IJKLMNOP"} {
		if strings.Contains(got, frag) {
			t.Fatalf("片段未脱敏 %q: %s", frag, got)
		}
	}
}

func TestMemoryStore(t *testing.T) {
	s := NewMemory()
	if _, err := s.Get("profile/a"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.Set("profile/a", NewSecret([]byte(strings.Repeat("x", MaxSize+1)))); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	if err := s.Set("profile/a", NewSecret(nil)); !errors.Is(err, ErrEmpty) {
		t.Fatal(err)
	}
	if err := s.Set("profile/a", NewSecret([]byte(strings.Repeat("y", MaxSize)))); err != nil {
		t.Fatal(err)
	}
	s.Set(ProjectAccount("p1", "a"), NewSecret([]byte("k1")))
	s.Set(ProjectAccount("p2", "a"), NewSecret([]byte("k2")))
	s.Set(ProjectAccount("p2", "ab"), NewSecret([]byte("k3")))
	accts, _ := ProjectProfileAccounts(s, "a")
	if len(accts) != 2 {
		t.Fatalf("%v", accts)
	}
	if err := s.Delete("profile/a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("profile/a"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestNaming(t *testing.T) {
	if TargetName(ProfileAccount("ds")) != "codei18n/profile/ds" {
		t.Fatal()
	}
	if TargetName(ProjectAccount("u-1", "ds")) != "codei18n/project/u-1/ds" {
		t.Fatal()
	}
}

// 审计 F2：编码后的回显、中间片段、JSON 转义、短令牌都要抹去。
func TestRedactEncodedAndPartialEchoes(t *testing.T) {
	tok := "AbCdEfGh/IjKlMnOp+QrStUvWx/YzAbCdEf=="
	s := NewSecret([]byte(tok))
	cases := map[string]string{
		"url":    "invalid credential " + url.QueryEscape(tok),
		"middle": "got " + tok[1:len(tok)-1],
		"base64": "header " + base64.StdEncoding.EncodeToString([]byte(tok)),
	}
	for name, msg := range cases {
		out := s.Redact(msg)
		for i := 0; i+8 <= len(tok); i++ {
			if strings.Contains(out, tok[i:i+8]) {
				t.Fatalf("%s: 片段 %q 未抹去: %s", name, tok[i:i+8], out)
			}
		}
		if strings.Contains(out, url.QueryEscape(tok)[10:20]) {
			t.Fatalf("%s: 编码片段未抹去: %s", name, out)
		}
	}
	js := NewSecret([]byte("ABCDEFGH&IJKLMNOP<QRSTUVWX"))
	enc, _ := json.Marshal(js.Reveal())
	if out := js.Redact(string(enc)); strings.Contains(out, "IJKLMNOP") || strings.Contains(out, "ABCDEFGH") {
		t.Fatalf("JSON 转义形式未抹去: %s", out)
	}
	short := NewSecret([]byte("abc"))
	if out := short.Redact("key=abc!"); strings.Contains(out, "abc") {
		t.Fatalf("短令牌精确匹配应抹去: %s", out)
	}
}
