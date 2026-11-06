// 依赖 internal/register 提供 stages / ledger / runner。
//
// register 是 DeepAI 账号“注册 → 登录 → 取 Session → 验活 → 入池”全链路工具。
//
// 纯 HTTP、无浏览器；邮箱后端可切换（CF Temp Email / 静态域名 / 固定邮箱）。
//
//	# 环境体检
//	go run ./cmd/register -doctor
//
//	# 注册 1 个账号并自动入池
//	go run ./cmd/register -count 1
//
//	# 并发注册 3 个，使用指定分组
//	go run ./cmd/register -count 3 -concurrency 2 -group batch-a
//
//	# 对台账里失败 / 未入池的账号补跑
//	go run ./cmd/register -resume
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/register"
)

var version = "dev"

func main() {
	var (
		count         = flag.Int("count", 1, "注册数量")
		concurrency   = flag.Int("concurrency", 1, "并发数")
		email         = flag.String("email", "", "指定邮箱（count 必须为 1；留空自动创建临时邮箱）")
		password      = flag.String("password", "", "指定密码（留空自动生成）")
		domain        = flag.String("domain", "", "邮箱域名 / 后缀（覆盖配置）")
		backend       = flag.String("mail-backend", "", "邮箱后端：cf / static / none（留空自动选择）")
		gateway       = flag.String("gateway", "", "2API 网关地址（默认取 DEEPAI2API_GATEWAY 或 http://127.0.0.1:10087）")
		adminPw       = flag.String("admin-password", "", "网关管理密码（默认取 DEEPAI2API_ADMIN_PASSWORD）")
		ledgerPath    = flag.String("ledger", "", "台账路径（默认 exports/register-ledger.jsonl）")
		resultDir     = flag.String("result-dir", "", "交付物目录（默认 result/）")
		group         = flag.String("group", "", "号池分组（默认 default）")
		proxy         = flag.String("proxy", "", "单个代理 URL（覆盖代理池）")
		proxyPool     = flag.String("proxy-pool", "", "代理池（换行/逗号分隔）")
		proxyFile     = flag.String("proxy-file", "", "代理池文件（每行一个）")
		noFingerprint = flag.Bool("no-fingerprint", false, "关闭浏览器指纹库")
		checkProxies  = flag.Bool("check-proxies", false, "只检查代理池连通性后退出")
		verify        = flag.Bool("verify", true, "是否自动收信并点击验证链接")
		probeChat     = flag.Bool("probe-chat", false, "验活时额外做一次最小对话探针")
		resume        = flag.Bool("resume", false, "补跑台账中未入池的账号")
		doctor        = flag.Bool("doctor", false, "只做环境体检，不出网注册")
		noEnroll      = flag.Bool("no-enroll", false, "只注册/验活，不写入网关号池")
		timeout       = flag.Duration("timeout", 0, "等待验证邮件超时（默认 120s）")
		interval      = flag.Duration("interval", 0, "收信轮询间隔（默认 1s）")
		delay         = flag.Duration("delay", 0, "两次注册之间的最小间隔（默认 800ms）")
		showVer       = flag.Bool("version", false, "打印版本后退出")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("deepaicode2api-register", version)
		return
	}

	cfg := register.LoadConfig()
	if *gateway != "" {
		cfg.Gateway = *gateway
	}
	if *adminPw != "" {
		cfg.AdminPassword = *adminPw
	}
	if *domain != "" {
		cfg.DefaultDomain = *domain
	}
	if *backend != "" {
		cfg.MailBackend = *backend
	}
	if *group == "" {
		*group = cfg.DefaultGroup
	}
	if *timeout > 0 {
		cfg.VerifyWait = *timeout
	}
	if *interval > 0 {
		cfg.PollEvery = *interval
	}
	if *delay > 0 {
		cfg.MinInterval = *delay
	}

	wd, _ := os.Getwd()
	if *ledgerPath == "" {
		*ledgerPath = filepath.Join(wd, "exports", "register-ledger.jsonl")
	}
	if *resultDir == "" {
		*resultDir = filepath.Join(wd, "result")
	}

	// 代理池：命令行 > 文件 > 配置（.env）。
	poolRaw := cfg.ProxyPool
	if *proxyPool != "" {
		poolRaw = *proxyPool
	}
	if *proxyFile != "" {
		raw, err := register.ReadProxyFile(*proxyFile)
		if err != nil {
			fatalf("读取代理文件失败: %v", err)
		}
		poolRaw = raw
	}
	if *proxy != "" {
		poolRaw = *proxy
	}
	pool, err := register.NewProxyPool(poolRaw)
	if err != nil {
		fatalf("代理池配置错误: %v", err)
	}

	if *checkProxies {
		if pool == nil || pool.Len() == 0 {
			fatalf("未配置代理池（PROXY_POOL / -proxy-pool / -proxy-file）")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		fmt.Printf("检查代理池（%d 个）…\n", pool.Len())
		results := register.CheckProxyPool(ctx, pool, 5)
		labels := make([]string, 0, len(results))
		for label := range results {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		bad := 0
		for _, label := range labels {
			if err := results[label]; err != "" {
				bad++
				fmt.Printf("  ✗ %-36s %v\n", label, err)
			} else {
				fmt.Printf("  ✓ %s\n", label)
			}
		}
		fmt.Printf("结果：可用 %d / %d\n", len(results)-bad, len(results))
		if bad > 0 {
			os.Exit(1)
		}
		return
	}

	if *doctor {
		printDoctor(cfg, pool)
		return
	}

	if *count < 1 || *count > 200 {
		fatalf("count 必须在 1..200 之间")
	}
	if *concurrency < 1 {
		*concurrency = 1
	}
	if capped, ok := pool.CapConcurrency(*concurrency); ok {
		fmt.Fprintf(os.Stderr, "并发 %d 超过出口数 %d，已压到 %d（多出的 worker 只会排队）\n", *concurrency, capped, capped)
		*concurrency = capped
	}
	if *email != "" && *count > 1 {
		fatalf("指定 -email 时 count 必须为 1")
	}

	backendName := cfg.AutoBackend()
	if missing := cfg.Validate(backendName); len(missing) > 0 {
		fatalf("邮箱后端 %s 缺少配置：%s", backendName, strings.Join(missing, ", "))
	}

	provider, err := register.NewProvider(cfg, backendName, cfg.DefaultDomain, *email)
	if err != nil {
		fatalf("初始化邮箱后端失败: %v", err)
	}

	var enroller register.Enroller
	if !*noEnroll {
		enroller = register.NewHTTPEnroller(cfg.Gateway, cfg.AdminPassword)
	}

	ledger := register.NewLedger(*ledgerPath)
	var printMu sync.Mutex
	opts := register.Options{
		Config:        cfg,
		Provider:      provider,
		Enroller:      enroller,
		Ledger:        ledger,
		Group:         *group,
		ProxyPool:     pool,
		NoFingerprint: *noFingerprint,
		Verify:        *verify,
		ProbeChat:     *probeChat,
		VerifyTimeout: cfg.VerifyWait,
		PollInterval:  cfg.PollEvery,
		MinInterval:   cfg.MinInterval,
		Log: func(format string, args ...any) {
			printMu.Lock()
			defer printMu.Unlock()
			fmt.Printf("%s "+format+"\n", append([]any{time.Now().Format("15:04:05")}, args...)...)
		},
		OnUpdate: func(rec *register.Record) {
			if rec == nil {
				return
			}
			key := "-"
			if rec.APIKey != "" {
				key = deepai.MaskSecret(rec.APIKey)
			}
			printMu.Lock()
			defer printMu.Unlock()
			fmt.Printf("   => %-10s %-42s key=%s theos=%s\n", rec.Status, rec.Email, key, rec.TheosGranted)
		},
	}
	pipe := register.New(opts)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("DeepAICode2API register %s | 后端=%s 网关=%s 分组=%s\n", version, backendName, cfg.Gateway, *group)
	if cfg.AdminPassword != "" {
		fmt.Println("管理密码：已配置")
	} else {
		fmt.Println("管理密码：未配置（仅本机回环管理接口可用）")
	}
	if *noEnroll {
		fmt.Println("模式：只注册/验活，不入池")
	}

	var records []*register.Record
	start := time.Now()
	if *resume {
		all, err := ledger.Load()
		if err != nil {
			fatalf("读取台账失败: %v", err)
		}
		pending := make([]*register.Record, 0, len(all))
		for _, rec := range all {
			if rec == nil {
				continue
			}
			if rec.Status == register.StatusEnrolled && rec.APIKey != "" {
				continue
			}
			pending = append(pending, rec)
		}
		if len(pending) == 0 {
			fmt.Println("台账里没有需要补跑的账号。")
			return
		}
		fmt.Printf("补跑 %d 个账号…\n", len(pending))
		records = pipe.RunRecords(ctx, pending, *concurrency)
	} else {
		fmt.Printf("开始注册 %d 个账号（并发 %d）…\n", *count, *concurrency)
		records = pipe.RunBatch(ctx, *count, *concurrency, *email, *password)
	}

	summary := register.Summarize(records)
	fmt.Printf("\n完成，用时 %s：总计 %d，入池 %d，有 Key %d，邮箱已验证 %d，失败 %d\n",
		time.Since(start).Round(time.Second), summary.Total, summary.Enrolled, summary.WithKey, summary.VerifiedE, summary.Failed)

	if summary.WithKey > 0 {
		out, err := register.WriteDeliverables(*resultDir, records)
		if err != nil {
			fmt.Printf("写交付物失败: %v\n", err)
		} else {
			fmt.Printf("交付物：\n  %s\n  %s\n  %s\n", out.KeysTxt, out.APIKeysTxt, out.SuccessJSON)
		}
	}

	if summary.WithKey == 0 {
		os.Exit(1)
	}
}

func printDoctor(cfg *register.Config, pool *register.ProxyPool) {
	fmt.Printf("deepaicode2api-register %s\n\n", version)
	if id := deepai.MachineID(); id != "" {
		fmt.Printf("机器指纹：%s…（必需头 X-DeepAI-Code-Machine）\n", id[:12])
	} else {
		fmt.Println("机器指纹：不可用 —— 上游会拒绝所有模型请求；请设置 DEEPAI_MACHINE_ID")
	}

	backend := cfg.AutoBackend()
	fmt.Printf("邮箱后端：%s\n", backend)
	if missing := cfg.Validate(backend); len(missing) > 0 {
		fmt.Printf("  缺少配置：%s\n", strings.Join(missing, ", "))
	} else {
		fmt.Println("  配置项：齐全")
	}
	if backend == "cf" {
		fmt.Printf("  Worker：%s（域名 %s）\n", maskURL(cfg.TempMailBase), pick(cfg.DefaultDomain, cfg.TempMailDomain))
		fmt.Printf("  Admin Key：%s\n", maskSecret(cfg.TempMailAdminKey))
	}
	if backend == "static" {
		fmt.Printf("  域名：%s\n", cfg.DefaultDomain)
		fmt.Println("  注意：static 后端没有收件箱，无法自动完成邮箱验证")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if backend != "none" {
		if err := register.CheckProvider(ctx, cfg, backend, cfg.DefaultDomain); err != nil {
			fmt.Printf("  健康检查：失败 —— %v\n", err)
		} else {
			fmt.Println("  健康检查：通过")
		}
	}

	fmt.Printf("网关：%s\n", cfg.Gateway)
	if err := register.NewHTTPEnroller(cfg.Gateway, cfg.AdminPassword).Health(ctx); err != nil {
		fmt.Printf("  健康检查：失败 —— %v\n", err)
	} else {
		fmt.Println("  健康检查：通过")
	}
	fmt.Printf("管理密码：%s\n", boolLabel(cfg.AdminPassword != ""))
	if pool != nil {
		fmt.Printf("代理池：%d 条出口（并发建议不超过该数）\n", pool.Len())
	} else {
		fmt.Println("代理池：未配置（直连；容易触发上游风控）")
	}
	fmt.Println("\n环境变量：TEMPMAIL_BASE / TEMPMAIL_ADMIN_KEY / TEMPMAIL_DOMAIN，")
	fmt.Println("          或 DEEPAI_REGISTER_DOMAIN + -mail-backend static；")
	fmt.Println("          网关 DEEPAI2API_GATEWAY / DEEPAI2API_ADMIN_PASSWORD。")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func boolLabel(v bool) string {
	if v {
		return "已配置"
	}
	return "未配置"
}

func maskSecret(s string) string { return deepai.MaskSecret(strings.TrimSpace(s)) }

func maskURL(s string) string {
	s = strings.TrimSpace(s)
	if u, err := parseURL(s); err == nil {
		return u
	}
	return s
}

func parseURL(s string) (string, error) {
	// 只展示主机名，避免输出过长路径。
	i := strings.Index(s, "://")
	if i < 0 {
		return "", fmt.Errorf("no scheme")
	}
	rest := s[i+3:]
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest = rest[:j]
	}
	return s[:i+3] + rest, nil
}

func pick(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
