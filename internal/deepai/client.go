package deepai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"deepaicode2api/internal/configenv"
	"deepaicode2api/internal/machine"
	"deepaicode2api/internal/proxy"
)

// maxJSONBytes 限制上游 JSON 响应体大小（模型目录 / 配额 / 搜索）。
const maxJSONBytes = 8 << 20

// RetryAfter 解析 Retry-After 头（秒），默认 fallback。
//
// 与桌面端 waitForDeepAICapacity 同形：可解析的秒数夹到 [1, 30]，
// 头部缺失 / 非数字（含 HTTP-date）时才用调用方的 fallback。
func RetryAfter(resp *http.Response, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if raw == "" {
		return fallback
	}
	var secs int
	if _, err := fmt.Sscanf(raw, "%d", &secs); err != nil {
		return fallback
	}
	if secs < 1 {
		secs = 1
	}
	if secs > 30 {
		secs = 30
	}
	return time.Duration(secs) * time.Second
}

// transientMarkers 是已知的“出口/网络抖动”错误特征。
//
// 有些代理会在隧道建立阶段返回自定义文案（例如 "No exit node"），
// 它既不是 net.Error 也不包装 io.EOF，只能按文案识别。
var transientMarkers = []string{
	"no exit node",
	"unexpected eof",
	"tls handshake timeout",
	"connection reset",
	"connection refused",
	"broken pipe",
	"i/o timeout",
	"timeout awaiting response",
	"temporary failure in name resolution",
}

