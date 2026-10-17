// Package logging 定义库代码使用的可注入日志接口。库代码只通过 Logger 输出，不直接写 stdout/stderr。
package logging

// Logger 是库代码使用的日志接口。实现方负责格式化与输出位置（CLI 写 stderr，GUI 写日志面板）。
// 任何实现都不会收到 API 令牌明文：库代码从不把令牌放进日志参数。
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
}

// Nop 丢弃所有日志。
type Nop struct{}

func (Nop) Debugf(string, ...any) {}
func (Nop) Infof(string, ...any)  {}
func (Nop) Warnf(string, ...any)  {}

// OrNop 在 l 为 nil 时返回 Nop。
func OrNop(l Logger) Logger {
	if l == nil {
		return Nop{}
	}
	return l
}
