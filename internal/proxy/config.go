package proxy

import (
	"os"
	"strings"

	"deepaicode2api/internal/envfile"
)

// Config 是代理池的来源配置。
type Config struct {
	// Pool 是直接写在环境变量里的代理列表（PROXY_POOL）。
	Pool string
	// File 指向代理文件（PROXY_POOL_FILE），Pool 为空时才读取。
	File string
}

// LoadConfig 读取代理池配置；真实环境变量优先于 .env。
//
// DEEPAI_REGISTER_PROXY_POOL 是注册链路时期的别名，保留以兼容既有 .env。
func LoadConfig() *Config {
	envfile.Load()
	return &Config{
		Pool: firstNonEmpty(os.Getenv("PROXY_POOL"), os.Getenv("DEEPAI_REGISTER_PROXY_POOL")),
		File: strings.TrimSpace(os.Getenv("PROXY_POOL_FILE")),
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
