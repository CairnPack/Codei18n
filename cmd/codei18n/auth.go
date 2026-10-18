package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/CairnPack/codei18n"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/secrets"
	"github.com/CairnPack/codei18n/internal/log"
)

var (
	authProfile string
	authProject bool
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "管理翻译 API 令牌（保存在系统凭据库）",
	Long: `令牌只保存在系统凭据库（Windows 凭据管理器），桌面端、CLI 与 git hook 共用同一份。
读取优先级：环境变量 CODEI18N_API_KEY（兼容 OPENAI_API_KEY）> 项目级覆盖令牌 > 所选 profile 的全局令牌。`,
}

var authSetCmd = &cobra.Command{
	Use:   "set",
	Short: "保存令牌（从标准输入或隐藏输入读取，不接受命令行参数）",
	Args:  rejectArgsQuietly,
	Run:   func(cmd *cobra.Command, args []string) { runAuthSet() },
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "显示令牌是否已保存、末 4 位与来源层",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) { runAuthStatus() },
}

var authDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除已保存的令牌",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) { runAuthDelete() },
}

var authMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "把配置文件中的旧版明文令牌迁入系统凭据库，并从文件中删除",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) { runAuthMigrate() },
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authSetCmd, authStatusCmd, authDeleteCmd, authMigrateCmd)
	for _, c := range []*cobra.Command{authSetCmd, authStatusCmd, authDeleteCmd} {
		c.Flags().StringVar(&authProfile, "profile", "", "配置档名（默认使用当前生效的配置档）")
	}
	for _, c := range []*cobra.Command{authSetCmd, authDeleteCmd} {
		c.Flags().BoolVar(&authProject, "project", false, "操作当前项目的覆盖令牌，而不是全局令牌")
	}
}

// rejectArgsQuietly 拒绝位置参数且不回显它们：误把令牌当参数传入时，错误信息（可能进入 CI 日志）里不能出现令牌。
func rejectArgsQuietly(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return errors.New("该命令不接受参数；请通过标准输入传入令牌，例如：echo <令牌> | codei18n auth set --profile <名称>")
	}
	return nil
}

func openGlobal() *codei18n.Global {
	g, err := codei18n.OpenGlobal(codei18n.WithLogger(cliLogger{}))
	if err != nil {
		log.Fatal("打开全局配置失败: %v", err)
	}
	return g
}

// profileName 返回 --profile 或当前生效的配置档名。
func profileName(p *codei18n.Project) string {
	if authProfile != "" {
		if err := codei18n.ValidateProfileName(authProfile); err != nil {
			log.Fatal("%v", err)
		}
		return authProfile
	}
	return effectiveConfig(p).ProfileName()
}

// readSecret 从管道读取，或在终端中隐藏输入读取。
func readSecret() codei18n.Secret {
	fd := int(os.Stdin.Fd())
	var data []byte
	var err error
	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, "请输入令牌（输入不会显示）: ")
		data, err = term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
	} else {
		data, err = io.ReadAll(io.LimitReader(os.Stdin, secrets.MaxSize*2))
	}
	if err != nil {
		log.Fatal("读取令牌失败: %v", err)
	}
	s := codei18n.NewSecret(data)
	for i := range data {
		data[i] = 0
	}
	if s.IsZero() {
		log.Fatal("令牌为空")
	}
	return s
}

func runAuthSet() {
	p := openProject(config.Overrides{})
	profile := profileName(p)
	s := readSecret()
	var err error
	if authProject {
		err = p.SetProjectToken(profile, s)
	} else {
		err = openGlobal().SetProfileToken(profile, s)
	}
	if err != nil {
		if errors.Is(err, codei18n.ErrNotInitialized) {
			log.Fatal("项目尚未初始化，请先运行 codei18n init")
		}
		log.Fatal("保存令牌失败: %v", err)
	}
	scope := "全局"
	if authProject {
		scope = "项目"
	}
	log.Success("已保存%s令牌（profile %s）: %s", scope, profile, s.Masked())
}

