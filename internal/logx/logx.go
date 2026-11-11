// Package logx 把网关日志统一成「一行一条 JSON」。
//
// 背景：排障时真正需要的是可 grep / 可 jq 的字段（request_id / account /
// model / latency），而 `log.Printf` 的自由文本只能靠正则。这里做两件事：
//
//  1. Setup 把标准库 log 的输出接管成 JSON 行——既有 50 余处 log.Printf
//     一行不改就变成结构化日志（msg 字段保留原文），不必逐处重写；
//  2. Info/Warn/Error 供新增代码直接写带字段的记录。
//
// 刻意不引入第三方日志库：stdlib 的 log/slog 已经够用，也符合「零依赖」目标。
package logx

import (
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
)

var (
	mu     sync.RWMutex
	logger *slog.Logger
)

// Setup 初始化全局日志：JSON 输出到 w（传 nil 用 stderr）。
//
// 必须在任何日志输出之前调用一次（main 的第一行）。
func Setup(w io.Writer) {
	if w == nil {
		w = os.Stderr
	}
	mu.Lock()
	logger = slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
	l := logger
	mu.Unlock()

	log.SetFlags(0)
	log.SetOutput(&lineWriter{logger: l, fallback: w})
}

// get 返回已配置的 logger；未调用 Setup 时回落到默认 logger。
func get() *slog.Logger {
	mu.RLock()
	l := logger
	mu.RUnlock()
	if l != nil {
		return l
	}
	return slog.Default()
}

// Info / Warn / Error 写一条带字段的结构化日志。
func Info(msg string, args ...any)  { get().Info(msg, args...) }
func Warn(msg string, args ...any)  { get().Warn(msg, args...) }
func Error(msg string, args ...any) { get().Error(msg, args...) }

// lineWriter 把标准库 log 的每一行包装成一条 JSON 记录。
//
// log 包可能在一条 Write 里带多行（不会发生在当前代码里，但保持健壮），
// 这里按行拆分逐条输出，避免把多行塞进一个 msg 破坏 JSONL 结构。
type lineWriter struct {
	logger   *slog.Logger
	fallback io.Writer
}

func (w *lineWriter) Write(p []byte) (int, error) {
	if w.logger == nil {
		return w.fallback.Write(p)
	}
	text := strings.TrimRight(string(p), "\r\n")
	if text == "" {
		return len(p), nil
	}
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			continue
		}
		// slog 的 JSON handler 自带 time/level/msg 字段，这里只补原文。
		w.logger.Info(line)
	}
	return len(p), nil
}
