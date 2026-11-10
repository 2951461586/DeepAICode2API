package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// 启动参数与环境变量解析。
// 抽出来的目的：main() 只负责装配，环境读取的默认值集中在一处便于对照 README §五。

func defaultDataPath() string {
	const name = "deepaicode2api-data.json"

	if exe, err := os.Executable(); err == nil {
		legacy := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(legacy); err == nil {
			return legacy
		}
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "deepaicode2api", name)
	}
	return name
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

// isLoopbackListen 判断监听地址是否只接受本机连接。
//
// 空串 / 0.0.0.0 / :: 都表示监听全部网卡，属于对外暴露，不算回环。
func isLoopbackListen(host string) bool {
	host = strings.TrimSpace(host)
	switch strings.ToLower(host) {
	case "", "0.0.0.0", "::", "[::]":
		return false
	case "localhost":
		return true
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
