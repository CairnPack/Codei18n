package translator

import (
	"testing"

	"github.com/CairnPack/codei18n/core/config"
)

// Verification history providers (google/deepl) will be explicitly rejected with migration path instructions.
func TestNewFromConfig_RejectsLegacyProviders(t *testing.T) {
	cases := []string{"google", "deepl"}
	for _, provider := range cases {
		cfg := &config.Config{TranslationProvider: provider}
		tr, err := NewFromConfig(cfg)
		if err == nil {
			t.Fatalf("expected error for provider %q, got nil", provider)
		}
		if tr != nil {
			t.Fatalf("expected nil translator for provider %q", provider)
		}
	}
}

// Verify that unknown providers are rejected.
func TestNewFromConfig_UnknownProvider(t *testing.T) {
	cfg := &config.Config{TranslationProvider: "unknown-provider"}
	tr, err := NewFromConfig(cfg)
	if err == nil {
		t.Fatalf("expected error for unknown provider, got nil")
	}
	if tr != nil {
		t.Fatalf("expected nil translator for unknown provider")
	}
}
