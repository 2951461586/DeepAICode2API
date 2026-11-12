// 控制台主模块（装配层）。
//
// 分层：
//   - render/*.js      只画表格（accounts / keys / logs / settings）
//   - overview.js      KPI、模型表、/health 状态胶囊
//   - oauth.js         浏览器登录面板 + 回调粘贴
//   - settings-form.js 设置表单保存
//   - events.js        事件接线（导航 / 顶栏 / 筛选 / 排序 / 语言 / 脱敏）
//   - theme.js         主题、侧边栏、移动端抽屉
//   - pager.js         账号分页控件
//   - 本文件           取数（refresh / loadAccounts / refreshLogs）、URL 状态、
//                       启动顺序与登录门
//
// 约定：往下层传的是回调（initEvents({refresh, loadAccounts, …})）而不是 import
// 本文件的函数，否则 app.js ⇄ 子模块会循环导入（ESM 下表现为 TDZ 报错）。
import { t, translateDom, applyLang } from './i18n.js';
import { ui } from './state.js';
import { $ } from './dom.js';
import { sortState } from './table.js';
import { toast } from './ui.js';
import { api, pw } from './api.js';
import { setReload } from './actions.js';
import { renderAccounts } from './render/accounts.js';
import { renderKeys } from './render/keys.js';
import { renderSettings } from './render/settings.js';
import { renderOverview, loadHealth } from './overview.js';
import { initOAuth } from './oauth.js';
import { initSettingsForm } from './settings-form.js';
import { checkUpdate } from './version.js';
import { initEvents } from './events.js';
import { initTheme, initSidebar, initDrawer } from './theme.js';
import { initAccountPager } from './pager.js';

var state = { data: null };

