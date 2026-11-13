// 账号列表渲染：服务端分页表 + 行内操作菜单。

import { $, mk, cell, row, pill, emptyRow, menuToggle, openMenu } from '../dom.js';
import { t, translateDom } from '../i18n.js';
import { fmtTime, theosText } from '../format.js';
import { ui } from '../state.js';
import { accountOp, checkin } from '../actions.js';
import { toast } from '../ui.js';
import { confirmDialog, promptDialog } from '../dialog.js';

export function rowMenu(a){
  var m = menuToggle("操作");
  m.addEventListener("click", function(ev){
    ev.stopPropagation();
    var items = [
      { label: "配额", fn: function(){ checkin(a.id); } },
      { label: a.enabled ? "停用" : "启用", fn: function(){ accountOp("toggle", {id:a.id}); } }
    ];
    if (a.failures > 0) {
      items.push({ label: "清错", fn: function(){ accountOp("clear-errors", {id:a.id}); } });
    }
    // --- 存量账号治理（后端 action 早已就绪，之前没接） ---
    items.push({ sep: true });
    items.push({ label: a.proxy ? "重绑代理" : "补绑代理", fn: function(){
      var msg = a.proxy
        ? t("该账号已绑定代理（{p}），从代理池重新分配？", {p: a.proxy})
        : t("从代理池为 {name} 分配一个出口？", {name: a.name || a.id});
      confirmDialog({title: t("确认操作"), message: msg}).then(function(ok){
        if (ok) accountOp("assign-proxy", {id: a.id, reassign: !!a.proxy});
      });
    }});
    if (a.proxy) {
      items.push({ label: "清除代理", fn: function(){
        confirmDialog({title: t("确认操作"), message: t("清除 {name} 的代理（回到直连）？", {name: a.name || a.id})})
          .then(function(ok){ if (ok) accountOp("assign-proxy", {id: a.id, clear: true}); });
      }});
    }
    if (!a.has_machine) {
      items.push({ label: "分配机器指纹", fn: function(){
        confirmDialog({title: t("确认操作"), message: t("为 {name} 生成独立机器指纹？（只补空值，不覆盖已有指纹）", {name: a.name || a.id})})
          .then(function(ok){ if (ok) accountOp("assign-machine", {id: a.id}); });
      }});
    }
    items.push({ label: "改分组", fn: function(){
      promptDialog({title: t("改分组"), label: t("新分组名"), value: a.group || "default", okText: t("保存")})
        .then(function(g){
          if (g === null) return;
          g = String(g).trim();
          if (!g) { toast(t("分组不能为空"), "err"); return; }
          accountOp("set-group", {id: a.id, group: g});
        });
    }});
    if (!a.email_verified) {
      items.push({ label: "标记已验证", fn: function(){
        confirmDialog({title: t("确认操作"), message: t("把 {name} 标记为邮箱已验证？", {name: a.name || a.id})})
          .then(function(ok){ if (ok) accountOp("mark-verified", {id: a.id, email: a.email}); });
      }});
    }
    items.push({ sep: true });
    items.push({ label: "删除", danger: true, fn: function(){
      confirmDialog({title: t("删除账号"), message: t("确定删除账号 {name}？", {name: a.name || a.email || a.id}), danger: true})
        .then(function(ok){ if (ok) accountOp("remove", {id: a.id}); });
    }});
    openMenu(m, items);
  });
  return m;
}

// maskEmail 把邮箱脱敏成 u***@domain；非邮箱（纯名称）走 maskWord。
//
// 只保留首字母与域名：既能让人对上号，又不把完整账号落到屏幕/截图/录屏里。
export function maskEmail(s){
  s = String(s || "");
  var at = s.indexOf("@");
  if (at <= 0) return maskWord(s);
  return s.slice(0, 1) + "***" + s.slice(at);
}

// maskWord 把普通标识符脱敏成首字母 + ***（长于 4 时保留末位）。
export function maskWord(s){
  s = String(s || "");
  if (!s) return "";
  if (s.length <= 2) return s.slice(0, 1) + "*";
  return s.slice(0, 1) + "***" + (s.length > 4 ? s.slice(-1) : "");
}

// show 根据全局脱敏开关决定是否过脱敏。
function show(s, maskFn){
  if (!ui.mask) return s;
  return (maskFn || maskWord)(s);
}

export function renderAccounts(items, total){
  var tb = $("#accounts");
  tb.textContent = "";
  var accts = items || [];
  if (!accts.length) {
    tb.appendChild(emptyRow(10, total ? "本页无数据" : "暂无账号，点击「导入本机桌面端」或「浏览器登录」"));
    updateAcctPager(total || 0);
    return;
  }
  accts.forEach(function(a){
    var statusCell = cell("");
    if (a.enabled) {
      var okStatus = a.status === "ok";
      var label = a.status || "error";
      if (a.status === "unknown") label = "未探测";
      var statusCls = "err";
      if (a.status === "unknown") statusCls = "warn";
      else if (okStatus) statusCls = "ok";
      statusCell.appendChild(pill(label, statusCls));
    } else {
      statusCell.appendChild(pill("已停用"));
    }

    var ops = cell("");
    ops.appendChild(rowMenu(a));

    var acctCell = cell("");
    acctCell.appendChild(mk("div", null, show(a.email || a.name, maskEmail)));
    acctCell.appendChild(mk("div", "muted mono", show(a.username) || ""));
    if (a.proxy) acctCell.appendChild(mk("div", "muted mono", a.proxy + (a.fingerprint ? " · " + a.fingerprint : "")));
    var planText = a.is_pro ? "Pro" : (a.plan || "Free");
    planText += " · " + t(a.email_verified ? "已验证" : "未验证");
    var r = row([
      acctCell, cell(a.group || "default", "mono"), cell(a.source, "muted"),
      cell(a.key, "mono"), cell(planText, a.is_pro ? "" : "muted"), cell(theosText(a.theos), "mono"),
      statusCell, cell(a.failures, "mono"), cell(fmtTime(a.last_used), "muted"), ops
    ]);
    if (a.last_err) r.title = a.last_err;
    if (a.theos && a.theos.refresh_at) r.title = (a.last_err ? a.last_err + "\n" : "") + "配额刷新：" + a.theos.refresh_at;
    tb.appendChild(r);
  });
  updateAcctPager(total || 0);
  translateDom(tb);
}

export function updateAcctPager(total){
  var info = $("#acctInfo"), prev = $("#acctPrev"), next = $("#acctNext");
  if (!info || !prev || !next) return;
  var pages = Math.max(1, Math.ceil(total / ui.acctPageSize));
  if (!total) {
    info.textContent = t("共 {n} 个账号", {n: 0});
    prev.disabled = true;
    next.disabled = true;
    return;
  }
  var from = ui.acctPage * ui.acctPageSize + 1;
  var to = Math.min(from + ui.acctPageSize - 1, total);
  info.textContent = t("共 {n} 个账号 · 第 {p}/{t} 页 · 显示 {a}-{b}",
    {n: total, p: ui.acctPage + 1, t: pages, a: from, b: to});
  prev.disabled = ui.acctPage <= 0;
  next.disabled = ui.acctPage >= pages - 1;
}
