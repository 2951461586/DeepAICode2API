// 控制台主模块（装配层）。
//
// 分层：
//   - render/*.js      只画表格（accounts / keys / logs / settings）
//   - overview.js      KPI、模型表、/health 状态胶囊
//   - oauth.js         浏览器登录面板 + 回调粘贴
//   - settings-form.js 设置表单保存
//   - 本文件            取数（refresh / loadAccounts / refreshLogs）、导航、
//                       分页与筛选、表头排序、主题、抽屉
//
// 约定：往下层传的是回调（initOAuth({refresh})）而不是 import 本文件的函数，
// 否则 oauth.js ⇄ app.js 会循环导入（ESM 下表现为 TDZ 报错）。
import {
  t, translateDom, applyLang, setLang, getLang,
} from './i18n.js';
import { ui } from './state.js';
import { $, closeMenu } from './dom.js';
import { sortState, markSort, toggleSort } from './table.js';
import { toast } from './ui.js';
import { api, pw, clearPw } from './api.js';
import { accountOp, checkin, setReload } from './actions.js';
import { renderAccounts } from './render/accounts.js';
import { renderKeys } from './render/keys.js';
import { renderLogs } from './render/logs.js';
import { renderSettings } from './render/settings.js';
import { renderOverview, loadHealth } from './overview.js';
import { initOAuth } from './oauth.js';
import { initSettingsForm } from './settings-form.js';

var state = { data: null };

/* ================= 行内操作菜单 ================= */
document.addEventListener("click", function () { closeMenu(); });
document.addEventListener("keydown", function (e) { if (e.key === "Escape") closeMenu(); });
window.addEventListener("resize", closeMenu);
window.addEventListener("scroll", closeMenu, true);

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

/* ================= 导航 ================= */
document.querySelectorAll("nav button").forEach(function (b) {
  b.onclick = function () {
    document.querySelectorAll("nav button").forEach(function (x) { x.classList.remove("on"); });
    document.querySelectorAll("section").forEach(function (x) { x.classList.remove("on"); });
    b.classList.add("on");
    $("#tab-" + b.dataset.tab).classList.add("on");
    if (b.dataset.tab === "logs") refreshLogs();
  };
});

/* ================= 顶部按钮 ================= */
// 退出登录：只清本标签页的口令，不动别的状态。
$("#logout").onclick = function () {
  clearPw();
  location.replace("/login");
};
$("#refresh").onclick = function () { refresh(); if ($("#tab-logs").classList.contains("on")) refreshLogs(); };
$("#importLocal").onclick = function () { accountOp("import", {}); };
$("#addManual").onclick = function () {
  var key = $("#manualKey").value.trim();
  if (!key) { toast("请填写 API Key", "err"); return; }
  accountOp("manual", { api_key: key, name: $("#manualName").value.trim() }).then(function () {
    $("#manualKey").value = ""; $("#manualName").value = "";
  });
};
$("#checkinAll").onclick = function () { checkin(null); };
$("#refreshModels").onclick = function () {
  toast("正在从上游刷新模型目录…");
  api("/admin/api/models", { method: "POST", body: {} }).then(function (r) {
    toast("模型目录已更新：共 " + r.count + " 个模型", "ok");
    return refresh();
  }).catch(function (e) { toast("刷新失败：" + e.message, "err"); });
};
$("#addKey").onclick = function () {
  api("/admin/api/keys", { method: "POST", body: { name: $("#keyName").value.trim(), group: $("#keyGroup").value.trim() } }).then(function (r) {
    toast("已创建密钥（仅显示一次）：\n" + r.key, "ok");
    $("#keyName").value = "";
    return refresh();
  }).catch(function (e) { toast("创建失败：" + e.message, "err"); });
};
$("#reloadLogs").onclick = refreshLogs;
$("#clearLogs").onclick = function () {
  if (!confirm("确定清空全部日志？")) return;
  api("/admin/api/logs", { method: "DELETE" }).then(function () { toast("已清空", "ok"); return refreshLogs(); })
    .catch(function (e) { toast("失败：" + e.message, "err"); });
};
$("#clearStats").onclick = function () {
  if (!confirm("确定重置统计？")) return;
  api("/admin/api/stats", { method: "DELETE" }).then(function () { toast("已重置", "ok"); return refresh(); })
    .catch(function (e) { toast("失败：" + e.message, "err"); });
};
$("#logLimit").onchange = refreshLogs;

/* ================= 子面板（各自独立的模块） ================= */
initOAuth({ refresh: refresh });
initSettingsForm({ refresh: refresh });

/* ================= 登录状态 ================= */
// 清理旧版本遗留在 localStorage 里的口令（那时顶栏还有输入框）。
try { localStorage.removeItem("deepaicode2api_pw"); } catch { /* 隐私模式忽略 */ }

