package codei18n

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CairnPack/codei18n/core/config"
	"github.com/CairnPack/codei18n/core/secrets"
)

// Global 管理全局配置（~/.codei18n/config.json）中的 provider 配置档及其令牌，不依赖任何项目。
type Global struct {
	s settings
}

// ErrProfileExists / ErrProfileNotFound 用于配置档管理。
var (
	ErrProfileExists   = errors.New("配置档已存在")
	ErrProfileNotFound = errors.New("配置档不存在")
)

// OpenGlobal 打开全局配置。支持 WithGlobalDir、WithSecretStore、WithLogger、WithEnv。
func OpenGlobal(opts ...Option) (*Global, error) {
	g := &Global{}
	for _, o := range opts {
		o(&g.s)
	}
	if g.s.getenv == nil {
		g.s.getenv = os.Getenv
	}
	if g.s.globalDir == "" {
		dir, err := config.GlobalDir()
		if err != nil {
			return nil, err
		}
		g.s.globalDir = dir
	}
	return g, nil
}

// Path 返回全局配置文件路径。
func (g *Global) Path() string { return filepath.Join(g.s.globalDir, config.FileName) }

func (g *Global) store() SecretStore {
	if g.s.secrets == nil {
		return secrets.System()
	}
	return g.s.secrets
}

// File 读取全局配置文件。
func (g *Global) File() (*config.File, error) { return config.ReadFile(g.Path()) }

// Update 原子地修改全局配置文件（保留未识别字段）。
func (g *Global) Update(fn func(*config.File) error) error { return config.UpdateFile(g.Path(), fn) }

// Profiles 返回所有配置档名（排序）。
func (g *Global) Profiles() ([]string, error) {
	f, err := g.File()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(f.Profiles))
	for n := range f.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// ValidateProfileName 检查配置档名：非空，不含 / \ 与空白（它会成为凭据库 account 的一部分）。
func ValidateProfileName(name string) error {
	if name == "" || strings.ContainsAny(name, "/\\ \t\r\n") {
		return fmt.Errorf("无效的配置档名 %q：不能为空，不能包含 / \\ 或空白", name)
	}
	return nil
}

// SaveProfile 新建或覆盖一个配置档的字段（不涉及令牌）。
func (g *Global) SaveProfile(name string, fields config.ProviderFields) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	return g.Update(func(f *config.File) error {
		if f.Profiles == nil {
			f.Profiles = map[string]*config.ProviderFields{}
		}
		cp := fields
		f.Profiles[name] = &cp
		return nil
	})
}

// CopyProfile 复制配置档的字段为新名称。令牌不复制（避免在凭据库中悄悄产生副本），需要单独保存。
func (g *Global) CopyProfile(from, to string) error {
	if err := ValidateProfileName(to); err != nil {
		return err
	}
	return g.Update(func(f *config.File) error {
		src, ok := f.Profiles[from]
		if !ok {
			return ErrProfileNotFound
		}
		if _, exists := f.Profiles[to]; exists {
			return ErrProfileExists
		}
		data, err := src.MarshalJSON()
		if err != nil {
			return err
		}
		var cp config.ProviderFields
		if err := cp.UnmarshalJSON(data); err != nil {
			return err
		}
		f.Profiles[to] = &cp
		return nil
	})
}

// RenameProfile 重命名配置档，并把全局令牌与各项目的覆盖令牌迁移到新名称；全局 profile 选择同步更新。
// 项目配置文件中对旧名称的 profile 引用无法枚举，需要由调用方（桌面端对已知项目）更新。
func (g *Global) RenameProfile(from, to string) error {
	if err := ValidateProfileName(to); err != nil {
		return err
	}
	f, err := g.File()
	if err != nil {
		return err
	}
	if _, ok := f.Profiles[from]; !ok {
		return ErrProfileNotFound
	}
	if _, exists := f.Profiles[to]; exists {
		return ErrProfileExists
	}
	// 先迁移令牌（写新、删旧），再改文件。
	store := g.store()
	moves := map[string]string{secrets.ProfileAccount(from): secrets.ProfileAccount(to)}
	if accts, err := secrets.ProjectProfileAccounts(store, from); err == nil {
		for _, a := range accts {
			moves[a] = strings.TrimSuffix(a, from) + to
		}
	}
	for oldA, newA := range moves {
		s, err := store.Get(oldA)
		if errors.Is(err, secrets.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if err := store.Set(newA, s); err != nil {
			return err
		}
		if err := store.Delete(oldA); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			return err
		}
	}
	return g.Update(func(f *config.File) error {
		f.Profiles[to] = f.Profiles[from]
		delete(f.Profiles, from)
		if f.Profile != nil && *f.Profile == from {
			f.Profile = config.Ptr(to)
		}
		return nil
	})
}

