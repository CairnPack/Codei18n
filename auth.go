package codei18n

import (
	"errors"
	"fmt"
	"sync"

	"github.com/CairnPack/codei18n/adapters/translator"
	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/logging"
	"github.com/CairnPack/codei18n/core/secrets"
)

// SecretStore 是凭据库接口（默认为系统凭据库）。
type SecretStore = secrets.Store

// Secret 持有令牌；打印、格式化、JSON 编码时只输出脱敏形式。
type Secret = secrets.Secret

// NewSecret 从字节构造令牌（去掉首尾空白）。
func NewSecret(b []byte) Secret { return secrets.NewSecret(b) }

// WithSecretStore 注入凭据库（测试用内存实现）。
func WithSecretStore(s SecretStore) Option { return func(st *settings) { st.secrets = s } }

// WithEnv 注入环境变量读取函数（测试用）。
func WithEnv(getenv func(string) string) Option { return func(st *settings) { st.getenv = getenv } }

// 环境变量名。
const (
	EnvAPIKey       = "CODEI18N_API_KEY"
	EnvLegacyAPIKey = "OPENAI_API_KEY"
)

// TokenSource 表示最终生效的令牌来自哪一层。
type TokenSource string

const (
	TokenSourceEnv     TokenSource = "env"         // 环境变量
	TokenSourceProject TokenSource = "project"     // 系统凭据库中的项目级覆盖令牌
	TokenSourceGlobal  TokenSource = "global"      // 系统凭据库中所选 profile 的全局令牌
	TokenSourceFile    TokenSource = "config-file" // 配置文件中的旧版明文字段（应迁移）
	TokenSourceNone    TokenSource = "none"
)

// PlaintextToken 描述配置文件中发现的一个旧版明文令牌字段（不含值）。
type PlaintextToken struct {
	// Layer 为 global 或 project。
	Layer    config.Layer `json:"layer"`
	Path     string       `json:"path"`
	Location string       `json:"location"`
	Profile  string       `json:"profile,omitempty"`
}

// TokenStatus 描述某个 profile 的令牌状态。不包含任何明文，只有末 4 位。
type TokenStatus struct {
	Profile string `json:"profile"`
	// Required 表示当前 provider 需要令牌（ollama / mock 不需要）。
	Required bool        `json:"required"`
	Source   TokenSource `json:"source"`
	Last4    string      `json:"last4,omitempty"`
	// EnvVar 是生效的环境变量名（Source 为 env 时）。
	EnvVar string `json:"envVar,omitempty"`

	ProjectAccount string `json:"projectAccount,omitempty"`
	ProjectSaved   bool   `json:"projectSaved"`
	ProjectLast4   string `json:"projectLast4,omitempty"`
	GlobalAccount  string `json:"globalAccount"`
	GlobalSaved    bool   `json:"globalSaved"`
	GlobalLast4    string `json:"globalLast4,omitempty"`
	// GlobalBlocked 表示项目配置把 baseUrl / endpoint 改成了与全局配置不同的地址，因此不使用全局令牌，
	// 防止随仓库分发的配置把用户的令牌发往不受信任的主机。
	GlobalBlocked bool `json:"globalBlocked,omitempty"`

	Plaintext []PlaintextToken `json:"plaintext,omitempty"`
	// StoreError 是凭据库不可用时的错误（例如非 Windows 平台）。
	StoreError string `json:"storeError,omitempty"`
}

// ErrNoToken 表示找不到可用的令牌。
var ErrNoToken = errors.New("未找到 API 令牌")

func (p *Project) store() SecretStore {
	if p.s.secrets == nil {
		return secrets.System()
	}
	return p.s.secrets
}

// globalAccount 返回所选 profile 全局令牌的 account：profile 的 apiKeyRef 优先，否则 profile/<name>。
func globalAccount(cfg *config.Effective) string {
	if ref := cfg.Provider.APIKeyRef.Value; ref != "" {
		return ref
	}
	return secrets.ProfileAccount(cfg.ProfileName())
}

