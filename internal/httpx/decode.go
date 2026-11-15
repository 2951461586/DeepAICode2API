package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// MaxRequestBody 限制入站请求体大小（32 MiB）。
const MaxRequestBody = 32 << 20

// ReadBodyWithLimit 读取入站请求体（上限 32 MiB）。
func ReadBodyWithLimit(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(w, r.Body, MaxRequestBody))
}

// DecodeOptionalJSON 解析「可以不带的」JSON 请求体。
//
// 空体是合法的（典型：刷新全部配额不带任何筛选），但**畸形 JSON 必须报错**：
// 静默把它当成空请求会让调用方以为参数已生效（历史行为：POST /admin/api/keys
// 发错字段时建出一把空名字的密钥，仍回 200）。
func DecodeOptionalJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(dst)
	if err == nil || errors.Is(err, io.EOF) {
		return true
	}
	WriteErr(w, http.StatusBadRequest, "请求体不是合法 JSON: "+err.Error(), "invalid_request_error")
	return false
}

// DecodeJSONRequest 读取并解析入站 JSON 请求体。
//
// 三个推理入口（chat / responses / anthropic）此前各写一遍「读体 → 判超限 →
// 反序列化 → 写错误」，状态码与文案改一处漏两处。协议差异（OpenAI / Anthropic
// 错误信封）由 fail 回调注入，剩下的只有这一份实现。
func DecodeJSONRequest[T any](w http.ResponseWriter, r *http.Request, fail func(code int, msg string)) (T, bool) {
	var v T
	raw, err := ReadBodyWithLimit(w, r)
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