/* ================= 页面状态（URL 同步） ================= */
// 筛选条件与页码写进 hash：#accounts?group=g1&status=ok&page=2
// 刷新 / 分享链接都能回到同一视图；切换标签页时只保留当前页的参数。
function readHash() {
  var h = String(location.hash || "").replace(/^#/, "");
  var i = h.indexOf("?");
  var tab = i >= 0 ? h.slice(0, i) : h;
  var qs = i >= 0 ? h.slice(i + 1) : "";
  var params;
  try { params = new URLSearchParams(qs); } catch { params = new URLSearchParams(); }
  return { tab: tab, params: params };
}

function writeHash(tab, params) {
  var qs = params ? params.toString() : "";
  var next = "#" + tab + (qs ? "?" + qs : "");
  if (location.hash === next) return;
  try { history.replaceState(null, "", next); } catch { location.hash = next; }
}

// currentAcctParams 把账号表格的筛选 / 页码收成一个 URLSearchParams。
function currentAcctParams() {
  var p = new URLSearchParams();
  if (ui.acctPage > 0) p.set("page", String(ui.acctPage));
  var g = String(($("#acctGroup") || {}).value || "").trim();
  var s = String(($("#acctStatus") || {}).value || "").trim();
  var q = String(($("#acctQuery") || {}).value || "").trim();
  if (g) p.set("group", g);
  if (s) p.set("status", s);
  if (q) p.set("q", q);
  return p;
}

function currentLogParams() {
  var p = new URLSearchParams();
  var pairs = [["endpoint", "#logEndpoint"], ["account", "#logAccount"],
    ["min_status", "#logMinStatus"], ["q", "#logKeyword"], ["since", "#logSince"]];
  pairs.forEach(function (kv) {
    var v = String(($(kv[1]) || {}).value || "").trim();
    if (v) p.set(kv[0], v);
  });
  var lim = String(($("#logLimit") || {}).value || "").trim();
  if (lim) p.set("limit", lim);
  return p;
}

function syncHash(tab) {
  if (tab === "accounts") writeHash(tab, currentAcctParams());
  else if (tab === "logs") writeHash(tab, currentLogParams());
  else writeHash(tab, null);
}

// restoreFromHash 把 hash 里的筛选 / 页码回填到表单。返回要打开的标签页。
function restoreFromHash() {
  var h = readHash();
  if (h.tab === "logs") {
    var m = { endpoint: "#logEndpoint", account: "#logAccount", min_status: "#logMinStatus",
      q: "#logKeyword", since: "#logSince", limit: "#logLimit" };
    h.params.forEach(function (v, k) {
      var el = m[k] ? $(m[k]) : null;
      if (el) el.value = v;
    });
  }
  if (h.tab === "accounts") {
    var page = parseInt(h.params.get("page") || "0", 10);
    if (page > 0) ui.acctPage = page;
    var map = { group: "#acctGroup", status: "#acctStatus", q: "#acctQuery" };
    Object.keys(map).forEach(function (k) {
      var el = $(map[k]);
      if (el && h.params.get(k)) el.value = h.params.get(k);
    });
  }
  return h.tab;
}

/* ================= 取数 ================= */

// 首屏只拉 overview（不含账号列表），账号走服务端分页。
function refresh() {
  return Promise.all([api("/admin/api/overview"), loadAccounts()]).then(function (res) {
    var d = res[0];
    state.overview = d;
    renderOverview(d);
    renderKeys(d);
    renderSettings(d.settings || {});
    loadHealth();
    translateDom(document.body);
  }).catch(function (e) {
    // 401 由 api() 统一处理（清口令 + 跳登录页），这里只报错误本身。
    toast(t("加载失败：") + e.message, "err");
  });
}

// loadAccounts 按当前页码拉一页账号（服务端分页）。
function loadAccounts() {
  var q = "offset=" + (ui.acctPage * ui.acctPageSize) + "&limit=" + ui.acctPageSize;
  var st = sortState.accounts;
  if (st.key) q += "&sort=" + encodeURIComponent(st.key) + "&order=" + (st.desc ? "desc" : "asc");
  // 服务端分页 + 过滤（AccountPage 支持 group / status / q）。
  var g = String(($("#acctGroup") || {}).value || "").trim();
  var s = String(($("#acctStatus") || {}).value || "").trim();
  var kw = String(($("#acctQuery") || {}).value || "").trim();
  if (g) q += "&group=" + encodeURIComponent(g);
  if (s) q += "&status=" + encodeURIComponent(s);
  if (kw) q += "&q=" + encodeURIComponent(kw);
  return api("/admin/api/accounts?" + q).then(function (page) {
    renderAccounts(page.items, page.total);
    return page;
  });
}

function refreshLogs() {
  var q = "limit=" + encodeURIComponent($("#logLimit").value);
  // QueryLogs 支持 endpoint / account / min_status / q / since（秒）。
  var ep = $("#logEndpoint").value;
  if (ep) q += "&endpoint=" + encodeURIComponent(ep);
  var acc = $("#logAccount").value.trim();
  if (acc) q += "&account=" + encodeURIComponent(acc);
  var minStatus = $("#logMinStatus").value.trim();
  if (minStatus) q += "&min_status=" + encodeURIComponent(minStatus);
  var kw = $("#logKeyword").value.trim();
  if (kw) q += "&q=" + encodeURIComponent(kw);
  var since = $("#logSince").value;
  if (since) {
    var ts = Math.floor(new Date(since).getTime() / 1000);
    if (ts > 0) q += "&since=" + ts;
  }
  return api("/admin/api/logs?" + q).then(function (d) {
    state.logs = d.logs || [];
    renderLogs(state.logs);
  }).catch(function (e) { toast(t("日志加载失败：") + e.message, "err"); });
}

/* ================= 装配 ================= */
// 顺序即依赖：先装上交互接线，再装子面板，最后才启动取数。
initMenusAndPanels();

function initMenusAndPanels() {
  initTheme();
  initSidebar();
  initDrawer();
  initEvents({
    refresh: refresh,
    loadAccounts: loadAccounts,
    refreshLogs: refreshLogs,
    syncHash: syncHash,
    state: state,
  });
  // 分页器只改 ui 状态，取数与 hash 同步仍由本文件负责。
  initAccountPager(function () {
    syncHash("accounts");
    loadAccounts();
  });
  initOAuth({ refresh: refresh });
  initSettingsForm({ refresh: refresh });
  setReload(refresh);
  applyLang();
}

/* ================= 健康心跳 ================= */
// 后台标签页不轮询：既省上游配额，也不用为空页面反复唤醒定时器。
var healthTimer = null;
function startHealthPolling() {
  if (healthTimer) return;
  healthTimer = setInterval(function () {
    if (document.visibilityState === "visible") loadHealth();
  }, 20000);
}
document.addEventListener("visibilitychange", function () {
  if (document.visibilityState === "visible") loadHealth();
});

// start 是真正进入控制台：先按 hash 回填视图，再拉数据 + 20s 健康心跳。
function start() {
  var want = restoreFromHash();
  if (want) {
    var target = document.querySelector('nav.nav button[data-tab="' + want + '"]');
    if (target && !target.classList.contains("on")) target.click();
  }
  refresh();
  if ($("#tab-logs").classList.contains("on")) refreshLogs();
  startHealthPolling();
  // 版本更新检查：服务端缓存 6 小时，打开控制台多打一次请求不会压到 GitHub。
  checkUpdate();
}

// 登录门：没设管理口令（本机放行）直接进；设了就必须先有口令，否则去 /login。
// 依据是 /health 的 admin_required —— 公开端点，没有口令也读得到。
if (pw()) {
  start();
} else {
  fetch("/health").then(function (r) { return r.json(); }).then(function (h) {
    if (h && h.admin_required) { location.replace("/login"); return; }
    start();
  }).catch(start);
}
