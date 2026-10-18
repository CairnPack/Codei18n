//go:build !windows

package secrets

import (
	"errors"
	"fmt"
)

// System 在非 Windows 平台上返回不可用的 Store：所有操作返回 errors.ErrUnsupported，并提示改用环境变量。
func System() Store { return unsupportedStore{} }

type unsupportedStore struct{}

var errUnsupported = fmt.Errorf("%w: 当前平台暂不支持系统凭据库，请改用环境变量 CODEI18N_API_KEY", errors.ErrUnsupported)

func (unsupportedStore) Get(string) (Secret, error)    { return Secret{}, errUnsupported }
func (unsupportedStore) Set(string, Secret) error      { return errUnsupported }
func (unsupportedStore) Delete(string) error           { return errUnsupported }
func (unsupportedStore) List(string) ([]string, error) { return nil, errUnsupported }
