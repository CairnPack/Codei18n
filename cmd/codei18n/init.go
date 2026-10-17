package main

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n/core/config"
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
	p := openProject(config.Overrides{})

	// 1. 合并全局与默认配置，作为项目配置的快照（旧版 init 行为）
	cfg := effectiveConfig(p).Legacy()

	// 2. 只有用户显式传入的 flag 才覆盖
	if cmd.Flags().Changed("source-lang") {
		cfg.SourceLanguage = initSourceLang
	}
	if cmd.Flags().Changed("target-lang") {
		cfg.LocalLanguage = initTargetLang
	}
	if cmd.Flags().Changed("provider") {
		cfg.TranslationProvider = initProvider
	}

	// 3. 写入项目配置（去掉从全局继承的敏感字段，生成 projectId）
	if err := p.Init(codei18n.InitOptions{Snapshot: cfg, Force: initForce}); err != nil {
		if errors.Is(err, codei18n.ErrAlreadyInitialized) {
			log.Fatal("配置文件已存在 (使用 --force 覆盖)")
		}
		log.Fatal("保存配置失败: %v", err)
	}
	log.Success("项目配置文件已生成")

	// 4. Git 集成
	if p.IsGitRepo() {
		if changed, err := p.EnsureGitignore(); err != nil {
			log.Warn("更新 .gitignore 失败: %v", err)
		} else if changed {
			log.Success("已将 .codei18n/ 添加到 .gitignore")
		}
		if err := p.InstallHooks(); err != nil {
			log.Warn("安装 Git Hook 失败: %v", err)
		} else {
			log.Success("Git Pre-commit Hook 已自动安装")
			log.Success("Git Commit-msg Hook 已自动安装")
		}
	}

	ctx, cancel := signalContext()
	defer cancel()

	// 5. 初次扫描
	log.Info("正在初始化注释映射...")
	mapResult, err := p.UpdateMappings(ctx, codei18n.MapOptions{})
	if err != nil {
		log.Warn("映射初始化失败: %v", err)
	} else {
		log.Success("已扫描 %d 条注释，生成初始映射", mapResult.TotalComments)
	}

	// 6. 可选的初次翻译
	if initWithTranslate {
		log.Info("正在执行初次翻译...")
		transResult, err := runTranslateWithSpinner(ctx, p, codei18n.TranslateOptions{Concurrency: 5})
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
