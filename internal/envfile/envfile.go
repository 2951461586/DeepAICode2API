// Package envfile 把 .env 注入进程环境，供网关与注册 CLI 共用。
//
// 为什么必须是独立包：`os.Setenv` 是进程级副作用，只有在读取配置
// （store.Open 的主密钥、flag 默认值求值）之前调用才有意义。此前这段
// 逻辑埋在 internal/register 里、由 LoadConfig 惰性触发，导致网关启动时
// 永远读不到 .env 中的 DEEPAI2API_MASTER_KEY，状态文件只能保持明文。
package envfile

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Load 按优先级加载 .env，仅执行一次。
//
// 路径顺序：
//  1. DEEPAI2API_ENV_FILE 显式指定；
//  2. 工作目录 .env（源码 / 解压部署）；
//  3. 可执行文件同目录 .env（分发部署）；
//  4. 可执行文件父目录 .env（bin/ 下构建的产物）。
//
// 真实环境变量优先：已存在的键不会被覆盖（loadDotEnv 会跳过）。
func Load() {
	once.Do(func() { LoadFiles(defaultPaths()...) })
}

var once sync.Once

// defaultPaths 返回默认的 .env 候选路径（按优先级）。
func defaultPaths() []string {
	var paths []string
	if p := strings.TrimSpace(os.Getenv("DEEPAI2API_ENV_FILE")); p != "" {
		paths = append(paths, p)
	}
	if wd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(wd, ".env"))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		paths = append(paths, filepath.Join(dir, ".env"), filepath.Join(dir, "..", ".env"))
	}
	return paths
}

// LoadFiles 按顺序加载给定 .env 文件，返回实际注入的键数量。
//
// 后加载的文件不会覆盖先前注入的值，真实环境变量也不会被覆盖。
// 供测试与需要自定义路径的调用方使用；常规路径用 Load。
func LoadFiles(paths ...string) int {
	n := 0
	for _, p := range paths {
		n += loadDotEnv(p)
	}
	return n
}

// loadDotEnv 解析一个 .env 文件并注入环境变量，返回注入的键数量。
//
// 支持 `KEY=VALUE`、`export KEY=VALUE`、# 注释与成对引号；
// 解析失败的行静默跳过（与既往行为一致，避免一个坏行拖垮启动）。
func loadDotEnv(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		if k == "" || os.Getenv(k) != "" {
			continue
		}
		if err := os.Setenv(k, v); err == nil {
			n++
		}
	}
	return n
}
