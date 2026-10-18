package config

import (
	"encoding/json"
	"sort"
	"strings"
)

// Plaintext 是配置文件中发现的旧版明文令牌字段。
type Plaintext struct {
	// Location 是字段位置，例如 "apiKey"、"translationConfig.api_key"、"profiles.ds.token"。
	Location string
	// Profile 是该令牌所属的配置档；顶层与 translationConfig 中的字段属于未选用 profile 时的默认配置（空串）。
	Profile string
	// Value 是明文值。调用方必须立即包装为 secrets.Secret，不得记录。
	Value string
}

// sensitiveOrder 决定同一位置出现多个明文字段时取哪一个。
var sensitiveOrder = []string{"apikey", "api_key", "token", "secret"}

func rank(k string) int {
	for i, s := range sensitiveOrder {
		if strings.EqualFold(k, s) {
			return i
		}
	}
	return len(sensitiveOrder)
}

// PlaintextTokens 列出本文件中的旧版明文令牌字段，按"顶层/translationConfig → profileOverrides → profiles"与字段优先级排序。
func (f *File) PlaintextTokens() []Plaintext {
	var out []Plaintext
	var top []Plaintext
	for k, raw := range f.extra {
		if IsSensitiveKey(k) {
			if v := rawString(raw); v != "" {
				top = append(top, Plaintext{Location: k, Value: v})
			}
		}
	}
	for k, v := range f.TranslationConfig {
		if IsSensitiveKey(k) && v != "" {
			top = append(top, Plaintext{Location: "translationConfig." + k, Value: v})
		}
	}
	sortByRank(top)
	out = append(out, top...)
	if f.ProfileOverrides != nil {
		out = append(out, providerPlaintext(f.ProfileOverrides, "profileOverrides", "")...)
	}
	names := make([]string, 0, len(f.Profiles))
	for name := range f.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if p := f.Profiles[name]; p != nil {
			out = append(out, providerPlaintext(p, "profiles."+name, name)...)
		}
	}
	return out
}

// RemovePlaintext 删除 Location 处的明文字段。
func (f *File) RemovePlaintext(loc string) {
	switch {
	case strings.HasPrefix(loc, "translationConfig."):
		delete(f.TranslationConfig, strings.TrimPrefix(loc, "translationConfig."))
	case strings.HasPrefix(loc, "profileOverrides."):
		if f.ProfileOverrides != nil {
			f.ProfileOverrides.DeleteExtra(strings.TrimPrefix(loc, "profileOverrides."))
		}
	case strings.HasPrefix(loc, "profiles."):
		rest := strings.TrimPrefix(loc, "profiles.")
		if i := strings.LastIndexByte(rest, '.'); i > 0 {
			if p := f.Profiles[rest[:i]]; p != nil {
				p.DeleteExtra(rest[i+1:])
			}
		}
	default:
		f.DeleteExtra(loc)
	}
}

func providerPlaintext(p *ProviderFields, prefix, profile string) []Plaintext {
	var out []Plaintext
	for k, raw := range p.extra {
		if IsSensitiveKey(k) {
			if v := rawString(raw); v != "" {
				out = append(out, Plaintext{Location: prefix + "." + k, Profile: profile, Value: v})
			}
		}
	}
	sortByRank(out)
	return out
}

func sortByRank(ps []Plaintext) {
	sort.SliceStable(ps, func(i, j int) bool {
		ki := ps[i].Location[strings.LastIndexByte(ps[i].Location, '.')+1:]
		kj := ps[j].Location[strings.LastIndexByte(ps[j].Location, '.')+1:]
		return rank(ki) < rank(kj)
	})
}

func rawString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return strings.TrimSpace(s)
}
