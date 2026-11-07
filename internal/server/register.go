package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/register"
)

// registerJob 是一次批量注册任务的运行状态。
type registerJob struct {
	ID          string
	StartedAt   int64
	FinishedAt  int64
	Count       int
	Concurrency int
	Group       string
	Backend     string
	Domain      string

	mu      sync.Mutex
	running bool
	lastErr string
	records map[string]*register.Record
	order   []string
	cancel  context.CancelFunc
}

type registerJobs struct {
	mu sync.Mutex
	m  map[string]*registerJob
}

func newRegisterJobs() *registerJobs { return &registerJobs{m: map[string]*registerJob{}} }

func (js *registerJobs) put(j *registerJob) {
	js.mu.Lock()
	defer js.mu.Unlock()
	for id, old := range js.m {
		if time.Since(time.Unix(old.StartedAt, 0)) > 24*time.Hour && !old.isRunning() {
			delete(js.m, id)
		}
	}
	js.m[j.ID] = j
}

func (js *registerJobs) get(id string) *registerJob {
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.m[id]
}

func (js *registerJobs) list() []*registerJob {
	js.mu.Lock()
	defer js.mu.Unlock()
	out := make([]*registerJob, 0, len(js.m))
	for _, j := range js.m {
		out = append(out, j)
	}
	return out
}

func (j *registerJob) isRunning() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.running
}

func (j *registerJob) update(rec *register.Record) {
	if rec == nil {
		return
	}
	rec.Normalize()
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.records == nil {
		j.records = map[string]*register.Record{}
	}
	if _, ok := j.records[rec.Email]; !ok {
		j.order = append(j.order, rec.Email)
	}
	j.records[rec.Email] = rec
}

func (j *registerJob) finish(records []*register.Record) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.running = false
	j.FinishedAt = time.Now().Unix()
	for _, rec := range records {
		if rec == nil {
			continue
		}
		rec.Normalize()
		if j.records == nil {
			j.records = map[string]*register.Record{}
		}
		if _, ok := j.records[rec.Email]; !ok {
			j.order = append(j.order, rec.Email)
		}
		j.records[rec.Email] = rec
	}
}

func (j *registerJob) snapshot() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	records := make([]any, 0, len(j.order))
	for _, email := range j.order {
		if rec := j.records[email]; rec != nil {
			records = append(records, sanitizeRegisterRecord(rec))
		}
	}
	return map[string]any{
		"id":          j.ID,
		"running":     j.running,
		"count":       j.Count,
		"concurrency": j.Concurrency,
		"done":        len(records),
		"group":       j.Group,
		"backend":     j.Backend,
		"domain":      j.Domain,
		"started_at":  j.StartedAt,
		"finished_at": j.FinishedAt,
		"error":       j.lastErr,
		"records":     records,
	}
}

// sanitizeRegisterRecord 下发给控制台的记录（绝不包含密码 / Session）。
func sanitizeRegisterRecord(rec *register.Record) map[string]any {
	return map[string]any{
		"email":            rec.Email,
		"status":           rec.Status,
		"stage":            rec.Stage,
		"error":            rec.Error,
		"last_error":       rec.LastError,
		"key":              deepai.MaskSecret(rec.APIKey),
		"account_id":       rec.AccountID,
		"user_pk":          rec.UserPK,
		"username":         rec.Username,
		"plan":             rec.Plan,
		"email_verified":   rec.EmailVerified,
		"theos_balance":    rec.TheosBalance,
		"theos_granted":    rec.TheosGranted,
		"group":            rec.Group,
		"mailbox_provider": rec.MailboxProvider,
		"proxy":            rec.ProxyLabel,
		"fingerprint":      rec.Fingerprint,
		"stages":           rec.Stages,
		"timings":          rec.Timings,
		"created_at":       rec.CreatedAt,
		"finished_at":      rec.FinishedAt,
	}
}

// handleAdminRegister 启动 / 查询批量注册任务。
func (s *Server) handleAdminRegister(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id != "" {
			job := s.Registers.get(id)
			if job == nil {
				writeErr(w, http.StatusNotFound, "任务不存在或已过期", "invalid_request_error")
				return
			}
			writeJSON(w, http.StatusOK, job.snapshot())
			return
		}
		jobs := s.Registers.list()
		out := make([]map[string]any, 0, len(jobs))
		for _, job := range jobs {
			out = append(out, job.snapshot())
		}
		writeJSON(w, http.StatusOK, map[string]any{"jobs": out})
	case http.MethodPost:
		s.startRegisterJob(w, r)
	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if job := s.Registers.get(id); job != nil && job.cancel != nil {
			job.cancel()
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "不支持的方法", "invalid_request_error")
	}
}

