package translator

import (
	"context"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// prompt 必须要求保留占位符与 Markdown 结构（文档注释结构化翻译依赖它）。
func TestPromptsRequirePlaceholderPreservation(t *testing.T) {
	var prompts []string
	server := NewMockLLMServer(func(req *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
		prompts = append(prompts, req.Messages[0].Content)
		if strings.Contains(req.Messages[0].Content, "JSON array") {
			return createMockResponse(`["a","b"]`), nil
		}
		return createMockResponse("a"), nil
	})
	defer server.Close()
	tr := NewLLMTranslator("key", server.URL, "model")
	tr.Translate(context.Background(), "Returns ⟦0⟧.", "en", "zh-CN")
	tr.TranslateBatch(context.Background(), []string{"x ⟦0⟧", "y"}, "en", "zh-CN")
	if len(prompts) != 2 {
		t.Fatalf("%d", len(prompts))
	}
	for _, p := range prompts {
		if !strings.Contains(p, "⟦0⟧, ⟦1⟧ are placeholders") || !strings.Contains(p, "Markdown structure") {
			t.Fatalf("prompt 缺少占位符 / Markdown 规则:\n%s", p)
		}
	}
}
