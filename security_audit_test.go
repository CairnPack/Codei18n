package codei18n

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/secrets"
)

// 审计 F1：随仓库分发的项目配置把 baseUrl 改到别处、或用 apiKeyRef 指向用户的其他令牌时，
// 不得把凭据库中的全局令牌发过去。
func TestProjectConfigCannotRedirectGlobalToken(t *testing.T) {
	f := newAuthFixture(t)
	f.globalCfg(`{"profile":"work","profiles":{"work":{"provider":"openai","baseUrl":"https://api.example.com/v1"}}}`)
	f.store.Set("profile/work", NewSecret([]byte("sk-WORK-real-token-2222")))
	f.store.Set("profile/other", NewSecret([]byte("sk-OTHER-real-token-3333")))
	p := f.project()

	// 1. apiKeyRef 来自项目文件：忽略，仍使用所选 profile 的默认引用
	p.UpdateConfig(func(c *config.File) error {
		c.ProfileOverrides = &config.ProviderFields{APIKeyRef: config.Ptr("profile/other")}
		return nil
	})
	if st := f.status(p); st.GlobalAccount != "profile/work" || st.Last4 != "2222" {
		t.Fatalf("项目文件不能改变令牌引用: %+v", st)
	}

	// 2. baseUrl 被项目文件改到别处：不使用全局令牌，并给出明确提示
	p.UpdateConfig(func(c *config.File) error {
		c.ProfileOverrides = &config.ProviderFields{BaseURL: config.Ptr("http://evil.example/v1")}
		return nil
	})
	st := f.status(p)
	if !st.GlobalBlocked || st.Source != TokenSourceNone {
		t.Fatalf("应阻止全局令牌: %+v", st)
	}
	_, _, err := p.resolveToken(mustCfg(t, p))
	if !errors.Is(err, ErrNoToken) || !strings.Contains(err.Error(), "evil.example") || !strings.Contains(err.Error(), "--project") {
		t.Fatalf("%v", err)
	}

	// 3. 用户为本项目明确保存的令牌、或环境变量，仍然可用
	if err := p.SetProjectToken("work", NewSecret([]byte("sk-PROJECT-own-token-4444"))); err != nil {
		t.Fatal(err)
	}
	if st := f.status(p); st.Source != TokenSourceProject || st.Last4 != "4444" {
		t.Fatalf("%+v", st)
	}

	// 4. 旧版 init 快照写入的、与全局相同的地址照常使用全局令牌
	p.DeleteProjectToken("work")
	p.UpdateConfig(func(c *config.File) error {
		c.ProfileOverrides = &config.ProviderFields{BaseURL: config.Ptr("https://api.example.com/v1")}
		return nil
	})
	if st := f.status(p); st.GlobalBlocked || st.Source != TokenSourceGlobal {
		t.Fatalf("与全局一致的地址不应阻止: %+v", st)
	}
	// 调用方（CLI 参数 / GUI）覆盖的地址是用户本人的选择，属于可信层
	ov := WithOverrides(config.Overrides{Provider: config.ProviderFields{BaseURL: config.Ptr("http://localhost:9/v1")}})
	p2, _ := Open(f.root, append(f.opts(), ov)...)
	if st, _ := p2.TokenStatus(); st.GlobalBlocked {
		t.Fatalf("调用方覆盖不应被阻止: %+v", st)
	}
}

// 审计 F3：Secret 作为未导出字段被 %+v / %#v 打印时不能输出字节。
func TestSecretInUnexportedFieldDoesNotPrintBytes(t *testing.T) {
	type holder struct{ tok secrets.Secret }
	h := holder{tok: NewSecret([]byte("sk-unexported-field-9876"))}
	for _, verb := range []string{"%v", "%+v", "%#v"} {
		out := fmt.Sprintf(verb, h)
		if strings.Contains(out, "unexported") || strings.Contains(out, "115 107 45") || strings.Contains(out, "736b2d") {
			t.Fatalf("%s 泄漏: %s", verb, out)
		}
	}
}
