package tests

import (
	"os/exec"
	"strings"
	"testing"
)

// 审计 F5：误把令牌当作参数传给 auth set 时，错误输出（可能进入 CI 日志）里不得回显它。
func TestAuthSetRejectsArgumentWithoutEcho(t *testing.T) {
	cmd := exec.Command(GetBinaryPath(t), "auth", "set", "--profile", "x", "sk-test-ECHO1234567890")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("应拒绝位置参数")
	}
	if strings.Contains(string(out), "ECHO1234567890") {
		t.Fatalf("错误输出回显了令牌: %s", out)
	}
	if !strings.Contains(string(out), "标准输入") {
		t.Fatalf("应提示改用标准输入: %s", out)
	}
}
