// Package secrets 管理 API 令牌：只存系统凭据库（本次实现 Windows 凭据管理器），绝不写入配置文件或日志。
//
// 凭据格式与 mygo Secrets（keytar 兼容）一致：generic credential，TargetName 为 "codei18n/<account>"，
// UserName 为 "<account>"。因此桌面端、CLI 与 git hook 共用同一份令牌。
package secrets

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Service 是固定的服务名。桌面端若直接调用 mygo，必须使用 mygo.Secrets.Service("codei18n")。
const Service = "codei18n"

// MaxSize 是单个令牌的字节上限（Windows 凭据管理器的限制）。
const MaxSize = 2560

var (
	// ErrNotFound 表示凭据库中没有该 account。
	ErrNotFound = errors.New("凭据不存在")
	// ErrTooLarge 表示令牌超过 MaxSize。
	ErrTooLarge = fmt.Errorf("令牌超过 %d 字节上限", MaxSize)
	// ErrEmpty 表示令牌为空。
	ErrEmpty = errors.New("令牌为空")
)

// Store 是凭据库接口。account 不含 "codei18n/" 前缀。
type Store interface {
	Get(account string) (Secret, error)
	Set(account string, secret Secret) error
	Delete(account string) error
	// List 返回以 prefix 开头的所有 account。
	List(prefix string) ([]string, error)
}

// TargetName 返回凭据库中的目标名。
func TargetName(account string) string { return Service + "/" + account }

// ProfileAccount 返回全局 profile 令牌的 account。
func ProfileAccount(profile string) string { return "profile/" + profile }

// ProjectAccount 返回项目级覆盖令牌的 account。projectID 是项目 config.json 中的 UUID。
func ProjectAccount(projectID, profile string) string { return "project/" + projectID + "/" + profile }

// ProjectProfileAccounts 列出所有项目中 profile 的覆盖令牌 account。
func ProjectProfileAccounts(s Store, profile string) ([]string, error) {
	all, err := s.List("project/")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, a := range all {
		if strings.HasSuffix(a, "/"+profile) && strings.Count(a, "/") == 2 {
			out = append(out, a)
		}
	}
	return out, nil
}

func validate(s Secret) error {
	switch {
	case s.Len() == 0:
		return ErrEmpty
	case s.Len() > MaxSize:
		return ErrTooLarge
	}
	return nil
}

// Memory 是内存中的 Store，用于测试。并发安全。
type Memory struct {
	mu sync.Mutex
	m  map[string][]byte
}

// NewMemory 创建内存 Store。
func NewMemory() *Memory { return &Memory{m: map[string][]byte{}} }

func (s *Memory) Get(account string) (Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.m[account]
	if !ok {
		return Secret{}, ErrNotFound
	}
	return NewSecret(b), nil
}

func (s *Memory) Set(account string, secret Secret) error {
	if err := validate(secret); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[account] = append([]byte(nil), secret.bytes()...)
	return nil
}

func (s *Memory) Delete(account string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[account]; !ok {
		return ErrNotFound
	}
	delete(s.m, account)
	return nil
}

func (s *Memory) List(prefix string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for k := range s.m {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}
