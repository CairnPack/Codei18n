package index

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/CairnPack/codei18n/internal/atomicfile"
)

// OverridesFileName 是用户手动纠正原语言的持久化文件名。
const OverridesFileName = "overrides.json"

// Overrides 是 overrides.json 的结构：注释 ID → 用户纠正后的原语言（规范码）。
type Overrides struct {
	Version   string            `json:"version"`
	Languages map[string]string `json:"languages"`
}

// ReadOverrides 读取纠正记录；文件不存在时返回空记录。
func ReadOverrides(path string) (*Overrides, error) {
	o := &Overrides{Version: Version, Languages: map[string]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return o, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, o); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	if o.Languages == nil {
		o.Languages = map[string]string{}
	}
	return o, nil
}

// WriteOverrides 原子写出纠正记录。
func WriteOverrides(path string, o *Overrides) error {
	o.Version = Version
	return atomicfile.WriteJSON(path, o)
}