func (s *Server) startRegisterJob(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Count         int    `json:"count"`
		Concurrency   int    `json:"concurrency"`
		Email         string `json:"email"`
		Password      string `json:"password"`
		Domain        string `json:"domain"`
		MailBackend   string `json:"mail_backend"`
		Group         string `json:"group"`
		Verify        *bool  `json:"verify"`
		Proxy         string `json:"proxy"`
		ProxyPool     string `json:"proxy_pool"`
		NoFingerprint bool   `json:"no_fingerprint"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体解析失败", "invalid_request_error")
		return
	}
	if body.Count <= 0 {
		body.Count = 1
	}
	if body.Count > 50 {
		writeErr(w, http.StatusBadRequest, "单次最多注册 50 个账号", "invalid_request_error")
		return
	}
	if body.Concurrency <= 0 {
		body.Concurrency = 1
	}
	if body.Concurrency > 5 {
		body.Concurrency = 5
	}
	if body.Concurrency > body.Count {
		body.Concurrency = body.Count
	}
	body.Email = strings.TrimSpace(body.Email)
	if body.Email != "" && body.Count > 1 {
		writeErr(w, http.StatusBadRequest, "指定邮箱时 count 必须为 1", "invalid_request_error")
		return
	}
	group := strings.TrimSpace(body.Group)
	if group == "" {
		group = "default"
	}

	cfg := register.LoadConfig()
	if body.Domain != "" {
		cfg.DefaultDomain = strings.TrimSpace(body.Domain)
	}
	if body.MailBackend != "" {
		cfg.MailBackend = strings.ToLower(strings.TrimSpace(body.MailBackend))
	}
	if body.Proxy != "" {
		cfg.Proxy = strings.TrimSpace(body.Proxy)
	}
	poolRaw := strings.TrimSpace(body.ProxyPool)
	if poolRaw == "" {
		poolRaw = cfg.ProxyPool
	}
	if poolRaw == "" && cfg.ProxyFile != "" {
		if raw, err := register.ReadProxyFile(cfg.ProxyFile); err == nil {
			poolRaw = raw
		}
	}
	pool, err := register.NewProxyPool(poolRaw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "代理池配置错误: "+err.Error(), "invalid_request_error")
		return
	}
	backend := cfg.AutoBackend()
	if missing := cfg.Validate(backend); len(missing) > 0 {
		writeErr(w, http.StatusBadRequest,
			fmt.Sprintf("邮箱后端 %s 缺少配置：%s（请在 .env / 环境变量中填写）", backend, strings.Join(missing, ", ")),
			"invalid_request_error")
		return
	}
	verify := true
	if body.Verify != nil {
		verify = *body.Verify
	}

	provider, err := register.NewProvider(cfg, backend, cfg.DefaultDomain, body.Email)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "初始化邮箱后端失败: "+err.Error(), "invalid_request_error")
		return
	}

	jobID := deepai.RandomHex(8)
	ctx, cancel := context.WithCancel(context.Background())
	job := &registerJob{
		ID:          jobID,
		StartedAt:   time.Now().Unix(),
		Count:       body.Count,
		Concurrency: body.Concurrency,
		Group:       group,
		Backend:     backend,
		Domain:      cfg.DefaultDomain,
		running:     true,
		records:     map[string]*register.Record{},
		cancel:      cancel,
	}
	s.Registers.put(job)

	go s.runRegisterJob(ctx, job, cfg, provider, body.Email, body.Password, verify, pool, body.NoFingerprint)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": jobID})
}

func (s *Server) runRegisterJob(ctx context.Context, job *registerJob, cfg *register.Config, provider register.Provider, email, password string, verify bool, pool *register.ProxyPool, noFingerprint bool) {
	if capped, ok := pool.CapConcurrency(job.Concurrency); ok {
		log.Printf("[register] 并发 %d 超过出口数 %d，压到 %d", job.Concurrency, capped, capped)
		job.Concurrency = capped
	}

	baseDir := filepath.Dir(s.Store.Path())
	ledger := register.NewLedger(filepath.Join(baseDir, "exports", "register-ledger.jsonl"))
	// 缓冲写：任务结束必须刷盘并关闭句柄。
	defer func() { _ = ledger.Close() }()

	pipe := register.New(register.Options{
		Config:   cfg,
		Provider: provider,
		Ledger:   ledger,
		Enroller: localEnroller{s: s},
		Group:    job.Group,
		Verify:   verify,
		// 0.3.19 起上游新增 social_login_required 策略：/theos 可能 200 而 chat 403。
		// 入池前多跑一次最小对话探针，把这类账号挡在池外。
		ProbeChat:     true,
		ProxyPool:     pool,
		NoFingerprint: noFingerprint,
		VerifyTimeout: cfg.VerifyWait,
		PollInterval:  cfg.PollEvery,
		MinInterval:   cfg.MinInterval,
		Log: func(format string, args ...any) {
			log.Printf("[register] "+format, args...)
		},
		OnUpdate: job.update,
	})

	log.Printf("[register] 任务 %s 启动：count=%d concurrency=%d group=%s backend=%s", job.ID, job.Count, job.Concurrency, job.Group, job.Backend)
	records := pipe.RunBatch(ctx, job.Count, job.Concurrency, email, password)
	job.finish(records)
	log.Printf("[register] 任务 %s 完成：%d 个记录", job.ID, len(records))

	// 验证邮件晚到时记录会停在 verify:pending（R2 实测 2/50）。
	// 批次结束补一轮，避免账号长期未验证。
	s.reverifyPending(ctx, cfg, provider, records)

	if _, err := register.WriteDeliverables(filepath.Join(baseDir, "result"), records); err != nil {
		log.Printf("[register] 写交付物失败: %v", err)
	}
}

// reverifyPending 对批次里仍未验证的账号补一轮收信 + 点验证链接。
//
// 只在有未验证账号时才启动，并发与等待时间由 register.Reverify 的默认值控制
// （上游 /verify_email 单次约 13s，并发过高会被 Passenger 打回 502）。
func (s *Server) reverifyPending(ctx context.Context, cfg *register.Config, provider register.Provider, records []*register.Record) {
	targets := make([]register.VerifyTarget, 0, len(records))
	for _, rec := range records {
		if rec == nil || rec.EmailVerified || strings.TrimSpace(rec.Email) == "" {
			continue
		}
		targets = append(targets, register.VerifyTarget{
			Email: rec.Email, Group: rec.Group,
			Proxy: rec.Proxy, Fingerprint: rec.Fingerprint,
		})
	}
	if len(targets) == 0 {
		return
	}

	log.Printf("[register] 批次结束仍有 %d 个账号未验证，补一轮", len(targets))
	res := register.Reverify(ctx, register.ReverifyOptions{
		Config:   cfg,
		Provider: provider,
		Marker:   localEnroller{s: s},
		Log:      func(format string, args ...any) { log.Printf("[reverify] "+format, args...) },
	}, targets)

	ok := 0
	for _, r := range res {
		if r.OK {
			ok++
			continue
		}
		log.Printf("[reverify] %s 失败: %s", r.Email, r.Err)
	}
	log.Printf("[register] 补验证完成：成功 %d / %d", ok, len(targets))
}

// localEnroller 是进程内入池器：既写入号池，也能回填验证状态。
//
// 实现 register.VerifiedMarker，因此后台验证完成后会自动回填 email_verified。
type localEnroller struct{ s *Server }

func (e localEnroller) Enroll(_ context.Context, rec *register.Record) (string, error) {
	return e.s.enrollRecord(rec)
}

func (e localEnroller) MarkVerified(_ context.Context, accountID, email string) error {
	return e.s.markAccountVerified(accountID, email)
}

// markAccountVerified 把账号标记为邮箱已验证（按 ID 或邮箱定位）。
func (s *Server) markAccountVerified(id, email string) error {
	// 幂等：重复置 true 无害。
	mark := func(a *deepai.Account) { a.EmailVerified = true }

	// 先按 ID 定位；补跑时台账里的 ID 可能已失效，再按邮箱回退。
	if id = strings.TrimSpace(id); id != "" && s.Store.UpdateAccount(id, mark) {
		return s.Store.Save()
	}
	if want := strings.TrimSpace(email); want != "" {
		for _, a := range s.Store.Accounts() {
			if strings.EqualFold(a.Email, want) && s.Store.UpdateAccount(a.ID, mark) {
				return s.Store.Save()
			}
		}
	}
	return errors.New("账号不存在")
}

// enrollRecord 把注册链路的结果写入号池（服务端内进程调用）。
func (s *Server) enrollRecord(rec *register.Record) (string, error) {
	if rec == nil {
		return "", errors.New("空记录")
	}
	reg := deepai.Registration{
		Email:         rec.Email,
		Password:      rec.Password,
		Username:      rec.Username,
		APIKey:        rec.APIKey,
		SessionID:     rec.SessionID,
		CSRFToken:     rec.CSRFToken,
		UserPK:        rec.UserPK,
		Plan:          rec.Plan,
		EmailVerified: rec.EmailVerified,
		Group:         rec.Group,
		Proxy:         rec.Proxy,
		Fingerprint:   rec.Fingerprint,
		Source:        "register",
	}
	id, _, err := s.enrollRegistration(reg, allowanceFromEnroll(rec.TheosBalance, rec.TheosGranted, rec.TheosRefreshAt))
	return id, err
}

// enrollRegistration 是入池的唯一写入口（管理接口与注册任务共用）。
func (s *Server) enrollRegistration(reg deepai.Registration, al *deepai.Allowance) (string, bool, error) {
	acct, err := deepai.AccountFromRegistration(reg)
	if err != nil {
		return "", false, err
	}
	now := time.Now().Unix()
	acct.Status = "ok"
	acct.LastUsed = now
	acct.LastVerified = now
	acct.Failures = 0
	acct.LastErr = ""
	if al != nil {
		acct.Theos = al
	}

	isNew := s.Store.UpsertAccount(acct)
	// Upsert 会保留旧运行态；把本次验证结果显式写回。
	s.Store.UpdateAccount(acct.ID, func(stored *deepai.Account) {
		stored.Status = "ok"
		stored.Failures = 0
		stored.LastErr = ""
		stored.LastUsed = now
		stored.LastVerified = now
		stored.EmailVerified = reg.EmailVerified
		stored.Plan = reg.Plan
		if al != nil {
			stored.Theos = al.Clone()
		}
	})
	if err := s.Store.Save(); err != nil {
		return "", isNew, err
	}

	// 刚注册完的账号，上游往往还没把当日额度发下来：此时 /theos 会返回
	// balance=0，但几分钟后同一个 Key 再查就是 200。若不复探，这个 0 会被
	// 当成事实缓存下来，调度侧就会长期跳过该账号。
	if al == nil || !positiveQuota(al) {
		s.scheduleQuotaReprobe(acct.ID)
	}
	return acct.ID, isNew, nil
}

// positiveQuota 报告配额是否明确大于 0。
//
// 解析不了 / 字段缺失都按“未知”（返回 false）处理：宁可多探一次，
// 也不要把可用账号判死。
func positiveQuota(al *deepai.Allowance) bool {
	if al == nil || !al.Enabled {
		return false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(al.Balance), 64)
	return err == nil && v > 0
}

// allowanceFromEnroll 构造入池时携带的配额快照。
//
// 上游在账号刚创建时常常返回 balance=0（当日额度随后才下发）。这个 0 不是
// 「确认耗尽」而是「未知」：按确认值缓存会让调度第一轮把它排除，最长 15 分钟
// （R2 实测：新批入池后立刻压测，50 个号只有 12~15 个可被选中，
// 数据面仅覆盖 19/50）。所以 0 额度时把 CheckedAt 留 0，让 pool.HasQuota
// 按「未知」放行；真没额度时上游会拒，失败的账号会被自动降权。
func allowanceFromEnroll(balance, granted, refreshAt string) *deepai.Allowance {
	al := &deepai.Allowance{Enabled: true, Balance: balance, Granted: granted, RefreshAt: refreshAt}
	if !positiveQuota(al) {
		return al
	}
	al.CheckedAt = time.Now().Unix()
	return al
}

// quotaReprobeDelays 是入池后 0 额度的复探退避序列。
//
// 实测（R2 压测）：入池瞬间 50 个新号里只有 12~15 个拿到额度；
// 原 3/6/12 分钟序列要到第 9 分钟才集齐 47/50。改成 45s/2min/6min 后，
// 「可调度」的空窗从 9 分钟压到 1 分钟以内；拿到正额度即停。
var quotaReprobeDelays = []time.Duration{45 * time.Second, 2 * time.Minute, 6 * time.Minute}

// scheduleQuotaReprobe 异步复探某个账号的配额（仅当它仍是 0 时）。
//
// 每个账号一个 goroutine，次数有上限，不会自我放大。
func (s *Server) scheduleQuotaReprobe(id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	delays := s.quotaReprobeDelays
	if len(delays) == 0 {
		return
	}
	go func() {
		for _, d := range delays {
			time.Sleep(d)
			a := s.Store.Account(id)
			if a == nil {
				return // 已被删除
			}
			if positiveQuota(a.Theos) {
				return // 期间被别的路径探到了
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			al, err := s.clientFor(a).Theos(ctx, a)
			cancel()
			s.Store.SetAccountAllowance(id, al, err)
			if err := s.Store.Save(); err != nil {
				log.Printf("[quota] 保存复探结果失败: %v", err)
			}
			if err == nil && positiveQuota(al) {
				log.Printf("[quota] %s 复探拿到额度: balance=%s granted=%s", a.Name, al.Balance, al.Granted)
				return
			}
		}
	}()
}
