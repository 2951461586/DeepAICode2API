// 事件接线：导航、顶栏按钮、账号/日志筛选、表头排序、语言、脱敏开关。
//
// 这里几乎全是 DOM 副作用，没有任何可单测的业务逻辑 —— 拆出来是为了让 app.js
// 回到「取数 + 装配」两件事：数据从哪来、模块按什么顺序装上。
//
// 依赖一律注入（{ refresh, loadAccounts, refreshLogs, syncHash, state }），
// 不 import app.js：ESM 下 app.js ⇄ events.js 循环导入会表现为 TDZ 报错。

import { $, closeMenu } from './dom.js';
import { ui } from './state.js';
import { t, translateDom, applyLang, setLang, getLang } from './i18n.js';
import { sortState, markSort, toggleSort } from './table.js';
import { toast } from './ui.js';
import { confirmDialog } from './dialog.js';
import { api, clearPw } from './api.js';
import { accountOp, checkin } from './actions.js';
import { renderOverview } from './overview.js';
import { renderKeys } from './render/keys.js';
import { renderLogs } from './render/logs.js';

// debounce：输入停下 250ms 再查，避免每敲一个字打一次接口。
function debounce(fn, ms) {
  var timer = null;
  return function () {
    clearTimeout(timer);
    timer = setTimeout(fn, ms);
  };
}

// initEvents 安装全部交互接线（按职责拆成 5 组，避免一个巨型函数）。
export function initEvents(ctx) {
  initMenus();
  initNav(ctx);
  initTopBar(ctx);
  initFilters(ctx);
  initSorting(ctx);
  initPrefs(ctx);
  // 语言切换后重新翻译一次静态标记（渲染过的表格由各 render 自己处理）。
  translateDom(document.body);
}

// initMenus 绑定「行内菜单随点击 / Esc / 滚动 / 缩放关闭」。
function initMenus() {
  document.addEventListener("click", function () { closeMenu(); });
  document.addEventListener("keydown", function (e) { if (e.key === "Escape") closeMenu(); });
  window.addEventListener("resize", closeMenu);
  window.addEventListener("scroll", closeMenu, true);

}

// initNav 绑定侧边栏导航。
function initNav(ctx) {
  var loadAccounts = ctx.loadAccounts;
  var refreshLogs = ctx.refreshLogs;
  var syncHash = ctx.syncHash;

  document.querySelectorAll("nav button").forEach(function (b) {
    b.onclick = function () {
      document.querySelectorAll("nav button").forEach(function (x) { x.classList.remove("on"); });
      document.querySelectorAll("section").forEach(function (x) { x.classList.remove("on"); });
      b.classList.add("on");
      $("#tab-" + b.dataset.tab).classList.add("on");
      if (b.dataset.tab === "logs") refreshLogs();
      if (b.dataset.tab === "accounts") loadAccounts();
      syncHash(b.dataset.tab);
    };
  });

}

// initTopBar 绑定顶栏动作按钮。
function initTopBar(ctx) {
  var refresh = ctx.refresh;
  var refreshLogs = ctx.refreshLogs;

  // 退出登录：只清本标签页的口令，不动别的状态。
  $("#logout").onclick = function () {
    clearPw();
    location.replace("/login");
  };
  $("#refresh").onclick = function () {
    refresh();
    if ($("#tab-logs").classList.contains("on")) refreshLogs();
  };
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

  // 日志「实时」开关：5s 轮询，但只在标签页可见且停在日志页时取数。
  // 定时器与 UI 状态都放在本模块，切走/隐藏不会累积多个 interval。
  var liveTimer = null;
  function setLive(on) {
    if (on && !liveTimer) {
      liveTimer = setInterval(function () {
        if (document.visibilityState !== "visible") return;
        if (!$("#tab-logs").classList.contains("on")) return;
        refreshLogs();
      }, 5000);
    } else if (!on && liveTimer) {
      clearInterval(liveTimer);
      liveTimer = null;
    }
    var b = $("#logLive");
    if (b) {
      b.setAttribute("aria-pressed", on ? "true" : "false");
      b.classList.toggle("on", !!on);
    }
  }
  $("#logLive").onclick = function () {
    var on = $("#logLive").getAttribute("aria-pressed") !== "true";
    setLive(on);
    if (on) refreshLogs();
  };
  $("#clearLogs").onclick = function () {
    confirmDialog({title: t("清空"), message: t("确定清空全部日志？"), danger: true}).then(function (ok) {
      if (!ok) return;
      api("/admin/api/logs", { method: "DELETE" }).then(function () { toast("已清空", "ok"); return refreshLogs(); })
        .catch(function (e) { toast("失败：" + e.message, "err"); });
    });
  };
  $("#clearStats").onclick = function () {
    confirmDialog({title: t("重置统计"), message: t("确定重置统计？"), danger: true}).then(function (ok) {
      if (!ok) return;
      api("/admin/api/stats", { method: "DELETE" }).then(function () { toast("已重置", "ok"); return refresh(); })
        .catch(function (e) { toast("失败：" + e.message, "err"); });
    });
  };
  $("#logLimit").onchange = refreshLogs;

}

