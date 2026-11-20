// Package poolsvc 是号池的**有状态**部分：在途槽位、配额探测与去重、会话粘性、
// 入池与代理分配、启动预热。
//
// 与 internal/pool 的分工：`pool` 是纯选号算法（无状态、可 bench），
// `poolsvc` 承接它的另一半——谁正在被用、谁的额度刚过期、某个会话钉在哪个号上。
//
// 边界：本包不认识 HTTP、路由或协议转换。它只依赖 `store`（账号与设置）、
// `deepai`（客户端与账号类型）与三个注入能力：
//   - ClientFor：按出口取客户端（含代理缓存，由 server 提供）；
//   - Save：落盘（错误处理与日志策略由 server 决定）；
//   - 全局停止信号（本包自己的 stopCh，由 Close 关闭）。
package poolsvc

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/logx"
	"deepaicode2api/internal/proxy"
	"deepaicode2api/internal/store"
)

// Config 是构造 Service 需要的注入能力。
type Config struct {
	// Store 是账号与设置的唯一状态源（本包不缓存账号，只按 ID 操作）。
	Store *store.Store
	// ClientFor 按账号的出口返回上游客户端（server 里有按代理的缓存）。
	ClientFor func(*deepai.Account) *deepai.Client
	// Save 落盘；失败处理由调用方负责（server 会记 warn 日志）。
	Save func() error
}

// Service 是号池的运行时状态。
type Service struct {
	store     *store.Store
	clientFor func(*deepai.Account) *deepai.Client
	save      func() error

	// inflightMu / inflight 是单账号在途请求计数（见 slots.go）。
	inflightMu sync.Mutex
	inflight   map[string]int

	// probeMu / probing 是在途探测的去重表（按账号 ID）。
	//
	// 三条路径会同时探同一批账号：手动 checkin、入池后延迟复探、推理时的
	// 按需复探。没有它时同一个号会被连打多次，既浪费额度也容易触发上游风控。
	probeMu sync.Mutex
	probing map[string]struct{}

	// probeSem 是全局在途探测上限（手动批次 / 后台守护 / 按需复探共用）。
	//
	// 653 个号的额度快照会同时变旧；若按需复探不加限制，一波流量就能
	// 叠出几十上百个并行的 /theos 请求——那是最容易被上游风控的形态。
	probeSem chan struct{}

	// quotaWG 跟踪已排期的后台落盘工作（配额复探 / 入池探活 / 按需复探）。
	//
	// 这些 goroutine 在写完内存后还要 Save() 落盘；测试必须等它完全收尾
	// 再结束，否则 t.TempDir() 的清理会和落盘撞上（CI 上表现为
	// "TempDir RemoveAll cleanup: directory not empty"）。
	// 进程退出时由 Close 统一等待，避免「最后一次 Save 之后又有后台写」。
	quotaWG sync.WaitGroup

	// stopCh 在 Close 时关闭，用于打断后台任务里的长睡眠（配额复探退避最长 12 分钟）。
	stopCh   chan struct{}
	stopOnce sync.Once

	// quotaReprobeDelays 是入池后 0 额度的复探退避序列（测试可注入短值）。
	quotaReprobeDelays []time.Duration

	// stickyMu / sticky 是会话粘性绑定（affinity → 账号），见 sticky.go。
	// stickyDirty 标记绑定表有变化，等待 FlushSticky 攒批写盘。
	stickyMu    sync.Mutex
	sticky      map[string]store.StickyBinding
	stickyDirty atomic.Bool

	// enrollPoolOnce / enrollPool 是入池自动分配代理用的进程内单例。
	//
	// 浏览器登录一次只入一个号；若每次新建池，轮询游标都会从 0 开始，
	// 所有号就都落在第一个出口上（等于没分配）。单例让连续入池能真正轮转。
	enrollPoolOnce sync.Once
	enrollPool     *proxy.ProxyPool
}

// New 构造号池运行时状态。
func New(cfg Config) *Service {
	return &Service{
		store:              cfg.Store,
		clientFor:          cfg.ClientFor,
		save:               cfg.Save,
		inflight:           map[string]int{},
		probing:            map[string]struct{}{},
		probeSem:           make(chan struct{}, probeWorkers),
		stopCh:             make(chan struct{}),
		quotaReprobeDelays: quotaReprobeDelays,
		sticky:             map[string]store.StickyBinding{},
	}
}

// Save 落盘并记录失败（注入的 Save 为空时跳过）。
//
// 日志与旧的 server.persist 保持同一口径：失败不改变调用方语义，
// 但必须留痕（「改动只存在于内存」正是最难排查的那类故障）。
func (p *Service) Save() {
	if p == nil || p.save == nil {
		return
	}
	if err := p.save(); err != nil {
		logx.Warn("状态文件保存失败（改动仅存在于内存）", "err", err, "path", p.store.Path())
	}
}

// Wait 阻塞到已排期的后台落盘工作全部收尾。
//
// 两类 goroutine 都先把结果写进内存、再 Save()：测试若只等「额度已更新」
// 就返回，后台落盘会与 t.TempDir() 清理竞争（-race 下更容易命中）。
func (p *Service) Wait() {
	if p == nil {
		return
	}
	p.quotaWG.Wait()
}

// Close 停止后台任务并等待已排期的落盘工作收尾（以 ctx 为上限）。
//
// 语义与旧 Server.Close 一致：关闭 stopCh 打断最长 12 分钟的退避睡眠，
// 再等 quotaWG；超时只记日志，不阻塞退出。
func (p *Service) Close(ctx context.Context) {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() {
		if p.stopCh != nil {
			close(p.stopCh)
		}
	})

	done := make(chan struct{})
	go func() {
		p.quotaWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		// 退出流程不能无限等：探测本身有 20s 超时，这里通常不会命中。
		log.Printf("[shutdown] 后台任务未在期限内收尾，继续退出（未落盘的探测结果会丢失）")
	}
}

// sleepOrStop 等待 d，收到停止信号时返回 false。
func (p *Service) sleepOrStop(d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-p.stopCh:
		return false
	case <-timer.C:
		return true
	}
}
