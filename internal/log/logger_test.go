package log_test

import (
	"testing"

	"github.com/CairnPack/codei18n/internal/log"
)

// This test is manual verification mainly, but ensures compilation and basic function availability
func TestLogging(t *testing.T) {
	// These should go to stderr
	log.Info("This is an info message")
	log.Warn("This is a warning")
	log.Error("This is an error")

	// This should go to stdout
	log.PrintJSON([]byte(`{"test": "json"}`))
}
