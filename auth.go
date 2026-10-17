package codei18n

import (
	"fmt"

	"github.com/CairnPack/codei18n/adapters/translator"
	"github.com/CairnPack/codei18n/core/config"
)

// keySource 返回按需读取 API 令牌的函数。
func (p *Project) keySource(_ *config.Effective) translator.KeySource {
	return func() (string, error) {
		if k := p.s.getenv("OPENAI_API_KEY"); k != "" {
			return k, nil
		}
		return "", fmt.Errorf("未设置 OPENAI_API_KEY 环境变量")
	}
}
