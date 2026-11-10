package gateway

// 这一层只做「JSON 里取值」的宽松转换：上游字段类型不稳定，
// 拿不到就当零值，绝不因为一个字段形状不对让整轮请求失败。

// StrOf 取字符串，非字符串返回空串。
func StrOf(v any) string {
	s, _ := v.(string)
	return s
}

// BoolOf 取布尔，非布尔返回 false。
func BoolOf(v any) bool {
	b, _ := v.(bool)
	return b
}

// ClientError 是「请求本身不合法」的错误（映射成 4xx）。
//
// 与上游错误区分：它不由 deepai.HTTPError 表达，调用方据此决定是
// 回 400 还是回 502。
type ClientError struct{ Message string }

func (e *ClientError) Error() string { return e.Message }

// ErrNoMessages 表示转换后没有任何可用消息。
var ErrNoMessages = &ClientError{Message: "input 不能为空"}
