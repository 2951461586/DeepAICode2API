package register

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 默认值（可在 .env / 环境变量 / 命令行覆盖）。
const (
	DefaultGateway     = "http://127.0.0.1:10087"
	DefaultLanguage    = "en"
	DefaultUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36"
	DefaultVerifyWait  = 120 * time.Second
	DefaultPollEvery   = 1 * time.Second
	DefaultMinInterval = 800 * time.Millisecond
	DefaultProbeModel  = "glm-5.3-flash"
)

// Config 是注册链路的运行配置。
type Config struct {
	TempMailBase     string
	TempMailAdminKey string
	TempMailDomain   string

	// DefaultDomain 是静态邮箱后端使用的域名。
	DefaultDomain string
	// MailBackend 为 cf / static / none；空串表示自动选择。
	MailBackend string

	Gateway       string
	AdminPassword string

	// Proxy / ProxyPool：单个代理与轮询代理池（完整 URL）。
	Proxy     string
	ProxyPool string
	ProxyFile string

	Language      string
	UserAgent     string
	VerifyWait    time.Duration
	PollEvery     time.Duration
	MinInterval   time.Duration
	ProbeModel    string
	DefaultGroup  string
	DefaultPrefix string
}

// LoadConfig 读取环境变量，并尝试加载当前目录 / 可执行文件目录的 .env。
func LoadConfig() *Config {
	loadEnvFiles()

	cfg := &Config{
		TempMailBase:     strings.TrimRight(env("TEMPMAIL_BASE"), "/"),
		TempMailAdminKey: env("TEMPMAIL_ADMIN_KEY"),
		TempMailDomain:   env("TEMPMAIL_DOMAIN"),
		DefaultDomain:    firstNonEmpty(env("DEEPAI_REGISTER_DOMAIN"), env("TEMPMAIL_DOMAIN")),
		MailBackend:      strings.ToLower(env("DEEPAI_REGISTER_MAIL_BACKEND")),
		Gateway:          firstNonEmpty(env("DEEPAI2API_GATEWAY"), DefaultGateway),
		AdminPassword:    env("DEEPAI2API_ADMIN_PASSWORD"),
		Proxy:            env("DEEPAI_REGISTER_PROXY"),
		ProxyPool:        firstNonEmpty(env("PROXY_POOL"), env("DEEPAI_REGISTER_PROXY_POOL")),
		ProxyFile:        env("PROXY_POOL_FILE"),
		Language:         firstNonEmpty(env("DEEPAI_REGISTER_LANGUAGE"), DefaultLanguage),
		UserAgent:        firstNonEmpty(env("DEEPAI_REGISTER_UA"), DefaultUserAgent),
		VerifyWait:       DefaultVerifyWait,
		PollEvery:        DefaultPollEvery,
		MinInterval:      DefaultMinInterval,
		ProbeModel:       firstNonEmpty(env("DEEPAI_REGISTER_PROBE_MODEL"), DefaultProbeModel),
		DefaultGroup:     firstNonEmpty(env("DEEPAI_REGISTER_GROUP"), "default"),
		DefaultPrefix:    firstNonEmpty(env("DEEPAI_REGISTER_PREFIX"), "dai"),
	}
	return cfg
}

// AutoBackend 返回自动选择的邮箱后端。
func (c *Config) AutoBackend() string {
	if c.MailBackend != "" {
		return c.MailBackend
	}
	if c.TempMailBase != "" && c.TempMailAdminKey != "" {
		return "cf"
	}
	if c.DefaultDomain != "" {
		return "static"
	}
	return "none"
}

// Validate 返回缺失的必需项（doctor 用）。
func (c *Config) Validate(backend string) []string {
	var missing []string
	switch backend {
	case "cf":
		if c.TempMailBase == "" {
			missing = append(missing, "TEMPMAIL_BASE")
		}
		if c.TempMailAdminKey == "" {
			missing = append(missing, "TEMPMAIL_ADMIN_KEY")
		}
		if c.TempMailDomain == "" && c.DefaultDomain == "" {
			missing = append(missing, "TEMPMAIL_DOMAIN")
		}
	case "static":
		if c.DefaultDomain == "" {
			missing = append(missing, "DEEPAI_REGISTER_DOMAIN")
		}
	}
	return missing
}

// loadEnvFiles 按优先级加载 .env；真实环境变量优先。
// envLoadOnce 保证 .env 只加载一次。
//
// loadEnvFiles 通过 os.Setenv 注入变量，是进程级副作用：重复调用不仅浪费 IO，
// 还会让 deepai.EnvOr / main 里直读 os.Getenv 的地方看到「取决于调用顺序」
// 的结果。用 Once 固定为「首次调用时加载一次」，顺序依赖就消失了：
// .env 永远在第一次解析配置前生效，且之后不再变化。
var envLoadOnce sync.Once

// loadEnvFiles 按优先级加载 .env，仅执行一次。
//
// 真实环境变量优先：loadDotEnv 会跳过已存在的键。
func loadEnvFiles() {
	envLoadOnce.Do(loadEnvFilesOnce)
}

func loadEnvFilesOnce() {
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
	for _, p := range paths {
		loadDotEnv(p)
	}
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
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
		_ = os.Setenv(k, v)
	}
}

func env(key string) string { return strings.TrimSpace(os.Getenv(key)) }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
