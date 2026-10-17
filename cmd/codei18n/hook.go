package main

import (
	"github.com/spf13/cobra"

	"github.com/CairnPack/codei18n/core/gitint"
	"github.com/CairnPack/codei18n/internal/log"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "管理 Git Hooks",
}

var hookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "安装 pre-commit hook",
	Run: func(cmd *cobra.Command, args []string) {
		runHookInstall()
	},
}

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "卸载 pre-commit hook",
	Run: func(cmd *cobra.Command, args []string) {
		runHookUninstall()
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
}

func runHookInstall() {
	if err := gitint.InstallPreCommit("."); err != nil {
		log.Fatal("安装 pre-commit hook 失败: %v", err)
	}
	if err := gitint.InstallCommitMsg("."); err != nil {
		log.Fatal("安装 commit-msg hook 失败: %v", err)
	}
	log.Success("Git Hooks 安装成功")
}

func runHookUninstall() {
	for _, name := range []string{"pre-commit", "commit-msg"} {
		if err := gitint.Uninstall(".", name); err != nil {
			log.Warn("卸载 %s hook 失败: %v", name, err)
		}
	}
	log.Success("Git Hooks 已卸载")
}
