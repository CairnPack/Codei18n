package workflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CairnPack/codei18n/core/config"
)

// 审计 F4：旧版 API（只读 OPENAI_API_KEY）也必须按实际令牌脱敏，不只是 sk- 形式。
func TestLegacyTranslateRedactsEnvToken(t *testing.T) {
	const tok = "gsk_LegacyGroqToken0123456789"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "Invalid API Key " + tok}})
	}))
	defer srv.Close()
	t.Setenv("OPENAI_API_KEY", tok)
	cfg := &config.Config{TranslationProvider: "openai", TranslationConfig: map[string]string{"baseUrl": srv.URL + "/v1"}}
	_, err := TranslateText(cfg, TranslateOptions{}, "hello")
	if err == nil || strings.Contains(err.Error(), "LegacyGroqToken") {
		t.Fatalf("错误应已脱敏: %v", err)
	}
	_ = context.Background()
}