// IsTransient 报告错误是否属于“再试一次可能成功”的瞬时故障。
//
// 用于 /theos 验活、入池探测这类关键路径的退避重试判断：
// 网络抖动与 5xx/429 值得重试；4xx 是请求或账号本身的问题，重试无意义。
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	// 调用方主动取消不算可重试。
	if errors.Is(err, context.Canceled) {
		return false
	}
	var he *HTTPError
	if errors.As(err, &he) {
		switch he.Status {
		case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
			return true
		}
		return he.Status >= 500
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range transientMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// Client 是 DeepAI 上游客户端。
//
// 一个 Client 可以服务多个账号（按代理分桶复用），签到状态按 API Key 隔离。
type Client struct {
	Origin    string
	HTTP      *http.Client
	Machine   string
	UserAgent string
	Proxy     string

	// ProxyInvalid 非 nil 表示 Proxy 串解析失败（缺 scheme / 不是 http|https|socks5）。
	//
	// 此时 transport 不会挂专属出口，请求会退回环境变量里的全局代理；
	// 调用方可以把这件事上报到控制台，而不是静默地「以为在用专属出口」。
	ProxyInvalid error

	// Runtime 非 nil 时，车道与签到预算改为**拉取式**：每次请求现取一次。
	//
	// 为什么要有它：这两项都能在控制台运行期调整，此前靠 Server 主动
	// 「推」给每个已缓存的客户端（直连的 s.Up + 每个代理出口的客户端），
	// 而新建客户端时还得再补一遍。三个地方漏一个，就表现为「走代理的号
	// 与直连号行为不一致」——只在生产暴露。改成拉取后只有一处真源。
	//
	// nil 时回落到实例字段（SetLane / SetCheckInBudget），保留给测试与
	// 独立使用客户端的场景。
	Runtime RuntimeSource

	// lane 是当前使用的 Theos 车道版本（atomic：探测/换道时会被并发读取）。
	// 零值表示用编译期的 LaneVersion。
	lane atomic.Value

	// Instance 是进程级实例号（16 字节随机 hex），随所有 JSON 请求发出。
	Instance string

	// BrowserFingerprint 决定是否把账号的浏览器指纹头（User-Agent /
	// sec-ch-ua / Accept-Language）带到上游请求。
	//
	// 桌面端走 Node fetch，**从不发这些头**；带上去等于在推理接口上自报
	// “我不是客户端”。注册链路需要它们（和网页注册时保持一致），
	// 因此只有 NewClientWithFingerprint 打开它。
	BrowserFingerprint bool

	// CheckInBudget 是推理前同步签到的等待预算（0 = 默认 10s）。
	//
	// 只看不改语义：超过预算就带着已有 nonce 放行推理，下一轮再补。
	// 控制台的「签到阻塞预算」通过 SetCheckInBudget 写它。
	CheckInBudget time.Duration

	// LaneHook 在 /theos 成功解析出 lane_version 时被调用（可为 nil）。
	//
	// 只用于观测「上游车道 > 网关编译期车道」的漂移：那意味着客户端已经
	// 抬了车道，而网关还在发旧值。上游在 2026-11-19 放宽了门槛（≤7 全接受），
	// 所以 /models 探测再也发现不了这件事，只能靠 /theos 回的 lane_version。
	// 不参与协议决策：换车道仍走 DetectLane / 升级网关。
	LaneHook func(int)

	// checkIns 缓存每个 API Key 的签到挑战与心跳时间。
	checkIns *checkInStore
}

// NewClient 构造上游客户端（默认读取环境代理）。
//
// DEEPAI_API_ORIGIN 可覆盖上游地址（仅接受 http/https）。
func NewClient() *Client { return inferClient(NewClientWithFingerprint("", "")) }

// NewClientWithProxy 构造走指定代理的上游客户端。
func NewClientWithProxy(proxyRaw string) *Client {
	return inferClient(NewClientWithFingerprint(proxyRaw, ""))
}

// inferClient 把客户端切成「推理形态」：不带浏览器指纹。
//
// 注册链路专用 NewClientWithFingerprint（默认带账号的 Chrome 指纹）；
// 其余调用点都是推理 / 探测，必须与桌面端一致。
func inferClient(c *Client) *Client {
	c.BrowserFingerprint = false
	return c
}

// NewClientWithFingerprint 构造带代理与 User-Agent 的上游客户端。
func NewClientWithFingerprint(proxyRaw, userAgent string) *Client {
	origin := strings.TrimRight(configenv.Or("DEEPAI_API_ORIGIN", DefaultOrigin), "/")
	if u, err := url.Parse(origin); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		origin = DefaultOrigin
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	proxyRaw = strings.TrimSpace(proxyRaw)
	// 代理串规范化：线上账号里存的是 `host:port`（没有 scheme），而 url.Parse 会把
	// 主机名当成 scheme（点号在 scheme 字符集里合法），于是 u.Host 为空、
	// transport.Proxy 没被设置 —— **按账号分出口静默失效**，全部退回环境变量里的
	// 全局代理（2026-11-22 线上发现）。这里统一补 http://，解析不了就不设置，
	// 并记在 ProxyInvalid 上供调用方判断（不静默降级）。
	proxyURL, proxyErr := proxy.Parse(proxyRaw)
	if proxyRaw == "" {
		proxyErr = nil
	}
	if proxyErr == nil && proxyURL != nil {
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return &Client{
		Origin:             origin,
		Machine:            machine.ID(),
		UserAgent:          strings.TrimSpace(userAgent),
		Proxy:              proxyRaw,
		ProxyInvalid:       proxyErr,
		Instance:           newInstanceID(),
		BrowserFingerprint: true,
		checkIns:           newCheckInStore(),
		HTTP: &http.Client{
			Transport: transport,
			// 上游会 302 跳到登录页，跟随只会掩盖鉴权失败。
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// ModelURL 返回 /deepai_code 下的完整地址。
func (c *Client) ModelURL(path string) string { return c.Origin + ModelBasePath + path }

// RootURL 返回根路径完整地址。
func (c *Client) RootURL(path string) string { return c.Origin + path }

// SignInURL 返回桌面端同款浏览器登录地址。
func (c *Client) SignInURL(state string, port int) string {
	u := c.ModelURL(PathSignIn)
	q := url.Values{}
	q.Set("state", state)
	q.Set("port", fmt.Sprintf("%d", port))
	return u + "?" + q.Encode()
}

// desktopInferenceHeaders 复刻桌面端 Node fetch（undici）的默认请求头。
//
// 实测 Node 22 的全局 fetch 发出的是：
//
//	user-agent: node / accept: */* / accept-language: * / sec-fetch-mode: cors
//
// 没有浏览器的 UA，也没有 sec-ch-ua。网关此前把注册用的 Chrome 指纹带到
// 推理请求上，与真实客户端不一致；这里按实测值对齐。
var desktopInferenceHeaders = map[string]string{
	"User-Agent":      "node",
	"Accept":          "*/*",
	"Accept-Language": "*",
	"Sec-Fetch-Mode":  "cors",
}

// commonHeaders 写入所有上游 JSON 请求的公共头。
//
// Content-Type 无条件设置（含无 body 的 GET）：桌面端 requestDeepAIJson 的
// header 字面量对 /models、/theos、/get_usage_summary 同样带 application/json。
func (c *Client) commonHeaders(req *http.Request, acct *Account) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderLane, c.laneVersion())
	if c.Instance != "" {
		req.Header.Set(HeaderInstance, c.Instance)
	}
	// 机器指纹优先取账号级：上游按它发放每日免费额度，共用会互相抵消。
	machineFP := c.Machine
	if acct != nil && acct.MachineID != "" {
		if derived := machine.Fingerprint(acct.MachineID); derived != "" {
			machineFP = derived
		}
	}
	if machineFP != "" {
		req.Header.Set(HeaderMachine, machineFP)
	}

	// 两种指纹形态二选一，不混用：
	//   - BrowserFingerprint=true：注册链路，带账号的 Chrome 指纹；
	//   - 否则：与桌面端推理一致，只带 Node fetch 的默认头。
	//
	// 注意：这里不能 return —— 后面还有 Api-Key（鉴权头）要写。
	if !c.BrowserFingerprint {
		for k, v := range desktopInferenceHeaders {
			req.Header.Set(k, v)
		}
	} else {
		req.Header.Set("Accept", "application/json")
		// 指纹头也优先取账号级：注册与推理必须用同一套特征。
		ua := c.UserAgent
		if acct != nil && acct.UserAgent != "" {
			ua = acct.UserAgent
		}
		if ua != "" {
			req.Header.Set("User-Agent", ua)
		}
		if acct != nil {
			if acct.AcceptLanguage != "" {
				req.Header.Set("Accept-Language", acct.AcceptLanguage)
			}
			if acct.SecCHUA != "" {
				req.Header.Set("sec-ch-ua", acct.SecCHUA)
				req.Header.Set("sec-ch-ua-mobile", "?0")
			}
			if acct.Platform != "" {
				req.Header.Set("sec-ch-ua-platform", acct.Platform)
			}
		}
	}
	if acct != nil && acct.APIKey != "" {
		req.Header.Set(HeaderKeyName, acct.APIKey)
	}
}

// laneGuessAhead 是车道探测向上猜测的步数（上游可能先抬车道再发客户端）。
const laneGuessAhead = 6

// laneCandidates 返回车道探测序列：编译期默认值优先，其次向上猜更新的车道，
// 最后用旧车道兜底。
//
// 顺序不能反过来：探测必须从编译期车道开始试。0.3.37 时代 `/models` 对旧车道
// 很宽容（5 / 6 都回 200），从旧值开始试会把网关钉在旧车道上。
//
// 注意探测的局限（2026-11-19 上游放宽门槛后更明显）：≤7 全部回 200，所以探测
// 只能回答“哪一档能用”，不能回答“当前车道是几”，也看不到上游已经抬到 8。
// 后者只能靠 /theos 回的 lane_version（见 Client.LaneHook）。
func laneCandidates() []string {
	base, err := strconv.Atoi(LaneVersion)
	if err != nil {
		return []string{LaneVersion}
	}
	out := make([]string, 0, laneGuessAhead+base)
	for lane := base; lane <= base+laneGuessAhead; lane++ {
		out = append(out, strconv.Itoa(lane))
	}
	for lane := base - 1; lane >= 2; lane-- {
		out = append(out, strconv.Itoa(lane))
	}
	return out
}

// RuntimeConfig 是「运行期可调」的两项上游参数。
type RuntimeConfig struct {
	// Lane 是本次请求要发的 Theos 车道（空 = 用编译期 LaneVersion）。
	Lane string
	// CheckInBudget 是推理前同步签到的等待预算（<=0 = 用默认 10s）。
	CheckInBudget time.Duration
}

// RuntimeSource 返回当前的运行期配置（每次请求调用一次，必须便宜）。
type RuntimeSource func() RuntimeConfig

// SetLane 设置当前车道（空串 = 用编译期默认值）。
//
// 注意：Client.Runtime 非 nil 时以 Runtime 为准，这里的值只在 Runtime
// 未提供的字段上生效。Server 侧已改用拉取式（见 Runtime）。
func (c *Client) SetLane(lane string) {
	c.lane.Store(strings.TrimSpace(lane))
}

// laneVersion 返回本次请求要发的车道版本。
//
// 优先级：Runtime 拉取值 → 实例值（SetLane）→ 编译期 LaneVersion。
func (c *Client) laneVersion() string {
	if c.Runtime != nil {
		if v := strings.TrimSpace(c.Runtime().Lane); v != "" {
			return v
		}
	}
	if v, ok := c.lane.Load().(string); ok && v != "" {
		return v
	}
	return LaneVersion
}

// Lane 返回当前生效的车道版本。
func (c *Client) Lane() string { return c.laneVersion() }

// DetectLane 用 /models 探测可用的车道版本。
//
// 2026-11-14 起 /models 也要求 Api-Key（不带一律 401 auth_required，见
// docs/recon.md §2），所以探测必须带上一个账号凭据；acct 为 nil 时发匿名
// 请求，上游会回 401，此时探测只会得到「鉴权失败」而无法判断车道。
// 命中即写回 Client，后续请求自动带新车道；全部候选失败时返回最后一个错误。
func (c *Client) DetectLane(ctx context.Context, acct *Account) (string, error) {
	var lastErr error
	for _, lane := range laneCandidates() {
		ok, err := c.laneWorks(ctx, lane, acct)
		if ok {
			c.SetLane(lane)
			return lane, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("所有候选车道都被上游拒绝（426）")
	}
	return "", lastErr
}

// laneWorks 用候选车道请求一次 /models。
//
// 只有 2xx 才算通过：5xx 说明上游抖动而不是车道不对，继续试下一个候选；
// 401 说明缺凭据（上游 2026-11-14 起对 /models 也要求 Api-Key），
// 与车道无关，同样继续试但结果必然是全部失败。
func (c *Client) laneWorks(ctx context.Context, lane string, acct *Account) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ModelURL(PathModels), nil)
	if err != nil {
		return false, err
	}
	c.commonHeaders(req, acct)
	req.Header.Set(HeaderLane, lane) // 覆盖成待测值
	resp, err := c.do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode/100 == 2, nil
}

// do 发送请求并返回原样响应（调用方负责关闭 Body）。
//
// 3xx 一律归一成 401：上游对无效 Key 会 302 跳到登录页。跟随会把鉴权失败
// 伪装成 200（所以 CheckRedirect 返回 ErrUseLastResponse），而原样透传在
// API 客户端那边又是个没有 Location 的裸 3xx。统一成 auth_required 后，
// 调用方会标账号失败并换号，客户端拿到的是可识别的 401。
func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 == 3 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, &HTTPError{
			Op:     "上游鉴权",
			Status: http.StatusUnauthorized,
			Body: fmt.Appendf(nil,
				`{"error":{"type":%q,"code":%q,"message":"上游把请求重定向到登录页（API Key 无效或已失效，status=%d）"}}`,
				CodeAuthRequired, CodeAuthRequired, resp.StatusCode),
		}
	}
	return resp, nil
}

