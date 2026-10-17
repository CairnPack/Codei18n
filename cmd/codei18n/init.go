package main

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/workflow"
	"github.com/CairnPack/codei18n/internal/log"
)

var (
	initSourceLang    string
	initTargetLang    string
	initProvider      string
	initWithTranslate bool
	initForce         bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化项目配置",
	Long:  `初始化项目配置，继承全局设置，并自动配置 Git 环境。可选执行初次扫描和翻译。`,
	Run: func(cmd *cobra.Command, args []string) {
		runInit(cmd)
	},
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().StringVar(&initSourceLang, "source-lang", "en", "源码语言")
	initCmd.Flags().StringVar(&initTargetLang, "target-lang", "zh-CN", "本地目标语言")
	initCmd.Flags().StringVar(&initProvider, "provider", "", "翻译提供商 (openai/llm/ollama)")
	initCmd.Flags().BoolVar(&initWithTranslate, "with-translate", false, "初始化后立即执行翻译")
	initCmd.Flags().BoolVar(&initForce, "force", false, "如果配置已存在，强制覆盖")
}

func runInit(cmd *cobra.Command) {
	// 1. Load effective config (Global + Defaults)
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Warn("无法加载配置，使用默认值: %v", err)
		cfg = config.DefaultConfig()
	}

	// 2. Apply flags
	// Note: Flags defined in init() act as overrides or defaults.
	// Cobra flags don't automatically populate cfg, we map them here.
	// But we need to distinguish between "default value" and "user provided value".
	// Since we set default values in flags, we might overwrite global config if we are not careful.
	// Actually, `initSourceLang` defaults to "en". If global config has "fr", `initSourceLang` will still be "en" if user didn't provide it?
	// No, cobra flags have default values.
	// We should only override if user EXPLICITLY provided the flag.
	// How to check if flag was changed? cmd.Flags().Changed("source-lang")

	if cmd.Flags().Changed("source-lang") {
		cfg.SourceLanguage = initSourceLang
	}
	if cmd.Flags().Changed("target-lang") {
		cfg.LocalLanguage = initTargetLang
	}
	if cmd.Flags().Changed("provider") {
		cfg.TranslationProvider = initProvider
	}

	// 3. Sanitize (Strip secrets inherited from global)
	// We create a new sanitized config object to save locally
	projectCfg := cfg.Sanitize()

	// 4. Save Config
	if _, err := os.Stat(".codei18n/config.json"); err == nil && !initForce {
		log.Fatal("配置文件已存在 (使用 --force 覆盖)")
	}

	if err := config.SaveConfig(projectCfg); err != nil {
		log.Fatal("保存配置失败: %v", err)
	}
	log.Success("项目配置文件已生成")

	// 5. Git Integration
	setupGitIntegration()

	// 6. Map Update (Auto Scan)
	log.Info("正在初始化注释映射...")
	mapResult, err := workflow.MapUpdate(projectCfg, ".", false)
	if err != nil {
		log.Warn("映射初始化失败: %v", err)
	} else {
		log.Success("已扫描 %d 条注释，生成初始映射", mapResult.TotalComments)
	}

	// 7. Conditional Translation
	if initWithTranslate {
		log.Info("正在执行初次翻译...")
		transResult, err := workflow.Translate(projectCfg, workflow.TranslateOptions{
			Concurrency: 5,
			// Use provider/model from config unless overridden (which we handled in step 2)
		})
		if err != nil {
			log.Warn("翻译失败: %v", err)
		} else {
			log.Success("翻译完成！共处理 %d 条注释", transResult.SuccessCount)
		}
	} else {
		log.Info("提示: 运行 'codei18n translate' 可生成翻译")
	}

	log.Success("项目初始化完成！")
}

func setupGitIntegration() {
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		return // Not a git repo
	}

	// 1. Update .gitignore
	gitignorePath := ".gitignore"
	content, _ := os.ReadFile(gitignorePath) // ignore error, treat as empty if missing
	sContent := string(content)
	if !strings.Contains(sContent, ".codei18n/") {
		f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Warn("更新 .gitignore 失败: %v", err)
		} else {
			if len(content) > 0 && !strings.HasSuffix(sContent, "\n") {
				f.WriteString("\n")
			}
			f.WriteString(".codei18n/\n")
			f.Close()
			log.Success("已将 .codei18n/ 添加到 .gitignore")
		}
	}

	// 2. Install hook
	// Assuming installHook is available in package main (from hook.go)
	if err := installPreCommitHook(); err != nil {
		log.Warn("安装 pre-commit hook 失败: %v", err)
	} else {
		log.Success("Git Pre-commit Hook 已自动安装")
	}
	if err := installCommitMsgHook(); err != nil {
		log.Warn("安装 commit-msg hook 失败: %v", err)
	} else {
		log.Success("Git Commit-msg Hook 已自动安装")
	}
}
