package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"deepaicode2api/internal/logx"
)

// --- 版本与更新检查 ---
//
// 控制台顶部显示当前版本；点一下会问「有没有新版本」。查询在**服务端**做：
//   - 前端 CSP 是 connect-src 'self'，本来就不允许直连 api.github.com；
//   - 只签发过一次请求的任务没必要让每台机器各自打 GitHub（有速率限制）。
//
// 缓存 6 小时：既能让升级提示及时出现，又不会把控制台刷新变成对 GitHub 的压测。
// 拉不到（离线 / 被墙 / 仓库私有）不算错误路径：返回当前版本 + error 字段，
// 前端只显示版本号，不弹任何报错。

// updateFeedURL 是「最新 release」的 API 地址。
//
// 变量而不是常量：测试要指向 httptest，离线环境也可以改成本地镜像。
var updateFeedURL = "https://api.github.com/repos/2951461586/DeepAICode2API/releases/latest"

const (
	// updateCacheTTL 是查询结果的有效期。
	updateCacheTTL = 6 * time.Hour
	// updateTimeout 是单次上游查询预算。
	updateTimeout = 8 * time.Second
)

// versionInfo 是 /admin/api/version 的响应。
type versionInfo struct {
	Current         string `json:"current"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url,omitempty"`
	PublishedAt     string `json:"published_at,omitempty"`
	// Cached 表示本次结果来自缓存（调试用）。
	Cached bool `json:"cached,omitempty"`
	// Error 非空表示查询失败；前端忽略即可，版本号照样显示。
	Error string `json:"error,omitempty"`
}

// SetVersion 记录构建版本（由 main 用 -ldflags 注入的 version 调用）。
func (s *Server) SetVersion(v string) {
	s.Version = strings.TrimSpace(v)
}

// currentVersion 返回对外展示的版本号（空值归一成 dev）。
func (s *Server) currentVersion() string {
	if v := strings.TrimSpace(s.Version); v != "" {
		return v
	}
	return "dev"
}

// handleAdminVersion 返回当前版本与最新 release 的比对结果。
func (s *Server) handleAdminVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "仅支持 GET", "invalid_request_error")
		return
	}
	writeJSON(w, http.StatusOK, s.versionStatus(r.Context()))
}

// versionStatus 带缓存地查询最新 release。
//
// 缓存冷时并发请求会合并：只让一个 goroutine 真去查，其余等它的结果。否则同时
// 打开多个控制台 / 多标签页刷新，会把「一个 8s 预算的 GitHub 请求」打成 N 个。
func (s *Server) versionStatus(ctx context.Context) versionInfo {
	current := s.currentVersion()

	for {
		s.versionMu.Lock()
		if s.versionCache != nil && time.Since(s.versionAt) < updateCacheTTL {
			cached := *s.versionCache
			cached.Cached = true
			cached.Current = current
			s.versionMu.Unlock()
			return cached
		}
		if wait := s.versionInFlight; wait != nil {
			s.versionMu.Unlock()
			select {
			case <-wait:
				continue // 结果已写进缓存，回到循环顶部直接读
			case <-ctx.Done():
				return versionInfo{Current: current, Error: ctx.Err().Error()}
			}
		}
		done := make(chan struct{})
		s.versionInFlight = done
		s.versionMu.Unlock()

		// 查询不能挂在「第一个打开控制台的人」的请求上下文上：他一关标签页，
		// 在途的查询就被 abort，而失败结果会被缓存 6 小时——之后所有人都会看到
		// 「无法检查更新」。剔除取消信号，预算由 fetchLatestRelease 的 updateTimeout 兜底。
		info := s.queryLatestVersion(context.WithoutCancel(ctx), current)
		// 失败也写缓存：否则离线环境下每次刷新控制台都要等 8s 超时。
		s.storeVersionCache(info)

		s.versionMu.Lock()
		s.versionInFlight = nil
		s.versionMu.Unlock()
		close(done)
		return info
	}
}

// queryLatestVersion 真正去打 GitHub（调用方已负责缓存与并发合并）。
func (s *Server) queryLatestVersion(ctx context.Context, current string) versionInfo {
	info := versionInfo{Current: current}
	// 开发构建（无 tag）不做比对：比出来永远是「有新版本」，只会制造噪音。
	if current == "dev" {
		info.Error = "开发构建未注入版本号，不检查更新"
		return info
	}
	latest, err := fetchLatestRelease(ctx)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Latest = latest.Tag
	info.ReleaseURL = latest.URL
	info.PublishedAt = latest.PublishedAt
	info.UpdateAvailable = versionNewer(latest.Tag, current)
	logVersionCheck(info)
	return info
}

func (s *Server) storeVersionCache(info versionInfo) {
	s.versionMu.Lock()
	s.versionCache = &info
	s.versionAt = time.Now()
	s.versionMu.Unlock()
}

// release 是 GitHub release 里用到的字段。
type release struct {
	Tag         string `json:"tag_name"`
	URL         string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

// fetchLatestRelease 拉取最新 release（只读公开端点，不需要 token）。
func fetchLatestRelease(ctx context.Context) (*release, error) {
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, updateFeedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "deepaicode2api-update-check")
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &updateError{status: resp.StatusCode}
	}
	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	if strings.TrimSpace(rel.Tag) == "" {
		return nil, &updateError{status: resp.StatusCode, note: "release 缺少 tag_name"}
	}
	return &rel, nil
}

// updateHTTPClient 是查询更新用的客户端。
//
// 不复用号池客户端：后者带代理与浏览器指纹头，而这里要的是「控制台所在机器的
// 直连」；即便配了代理池，也不该用它去打 GitHub。
var updateHTTPClient = &http.Client{Timeout: updateTimeout}

// updateError 描述一次查询失败（带状态码，便于日志与前端展示）。
type updateError struct {
	status int
	note   string
}

func (e *updateError) Error() string {
	if e.note != "" {
		return e.note
	}
	return "查询最新版本失败：HTTP " + strconv.Itoa(e.status)
}

// versionNewer 报告 latest 是否比 current 新（宽松 semver：只比主.次.修订）。
//
// current 可能是 "dev" / "docker" / "v0.1.0-3-gabc" 这类非规范值：
// 解析不出数字时一律视为「不比」，避免误报升级。
func versionNewer(latest, current string) bool {
	l, ok1 := parseSemver(latest)
	c, ok2 := parseSemver(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// parseSemver 从 v1.2.3 / 1.2.3-rc1 / v1.2 里取出三个数字。
func parseSemver(v string) ([3]int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	// 去掉预发布 / 构建后缀：1.2.3-rc1+meta → 1.2.3
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return [3]int{}, false
	}
	var out [3]int
	parts := strings.Split(v, ".")
	if len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// logVersionCheck 记录一次更新查询结果（避免每次都在 handler 里写日志）。
func logVersionCheck(info versionInfo) {
	switch {
	case info.UpdateAvailable:
		logx.Info("update available", "current", info.Current, "latest", info.Latest)
	case info.Error != "":
		logx.Warn("update check failed", "current", info.Current, "error", info.Error)
	}
}
