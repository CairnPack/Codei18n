package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/workflow"
	"github.com/CairnPack/codei18n/internal/log"
)

var (
	translateProvider    string
	translateModel       string
	translateConcurrency int
	translateBatchSize   int
	translateTarget      string
	translateSource      string
)

var translateCmd = &cobra.Command{
	Use:   "translate",
	Short: "自动翻译缺失的注释",
	Long: `调用配置的翻译引擎（LLM(OpenAI/DeepSeek) 或本地 Ollama）自动翻译映射文件中缺失的条目。
如果通过管道传入文本，则直接翻译该文本并输出到标准输出。
可以使用 --target 和 --source 标志来覆盖默认的语言设置。`,
	Run: func(cmd *cobra.Command, args []string) {
		runTranslate()
	},
}

func init() {
	rootCmd.AddCommand(translateCmd)

	translateCmd.Flags().StringVar(&translateProvider, "provider", "", "覆盖配置文件中的提供商 (llm, openai, mock)")
	translateCmd.Flags().StringVar(&translateModel, "model", "", "指定模型 (如 gpt-3.5-turbo)")
	translateCmd.Flags().IntVar(&translateConcurrency, "concurrency", 5, "并发请求数")
	translateCmd.Flags().IntVar(&translateBatchSize, "batch-size", 0, "每批翻译的数量 (覆盖配置)")
	translateCmd.Flags().StringVarP(&translateTarget, "target", "t", "", "指定目标语言 (如 en, zh-CN)")
	translateCmd.Flags().StringVarP(&translateSource, "source", "s", "", "指定源语言 (如 zh-CN, en)")
}

func runTranslate() {
	// 1. Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Warn("无法加载配置: %v", err)
		cfg = config.DefaultConfig()
	}

	// Apply language overrides
	if translateTarget != "" {
		cfg.LocalLanguage = translateTarget
	}
	if translateSource != "" {
		cfg.SourceLanguage = translateSource
	}

	opts := workflow.TranslateOptions{
		Concurrency: translateConcurrency,
		Provider:    translateProvider,
		Model:       translateModel,
		BatchSize:   translateBatchSize,
	}

	// Check for stdin input
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Data is being piped to stdin
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal("Failed to read from stdin: %v", err)
		}
		text := string(bytes)
		if text == "" {
			return
		}

		translated, err := workflow.TranslateText(cfg, opts, text)
		if err != nil {
			log.Fatal("Translation failed: %v", err)
		}
		fmt.Print(translated)
		return
	}

	result, err := workflow.Translate(cfg, opts)
	if err != nil {
		log.Fatal("Translation failed: %v", err)
	}

	if result.TotalTasks == 0 {
		log.Success("所有注释已翻译，无需操作")
		return
	}

	if result.FailCount > 0 {
		log.Warn("翻译完成，但有 %d 条失败", result.FailCount)
	} else {
		log.Success("翻译完成！共处理 %d 条注释", result.SuccessCount)
	}
}