/* ================= 主题（白天 / 黑夜） ================= */
// 优先用上次的选择；没选过就跟随系统 prefers-color-scheme。
function applyTheme(mode) {
  document.documentElement.setAttribute("data-theme", mode);
  var moon = $("#themeIconMoon"), sun = $("#themeIconSun"), label = $("#themeLabel");
  if (moon) moon.style.display = mode === "light" ? "none" : "";
  if (sun) sun.style.display = mode === "light" ? "" : "none";
  if (label) {
    // 文案变了就得让 translateDom 重记中文原文，否则切回英文时会复现上一次的标签。
    label.__zh = undefined;
    label.textContent = mode === "light" ? "浅色" : "深色";
    translateDom(label.parentNode);
  }
}
(function initTheme() {
  var mode = null;
  try { mode = localStorage.getItem("deepaicode2api_theme"); } catch { /* 隐私模式忽略 */ }
  if (mode !== "light" && mode !== "dark") {
    mode = (window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches) ? "light" : "dark";
  }
  applyTheme(mode);
})();
$("#theme").onclick = function () {
  var next = document.documentElement.getAttribute("data-theme") === "light" ? "dark" : "light";
  applyTheme(next);
  try { localStorage.setItem("deepaicode2api_theme", next); } catch { /* 隐私模式忽略 */ }
};

/* ================= 账号分页控件 ================= */
$("#acctSize").value = String(ui.acctPageSize);
$("#acctPrev").onclick = function () { if (ui.acctPage > 0) { ui.acctPage--; loadAccounts(); } };
$("#acctNext").onclick = function () { ui.acctPage++; loadAccounts(); };
$("#acctSize").onchange = function () {
  var v = parseInt(this.value, 10);
  if (v !== 20 && v !== 50 && v !== 100) return;
  ui.acctPageSize = v;
  ui.acctPage = 0;
  try { localStorage.setItem("deepaicode2api_acct_page_size", String(v)); } catch { /* 隐私模式忽略 */ }
  loadAccounts();
};

/* ================= 账号 / 日志筛选 ================= */
function applyAcctFilters() { ui.acctPage = 0; loadAccounts(); }
$("#acctFilterApply").onclick = applyAcctFilters;
$("#acctFilterClear").onclick = function () {
  $("#acctGroup").value = ""; $("#acctStatus").value = ""; $("#acctQuery").value = "";
  applyAcctFilters();
};
$("#acctStatus").onchange = applyAcctFilters;
$("#acctGroup").onkeydown = $("#acctQuery").onkeydown = function (ev) {
  if (ev.key === "Enter") applyAcctFilters();
};
$("#logFilterApply").onclick = refreshLogs;
$("#logFilterClear").onclick = function () {
  $("#logEndpoint").value = ""; $("#logAccount").value = ""; $("#logMinStatus").value = "";
  $("#logKeyword").value = ""; $("#logSince").value = "";
  refreshLogs();
};
$("#logEndpoint").onchange = refreshLogs;
$("#logKeyword").onkeydown = $("#logAccount").onkeydown = function (ev) {
  if (ev.key === "Enter") refreshLogs();
};

/* ================= 表头排序 ================= */
// accounts 走服务端（分页必须如此），其余三张小表在前端排。
function wireSort(sel, name, onChange) {
  document.querySelectorAll(sel + " th[data-sort]").forEach(function (th) {
    th.addEventListener("click", function () {
      toggleSort(name, th.getAttribute("data-sort"));
      markSort(sel, sortState[name]);
      onChange();
    });
  });
}
wireSort("#acctTable", "accounts", function () { ui.acctPage = 0; loadAccounts(); });
wireSort("#modelTable", "models", function () { if (state.overview) renderOverview(state.overview); });
wireSort("#keyTable", "keys", function () { if (state.overview) renderKeys(state.overview); });
wireSort("#logTable", "logs", function () { renderLogs(state.logs || []); });

/* ================= 语言切换 ================= */
$("#lang").onclick = function () {
  setLang(getLang() === "en" ? "zh" : "en");
  try { localStorage.setItem("deepaicode2api_lang", getLang()); } catch { /* 隐私模式忽略 */ }
  applyLang();
  refresh();
  if ($("#tab-logs").classList.contains("on")) refreshLogs();
};

/* ================= 启动 ================= */
setReload(refresh);
applyLang();
// start 是真正进入控制台：拉数据 + 20s 健康心跳。
function start() {
  refresh();
  setInterval(loadHealth, 20000);
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

// 移动端侧边栏抽屉。包成 IIFE 是因为原来那一段是独立 <script>，
// 内部用 `return` 做「元素不齐就整体放弃」的早退；ES module 顶层不允许 return。
(function initDrawer() {
  var toggle = document.getElementById("navToggle");
  var sidebar = document.getElementById("sidebar");
  var scrim = document.getElementById("scrim");
  if (!toggle || !sidebar || !scrim) return;
  function setOpen(open) {
    sidebar.classList.toggle("open", open);
    scrim.classList.toggle("on", open);
  }
  toggle.addEventListener("click", function () { setOpen(!sidebar.classList.contains("open")); });
  scrim.addEventListener("click", function () { setOpen(false); });
  document.querySelectorAll("nav.nav button").forEach(function (b) {
    b.addEventListener("click", function () { setOpen(false); });
  });
  document.addEventListener("keydown", function (e) { if (e.key === "Escape") setOpen(false); });

  // 深链接：#accounts / #logs … 直接打开对应标签页。
  var want = (location.hash || "").replace(/^#/, "");
  if (want) {
    var target = document.querySelector('nav.nav button[data-tab="' + want + '"]');
    if (target) target.click();
  }
})();