// FetchModels 拉取上游模型目录（2026-11-14 起必须带 Api-Key）。
//
// 曾经的结论是「上游目录不随账号变化，带 / 不带 `Api-Key` 返回同一份目录」，
// 因此刻意保持免凭据。**2026-11-14 实测失效**：不带 Key 一律
// 401 `{"code":"auth_required","message":"Sign in to DeepAI to use DeepAI Code."}`，
// 带任意账号的 Key 才 200（见 docs/recon.md §2）。所以：
//   - acct 非 nil 时带该账号的 Key（目录与账号无关，随便一个就行）；
//   - acct 为 nil 时仍发匿名请求：只用于「没有账号时的连通性探测」，
//     上游会回 401，调用方据此区分「链路通」与「拉目录成功」。
//
// 注意上游目录**会变**：2026-11-14 一次实测从 21 个涨到 27 个（新增 6 个
// 免费 / 付费模型，无下线）。目录是缓存，重启或手动刷新才会更新。
func (c *Client) FetchModels(ctx context.Context, acct *Account) ([]Model, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ModelURL(PathModels), nil)
	if err != nil {
		return nil, "", err
	}
	c.commonHeaders(req, acct)

	resp, err := c.do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBytes))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode/100 != 2 {
		return nil, "", &HTTPError{Op: "模型目录", Status: resp.StatusCode, Body: body}
	}

	var payload struct {
		Models  []Model `json:"models"`
		Default string  `json:"default"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", fmt.Errorf("模型目录解析失败: %w", err)
	}
	models := make([]Model, 0, len(payload.Models))
	seen := map[string]bool{}
	for _, m := range payload.Models {
		if m.ID == "" || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		if m.Input == nil {
			m.Input = []string{"text"}
		}
		models = append(models, m)
	}
	if len(models) == 0 {
		return nil, "", errors.New("上游模型目录为空")
	}
	def := payload.Default
	if def == "" {
		def = models[0].ID
	}
	return models, def, nil
}

// OpenModel 发起一次推理请求，返回未经读取的响应。
//
// path 取 PathChat 或 PathResponses；affinity 会作为 X-Session-Affinity
// 透传（上游据此做会话粘性调度）。
func (c *Client) OpenModel(ctx context.Context, acct *Account, path string, body []byte, affinity string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ModelURL(path), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req, acct)
	// 桌面端在转发推理前会记一次 lastRelayAt，心跳里的 pi 字段由此得来。
	c.markRelay(acct)
	if affinity != "" {
		req.Header.Set(HeaderAffinity, affinity)
	}
	return c.do(req)
}

// Theos 查询账号的免费额度 / 余额。
//
// 0.3.19 起 /theos 同时是签到通道：响应（包括 403 拒绝）带 check_in，
// 下一次请求把 nonce 回传并附带心跳。
func (c *Client) Theos(ctx context.Context, acct *Account) (*Allowance, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ModelURL(PathTheos), nil)
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req, acct)
	c.applyCheckInHeaders(req, acct)
	sentNonce := req.Header.Get(HeaderCheckIn)

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// 请求带上了 nonce 就说明这一次是「回传」：签到完成（403 拒绝也算，
	// 桌面端对拒绝同样记一次 noteCheckIn）。
	c.markAcked(acct, sentNonce)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBytes))
	if err != nil {
		return nil, err
	}
	// 无论成功还是拒绝，都先吃掉签到挑战：上游在 403 里也会下发。
	c.captureCheckIn(acct, body)
	if resp.StatusCode/100 != 2 {
		httpErr := &HTTPError{Op: "查询 Theos", Status: resp.StatusCode, Body: body}
		// 签到被当日限流：给账号上静默锁，避免 EnsureCheckIn 每 30s 再撞一次。
		if httpErr.CheckInRefused() {
			c.lockCheckIn(acct, CheckInRefusedLock)
		}
		return nil, httpErr
	}
	var out Allowance
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("Theos 响应解析失败: %w", err)
	}
	out.CheckedAt = time.Now().Unix()
	// 上游回的 lane_version 是**服务端当前车道**，与本次请求发的车道无关：
	// 这是唯一能看出「客户端已经抬车道」的信号（/models 对旧车道也回 200）。
	if out.LaneVersion > 0 && c.LaneHook != nil {
		c.LaneHook(out.LaneVersion)
	}
	return &out, nil
}

// UsageSummary 查询账号订阅 / 用量。
//
// 与桌面端一致：这个根路径请求**不覆盖** User-Agent（保持 Node fetch 的
// `node`）。`DeepAI-Code` 这个 UA 只属于 /gravity-display-proxy 赞助位渲染
// （见 DesktopUserAgent），带到用量接口上反而又是一个异常信号。
func (c *Client) UsageSummary(ctx context.Context, acct *Account) (*UsageSummary, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.RootURL(PathUsageSummary), nil)
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req, acct)

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, &HTTPError{Op: "查询用量", Status: resp.StatusCode, Body: body}
	}
	var out UsageSummary
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("用量响应解析失败: %w", err)
	}
	return &out, nil
}

// WebSearch 调用上游联网搜索能力。
func (c *Client) WebSearch(ctx context.Context, acct *Account, payload []byte) ([]byte, error) {
	return c.postJSON(ctx, acct, PathWebSearch, payload, "联网搜索")
}

// FindService 调用上游服务推荐能力。
func (c *Client) FindService(ctx context.Context, acct *Account, payload []byte) ([]byte, error) {
	return c.postJSON(ctx, acct, PathFindService, payload, "服务搜索")
}

func (c *Client) postJSON(ctx context.Context, acct *Account, path string, payload []byte, op string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ModelURL(path), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	c.commonHeaders(req, acct)

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, &HTTPError{Op: op, Status: resp.StatusCode, Body: body}
	}
	return body, nil
}

// Transcribe 把音频转发到上游语音转文字接口。
func (c *Client) Transcribe(ctx context.Context, acct *Account, filename, contentType string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("音频内容为空")
	}
	if len(data) > 8<<20 {
		return "", errors.New("音频超过 8 MB")
	}
	if filename == "" {
		filename = "rec.webm"
	}
	if contentType == "" {
		contentType = "audio/webm"
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="voiceRecording"; filename="%s"`, escapeQuotes(filename)))
	h.Set("Content-Type", contentType)
	part, err := mw.CreatePart(h)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.RootURL(PathSpeechToText), &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if acct != nil {
		req.Header.Set("api-key", acct.APIKey)
	}

	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode/100 != 2 {
		return "", &HTTPError{Op: "语音转写", Status: resp.StatusCode, Body: body}
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("转写响应解析失败: %w", err)
	}
	if strings.TrimSpace(out.Text) == "" {
		return "", errors.New("上游未返回转写文本")
	}
	return strings.TrimSpace(out.Text), nil
}

// ExchangeCode 用桌面端登录回调里的 code 换取 API Key。
func (c *Client) ExchangeCode(ctx context.Context, code string) (*Credentials, error) {
	payload, _ := json.Marshal(map[string]string{"code": code})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ModelURL(PathExchange), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, &HTTPError{Op: "登录换取密钥", Status: resp.StatusCode, Body: body}
	}
	var out struct {
		APIKey   string `json:"api_key"`
		Username string `json:"username"`
		IsPro    bool   `json:"is_pro"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("登录响应解析失败: %w", err)
	}
	if out.APIKey == "" || strings.ContainsAny(out.APIKey, "\r\n") {
		return nil, errors.New("登录响应缺少有效 API Key")
	}
	return &Credentials{APIKey: out.APIKey, Username: out.Username, IsPro: out.IsPro}, nil
}

func escapeQuotes(s string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(s)
}
