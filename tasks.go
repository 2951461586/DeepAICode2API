package main

import (
	"context"
	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/server"
	"deepaicode2api/internal/store"
	"fmt"
	"log"
	"time"
)

// 后台任务与一次性命令（导入 / 封存）。
// 这一组都以「进程级」为前提：不接受请求作用域的 ctx，退出由 stop 通道控制。

func background(st *store.Store, srv *server.Server, stop <-chan struct{}) {
	// 首个目录拉取：与监听并行，失败只影响模型清单，不影响服务可用。
	// 用独立 ctx：即使 stop 关闭也让它自己超时结束，不阻塞退出流程。
	{
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		models, err := srv.RefreshModels(ctx)
		cancel()
		if err != nil {
			log.Printf("拉取模型目录失败（先用缓存/兜底）: %v", err)
		} else {
			log.Printf("模型目录已更新：%d 个模型，默认 %s", len(models), st.DefaultModel())
		}
	}

	saveTick := time.NewTicker(30 * time.Second)
	defer saveTick.Stop()
	modelTick := time.NewTicker(6 * time.Hour)
	defer modelTick.Stop()

	for {
		select {
		case <-stop:
			return
		case <-saveTick.C:
			if err := st.Save(); err != nil {
				log.Printf("定期保存状态失败: %v", err)
			}
		case <-modelTick.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			models, err := srv.RefreshModels(ctx)
			cancel()
			if err != nil {
				log.Printf("定期刷新模型目录失败: %v", err)
			} else {
				log.Printf("模型目录已刷新：%d 个模型", len(models))
			}
		}
	}
}

// quotaGuard 是后台额度守护：按 refresh_at / 陈旧度分批复探，
// 不再需要人工点「刷新全部配额」（653 号全量探测要 11 分钟）。
//
// 单独一个 goroutine 而不是塞进 background：上游挂住时一批要几分钟
// （24 个 / 4 并发 × 20s 超时），会拖住 30s 的周期保存。
func quotaGuard(srv *server.Server, stop <-chan struct{}) {
	tick := time.NewTicker(server.QuotaGuardInterval)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			if n := srv.ReprobeStale(server.QuotaGuardBatch); n > 0 {
				log.Printf("[quota] 后台已复探 %d 个账号的额度", n)
			}
		}
	}
}

// autoImport 在启动时尝试导入本机桌面端凭据。
func autoImport(st *store.Store, dir string) {
	results, err := deepai.ImportAllFromDesktop(dir)
	if err != nil {
		log.Printf("未自动导入桌面端凭据（可在控制台手动导入）: %v", err)
		return
	}
	added, updated := 0, 0
	for _, res := range results {
		if st.UpsertAccount(res.Account) {
			added++
		} else {
			updated++
		}
	}
	if err := st.Save(); err != nil {
		log.Printf("保存账号失败: %v", err)
		return
	}
	for _, res := range results {
		log.Printf("已导入账号 %s（%s，username=%s）", res.Account.Name, res.Path, res.Account.Username)
	}
	log.Printf("桌面端凭据导入完成：新增 %d，更新 %d", added, updated)
}

func runImport(st *store.Store, dir string) {
	results, err := deepai.ImportAllFromDesktop(dir)
	if err != nil {
		log.Fatalf("导入失败: %v", err)
	}
	added, updated := 0, 0
	for _, res := range results {
		isNew := st.UpsertAccount(res.Account)
		state := "更新"
		if isNew {
			state = "新增"
			added++
		} else {
			updated++
		}
		fmt.Printf("%s账号 %s\n凭据文件: %s\nusername: %s\nKey: %s\n\n",
			state, res.Account.Name, res.Path, res.Account.Username, deepai.MaskSecret(res.Account.APIKey))
	}
	if err := st.Save(); err != nil {
		log.Fatalf("保存失败: %v", err)
	}
	fmt.Printf("共导入 %d 个账号（新增 %d，更新 %d）→ %s\n", len(results), added, updated, st.Path())
}

// runSeal 执行一次性存量封存：把主文件、统计与日志里的明文全部改写为密文。
//
// 与普通 Save 的区别：Save 只重写「之后变化」的内容，append-only 的日志
// 不会被回填，所以需要单独的 -seal 入口。
func runSeal(st *store.Store, yes bool) {
	if !st.MasterKeySet() {
		log.Fatalf("未设置 %s：先写入 .env 或导出环境变量（openssl rand -hex 32）\n"+
			"  注意：封存后密钥丢失即号池不可恢复。", store.EnvMasterKey)
	}
	if !yes {
		log.Fatalf("将把 %s 及其统计/日志旁路文件改写为密文（不可逆）。\n"+
			"  请先停止正在运行的网关，再执行：-seal -yes\n  密钥指纹: %s",
			st.Path(), st.MasterKeyFingerprint())
	}
	rep, err := st.Reseal()
	if err != nil {
		log.Fatalf("封存失败: %v", err)
	}
	fmt.Printf("已封存 %d 个文件（账号 %d，历史明文日志行 %d，原样保留 %d）\n",
		len(rep.Files), rep.Accounts, rep.SealedLines, rep.KeptLines)
	for _, f := range rep.Files {
		fmt.Printf("  - %s\n", f)
	}
	fmt.Printf("密钥指纹: %s\n", st.MasterKeyFingerprint())
	fmt.Printf("⚠ 务必备份 %s：丢失后号池无法解密。\n", store.EnvMasterKey)
}

// defaultDataPath 返回默认状态文件路径。
//
// 优先使用用户私有配置目录（Windows 上是 %AppData%，Unix 上是 ~/.config）：
// 状态文件含上游 Key 与账号密码，放在可执行文件同目录时往往会落在
// 共享目录（Program Files、共享盘）里，同机任意用户都能读到。
//
// 为不打断既有部署：如果旧位置（可执行文件同目录）已经有状态文件，继续用它，
// 否则号池看起来会“凭空消失”。
