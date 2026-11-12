package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/store"
)

// --- HTTP 响应与脱敏工具 ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("写响应失败: %v", err)
	}
}

func writeErr(w http.ResponseWriter, code int, msg, typ string) {
	writeJSON(w, code, map[string]any{
		"error": map[string]any{"message": msg, "type": typ, "code": code},
	})
}

func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

// decodeOptionalJSON 解析「可以不带的」JSON 请求体。
//
// 空体是合法的（典型：刷新全部配额不带任何筛选），但**畸形 JSON 必须报错**：
// 静默把它当成空请求会让调用方以为参数已生效（历史行为：POST /admin/api/keys
// 发错字段时建出一把空名字的密钥，仍回 200）。
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(dst)
	if err == nil || errors.Is(err, io.EOF) {
		return true
	}
	writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON: "+err.Error(), "invalid_request_error")
	return false
}

// decodeJSONRequest 读取并解析入站 JSON 请求体。
//
// 三个推理入口（chat / responses / anthropic）此前各写一遍「读体 → 判超限 →
// 反序列化 → 写错误」，状态码与文案改一处漏两处。协议差异（OpenAI / Anthropic
// 错误信封）由 fail 回调注入，剩下的只有这一份实现。
func decodeJSONRequest[T any](w http.ResponseWriter, r *http.Request, fail func(code int, msg string)) (T, bool) {
	var v T
	raw, err := readBodyWithLimit(w, r)
	if err != nil {
		fail(http.StatusRequestEntityTooLarge, "请求体过大")
		return v, false
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		fail(http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return v, false
	}
	return v, true
}

// statusOf 把一次请求的错误映射成日志里的状态码。
//
// 必须优先采信 HTTPError 里的真实状态：上游的 400（请求体/模型不被接受）、
// 426（车道）、429（容量）都会被 writeUpstreamErr 原样回给客户端，若这里统一
// 记成 502，控制台的「状态 ≥ 400」筛选、错误率与归因就全是错的。
func statusOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var he *deepai.HTTPError
	if errors.As(err, &he) && he.Status >= 400 && he.Status < 600 {
		return he.Status
	}
	// 这两个必须与 writeUpstreamErr 的实际响应一致，否则日志里的状态码会撒谎：
	//   - 客户端断开：writeUpstreamErr 什么都不写（连接已经没了），日志记 499，
	//     不计失败（见 store.Record）；
	//   - 我们自己的超时：响应码是 504，日志却记 502，会让「上游 5xx」与
	//     「请求超时」混在一起。
	if errors.Is(err, context.Canceled) {
		return store.StatusClientGone
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}
