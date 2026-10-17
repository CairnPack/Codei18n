package translator

import (
	"fmt"
	"os"
	"strings"

	"github.com/CairnPack/codei18n/core"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/internal/log"
)

// NewFromConfig creates an appropriate translation engine implementation based on the configuration.
//
// Conventions:
//   - When provider is "openai" or "llm", use LLM translation based on OpenAI-compatible protocol
//   - When provider is "ollama", use the local Ollama service
//   - When provider is "mock", use the MockTranslator for testing
//   - When provider is "google" or "deepl", it is considered deprecated and an error is returned
//   - If provider is empty, it defaults to "openai"
func NewFromConfig(cfg *config.Config) (core.Translator, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.TranslationProvider))
	if provider == "" {
		provider = "openai"
	}

	// Handle compatibility with the llm-api naming that may appear in the documentation
	if provider == "llm-api" {
		provider = "openai"
	}

	switch provider {
	case "google", "deepl":
		return nil, fmt.Errorf("翻译提供商 %q 已不再支持，请修改配置为 \"openai\" 或 \"ollama\"", provider)
	case "mock":
		return NewMockTranslator(), nil
	case "openai", "llm":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("未设置 OPENAI_API_KEY 环境变量")
		}

		baseURL := os.Getenv("OPENAI_BASE_URL")
		if cfg.TranslationConfig != nil {
			// Compatibility for different cases and naming conventions
			if val, ok := cfg.TranslationConfig["baseUrl"]; ok && val != "" {
				baseURL = val
			} else if val, ok := cfg.TranslationConfig["BaseUrl"]; ok && val != "" {
				baseURL = val
			} else if val, ok := cfg.TranslationConfig["baseURL"]; ok && val != "" {
				baseURL = val
			} else if val, ok := cfg.TranslationConfig["base_url"]; ok && val != "" {
				baseURL = val
			} else if val, ok := cfg.TranslationConfig["baseurl"]; ok && val != "" {
				baseURL = val
			}
		}

		model := "gpt-3.5-turbo"
		if cfg.TranslationConfig != nil {
			if m, ok := cfg.TranslationConfig["model"]; ok && m != "" {
				model = m
			}
		}

		// DeepSeek model default BaseURL auto-detection
		if baseURL == "" && (model == "deepseek-chat" || model == "deepseek-coder") {
			baseURL = "https://api.deepseek.com"
			log.Info("自动检测到 DeepSeek 模型，设置 BaseURL 为 %s", baseURL)
		}

		log.Info("Using LLM: BaseURL=%s, Model=%s, BatchSize=%d", baseURL, model, cfg.BatchSize)
		return NewLLMTranslator(apiKey, baseURL, model), nil
	case "ollama":
		endpoint := "http://localhost:11434"
		model := "llama3"
		if cfg.TranslationConfig != nil {
			if v, ok := cfg.TranslationConfig["endpoint"]; ok && v != "" {
				endpoint = v
			}
			if m, ok := cfg.TranslationConfig["model"]; ok && m != "" {
				model = m
			}
		}
		log.Info("Using Ollama: Endpoint=%s, Model=%s, BatchSize=%d", endpoint, model, cfg.BatchSize)
		return NewOllamaTranslator(endpoint, model), nil
	default:
		return nil, fmt.Errorf("不支持的翻译提供商: %s", provider)
	}
}
