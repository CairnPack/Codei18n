// Package atomicfile 提供"写临时文件 + rename"的原子写入，读方永远不会读到半截文件。
package atomicfile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// WriteFile 原子地把 data 写入 path：先写同目录下的临时文件并 fsync，再 rename 覆盖目标。
// 目标目录不存在时自动创建。
func WriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil && runtime.GOOS != "windows" {
		cleanup()
		return err
	}
	if err := rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("替换 %s 失败: %w", path, err)
	}
	return nil
}

// WriteJSON 以两空格缩进编码 v（不转义 HTML 字符，结尾带换行）并原子写入。
func WriteJSON(path string, v any) error {
	data, err := MarshalJSON(v)
	if err != nil {
		return err
	}
	return WriteFile(path, data, 0o644)
}

// MarshalJSON 与 WriteJSON 使用相同的编码格式，便于比较内容是否变化。
func MarshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// rename 在 Windows 上，目标文件被其他进程以不含 FILE_SHARE_DELETE 的方式打开（如正在读取）时会被拒绝，
// 以指数退避重试，总时长约 3 秒。
func rename(from, to string) error {
	var err error
	delay := 5 * time.Millisecond
	for deadline := time.Now().Add(3 * time.Second); ; {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		if runtime.GOOS != "windows" || time.Now().After(deadline) {
			return err
		}
		time.Sleep(delay)
		delay = min(delay*2, 200*time.Millisecond)
	}
}
