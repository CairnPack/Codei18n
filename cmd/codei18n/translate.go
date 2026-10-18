package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n/core/config"
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
		runTranslate(cmd)
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

func runTranslate(cmd *cobra.Command) {
	ov := config.Overrides{}
	if translateTarget != "" {
		ov.LocalLanguage = config.Ptr(translateTarget)
	}
	if translateSource != "" {
		ov.SourceLanguage = config.Ptr(translateSource)
	}
	if translateProvider != "" {
		ov.Provider.Provider = config.Ptr(translateProvider)
	}
	if translateModel != "" {
		ov.Provider.Model = config.Ptr(translateModel)
	}
	if translateBatchSize > 0 {
		ov.Provider.BatchSize = config.Ptr(translateBatchSize)
	}
	if cmd.Flags().Changed("concurrency") && translateConcurrency > 0 {
		ov.Provider.Concurrency = config.Ptr(translateConcurrency)
	}
	p := openProject(ov)
	ctx, cancel := signalContext()
	defer cancel()

	// 管道输入：直接翻译该文本
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal("Failed to read from stdin: %v", err)
		}
		text := string(bytes)
		if text == "" {
			return
		}
		translated, err := p.TranslateText(ctx, text, "", "")
		if err != nil {
			log.Fatal("Translation failed: %v", err)
		}
		fmt.Print(translated)
		return
	}

	result, err := runTranslateWithSpinner(ctx, p, codei18n.TranslateOptions{})
	if err != nil {
		if errors.Is(err, context.Canceled) && result != nil {
			log.Warn("翻译已取消：已完成 %d 条，结果已保存", result.SuccessCount)
			os.Exit(130)
		}
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

// runTranslateWithSpinner 执行翻译并在终端显示进度。spinner 只存在于 CLI 层。
func runTranslateWithSpinner(ctx context.Context, p *codei18n.Project, opts codei18n.TranslateOptions) (*codei18n.TranslateResult, error) {
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.Suffix = " 正在翻译..."
	s.Writer = os.Stderr
	var once sync.Once
	opts.Progress = func(e codei18n.Event) {
		if e.Phase != codei18n.PhaseTranslate {
			return
		}
		once.Do(s.Start)
		s.Lock()
		s.Suffix = fmt.Sprintf(" 正在翻译... (%d/%d)", e.Done, e.Total)
		s.Unlock()
	}
	defer s.Stop()
	return p.Translate(ctx, opts)
}
