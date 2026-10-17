//go:build windows

package secrets

import (
	"errors"
	"strings"

	"github.com/danieljoos/wincred"
)

// System 返回 Windows 凭据管理器。
func System() Store { return windowsStore{} }

type windowsStore struct{}

func (windowsStore) Get(account string) (Secret, error) {
	cred, err := wincred.GetGenericCredential(TargetName(account))
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return Secret{}, ErrNotFound
		}
		return Secret{}, err
	}
	return NewSecret(cred.CredentialBlob), nil
}

func (windowsStore) Set(account string, secret Secret) error {
	if err := validate(secret); err != nil {
		return err
	}
	cred := wincred.NewGenericCredential(TargetName(account))
	cred.UserName = account
	cred.CredentialBlob = []byte(secret.Reveal())
	// 与 mygo Secrets 一致（CRED_PERSIST_ENTERPRISE）。
	cred.Persist = wincred.PersistEnterprise
	return cred.Write()
}

func (windowsStore) Delete(account string) error {
	cred, err := wincred.GetGenericCredential(TargetName(account))
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return ErrNotFound
		}
		return err
	}
	return cred.Delete()
}

func (windowsStore) List(prefix string) ([]string, error) {
	creds, err := wincred.FilteredList(TargetName(prefix) + "*")
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, c := range creds {
		if a, ok := strings.CutPrefix(c.TargetName, Service+"/"); ok && strings.HasPrefix(a, prefix) {
			out = append(out, a)
		}
	}
	return out, nil
}