// TokenStatus 返回当前生效 profile 的令牌状态。
func (p *Project) TokenStatus() (*TokenStatus, error) {
	cfg, err := p.Config()
	if err != nil {
		return nil, err
	}
	_, st, err := p.resolveToken(cfg)
	if err != nil && !errors.Is(err, ErrNoToken) {
		return nil, err
	}
	return st, nil
}

// resolveToken 按优先级查找令牌：环境变量 > 项目级覆盖令牌 > 所选 profile 的全局令牌 > 旧版明文字段。
func (p *Project) resolveToken(cfg *config.Effective) (Secret, *TokenStatus, error) {
	profile := cfg.ProfileName()
	st := &TokenStatus{
		Profile:       profile,
		Required:      translator.NeedsKey(cfg.Provider.Name.Value),
		Source:        TokenSourceNone,
		GlobalAccount: globalAccount(cfg),
	}
	store := p.store()
	var found Secret
	use := func(s Secret, src TokenSource) {
		if found.IsZero() && !s.IsZero() {
			found, st.Source, st.Last4 = s, src, s.Last4()
		}
	}

	for _, name := range []string{EnvAPIKey, EnvLegacyAPIKey} {
		if v := p.s.getenv(name); v != "" {
			if found.IsZero() {
				st.EnvVar = name
			}
			use(secrets.NewSecret([]byte(v)), TokenSourceEnv)
		}
	}
	if cfg.ProjectID != "" {
		st.ProjectAccount = secrets.ProjectAccount(cfg.ProjectID, profile)
		s, err := store.Get(st.ProjectAccount)
		switch {
		case err == nil:
			st.ProjectSaved, st.ProjectLast4 = true, s.Last4()
			use(s, TokenSourceProject)
		case !errors.Is(err, secrets.ErrNotFound):
			st.StoreError = err.Error()
		}
	}
	st.GlobalBlocked = !cfg.EndpointTrusted()
	if s, err := store.Get(st.GlobalAccount); err == nil {
		st.GlobalSaved, st.GlobalLast4 = true, s.Last4()
		if !st.GlobalBlocked {
			use(s, TokenSourceGlobal)
		}
	} else if !errors.Is(err, secrets.ErrNotFound) && st.StoreError == "" {
		st.StoreError = err.Error()
	}

	plain, err := p.plaintext(cfg)
	if err != nil {
		return Secret{}, nil, err
	}
	for _, pt := range plain {
		st.Plaintext = append(st.Plaintext, pt.info)
		if pt.applies && found.IsZero() {
			logging.OrNop(p.s.logger).Warnf("配置文件 %s 中的 %s 是明文令牌，已照常使用；请运行 codei18n auth migrate 迁移到系统凭据库", pt.info.Path, pt.info.Location)
			use(secrets.NewSecret([]byte(pt.value)), TokenSourceFile)
		}
	}
	if found.IsZero() && st.GlobalBlocked && st.GlobalSaved {
		addr := cfg.Provider.BaseURL.Value
		if addr == "" {
			addr = cfg.Provider.Endpoint.Value
		}
		return Secret{}, st, fmt.Errorf("%w：项目配置把翻译地址改成了 %q（与全局配置不同）。为防止令牌被发往不受信任的地址，"+
			"不会使用全局令牌。请为本项目保存专用令牌（codei18n auth set --project --profile %s），或把该地址写入全局配置的 profile",
			ErrNoToken, addr, profile)
	}
	if found.IsZero() {
		return Secret{}, st, fmt.Errorf("%w（profile %q）。请任选一种方式设置：\n"+
			"  1. 运行 codei18n auth set --profile %s，或在桌面端设置页保存（存入系统凭据库）\n"+
			"  2. 设置环境变量 %s", ErrNoToken, profile, profile, EnvAPIKey)
	}
	return found, st, nil
}

type plaintextHit struct {
	info    PlaintextToken
	value   string
	applies bool // 是否适用于当前生效的 profile
}

