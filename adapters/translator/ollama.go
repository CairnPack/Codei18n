package translator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// OllamaTranslator uses the local Ollama service to perform translation.
// It interacts with the local model via the /api/chat interface.
type OllamaTranslator struct {
	endpoint   string
	model      string
	httpClient *http.Client
}

// NewOllamaTranslator creates a new OllamaTranslator.
// endpoint e.g., http://localhost:11434
// model e.g., "llama3", "qwen2.5:7b", etc.
func NewOllamaTranslator(endpoint, model string) *OllamaTranslator {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	if model == "" {
		model = "llama3"
	}

	return &OllamaTranslator{
		endpoint: endpoint,
		model:    model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Translate implements single text translation.
func (t *OllamaTranslator) Translate(ctx context.Context, text, from, to string) (string, error) {
	prompt := fmt.Sprintf(
		"You are a professional code comment translator. Translate the following code comment from %s to %s.\n"+
			"Rules:\n"+
			"1. Keep technical terms, variable names, and code snippets unchanged.\n"+
			"2. Maintain the tone and style of the original comment.\n"+
			"3. Output ONLY the translated text, no explanations or quotes.\n"+
			"4. If the text is already in the target language, return it as is.\n"+
			"5. Preserve all line breaks and formatting.\n"+
			"6. Tokens like ⟦0⟧, ⟦1⟧ are placeholders for protected code, tags and references. Keep every placeholder exactly as written, the same number of times and in the same order; never translate, remove, add or reorder them.\n"+
			"7. Preserve the Markdown structure (lists, emphasis, inline code, code fences).\n\n"+
			"Original: %s",
		from, to, text,
	)

	reqBody := struct {
		Model    string          `json:"model"`
		Messages []ollamaMessage `json:"messages"`
		Stream   bool            `json:"stream"`
		// think controls the chain-of-thought mode. For thinking models like DeepSeek/Qwen, explicitly turning it off can avoid additional consumption.
		Think   bool              `json:"think"`
		Options map[string]string `json:"options,omitempty"`
	}{
		Model: t.model,
		Messages: []ollamaMessage{
			{Role: "user", Content: prompt},
		},
		Stream: false,
		Think:  false,
	}

	buf, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint+"/api/chat", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama 请求失败: %s", resp.Status)
	}

	var respBody struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return "", err
	}

	if respBody.Message.Content == "" {
		return "", fmt.Errorf("ollama 返回内容为空")
	}

	return strings.TrimSpace(respBody.Message.Content), nil
}

// TranslateBatch is implemented with sequential calls under Ollama to avoid prematurely introducing complex batch protocols.
func (t *OllamaTranslator) TranslateBatch(ctx context.Context, texts []string, from, to string) ([]string, error) {
	if len(texts) == 0 {
		return []string{}, nil
	}
	if len(texts) == 1 {
		res, err := t.Translate(ctx, texts[0], from, to)
		if err != nil {
			return nil, err
		}
		return []string{res}, nil
	}

	results := make([]string, len(texts))
	for i, text := range texts {
		res, err := t.Translate(ctx, text, from, to)
		if err != nil {
			return nil, err
		}
		results[i] = res
	}
	return results, nil
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
