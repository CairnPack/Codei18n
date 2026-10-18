package langcode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type vectors struct {
	Canonical     [][2]string `json:"canonical"`
	RawEquivalent [][]any     `json:"equivalent"`
}

// 与 IDEA 插件共享的测试向量，保证两端一致。
func TestSharedVectors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "testdata", "language-codes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Canonical) == 0 || len(v.RawEquivalent) == 0 {
		t.Fatal("测试向量为空")
	}
	for _, c := range v.Canonical {
		if got := Canonical(c[0]); got != c[1] {
			t.Errorf("Canonical(%q) = %q, want %q", c[0], got, c[1])
		}
	}
	for _, e := range v.RawEquivalent {
		a, b, want := e[0].(string), e[1].(string), e[2].(bool)
		if got := Equivalent(a, b); got != want {
			t.Errorf("Equivalent(%q, %q) = %v, want %v", a, b, got, want)
		}
	}
}

func TestFindKey(t *testing.T) {
	m := map[string]string{"en": "a", "zh-CN": "b"}
	if k := FindKey(m, "zh"); k != "zh-CN" {
		t.Fatalf("got %q", k)
	}
	if k := FindKey(m, "en"); k != "en" {
		t.Fatalf("got %q", k)
	}
	if k := FindKey(m, "ja"); k != "" {
		t.Fatalf("got %q", k)
	}
}
