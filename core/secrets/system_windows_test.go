//go:build windows

package secrets

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/danieljoos/wincred"
)

// 真实的 Windows 凭据管理器集成测试：使用测试专用 account 前缀，结束后清理。
// 设置 CODEI18N_SKIP_CREDMAN=1 可跳过（例如在无交互会话的 CI 中）。
func TestWindowsCredentialManager(t *testing.T) {
	if os.Getenv("CODEI18N_SKIP_CREDMAN") != "" {
		t.Skip("CODEI18N_SKIP_CREDMAN 已设置")
	}
	store := System()
	prefix := fmt.Sprintf("test-%d/", time.Now().UnixNano())
	account := prefix + "profile/demo"
	t.Cleanup(func() {
		if left, _ := store.List(prefix); len(left) > 0 {
			for _, a := range left {
				_ = store.Delete(a)
			}
		}
	})

	if _, err := store.Get(account); !errors.Is(err, ErrNotFound) {
		t.Fatalf("初始应不存在: %v", err)
	}
	if err := store.Set(account, NewSecret([]byte("integration-secret-1234"))); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(account)
	if err != nil || got.Reveal() != "integration-secret-1234" {
		t.Fatalf("读取失败: %v", err)
	}

	// 格式与 mygo Secrets / keytar 一致：TargetName = codei18n/<account>，UserName = <account>
	raw, err := wincred.GetGenericCredential("codei18n/" + account)
	if err != nil {
		t.Fatal(err)
	}
	if raw.UserName != account || string(raw.CredentialBlob) != "integration-secret-1234" {
		t.Fatalf("凭据格式不符: user=%q", raw.UserName)
	}

	list, err := store.List(prefix)
	if err != nil || len(list) != 1 || list[0] != account {
		t.Fatalf("List: %v %v", list, err)
	}
	if err := store.Delete(account); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(account); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除后应不存在: %v", err)
	}
}
