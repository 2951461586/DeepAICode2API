package httpx

import (
	"context"
	"errors"
	"net/http"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/store"
)

// StatusOf 把一次请求的错误映射成日志里的状态码。
//
// 必须优先采信 HTTPError 里的真实状态：上游的 400（请求体/模型不被接受）、
// 426（车道）、429（容量）都会被 writeUpstreamErr 原样回给客户端，若这里统一
// 记成 502，控制台的「状态 ≥ 400」筛选、错误率与归因就全是错的。
func StatusOf(err error) int {
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
