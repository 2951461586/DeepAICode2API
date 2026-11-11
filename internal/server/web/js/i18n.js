// 控制台 i18n：字典 + 文本替换。
//
// 键是**中文原文**：静态标记与动态渲染共用一套替换，新增文案只要补词条，
// 不必给每个节点加 data-i18n。带插值的句子走 t("共 {n} 个账号", {...})。
//
// 缺口由 console_i18n_test.go 兜底（差集即失败），补词条时该测试会告诉你还差哪些。
//
// 本模块零依赖：不 import 任何其它模块，方便单独测。

var I18N_EN = {
  // 导航 / 品牌
  "概览":"Overview", "账号池":"Accounts", "密钥":"API Keys",
  "日志":"Logs", "设置":"Settings", "控制台":"Console",
  // 顶栏
  "管理密码（如已设置）":"Admin password (if set)", "记住":"Remember", "刷新":"Refresh",
  // 页头
  "号池健康、用量统计与上游模型目录":"Pool health, usage and upstream model catalog",
  "刷新模型目录":"Refresh models", "每秒计费":"Per second",
  "上游账号、配额与调度状态":"Upstream accounts, quota and scheduling state",
  "导入本机桌面端":"Import desktop creds",
  "浏览器登录":"Browser sign-in",
  "刷新全部配额":"Refresh all quota",
  "网关对外签发的调用凭据，可绑定号池分组":"Gateway client keys, optionally bound to a pool group",
  "只记录元信息，不落对话内容":"Metadata only, no conversation content",
  "清空":"Clear",
  "运行参数、模型别名与管理密码":"Runtime options, model aliases and admin password",
  "保存设置":"Save",
  "重置统计":"Reset stats",
  // 面板
  "模型目录":"Model catalog", "手动添加":"Add manually", "账号列表":"Account list",
  "密钥列表":"Key list", "新建密钥":"New key", "运行参数":"Runtime options",
  // 按钮
  "添加 API Key":"Add API key", "创建密钥":"Create key", "上一页":"Prev", "下一页":"Next",
  "取消":"Cancel", "打开 DeepAI 登录页":"Open DeepAI sign-in",
  "配额":"Quota", "停用":"Disable", "启用":"Enable", "清错":"Clear errors", "删除":"Delete",
  "操作":"Actions", "配额刷新":"Quota refresh",
  // 筛选 / 行内治理
  "筛选":"Filter", "重置":"Reset", "分组（留空=全部）":"Group (empty = all)",
  "全部状态":"All statuses", "搜索邮箱 / 名称 / Key / 备注":"Search email / name / key / note",
  "全部端点":"All endpoints", "状态 ≥":"Status ≥", "关键字（错误 / 模型）":"Keyword (error / model)",
  "起始时间":"Since",
  "补绑代理":"Assign proxy", "重绑代理":"Reassign proxy", "清除代理":"Clear proxy",
  "分配机器指纹":"Assign machine ID", "改分组":"Change group", "标记已验证":"Mark verified",
  "新分组名":"New group name", "分组不能为空":"Group cannot be empty",
  "密钥绑定的分组（留空=全部）":"Key group (empty = all)",
  "该账号已绑定代理（{p}），从代理池重新分配？":"This account already has a proxy ({p}); reassign from the pool?",
  "从代理池为 {name} 分配一个出口？":"Assign an exit from the proxy pool to {name}?",
  "清除 {name} 的代理（回到直连）？":"Clear the proxy for {name} (back to direct)?",
  "为 {name} 生成独立机器指纹？（只补空值，不覆盖已有指纹）":"Generate a dedicated machine ID for {name}? (only fills empty values)",
  "把 {name} 标记为邮箱已验证？":"Mark {name} as email-verified?",
  // 输入占位 / 标签
  "账号名称（可选）":"Account name (optional)",
  "API Key（DeepAI 账号密钥）":"API key (DeepAI account key)",
  "密钥备注（可选）":"Key note (optional)", "分组（默认 default）":"Group (default)",
  "每页":"Per page",
  "号池分组":"Pool group",
  "请求超时（秒）":"Request timeout (s)", "最大重试次数":"Max retries",
  "日志保留条数":"Log keep", "CORS 允许来源（逗号分隔，留空=仅本机）":"CORS origins (comma separated; empty = loopback only)",
  "兜底模型（无法识别的模型名回落到它，留空=目录默认）":"Fallback model (empty = catalog default)",
  "管理密码（留空则保持；设置后管理接口必须携带）":"Admin password (empty = keep current)",
  "不修改请留空":"Leave empty to keep",
  // 下拉项
  "自动":"Auto",
  "最近 50":"Last 50", "最近 200":"Last 200", "全部":"All",
  // 表头
  "账号":"Account", "分组":"Group", "来源":"Source", "套餐 / 验证":"Plan / verified",
  "状态":"Status", "失败":"Failures", "最近使用":"Last used", "ID":"ID", "名称":"Name",
  "计费":"Billing", "Theos/次":"Theos/req", "上下文":"Context", "最大输出":"Max output",
  "端点":"Endpoint", "输入":"Input", "备注":"Note", "请求数":"Requests",
  "创建时间":"Created", "时间":"Time", "模型":"Model", "流式":"Stream", "耗时":"Latency",
  "字符":"Chars", "Token":"Tokens", "成本":"Cost", "错误":"Error",
  "邮箱":"Email",
  // 状态 / 片段
  "未探测":"untested", "已停用":"disabled", "已验证":"verified", "未验证":"unverified",
  "是":"yes", "否":"no", "无":"none", "推理":"reasoning",
  "本页无数据":"No rows on this page",
  "暂无账号，点击「导入本机桌面端」或「浏览器登录」":"No accounts yet — import desktop creds or use browser sign-in",
  "暂无密钥（未签发时 /v1 允许无密钥调用）":"No keys yet (/v1 allows keyless calls until one exists)",
  "暂无日志":"No logs",
  "等待浏览器完成登录…":"Waiting for browser sign-in…",
  "登录页会在新标签页打开；如果没有自动打开，请点击下面的按钮。":
    "The sign-in page opens in a new tab; use the button below if it did not.",
  "Docker / 远程部署时浏览器回调打不开（127.0.0.1 指的是你自己的机器）：把地址栏里的完整 URL 粘到下面，由网关完成兑换。":
    "In Docker or on a remote host the callback cannot open (127.0.0.1 is your own machine): paste the full URL from the address bar below and let the gateway finish the exchange.",
  "粘贴回调 URL 完成登录":"Finish sign-in with pasted URL",
  "先粘贴回调 URL":"Paste the callback URL first",
  "正在用粘贴的回调兑换…":"Exchanging the pasted callback…",
  "复制登录链接":"Copy sign-in link",
  "登录链接在发起后生成":"The link appears after a sign-in starts",
  "登录链接已复制":"Sign-in link copied",
  "先发起一次登录，再复制链接":"Start a sign-in first, then copy the link",
  "复制失败，已选中链接，请按 Ctrl+C":"Copy failed; the link is selected — press Ctrl+C",
  "链接含一次性 state，可以在另一个浏览器 / 机器上打开（同网络内直接访问网关地址即可）。":
    "The link carries a one-time state; it can be opened in another browser or machine that can reach this gateway.",
  "凭据已脱敏展示；完整 API Key 仅存于本机状态文件（权限 0600）。":
    "Credentials are masked; full keys live only in the local state file (mode 0600).",
  "目录由上游":"Catalog comes from upstream",
  "下发；计费为":"; models billed as", "的模型每天有免费 Theos 额度。":"get a daily free Theos allowance.",
  "密钥已脱敏。完整密钥只在创建时返回一次，请立即保存。":
    "Keys are masked. A full key is returned only once at creation — save it now.",
  "未签发任何密钥时，":"When no key exists, ",
  "允许无密钥调用。":"allows keyless calls.",
  "日志落在":"Logs are written to", "超过":"rotating at", "自动轮转；支持":"and support",
  "过滤。":"filters.",
  // toast / 对话框
  "操作成功":"Operation succeeded", "已清空":"Cleared", "已重置":"Reset",
  "已记住管理密码（仅本次会话）":"Admin password remembered (this session only)",
  "请填写 API Key":"Enter an API key",
  "正在查询上游配额…":"Querying upstream quota…",
  "正在从上游刷新模型目录…":"Refreshing model catalog…",
  "模型目录已更新":"Model catalog updated",
  "模型别名不是合法 JSON":"Model aliases is not valid JSON",
  "设置已保存":"Settings saved", "已创建密钥（仅显示一次）：":"Key created (shown once):",
  "确定清空全部日志？":"Clear all logs?", "确定重置统计？":"Reset statistics?",
  "确定删除该密钥？":"Delete this key?",
  "登录会话已过期，请重新发起。":"Sign-in session expired, please retry.",
  // 模板句（t() 插值）
  "账号 {a}/{b} 就绪 · 模型 {c}":"{a}/{b} ready · {c} models",
  "账号 {a}/{b} 就绪 · {d} 个近期失败 · 模型 {c}":"{a}/{b} ready · {d} recently failing · {c} models",
  "机器指纹 {m} · 默认 {d}":"Machine {m} · default {d}",
  "共 {n} 个账号":"{n} accounts",
  "共 {n} 个账号 · 第 {p}/{t} 页 · 显示 {a}-{b}":"{n} accounts · page {p}/{t} · showing {a}-{b}",
  "确定删除账号 {name}？":"Delete account {name}?",
  "不可用":"unavailable", "不可达":"unreachable",
  "加载失败：":"Load failed: ", "失败：":"Failed: ", "日志加载失败：":"Logs failed: ",
  "查询失败：":"Query failed: ", "刷新失败：":"Refresh failed: ", "创建失败：":"Create failed: ",
  "保存失败：":"Save failed: ",
  "配额刷新完成：":"Quota refresh done: ",
  "请在右上角填写管理密码":"Enter the admin password at top right",
  "登录成功：":"Signed in: ",
  "深色":"Dark", "浅色":"Light",
  "切换白天 / 黑夜模式":"Toggle light / dark mode",
  // KPI 标签
  "总请求":"Total requests", "失败请求":"Failed requests", "输出字符":"Output chars",
  "Token 入 / 出":"Tokens in / out", "上游成本":"Upstream cost",
  "账号（就绪/总）":"Accounts (ready/total)", "统计自":"Stats since",
  "更新于 {t} · 默认模型 {m}":"Updated {t} · default {m}",
  // 手工补齐：i18n 覆盖率测试（TestConsoleI18nCoverage）会检查差集。
  "严格模型模式":"Strict model mode",
  "改绑分组":"Rebind group",
  "检测中…":"Checking…",
  "请求日志":"Request logs",
  "未知错误":"Unknown error",
  "未设置":"Not set",
  "API 密钥":"API key",
  "已设置，留空则不修改":"Set; leave blank to keep",
  "未识别的模型名返回 404（不做静默回落）":"Unknown model names return 404 (no silent fallback)",
  "配额刷新：":"Quota refresh: ",
  "兑换失败：":"Exchange failed: ",
  "轮询失败：":"Poll failed: ",
  "发起登录失败：":"Failed to start login: ",
  "登录失败：":"Login failed: ",
  "上游 …":"Upstream …",
  "模型别名（JSON，如":"Model aliases (JSON, e.g. ",
};

