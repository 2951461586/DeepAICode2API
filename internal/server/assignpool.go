package server

import (
	"os"
	"strings"
	"sync"
	"time"

	"deepaicode2api/internal/poolsvc"
	"deepaicode2api/internal/proxy"
)

// 分配用代理池：进程内缓存，配置变化时自动重载。
//
// 为什么要缓存：`ProxyPool.Next()` 的游标必须跨调用保留，否则每次「分配代理」
// 都从第 0 条开始，新账号全堆在同一条出口上（v0.1.17 修过这个 bug）。
//
// 为什么要重载：运维往 `PROXY_POOL_FILE` 追加新代理后，如果只认启动时那份快照，
// 控制台会「看不到」新代理，必须先重启网关（v0.1.17 之前的默认行为）。
// 这里用「环境变量值 + 文件 mtime」做指纹：任一变化就重建池（游标归零，
// 对「刚加完一批出口」这个场景恰好是我们想要的）。
type assignPoolCache struct {
	mu    sync.Mutex
	pool  *proxy.ProxyPool
	stamp string
}

// assignProxyPool 返回分配用代理池（按需重载）。
func (s *Server) assignProxyPool() (*proxy.ProxyPool, error) {
	cfg := proxy.LoadConfig()
	stamp := cfg.Pool + "|" + cfg.File + "|" + poolFileStamp(cfg.File)

	s.assignPool.mu.Lock()
	defer s.assignPool.mu.Unlock()
	if s.assignPool.pool != nil && s.assignPool.stamp == stamp {
		return s.assignPool.pool, nil
	}
	pool, err := poolsvc.LoadProxyPool()
	if err != nil {
		return nil, err
	}
	s.assignPool.pool, s.assignPool.stamp = pool, stamp
	return pool, nil
}

// poolFileStamp 返回代理文件的 mtime（读不到时为空串，等同于「没变」）。
func poolFileStamp(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	st, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return time.Unix(0, st.ModTime().UnixNano()).UTC().Format(time.RFC3339Nano)
}