// plaintext 收集项目与全局配置中的明文令牌，并标记哪些适用于当前 profile（项目优先）。
func (p *Project) plaintext(cfg *config.Effective) ([]plaintextHit, error) {
	global, project, err := p.Files()
	if err != nil {
		return nil, err
	}
	selected := cfg.Profile.Value
	var out []plaintextHit
	for _, pt := range project.PlaintextTokens() {
		applies := pt.Profile == "" // 项目文件顶层 / translationConfig / profileOverrides 都属于本项目
		out = append(out, plaintextHit{PlaintextToken{config.LayerProject, p.ConfigPath(), pt.Location, pt.Profile}, pt.Value, applies})
	}
	for _, pt := range global.PlaintextTokens() {
		applies := pt.Profile == selected // 顶层字段对应"未选用 profile"
		out = append(out, plaintextHit{PlaintextToken{config.LayerGlobal, p.GlobalConfigPath(), pt.Location, pt.Profile}, pt.Value, applies})
	}
	return out, nil
}

// keySource 返回按需读取令牌的函数，并把读到的令牌登记到 redactor，用于抹去错误与日志中的令牌片段。
func (p *Project) keySource(cfg *config.Effective, r *redactor) translator.KeySource {
	return func() (string, error) {
		s, _, err := p.resolveToken(cfg)
		if err != nil {
			return "", err
		}
		r.add(s)
		return s.Reveal(), nil
	}
}

// redactor 记录本次任务用到的令牌，提供脱敏函数。
type redactor struct {
	mu      sync.Mutex
	secrets []Secret
}

func (r *redactor) add(s Secret) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.secrets = append(r.secrets, s)
}

func (r *redactor) redact(text string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.secrets {
		text = s.Redact(text)
	}
	return text
}

// SetProjectToken 把项目级覆盖令牌存入系统凭据库（项目没有 projectId 时生成并写入项目配置）。
func (p *Project) SetProjectToken(profile string, s Secret) error {
	if !p.Initialized() {
		return ErrNotInitialized
	}
	id, err := p.ensureProjectID()
	if err != nil {
		return err
	}
	return p.store().Set(secrets.ProjectAccount(id, profile), s)
}

// DeleteProjectToken 删除项目级覆盖令牌；不存在时返回 secrets.ErrNotFound。
func (p *Project) DeleteProjectToken(profile string) error {
	cfg, err := p.Config()
	if err != nil {
		return err
	}
	if cfg.ProjectID == "" {
		return secrets.ErrNotFound
	}
	return p.store().Delete(secrets.ProjectAccount(cfg.ProjectID, profile))
}

func (p *Project) ensureProjectID() (string, error) {
	var id string
	err := p.UpdateConfig(func(f *config.File) error {
		if f.ProjectID == "" {
			f.ProjectID = config.NewProjectID()
		}
		id = f.ProjectID
		return nil
	})
	return id, err
}

// MigratedToken 描述一次明文迁移。
type MigratedToken struct {
	PlaintextToken
	Account string `json:"account"`
}

// MigratePlaintextTokens 把项目配置中的明文令牌迁入系统凭据库（项目级覆盖令牌），并从文件中删除这些字段（原子写）。
// 同一位置有多个明文字段时取优先级最高的一个，其余一并删除。先写凭据库再改文件，失败时不会丢失令牌。
func (p *Project) MigratePlaintextTokens() ([]MigratedToken, error) {
	if err := p.lock(); err != nil {
		return nil, err
	}
	defer p.busy.Unlock()
	cfg, err := p.Config()
	if err != nil {
		return nil, err
	}
	_, project, err := p.Files()
	if err != nil {
		return nil, err
	}
	hits := project.PlaintextTokens()
	if len(hits) == 0 {
		return nil, nil
	}
	id, err := p.ensureProjectID()
	if err != nil {
		return nil, err
	}
	profile := cfg.ProfileName()
	account := secrets.ProjectAccount(id, profile)
	if err := p.store().Set(account, secrets.NewSecret([]byte(hits[0].Value))); err != nil {
		return nil, err
	}
	var out []MigratedToken
	err = p.UpdateConfig(func(f *config.File) error {
		for _, h := range hits {
			f.RemovePlaintext(h.Location)
			out = append(out, MigratedToken{PlaintextToken{config.LayerProject, p.ConfigPath(), h.Location, h.Profile}, account})
		}
		return nil
	})
	return out, err
}
