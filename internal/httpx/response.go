// Package httpx 收拢网关里与业务无关的 HTTP 工具：JSON 响应、错误信封、
// 请求体解码、gzip 中间件与错误状态码映射。
//
// 这些函数原先住在 internal/server 里，与 83 个文件、14.6k 行的路由/协议/
// 管理台代码混在一起。它们是纯叶子：只依赖标准库、deepai 的 HTTPError 与
// store 的状态码常量，不碰 *Server，因此可以整体抽出、单独测试。
package httpx

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// WriteJSON 写出一个 JSON 响应。
func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("写响应失败: %v", err)
	}
}

// WriteErr 写出网关统一错误信封。
//
// code 字段放的是 HTTP 状态码（历史行为，前端按 error.type 分类，
// 不依赖 code 的语义）。
func WriteErr(w http.ResponseWriter, code int, msg, typ string) {
	WriteJSON(w, code, map[string]any{
		"error": map[string]any{"message": msg, "type": typ, "code": code},
	})
}

// WriteRawJSON 直接写出已生成的 JSON 字节，避免再次序列化。
func WriteRawJSON(w http.ResponseWriter, code int, raw []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write(raw)
}

// ContextWithTimeout 给请求上下文加超时。
func ContextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}
