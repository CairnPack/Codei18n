package workflow

import (
	"context"
	"os"

	"github.com/CairnPack/codei18n/core/secrets"

	"github.com/CairnPack/codei18n/core/config"
)

// legacyEnv 用旧版扁平配置和当前目录构造 Env：旧 API 的路径都相对于 cwd。
func legacyEnv(cfg *config.Config, opts TranslateOptions) *Env {
	ov := config.Overrides{}
	if opts.Provider != "" {
		ov.Provider.Provider = config.Ptr(opts.Provider)
	}
	if opts.Model != "" {
		ov.Provider.Model = config.Ptr(opts.Model)
	}
	if opts.BatchSize > 0 {
		ov.Provider.BatchSize = config.Ptr(opts.BatchSize)
	}
	// 旧 API 直接读取环境变量中的令牌：同样登记到脱敏函数，避免后端错误回显非 sk- 形式的令牌。
	envKey := secrets.NewSecret([]byte(os.Getenv("OPENAI_API_KEY")))
	return &Env{Root: ".", Config: config.Resolve(nil, cfg.AsFile(), ov), Redact: envKey.Redact}
}

// MapUpdate 扫描 scanDir 并更新当前目录下的 .codei18n/mappings.json。
//
// Deprecated: 使用 UpdateMappings。
func MapUpdate(cfg *config.Config, scanDir string, dryRun bool) (*MapUpdateResult, error) {
	return UpdateMappings(context.Background(), legacyEnv(cfg, TranslateOptions{}), MapUpdateOptions{ScanDir: scanDir, DryRun: dryRun})
}

// Translate 翻译当前目录下 .codei18n/mappings.json 中缺失的条目。
//
// Deprecated: 使用 TranslateMissing。
func Translate(cfg *config.Config, opts TranslateOptions) (*TranslateResult, error) {
	return TranslateMissing(context.Background(), legacyEnv(cfg, opts), opts)
}
