package workflow

import (
	"context"
	"fmt"

	"github.com/CairnPack/codei18n/adapters/translator"
	"github.com/CairnPack/codei18n/core/config"
)

// TranslateText translates a single text string with options
func TranslateText(cfg *config.Config, opts TranslateOptions, text string) (string, error) {
	// Apply overrides
	if opts.Provider != "" {
		cfg.TranslationProvider = opts.Provider
	}
	if opts.Model != "" {
		if cfg.TranslationConfig == nil {
			cfg.TranslationConfig = make(map[string]string)
		}
		cfg.TranslationConfig["model"] = opts.Model
	}

	// Init Translator
	trans, err := translator.NewFromConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("初始化翻译引擎失败: %w", err)
	}

	// Translate
	// Default direction: Source -> Local
	return trans.Translate(context.Background(), text, cfg.SourceLanguage, cfg.LocalLanguage)
}