func runAuthStatus() {
	p := openProject(config.Overrides{})
	st, err := p.TokenStatus()
	if err != nil {
		log.Fatal("读取令牌状态失败: %v", err)
	}
	profile := st.Profile
	if authProfile != "" && authProfile != profile {
		// 查询非当前生效的配置档：只显示全局令牌
		saved, last4, err := openGlobal().ProfileTokenStatus(authProfile)
		if err != nil {
			log.Fatal("%v", err)
		}
		printSaved("全局令牌", "profile/"+authProfile, saved, last4)
		return
	}
	fmt.Printf("配置档: %s\n", profile)
	if !st.Required {
		fmt.Println("当前 provider 不需要令牌")
	}
	if st.ProjectAccount != "" {
		printSaved("项目令牌", st.ProjectAccount, st.ProjectSaved, st.ProjectLast4)
	} else {
		fmt.Println("项目令牌: 项目没有 projectId（运行 codei18n init 或 auth set --project 生成）")
	}
	printSaved("全局令牌", st.GlobalAccount, st.GlobalSaved, st.GlobalLast4)
	switch st.Source {
	case codei18n.TokenSourceNone:
		fmt.Println("生效来源: 无（翻译会失败）")
	case codei18n.TokenSourceEnv:
		fmt.Printf("生效来源: 环境变量 %s（••••%s）\n", st.EnvVar, st.Last4)
	default:
		fmt.Printf("生效来源: %s（••••%s）\n", sourceLabel(st.Source), st.Last4)
	}
	for _, pt := range st.Plaintext {
		log.Warn("%s 中的 %s 是明文令牌，请运行 codei18n auth migrate", pt.Path, pt.Location)
	}
	if st.StoreError != "" {
		log.Warn("系统凭据库不可用: %s", st.StoreError)
	}
}

func printSaved(label, account string, saved bool, last4 string) {
	if saved {
		fmt.Printf("%s: 已保存 ••••%s（%s）\n", label, last4, secrets.TargetName(account))
	} else {
		fmt.Printf("%s: 未保存（%s）\n", label, secrets.TargetName(account))
	}
}

func sourceLabel(s codei18n.TokenSource) string {
	switch s {
	case codei18n.TokenSourceProject:
		return "项目令牌"
	case codei18n.TokenSourceGlobal:
		return "全局令牌"
	case codei18n.TokenSourceFile:
		return "配置文件明文（请迁移）"
	}
	return string(s)
}

func runAuthDelete() {
	p := openProject(config.Overrides{})
	profile := profileName(p)
	var err error
	if authProject {
		err = p.DeleteProjectToken(profile)
	} else {
		err = openGlobal().DeleteProfileToken(profile)
	}
	if errors.Is(err, secrets.ErrNotFound) {
		log.Warn("没有已保存的令牌（profile %s）", profile)
		return
	}
	if err != nil {
		log.Fatal("删除令牌失败: %v", err)
	}
	log.Success("已删除令牌（profile %s）", profile)
}

func runAuthMigrate() {
	p := openProject(config.Overrides{})
	total := 0
	if p.Initialized() {
		m, err := p.MigratePlaintextTokens()
		if err != nil {
			log.Fatal("迁移项目配置中的明文令牌失败: %v", err)
		}
		for _, x := range m {
			log.Success("已迁移 %s 中的 %s → %s", x.Path, x.Location, secrets.TargetName(x.Account))
		}
		total += len(m)
	}
	m, err := openGlobal().MigratePlaintextTokens()
	if err != nil {
		log.Fatal("迁移全局配置中的明文令牌失败: %v", err)
	}
	for _, x := range m {
		log.Success("已迁移 %s 中的 %s → %s", x.Path, x.Location, secrets.TargetName(x.Account))
	}
	total += len(m)
	if total == 0 {
		log.Success("没有发现明文令牌")
	}
}