// DeleteProfile 删除配置档，并一并删除它在凭据库中的全局令牌与各项目覆盖令牌。
func (g *Global) DeleteProfile(name string) error {
	if err := g.Update(func(f *config.File) error {
		if _, ok := f.Profiles[name]; !ok {
			return ErrProfileNotFound
		}
		delete(f.Profiles, name)
		if f.Profile != nil && *f.Profile == name {
			f.Profile = nil
		}
		return nil
	}); err != nil {
		return err
	}
	store := g.store()
	accounts := []string{secrets.ProfileAccount(name)}
	if accts, err := secrets.ProjectProfileAccounts(store, name); err == nil {
		accounts = append(accounts, accts...)
	}
	for _, a := range accounts {
		if err := store.Delete(a); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			return err
		}
	}
	return nil
}

// SetProfileToken 保存配置档的全局令牌到系统凭据库。保存后 CLI 与 git hook 立即可用。
func (g *Global) SetProfileToken(profile string, s Secret) error {
	if err := ValidateProfileName(profile); err != nil {
		return err
	}
	return g.store().Set(secrets.ProfileAccount(profile), s)
}

// DeleteProfileToken 删除配置档的全局令牌；不存在时返回 secrets.ErrNotFound。
func (g *Global) DeleteProfileToken(profile string) error {
	return g.store().Delete(secrets.ProfileAccount(profile))
}

// ProfileTokenStatus 返回配置档全局令牌是否已保存及末 4 位。
func (g *Global) ProfileTokenStatus(profile string) (saved bool, last4 string, err error) {
	s, err := g.store().Get(secrets.ProfileAccount(profile))
	if errors.Is(err, secrets.ErrNotFound) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, s.Last4(), nil
}

// PlaintextTokens 列出全局配置中的明文令牌字段（不含值）。
func (g *Global) PlaintextTokens() ([]PlaintextToken, error) {
	f, err := g.File()
	if err != nil {
		return nil, err
	}
	var out []PlaintextToken
	for _, pt := range f.PlaintextTokens() {
		out = append(out, PlaintextToken{config.LayerGlobal, g.Path(), pt.Location, pt.Profile})
	}
	return out, nil
}

// MigratePlaintextTokens 把全局配置中的明文令牌迁入凭据库：顶层 / translationConfig 中的字段迁到
// profile/default，profiles.<name>.* 迁到 profile/<name>；然后从文件中删除这些字段（原子写）。
func (g *Global) MigratePlaintextTokens() ([]MigratedToken, error) {
	f, err := g.File()
	if err != nil {
		return nil, err
	}
	hits := f.PlaintextTokens()
	if len(hits) == 0 {
		return nil, nil
	}
	store := g.store()
	done := map[string]bool{}
	var out []MigratedToken
	for _, h := range hits {
		profile := h.Profile
		if profile == "" {
			profile = config.DefaultProfileName
		}
		account := secrets.ProfileAccount(profile)
		if !done[account] { // 同一 profile 只迁移优先级最高的一个
			if err := store.Set(account, secrets.NewSecret([]byte(h.Value))); err != nil {
				return out, err
			}
			done[account] = true
		}
		out = append(out, MigratedToken{PlaintextToken{config.LayerGlobal, g.Path(), h.Location, h.Profile}, account})
	}
	return out, g.Update(func(f *config.File) error {
		for _, h := range hits {
			f.RemovePlaintext(h.Location)
		}
		return nil
	})
}