var lang = "zh";
try {
  // ?lang=en 优先（便于分享/截图固定语言），其次本地偏好，最后跟随浏览器。
  var qs = (location.search.match(/[?&]lang=(en|zh)/) || [])[1];
  var savedLang = localStorage.getItem("deepaicode2api_lang");
  if (qs) lang = qs;
  else if (savedLang === "en" || savedLang === "zh") lang = savedLang;
  else if ((navigator.language || "").toLowerCase().indexOf("zh") !== 0) lang = "en";
} catch { /* localStorage 不可用时按语言环境判断 */ }

// tr 只做精确匹配（渲染后的整段文本）。
export function tr(s){
  if (lang !== "en") return s;
  return I18N_EN[s] !== undefined ? I18N_EN[s] : s;
}

// t 支持 {name} 插值，用于拼接句子。
export function t(s, vars){
  var out = tr(s);
  if (vars) Object.keys(vars).forEach(function(k){ out = out.split("{" + k + "}").join(String(vars[k])); });
  return out;
}

var SKIP_TAGS = { SCRIPT:1, STYLE:1, TEXTAREA:1 };

// translateDom 把一段 DOM 里的「纯中文叶子文本」翻成当前语言。
//
// 首次遇到某节点时把中文原文记在 node.__zh 上，切回中文时用它还原；
// 这样静态标记与动态渲染都走同一条路径，不必给每个节点加 data-i18n。
export function translateDom(root){
  if (!root) return;
  var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, null);
  var nodes = [], n;
  while ((n = walker.nextNode())) nodes.push(n);
  nodes.forEach(function(node){
    var parent = node.parentNode;
    if (parent && SKIP_TAGS[parent.nodeName]) return;
    var raw = node.nodeValue || "";
    var key = raw.trim();
    if (!key) return;
    if (node.__zh === undefined) node.__zh = key;
    var want = node.__zh;
    if (lang === "en" && I18N_EN[node.__zh] !== undefined) want = I18N_EN[node.__zh];
    if (want === key) return;
    var lead = raw.match(/^\s*/)[0], trail = raw.match(/\s*$/)[0];
    node.nodeValue = lead + want + trail;
  });
}

