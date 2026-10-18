package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"

	"github.com/CairnPack/codei18n/core/logging"
)

// LLMTranslator implements Translator using OpenAI compatible API
type LLMTranslator struct {
	client *openai.Client
	model  string
	log    logging.Logger
}

// NewLLMTranslator creates a new translator.
// apiKey: API Key
// baseURL: Optional base URL (for DeepSeek/others). If empty, uses OpenAI default.
// model: Model name (e.g. gpt-3.5-turbo, deepseek-chat)
func NewLLMTranslator(apiKey, baseURL, model string) *LLMTranslator {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	client := openai.NewClientWithConfig(config)
	return &LLMTranslator{
		client: client,
		model:  model,
		log:    logging.Nop{},
	}
}

// Translate translates a single text
func (t *LLMTranslator) Translate(ctx context.Context, text, from, to string) (string, error) {
	// Simple prompt construction
	prompt := fmt.Sprintf(
		"You are a professional code comment translator. Translate the following code comment from %s to %s.\n"+
			"Rules:\n"+
			"1. Keep technical terms, variable names, and code snippets unchanged.\n"+
			"2. Maintain the tone and style of the original comment.\n"+
			"3. Output ONLY the translated text, no explanations or quotes.\n"+
			"4. If the text is already in the target language, return it as is.\n"+
			"5. Preserve all line breaks and formatting.\n\n"+
			"Original: %s",
		from, to, text,
	)

	resp, err := t.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: t.model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
		},
	)

	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// TranslateBatch translates a batch of texts using a single LLM request with JSON array format.
// It falls back to sequential translation if batch processing fails.
func (t *LLMTranslator) TranslateBatch(ctx context.Context, texts []string, from, to string) ([]string, error) {
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

	// 1. Build Batch Prompt
	prompt := t.buildBatchPrompt(texts, from, to)

	// 2. Call LLM
	resp, err := t.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: t.model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
		},
	)

	if err != nil {
		t.log.Warnf("Batch translation API failed: %v. Falling back to sequential.", err)
		return t.translateSequential(ctx, texts, from, to)
	}

	if len(resp.Choices) == 0 {
		t.log.Warnf("Batch translation returned empty choices. Falling back to sequential.")
		return t.translateSequential(ctx, texts, from, to)
	}

	content := resp.Choices[0].Message.Content

	// 3. Parse JSON
	results, err := parseBatchResponse(content)
	if err != nil {
		t.log.Warnf("Batch translation JSON parse failed: %v. Content: %s... Falling back to sequential.", err, truncate(content, 50))
		return t.translateSequential(ctx, texts, from, to)
	}

	// 4. Verify Length
	if len(results) != len(texts) {
		t.log.Warnf("Batch translation length mismatch (in=%d, out=%d). Falling back to sequential.", len(texts), len(results))
		return t.translateSequential(ctx, texts, from, to)
	}

	return results, nil
}

func (t *LLMTranslator) buildBatchPrompt(texts []string, from, to string) string {
	inputJSON, _ := json.Marshal(texts)
	return fmt.Sprintf(
		"You are a code comment translator. Translate the following JSON array of comments from %s to %s.\n\n"+
			"Rules:\n"+
			"1. Maintain the JSON array format.\n"+
			"2. The output must be a valid JSON string array [\"...\",\"...\"].\n"+
			"3. The number of elements MUST match the input.\n"+
			"4. Keep technical terms, variable names, and code snippets unchanged.\n"+
			"5. If a comment is already in the target language, return it as is.\n"+
			"6. Preserve all line breaks and formatting.\n\n"+
			"Input:\n%s",
		from, to, string(inputJSON),
	)
}

func (t *LLMTranslator) translateSequential(ctx context.Context, texts []string, from, to string) ([]string, error) {
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

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