// initFilters 绑定账号 / 日志筛选（含输入防抖）。
function initFilters(ctx) {
  var loadAccounts = ctx.loadAccounts;
  var refreshLogs = ctx.refreshLogs;
  var syncHash = ctx.syncHash;

  function applyAcctFilters() {
    ui.acctPage = 0;
    syncHash("accounts");
    loadAccounts();
  }
  $("#acctFilterApply").onclick = applyAcctFilters;
  $("#acctFilterClear").onclick = function () {
    $("#acctGroup").value = ""; $("#acctStatus").value = ""; $("#acctQuery").value = "";
    applyAcctFilters();
  };
  $("#acctStatus").onchange = applyAcctFilters;

  var acctQueryDebounced = debounce(applyAcctFilters, 250);
  $("#acctGroup").onkeydown = $("#acctQuery").onkeydown = function (ev) {
    if (ev.key === "Enter") applyAcctFilters();
  };
  $("#acctGroup").oninput = $("#acctQuery").oninput = acctQueryDebounced;

  /* ---- 日志筛选 ---- */
  $("#logFilterApply").onclick = function () { syncHash("logs"); refreshLogs(); };
  $("#logFilterClear").onclick = function () {
    $("#logEndpoint").value = ""; $("#logAccount").value = ""; $("#logMinStatus").value = "";
    $("#logKeyword").value = ""; $("#logSince").value = "";
    syncHash("logs");
    refreshLogs();
  };
  $("#logEndpoint").onchange = function () { syncHash("logs"); refreshLogs(); };
  var logQueryDebounced = debounce(function () { syncHash("logs"); refreshLogs(); }, 250);
  $("#logKeyword").onkeydown = $("#logAccount").onkeydown = function (ev) {
    if (ev.key === "Enter") { syncHash("logs"); refreshLogs(); }
  };
  $("#logKeyword").oninput = $("#logAccount").oninput = logQueryDebounced;

}

// initSorting 绑定表头排序。
//
// accounts 走服务端（分页必须如此），其余三张小表在前端排。
function initSorting(ctx) {
  var loadAccounts = ctx.loadAccounts;
  var state = ctx.state || {};
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

}

// initPrefs 绑定语言与脱敏开关（两者都只影响展示层）。
function initPrefs(ctx) {
  var refresh = ctx.refresh;
  var refreshLogs = ctx.refreshLogs;
  var loadAccounts = ctx.loadAccounts;

  $("#lang").onclick = function () {
    setLang(getLang() === "en" ? "zh" : "en");
    try { localStorage.setItem("deepaicode2api_lang", getLang()); } catch { /* 隐私模式忽略 */ }
    applyLang();
    refresh();
    if ($("#tab-logs").classList.contains("on")) refreshLogs();
  };

  // 脱敏在渲染层做（不是后端字段）：开关能立即生效，且不影响搜索/筛选 ——
  // 搜索仍然对完整的邮箱 / 名称生效，只是显示时打码。
  $("#acctMask").checked = !!ui.mask;
  $("#acctMask").onchange = function () {
    ui.mask = !!this.checked;
    try { localStorage.setItem("deepaicode2api_acct_mask", ui.mask ? "1" : "0"); } catch { /* 隐私模式忽略 */ }
    loadAccounts();
  };
}
