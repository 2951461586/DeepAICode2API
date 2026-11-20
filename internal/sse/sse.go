// Package sse 是 Server-Sent Events 的编解码。
//
// 为什么单独成包：网关里有两套**刻意不同**的读法，此前各写一份 ——
//
//	Decode   规范级：把同一事件里的多行 data 用换行拼好再分发
//	         （cmd/probe 用它，只关心最终载荷）；
//	internal/server.readSSELines   行级：一字不改地拿到每一行，供「透传」使用
//	                               （注释帧、event: 名、上游扩展字段都要原样转发）。
//
// 两份实现各自演化，改一处很容易漏另一处（例如缓冲区上限、注释帧处理）。
// 现在两种模式都在这里，调用方按需要显式选择；写入侧也只有一份。
package sse

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// Event 是一个 SSE 事件（规范级解析的产物）。
type Event struct {
	ID    string
	Event string
	Data  string
}

// Decode 按 SSE 规范解析事件流。
//
// 上游会在同一事件里发送多行 data，规范要求以 "\n" 拼接后整体分发。
func Decode(r io.Reader, fn func(Event) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var (
		id, event string
		data      []string
		hasData   bool
	)
	dispatch := func() error {
		if !hasData && event == "" && id == "" {
			return nil
		}
		ev := Event{ID: id, Event: event, Data: strings.Join(data, "\n")}
		id, event, data, hasData = "", "", nil, false
		return fn(ev)
	}

	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if line == "" {
			if err := dispatch(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // 注释 / 心跳
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "id":
			id = value
		case "event":
			event = value
		case "data":
			data = append(data, value)
			hasData = true
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return dispatch()
}

// Lines 逐行读取 SSE（不做规范级合并）。
//
// 刻意不复用 deepai.ReadSSE：那个实现按 SSE 规范把多行 data 合并后再分发，
// 会丢失注释帧与 event: 行的原始形态，而网关的透传路径需要它们。
func Lines(body io.Reader, fn func(line string) bool) error {
	br := bufio.NewReaderSize(body, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			if !fn(line) {
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// WriteData 写入一条 SSE data 事件。
func WriteData(w io.Writer, payload []byte) error {
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	_, err := w.Write([]byte("\n\n"))
	return err
}

// WriteSSEEvent 写入带 event 名的 SSE 事件（Anthropic / Responses 使用）。
func WriteEvent(w io.Writer, event string, payload []byte) error {
	if event != "" {
		if _, err := io.WriteString(w, "event: "+event+"\n"); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "data: "); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n\n")
	return err
}
