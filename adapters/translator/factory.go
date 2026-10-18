package translator

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/CairnPack/codei18n/core"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/logging"
)

// Settings 是构造翻译后端所需的非敏感设置。令牌不放在这里，通过 KeySource 按需读取。
type Settings struct {
	Provider string
	BaseURL  string
	Model    string
	Endpoint string
	Logger   logging.Logger
}

// KeySource 在需要时返回 API 令牌。只有需要令牌的后端（openai/llm）才会调用它。
type KeySource func() (string, error)

// SettingsFrom 从合并后的配置提取后端设置。
func SettingsFrom(e *config.Effective, log logging.Logger) Settings {
	p := e.Provider
	return Settings{
		Provider: p.Name.Value,
		BaseURL:  p.BaseURL.Value,
		Model:    p.Model.Value,
		Endpoint: p.Endpoint.Value,
		Logger:   log,
	}
}

// NormalizeProvider 统一 provider 名称：空 → openai，llm-api → openai。
func NormalizeProvider(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	switch p {
	case "", "llm-api":
		return "openai"
	}
	return p
}

// NeedsKey 判断该 provider 是否需要 API 令牌。
func NeedsKey(provider string) bool {
	switch NormalizeProvider(provider) {
	case "openai", "llm":
		return true
	}
	return false
}

// New 根据设置构造翻译后端。
//
// 约定：
//   - provider 为 "openai" 或 "llm"：使用 OpenAI 兼容协议的 LLM 翻译
//   - provider 为 "ollama"：使用本地 Ollama 服务
//   - provider 为 "mock"：使用测试用的 MockTranslator
//   - provider 为 "google" 或 "deepl"：已不再支持，返回错误
//   - provider 为空：默认为 "openai"
func New(s Settings, key KeySource) (core.Translator, error) {
	log := logging.OrNop(s.Logger)
	provider := NormalizeProvider(s.Provider)
	switch provider {
	case "google", "deepl":
		return nil, fmt.Errorf("翻译提供商 %q 已不再支持，请修改配置为 \"openai\" 或 \"ollama\"", provider)
	case "mock":
		return NewMockTranslator(), nil
	case "openai", "llm":
		if key == nil {
			return nil, fmt.Errorf("未配置 API 令牌")
		}
		apiKey, err := key()
		if err != nil {
			return nil, err
		}
		baseURL := s.BaseURL
		if baseURL == "" {
			baseURL = os.Getenv("OPENAI_BASE_URL")
		}
		model := s.Model
		if model == "" {
			model = config.DefaultOpenAIModel
		}
		// DeepSeek 模型自动设置默认 BaseURL
		if baseURL == "" && (model == "deepseek-chat" || model == "deepseek-coder") {
			baseURL = "https://api.deepseek.com"
			log.Infof("自动检测到 DeepSeek 模型，设置 BaseURL 为 %s", baseURL)
		}
		log.Debugf("Using LLM: BaseURL=%s, Model=%s", redactURL(baseURL), model)
		t := NewLLMTranslator(apiKey, baseURL, model)
		t.log = log
		return t, nil
	case "ollama":
		endpoint := s.Endpoint
		if endpoint == "" {
			endpoint = config.DefaultOllamaEndpoint
		}
		model := s.Model
		if model == "" {
			model = config.DefaultOllamaModel
		}
		log.Debugf("Using Ollama: Endpoint=%s, Model=%s", redactURL(endpoint), model)
		return NewOllamaTranslator(endpoint, model), nil
	default:
		return nil, fmt.Errorf("不支持的翻译提供商: %s", provider)
	}
}

// redactURL 抹去 URL 中内嵌的用户名与密码（https://user:pass@host）。
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}

// EnvKey 从环境变量 OPENAI_API_KEY 读取令牌。
func EnvKey() (string, error) {
	if k := os.Getenv("OPENAI_API_KEY"); k != "" {
		return k, nil
	}
	return "", fmt.Errorf("未设置 OPENAI_API_KEY 环境变量")
}

// NewFromConfig 根据旧版扁平配置构造翻译后端，令牌读取环境变量 OPENAI_API_KEY。
//
// Deprecated: 使用 New 与 SettingsFrom。
func NewFromConfig(cfg *config.Config) (core.Translator, error) {
	e := config.Resolve(nil, cfg.AsFile(), config.Overrides{})
	return New(SettingsFrom(e, nil), EnvKey)
}
