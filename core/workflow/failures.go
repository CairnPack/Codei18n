package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/CairnPack/codei18n/internal/atomicfile"
)

// FailuresFileName 是翻译失败记录在缓存目录中的文件名。
const FailuresFileName = "translate-failures.json"

type failuresFile struct {
	Version  string    `json:"version"`
	Failures []Failure `json:"failures"`
}

// LoadFailures 读取上次翻译留下的失败记录。
func LoadFailures(env *Env) ([]Failure, error) {
	m, err := loadFailures(env)
	if err != nil {
		return nil, err
	}
	out := make([]Failure, 0, len(m))
	for _, f := range m {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func loadFailures(env *Env) (map[string]Failure, error) {
	out := map[string]Failure{}
	data, err := os.ReadFile(filepath.Join(env.CacheDir(), FailuresFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	var f failuresFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	for _, x := range f.Failures {
		out[x.ID+"\x00"+x.To] = x
	}
	return out, nil
}

func saveFailures(env *Env, m map[string]Failure) error {
	path := filepath.Join(env.CacheDir(), FailuresFileName)
	if len(m) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	f := failuresFile{Version: "1"}
	for _, x := range m {
		f.Failures = append(f.Failures, x)
	}
	sort.Slice(f.Failures, func(i, j int) bool { return f.Failures[i].ID < f.Failures[j].ID })
	return atomicfile.WriteJSON(path, f)
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(bearer\s+)?[^\s,;"']+`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{6,}`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{6,}`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|token|secret)\s*[:=]\s*)[^\s,;"']+`),
	regexp.MustCompile(`(?i)(provided:\s*)\S+`),
}

// SanitizeError 把错误转为字符串并抹去其中可能出现的令牌片段（Authorization 头、sk- 前缀密钥等）。
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	return RedactSecrets(err.Error())
}

// RedactSecrets 抹去文本中形似令牌的片段。
func RedactSecrets(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			sub := re.FindStringSubmatch(m)
			if len(sub) > 1 && sub[1] != "" {
				return sub[1] + "[REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return s
}
