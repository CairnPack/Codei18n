package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
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
