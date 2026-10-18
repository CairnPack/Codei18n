package workflow

import (
	"context"
	"fmt"

	"github.com/CairnPack/codei18n/core/config"
)

// TranslateTextWith 用项目配置的翻译后端翻译一段文本，方向为 from → to（为空时为源语言 → 本地语言）。
func TranslateTextWith(ctx context.Context, env *Env, text, from, to string) (string, error) {
	trans, err := env.translator()
	if err != nil {
		return "", fmt.Errorf("初始化翻译引擎失败: %w", err)
	}
	if from == "" {
		from = env.Config.SourceLanguage.Value
	}
	if to == "" {
		to = env.Config.LocalLanguage.Value
	}
	return trans.Translate(ctx, text, from, to)
}

// TranslateText 翻译单段文本。
//
// Deprecated: 使用 TranslateTextWith。
func TranslateText(cfg *config.Config, opts TranslateOptions, text string) (string, error) {
	env := legacyEnv(cfg, opts)
	return TranslateTextWith(context.Background(), env, text, cfg.SourceLanguage, cfg.LocalLanguage)
}
