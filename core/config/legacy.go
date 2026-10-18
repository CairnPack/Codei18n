package config

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// DirName 是项目与全局配置目录名。
const DirName = ".codei18n"

// FileName 是配置文件名。
const FileName = "config.json"

// GlobalDir 返回全局配置目录 ~/.codei18n。
func GlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, DirName), nil
}

// ProjectPath 返回项目配置文件路径 <root>/.codei18n/config.json。
func ProjectPath(root string) string { return filepath.Join(root, DirName, FileName) }

// Config 是旧版的扁平配置结构，保留给现有调用方与测试使用。
//
// Deprecated: 新代码请使用 File（单层文件）与 Effective（合并后的值及来源）。
type Config struct {
	SourceLanguage      string            `json:"sourceLanguage" mapstructure:"sourceLanguage"`
	LocalLanguage       string            `json:"localLanguage" mapstructure:"localLanguage"`
	ExcludePatterns     []string          `json:"excludePatterns" mapstructure:"excludePatterns"`
	TranslationProvider string            `json:"translationProvider" mapstructure:"translationProvider"`
	TranslationConfig   map[string]string `json:"translationConfig" mapstructure:"translationConfig"`
	BatchSize           int               `json:"batchSize" mapstructure:"batchSize"`
}

// DefaultConfig 返回默认配置。
func DefaultConfig() *Config {
	return &Config{
		SourceLanguage:      DefaultSourceLanguage,
		LocalLanguage:       DefaultLocalLanguage,
		ExcludePatterns:     DefaultExcludePatterns(),
		TranslationProvider: DefaultProvider,
		TranslationConfig:   make(map[string]string),
		BatchSize:           DefaultBatchSize,
	}
}

// LoadConfig 读取全局与当前目录下的项目配置并合并。
//
// Deprecated: 使用 codei18n.Open(root) 或 Resolve。
func LoadConfig() (*Config, error) {
	var global *File
	if dir, err := GlobalDir(); err == nil {
		g, err := ReadFile(filepath.Join(dir, FileName))
		if err != nil {
			return nil, err
		}
		global = g
	}
	project, err := ReadFile(ProjectPath("."))
	if err != nil {
		return nil, err
	}
	return Resolve(global, project, Overrides{}).Legacy(), nil
}

// sensitiveKeys 是旧版配置中可能出现的明文令牌字段名。
var sensitiveKeys = []string{"api_key", "apiKey", "token", "secret"}

// IsSensitiveKey 判断字段名是否是旧版明文令牌字段（不区分大小写）。
func IsSensitiveKey(k string) bool {
	for _, s := range sensitiveKeys {
		if strings.EqualFold(k, s) {
			return true
		}
	}
	return false
}

// Sanitize 返回去掉敏感字段的副本。
func (c *Config) Sanitize() *Config {
	newCfg := *c
	newCfg.ExcludePatterns = append([]string(nil), c.ExcludePatterns...)
	newCfg.TranslationConfig = make(map[string]string)
	for k, v := range c.TranslationConfig {
		if IsSensitiveKey(k) {
			continue
		}
		newCfg.TranslationConfig[k] = stripURLCredentials(v)
	}
	return &newCfg
}

// stripURLCredentials 去掉形如 https://user:pass@host 的 URL 中内嵌的凭据；非 URL 原样返回。
func stripURLCredentials(v string) string {
	u, err := url.Parse(v)
	if err != nil || u.User == nil || u.Host == "" {
		return v
	}
	u.User = nil
	return u.String()
}

// SaveConfig 把配置原子写入当前目录下的 .codei18n/config.json。
//
// Deprecated: 使用 WriteFile / UpdateFile。
func SaveConfig(cfg *Config) error {
	return UpdateFile(ProjectPath("."), func(f *File) error {
		f.ApplyLegacy(cfg.Sanitize())
		return nil
	})
}

// ApplyLegacy 把扁平配置的所有字段写入本层文件（覆盖同名字段，保留其他字段与 projectId）。
func (f *File) ApplyLegacy(c *Config) {
	f.SourceLanguage = ptr(c.SourceLanguage)
	f.LocalLanguage = ptr(c.LocalLanguage)
	f.ExcludePatterns = append([]string{}, c.ExcludePatterns...)
	f.TranslationProvider = ptr(c.TranslationProvider)
	f.TranslationConfig = make(map[string]string, len(c.TranslationConfig))
	for k, v := range c.TranslationConfig {
		f.TranslationConfig[k] = v
	}
	f.BatchSize = ptr(c.BatchSize)
}

// AsFile 把扁平配置视为一个配置层。
func (c *Config) AsFile() *File {
	f := &File{}
	f.ApplyLegacy(c)
	if c.TranslationProvider == "" {
		f.TranslationProvider = nil
	}
	if c.SourceLanguage == "" {
		f.SourceLanguage = nil
	}
	if c.LocalLanguage == "" {
		f.LocalLanguage = nil
	}
	if c.ExcludePatterns == nil {
		f.ExcludePatterns = nil
	}
	if c.BatchSize <= 0 {
		f.BatchSize = nil
	}
	return f
}

// Legacy 把合并后的配置转换为旧版扁平结构。
func (e *Effective) Legacy() *Config {
	tc := map[string]string{}
	p := e.Provider
	if p.Model.Value != "" && p.Model.Source != LayerDefault {
		tc["model"] = p.Model.Value
	}
	if p.BaseURL.Value != "" {
		tc["baseUrl"] = p.BaseURL.Value
	}
	if p.Endpoint.Value != "" && p.Endpoint.Source != LayerDefault {
		tc["endpoint"] = p.Endpoint.Value
	}
	return &Config{
		SourceLanguage:      e.SourceLanguage.Value,
		LocalLanguage:       e.LocalLanguage.Value,
		ExcludePatterns:     append([]string(nil), e.ExcludePatterns.Value...),
		TranslationProvider: p.Name.Value,
		TranslationConfig:   tc,
		BatchSize:           p.BatchSize.Value,
	}
}

func ptr[T any](v T) *T { return &v }

// Ptr 返回 v 的指针，便于构造 File / Overrides。
func Ptr[T any](v T) *T { return &v }
