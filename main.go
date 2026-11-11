// DeepAICode2API 把本机 DeepAI Code 桌面端的上游模型服务，
// 逆向复刻成一个 OpenAI / Anthropic 兼容的 Go 网关。
//
// 复用桌面端已登录的 API Key，无需另申请密钥；纯 Go 标准库实现。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/envfile"
	"deepaicode2api/internal/logx"
	"deepaicode2api/internal/server"
	"deepaicode2api/internal/store"
)

// version 由构建时注入：-ldflags "-X main.version=1.0.0"
var version = "dev"

func main() {
	// 日志先于一切：把标准库 log 的输出接管成一行一条 JSON。
	// 既有 log.Printf 调用不需要改，msg 字段保留原文。
	logx.Setup(nil)

	// 必须在任何环境变量读取之前执行：flag 默认值（envOr）与 store.Open 的
	// 主密钥都直读进程环境。此前 .env 只在注册链路里惰性加载，导致
	// DEEPAI2API_MASTER_KEY 对网关启动永远不生效（状态文件只能保持明文）。
	envfile.Load()

	var (
		host       = flag.String("host", envOr("DEEPAI2API_HOST", "127.0.0.1"), "监听地址")
		port       = flag.Int("port", envInt("DEEPAI2API_PORT", 10087), "监听端口")
		dataPath   = flag.String("data", envOr("DEEPAI2API_DATA_PATH", ""), "状态文件路径（默认在可执行文件同目录）")
		deepaiDir  = flag.String("deepai-dir", envOr("DEEPAI2API_DATA_DIR", ""), "DeepAI Code 数据目录（留空自动探测）")
		importOnly = flag.Bool("import", false, "仅执行一次桌面端凭据导入后退出")
		noImport   = flag.Bool("no-import", false, "启动时不自动导入桌面端凭据")
		adminPw    = flag.String("admin-password", envOr("DEEPAI2API_ADMIN_PASSWORD", ""), "管理密码（非空时覆盖状态文件中的设置）")
		allowOpen  = flag.Bool("allow-insecure", envBool("DEEPAI2API_ALLOW_INSECURE", false), "允许非回环监听但不设管理密码（不安全）")
		seal       = flag.Bool("seal", false, "把状态/统计/日志中的存量明文封存为密文后退出（需 "+store.EnvMasterKey+"）")
		sealYes    = flag.Bool("yes", false, "配合 -seal：确认执行不可逆的封存")
		showVer    = flag.Bool("version", false, "打印版本后退出")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("deepaicode2api", version)
		return
	}

	if *dataPath == "" {
		*dataPath = defaultDataPath()
	}

	st, err := store.Open(*dataPath)
	if err != nil {
		log.Fatalf("打开状态文件失败: %v", err)
	}
	log.Printf("状态文件: %s", st.Path())
	if st.MasterKeySet() {
		log.Printf("状态文件加密: 已启用（AES-256-GCM，密钥指纹 %s，来自 %s）", st.MasterKeyFingerprint(), store.EnvMasterKey)
		if !st.Encrypted() {
			log.Printf("  磁盘上仍是明文：首次保存会自动重写；历史日志可用 -seal 立即封存。")
		}
	} else {
		log.Printf("⚠ 状态文件未加密：上游 API Key / 账号密码以明文落盘。")
		log.Printf("  把 %s=$(openssl rand -hex 32) 写进 .env 后重启即可（.env 现在会在启动时加载），"+
			"再执行 -seal 把历史明文一并封存。", store.EnvMasterKey)
	}

	if *seal {
		runSeal(st, *sealYes)
		return
	}

	machine := deepai.MachineID()
	if machine != "" {
		st.SetMachineID(machine)
		log.Printf("机器指纹: %s…（用于上游按天发放免费 Theos）", machine[:12])
	} else {
		log.Printf("⚠ 未能读取机器指纹：上游会拒绝所有模型请求（426 lane_version_unsupported）")
		log.Printf("  请设置 DEEPAI_MACHINE_ID=<原始机器UUID>；Windows 可执行：")
		log.Printf("  powershell -Command \"(Get-CimInstance Win32_ComputerSystemProduct).UUID\"")
	}

	if *importOnly {
		runImport(st, *deepaiDir)
		return
	}

	// 命令行 / 环境变量优先级高于状态文件；设置后立即落盘。
	if *adminPw != "" {
		st.UpdateSettings(func(s *store.Settings) { s.AdminPassword = *adminPw })
		if err := st.Save(); err != nil {
			log.Printf("保存管理密码失败: %v", err)
		} else {
			log.Printf("已启用管理密码（来自命令行 / DEEPAI2API_ADMIN_PASSWORD）")
		}
	}

	if !*noImport {
		autoImport(st, *deepaiDir)
	}
	if envAcct := deepai.EnvAccount(); envAcct != nil {
		if isNew := st.UpsertAccount(envAcct); isNew {
			log.Printf("已从 DEEPAI_API_KEY 添加账号 %s", envAcct.Name)
			_ = st.Save()
		}
	}

	// 非回环监听意味着网关已接入局域网 / 公网，此时两项底线不能靠“默认值”撑着：
	// 管理面必须有密码，数据面必须已有密钥。否则从启动到人工创建密钥之间，
	// 上游账号池就是一个无需鉴权的开放中继。
	if !isLoopbackListen(*host) {
		if st.Settings().AdminPassword == "" && !*allowOpen {
			log.Fatalf("监听地址 %q 不是回环地址，但未设置管理密码：控制台会对局域网开放。\n"+
				"  请用 -admin-password 或 DEEPAI2API_ADMIN_PASSWORD 设置密码；\n"+
				"  确实需要无密码运行请显式加 -allow-insecure（或 DEEPAI2API_ALLOW_INSECURE=1）。", *host)
		}
		if !st.HasKeys() {
			k := st.AddKey("bootstrap", "default")
			if err := st.Save(); err != nil {
				log.Fatalf("生成引导密钥失败: %v", err)
			}
			log.Printf("⚠ 非回环监听且尚未签发密钥，已自动生成引导密钥（仅本次启动显示，请立即保存）：")
			log.Printf("    %s", k.Key)
			log.Printf("  之后可在控制台改名或吊销。")
		}
	}

	srv := server.New(st)
	srv.SetVersion(version)

	addr := net.JoinHostPort(*host, fmt.Sprintf("%d", *port))
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 20 * time.Second,
		// 请求体是 JSON（最大 64MB 附件走 extensions），60s 足够；
		// 没有它时慢速请求可以长期占着连接不释放。
		ReadTimeout: 60 * time.Second,
		// keep-alive 空闲连接回收；流式响应可能持续数分钟，故不设 WriteTimeout。
		IdleTimeout: 120 * time.Second,
	}

	// 后台任务：首个目录拉取 + 定期落盘 + 定期刷新模型目录。
	// 有意放在监听之前启动：首个目录拉取可能与上游耗上 20s 超时，
	// 先起它、再起监听，两者并行，互不阻塞。
	stopBG := make(chan struct{})
	// 车道探测：只打免鉴权的 /models，命中即落盘；失败沿用旧值，不阻塞启动。
	go func() {
		lane, err := srv.RefreshLane(context.Background())
		if err != nil {
			log.Printf("车道探测失败（沿用 %s）: %v", srv.CurrentLane(), err)
			return
		}
		log.Printf("车道版本: %s（免鉴权 /models 探测）", lane)
	}()

	go background(st, srv, stopBG)
	go quotaGuard(srv, stopBG)

	go func() {
		// 一行结构化摘要：加密状态、数据路径、号池规模这些排障第一眼看的东西
		// 之前散在多行中文日志里，机器采集只能靠正则。JSON 行可直接 grep/jq。
		total, ready := st.AccountCounts()
		summary, _ := json.Marshal(map[string]any{
			"event":         "startup",
			"version":       version,
			"listen":        addr,
			"data":          st.Path(),
			"encrypted":     st.Encrypted(),
			"accounts":      total,
			"ready":         ready,
			"keys":          len(st.Keys()),
			"models":        len(st.Models()),
			"default_model": st.DefaultModel(),
			"machine_ready": st.MachineID() != "",
		})
		log.Printf("startup %s", summary)

		log.Printf("DeepAICode2API %s 已启动: http://%s/", version, addr)
		if st.HasKeys() {
			log.Printf("已签发 API 密钥，调用需携带 Authorization: Bearer sk-...")
		} else {
			log.Printf("尚未签发 API 密钥：仅本机回环可无密钥调用，非回环来源返回 503（请在控制台创建）")
		}
		if st.Settings().AdminPassword != "" {
			log.Printf("管理接口已启用密码校验（X-Admin-Password / Bearer）")
		} else {
			log.Printf("未设置管理密码，管理接口仅允许本机访问（可用 -admin-password 设置）")
		}
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("监听失败: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	close(stopBG)
	log.Printf("正在关闭…")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("关闭异常: %v", err)
	}
	// 等后台探测 / 复探把已经写进内存的结果落盘，再保存退出。
	// 顺序不能反：反过来会让最后那次 Save 被后台的 Save 覆盖成旧快照。
	srv.Close(ctx)
	if err := st.Save(); err != nil {
		log.Printf("保存状态失败: %v", err)
	}
	log.Printf("已退出")
}

// background 负责首个模型目录拉取、定期持久化与周期性目录刷新。
//
// 首个拉取放在这里而不是 main() 里同步执行：上游不通时 RefreshModels 要等满
// 20s 超时（实测日志里 "机器指纹" 到 "已启动" 正好差 20 秒），同步做会把
// 「开始监听」整体拖后。目录为空时 /v1/models 与模型解析都回落到内置兜底
// 条目（server.catalog / registry.Resolve），所以先监听、后补目录是安全的。