// 输入框占位符与 title 也要跟着切换。
export function translateAttrs(root){
  if (!root || !root.querySelectorAll) return;
  root.querySelectorAll("[placeholder]").forEach(function(el){
    if (el.__ph === undefined) el.__ph = el.getAttribute("placeholder");
    el.setAttribute("placeholder", lang === "en" && I18N_EN[el.__ph] !== undefined ? I18N_EN[el.__ph] : el.__ph);
  });
  root.querySelectorAll("[title]").forEach(function(el){
    if (el.__title === undefined) el.__title = el.getAttribute("title");
    el.setAttribute("title", lang === "en" && I18N_EN[el.__title] !== undefined ? I18N_EN[el.__title] : el.__title);
  });
}

export function applyLang(){
  translateDom(document.body);
  translateAttrs(document.body);
  var b = document.getElementById("lang");
  if (b) b.textContent = lang === "en" ? "中" : "EN";
  document.documentElement.setAttribute("lang", lang === "en" ? "en" : "zh-CN");
}

/* ================= 行内操作菜单构造 ================= */
// setLang / getLang 是语言状态的唯一入口。
//
// lang 是模块私有变量：外部只能通过 setLang 改，避免各模块各拿一份副本
// 导致「切了语言但某些文案没跟着变」。
export function setLang(v) { lang = v === 'en' ? 'en' : 'zh'; }
export function getLang() { return lang; }
