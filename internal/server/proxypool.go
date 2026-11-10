package server

import (
	"log"
	"strings"

	"deepaicode2api/internal/proxy"
)

// loadProxyPool 按 PROXY_POOL → PROXY_POOL_FILE 的顺序构建轮询代理池。
//
// 配置来源与批量分配（action=assign-proxy）、浏览器登录入池保持一致；
// 未配置时返回 (nil, nil)，调用方据此跳过自动分配。
func loadProxyPool() (*proxy.ProxyPool, error) {
	cfg := proxy.LoadConfig()
	raw := cfg.Pool
	if strings.TrimSpace(raw) == "" && strings.TrimSpace(cfg.File) != "" {
		if fileRaw, err := proxy.ReadProxyFile(cfg.File); err == nil {
			raw = fileRaw
		}
	}
	return proxy.NewProxyPool(raw)
}

// enrollProxyPool 返回进程内复用的入池代理池。
//
// 解析失败只记日志并停用（不阻断入池）：代理是优化项，不是必需项。
func (s *Server) enrollProxyPool() *proxy.ProxyPool {
	s.enrollPoolOnce.Do(func() {
		pool, err := loadProxyPool()
		if err != nil {
			log.Printf("[enroll] 代理池配置错误，跳过自动分配: %v", err)
			return
		}
		s.enrollPool = pool
	})
	return s.enrollPool
}
