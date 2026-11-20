// Package configenv 统一「读环境变量 + 兜底默认值」的三种基本形态。
//
// 为什么收成一个包：这套逻辑此前有 5 份独立实现 ——
// 根包 env.go（envOr/envInt/envBool）、deepai.EnvOr/EnvInt、
// respstate.envPositiveInt、credimport.envOr、proxy 的解析。
// 它们对「空串 / 纯空白 / 非法数字 / 布尔写法」的处理细节各不相同，
// 于是「设了环境变量却没生效」每次都要重新查一遍是哪个实现。
//
// 语义（统一后）：
//   - 空串与纯空白都视为「未设置」，返回默认值；
//   - Int 解析失败返回默认值（不 panic、不报错）；
//   - Bool 接受 1/true/yes/on 与 0/false/no/off（大小写不敏感）。
package configenv

import (
	"os"
	"strconv"
	"strings"
)

// Or 返回环境变量值（去首尾空白），未设置或为空时返回 def。
func Or(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// Int 返回整型环境变量；未设置或解析失败时返回 def。
func Int(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

// Bool 返回布尔环境变量；未设置或无法识别时返回 def。
func Bool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

// PositiveInt 只接受正整数的环境变量（<=0 或解析失败时返回 def）。
//
// 用于「数量 / 时长」这类零值没有意义的配置：写成 0 通常是误操作，
// 静默当成 0 会让功能整体失效（例如把历史表上限设成 0）。
func PositiveInt(key string, def int) int {
	if n := Int(key, 0); n > 0 {
		return n
	}
	return def
}
