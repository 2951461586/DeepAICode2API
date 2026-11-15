package httpx

import "strings"

// ETagMatches 判断 If-None-Match 里是否包含目标 ETag。
//
// 支持逗号分隔列表、弱校验前缀（W/）与 "*"（匹配任意当前表示）。
// 控制台资产与 /admin/api/* 的 304 协商共用这一份实现。
func ETagMatches(header, etag string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	if header == "*" {
		return true
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, part := range strings.Split(header, ",") {
		if strings.TrimPrefix(strings.TrimSpace(part), "W/") == want {
			return true
		}
	}
	return false
}
