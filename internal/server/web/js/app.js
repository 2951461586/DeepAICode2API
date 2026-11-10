// 控制台主模块。
//
// 这是阶段 A 的纯搬家结果：内容与原先内联的 <script> 完全一致，
// 只是从 index.html 移到了独立文件（ES module 自带严格模式与独立作用域，
// 所以原来的 IIFE 外壳已去掉）。
import {
  t, translateDom, applyLang, setLang, getLang,
} from './i18n.js';
import { fmtTime, fmtNum, fmtCost, fmtCompact, perSecondText } from './format.js';
import { ui } from './state.js';
import { $, mk, cell, row, emptyRow, closeMenu } from './dom.js';
import { sortState, markSort, toggleSort, sortCopy } from './table.js';
import { toast } from './ui.js';
import { pw, api } from './api.js';
import { accountOp, checkin, setReload } from './actions.js';
import { renderAccounts } from './render/accounts.js';
import { renderKeys } from './render/keys.js';
import { renderLogs } from './render/logs.js';
import { renderSettings } from './render/settings.js';
  var state = { data:null };




  /* ================= 行内操作菜单 ================= */
  var menuEl = null;
  document.addEventListener("click", function(){ closeMenu(); });
  document.addEventListener("keydown", function(e){ if (e.key === "Escape") closeMenu(); });
  window.addEventListener("resize", closeMenu);
  window.addEventListener("scroll", closeMenu, true);

  /* ================= 表格排序 ================= */
  // accounts 是服务端分页，排序必须下推到后端；其余表数据量小，前端排即可。






  // 同一时刻对同一 GET 只发一次请求。
  //
  // 否则连点「刷新」或「操作 + 刷新」并发时，后返回的旧响应会覆盖新状态。

  // --- DOM 构建助手（全程 textContent，绝不拼接 HTML） ---


  function setHealth(cls, text){
    var el = $("#health");
    el.className = "badge " + cls;
    el.textContent = "";
    el.appendChild(mk("i", "dot"));
    el.appendChild(mk("span", null, text));
  }

  function loadHealth(){
    fetch("/health").then(function(r){ return r.json(); }).then(function(h){
      var ready = h.ready || 0, total = h.accounts || 0;
      var healthCls = "";
      if (total === 0) healthCls = "";
      else if (ready > 0) healthCls = "ok";
      else healthCls = "warn";
      setHealth(healthCls, t("账号 {a}/{b} 就绪 · 模型 {c}", {a:ready, b:total, c:h.models||0}));
      $("#upstream").textContent = t("机器指纹 {m} · 默认 {d}",
        {m: h.machine_id || t("不可用"), d: h.default_model || "-"});
    }).catch(function(){ setHealth("err", t("不可达")); });
  }

  function renderOverview(d){
    var s = d.stats || {};
    // overview 只给计数（不再下发全量账号）。
    var ready = d.ready || 0;
    var totalAccts = d.accounts || 0;
    var kpis = [
      ["总请求", fmtNum(s.total_requests)],
      ["失败请求", fmtNum(s.failed_requests)],
      ["输出字符", fmtCompact(s.total_chars_out)],
      ["Token 入 / 出", fmtCompact(s.total_tokens_in) + " / " + fmtCompact(s.total_tokens_out)],
      ["上游成本", fmtCost(s.total_cost)],
      ["账号（就绪/总）", ready + " / " + totalAccts],
      ["密钥", (d.keys||[]).length],
      ["统计自", fmtTime(s.reset_at || s.started_at)]
    ];
    var box = $("#kpis");
    box.textContent = "";
    kpis.forEach(function(k){
      var c = mk("div", "kpi");
      c.appendChild(mk("div", "k", k[0]));
      c.appendChild(mk("div", "v", k[1]));
      box.appendChild(c);
    });

    $("#modelsMeta").textContent = t("更新于 {t} · 默认模型 {m}",
      {t: fmtTime(d.models_updated), m: d.default_model || "-"});
    var tb = $("#models");
    tb.textContent = "";
    var models = sortCopy(d.models || [], sortState.models, {
      id: function(m){ return m.id || ""; },
      name: function(m){ return m.name || m.id || ""; },
      billing: function(m){ return m.premier ? 1 : 0; },
      theos: function(m){ return Number(m.theosPerRequest || 0); },
      theos2: function(m){ return perSecondText(m); },
      context: function(m){ return Number(m.contextWindow || 0); },
      max: function(m){ return Number(m.maxTokens || 0); },
      api: function(m){ return m.api || "chat"; },
      input: function(m){ return (m.input || []).join(",") + (m.reasoning ? ",r" : ""); }
    });
    if (!models.length) { tb.appendChild(emptyRow(9, "无")); return; }
    models.forEach(function(m){
      // 后端字段是 camelCase（theosPerRequest / contextWindow / maxTokens / input），
      // 之前按 Go 字段名（PascalCase）取，于是整列都是空的。
      var price = (m.theosPerRequest === undefined || m.theosPerRequest === null) ? "—" : m.theosPerRequest;
      // free 由后端 Model.Free() 下发（单一口径）；旧响应没有这个字段时才回退本地判断。
      var free = (m.free === undefined)
        ? (!m.premier && (price === "—" || String(price) === "0"))
        : !!m.free;
      var input = (m.input || []).join(" + ");
      if (m.reasoning) input = input ? input + " · " + t("推理") : t("推理");
      tb.appendChild(row([
        cell(m.id, "mono"),
        cell(m.name || m.id),
        cell(free ? "free" : "premier", free ? "muted" : ""),
        cell(price, "mono"),
        cell(perSecondText(m), "mono muted"),
        cell(fmtNum(m.contextWindow), "mono"),
        cell(fmtNum(m.maxTokens), "mono"),
        cell(m.api || "chat", "mono muted"),
        cell(input, "muted")
      ]));
    });
    translateDom($("#tab-overview"));
  }

  // accounts 分页由服务端完成：这里只渲染当前页。

  // 分页控件状态（总数为 0 时也给出明确文案）。




  // 首屏只拉 overview（不含账号列表），账号走服务端分页。
  function refresh(){
    return Promise.all([api("/admin/api/overview"), loadAccounts()]).then(function(res){
      var d = res[0];
      state.overview = d;
      renderOverview(d);
      renderKeys(d);
      renderSettings(d.settings || {});
      loadHealth();
      translateDom(document.body);
    }).catch(function(e){
      toast(t("加载失败：") + e.message + (e.status === 401 ? "\n" + t("请在右上角填写管理密码") : ""), "err");
    });
  }

  // loadAccounts 按当前页码拉一页账号（服务端分页）。
  function loadAccounts(){
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
    return api("/admin/api/accounts?" + q).then(function(page){
      renderAccounts(page.items, page.total);
      return page;
    });
  }
  function refreshLogs(){
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
    return api("/admin/api/logs?" + q).then(function(d){
      state.logs = d.logs || [];
      renderLogs(state.logs);
    }).catch(function(e){ toast(t("日志加载失败：") + e.message, "err"); });
  }

  // 分批探测：单批上限由服务端控制（653 号全量一次要 11 分钟）。
  // 探过的账号 checked_at 会推到现在，下一批自然取到没探过的，不会重复烧额度。


  // --- events ---
  document.querySelectorAll("nav button").forEach(function(b){
    b.onclick = function(){
      document.querySelectorAll("nav button").forEach(function(x){ x.classList.remove("on"); });
      document.querySelectorAll("section").forEach(function(x){ x.classList.remove("on"); });
      b.classList.add("on");
      $("#tab-" + b.dataset.tab).classList.add("on");
      if (b.dataset.tab === "logs") refreshLogs();
    };
  });

  $("#savePw").onclick = function(){
    // 管理密码只进 sessionStorage（关页即失效）：localStorage 里的值可被
    // 同源任意脚本长期读取，没必要为「记住密码」付这个代价。
    try { sessionStorage.setItem("deepaicode2api_pw", pw()); } catch {}
    try { localStorage.removeItem("deepaicode2api_pw"); } catch {}
    toast("已记住管理密码（仅本次会话）", "ok");
    refresh();
  };
  $("#refresh").onclick = function(){ refresh(); if ($("#tab-logs").classList.contains("on")) refreshLogs(); };
  $("#importLocal").onclick = function(){ accountOp("import", {}); };
  $("#addManual").onclick = function(){
    var key = $("#manualKey").value.trim();
    if (!key) { toast("请填写 API Key", "err"); return; }
    accountOp("manual", {api_key:key, name:$("#manualName").value.trim()}).then(function(){
      $("#manualKey").value = ""; $("#manualName").value = "";
    });
  };
  $("#checkinAll").onclick = function(){ checkin(null); };
  $("#refreshModels").onclick = function(){
    toast("正在从上游刷新模型目录…");
    api("/admin/api/models", {method:"POST", body:{}}).then(function(r){
      toast("模型目录已更新：共 " + r.count + " 个模型", "ok");
      return refresh();
    }).catch(function(e){ toast("刷新失败：" + e.message, "err"); });
  };
  $("#addKey").onclick = function(){
    api("/admin/api/keys", {method:"POST", body:{name:$("#keyName").value.trim(), group:$("#keyGroup").value.trim()}}).then(function(r){
      toast("已创建密钥（仅显示一次）：\n" + r.key, "ok");
      $("#keyName").value = "";
      return refresh();
    }).catch(function(e){ toast("创建失败：" + e.message, "err"); });
  };
  $("#reloadLogs").onclick = refreshLogs;
  $("#clearLogs").onclick = function(){
    if (!confirm("确定清空全部日志？")) return;
    api("/admin/api/logs", {method:"DELETE"}).then(function(){ toast("已清空","ok"); return refreshLogs(); })
      .catch(function(e){ toast("失败：" + e.message, "err"); });
  };
  $("#clearStats").onclick = function(){
    if (!confirm("确定重置统计？")) return;
    api("/admin/api/stats", {method:"DELETE"}).then(function(){ toast("已重置","ok"); return refresh(); })
      .catch(function(e){ toast("失败：" + e.message, "err"); });
  };
  $("#saveSettings").onclick = function(){
    var aliases;
    try { aliases = JSON.parse($("#sAliases").value || "{}"); }
    catch { toast("模型别名不是合法 JSON", "err"); return; }
    var body = {
      model_aliases: aliases,
      request_timeout_sec: parseInt($("#sTimeout").value, 10) || 0,
      max_retries: parseInt($("#sRetries").value, 10) || 0,
      log_keep: parseInt($("#sLogKeep").value, 10) || 0,
      cors_origin: $("#sCors").value.trim(),
      fallback_model: $("#sFallback").value.trim(),
      strict_model: $("#sStrict").checked
    };
    var np = $("#sPw").value;
    if (np !== "") body.admin_password = np;
    api("/admin/api/settings", {method:"POST", body:body}).then(function(r){
      $("#sPw").value = "";
      toast("设置已保存", "ok");
      renderSettings(r.settings || {});
      if (np !== "") { try { sessionStorage.setItem("deepaicode2api_pw", np); $("#pw").value = np; } catch {} }
      return refresh();
    }).catch(function(e){ toast("保存失败：" + e.message, "err"); });
  };
  $("#logLimit").onchange = refreshLogs;

  // --- 浏览器登录 ---
  var oauthTimer = null, oauthSid = null, oauthDeadline = 0;
  function stopOAuth(){
    if (oauthTimer) { clearInterval(oauthTimer); oauthTimer = null; }
    oauthSid = null;
  }
  function pollOAuth(){
    if (!oauthSid) return;
    if (Date.now() > oauthDeadline) {
      stopOAuth();
      $("#oauthStatus").textContent = "登录会话已过期，请重新发起。";
      return;
    }
    api("/admin/api/login/poll", {method:"POST", body:{session_id:oauthSid}}).then(function(r){
      if (r.status === "ok") {
        stopOAuth();
        $("#oauthPanel").style.display = "none";
        toast("登录成功：" + (r.username || "") + (r.is_pro ? "（Pro）" : ""), "ok");
        refresh();
      } else if (r.status === "error") {
        stopOAuth();
        $("#oauthStatus").textContent = "登录失败：" + (r.error || "未知错误");
      } else if (r.status === "expired") {
        stopOAuth();
        $("#oauthStatus").textContent = "登录会话已过期，请重新发起。";
      } else {
        $("#oauthStatus").textContent = "等待浏览器完成登录…";
      }
    }).catch(function(e){
      stopOAuth();
      $("#oauthStatus").textContent = "轮询失败：" + e.message;
    });
  }
  $("#oauthLogin").onclick = function(){
    api("/admin/api/login/start", {method:"POST", body:{}}).then(function(r){
      stopOAuth();
      oauthSid = r.session_id;
      oauthDeadline = Date.now() + (r.expires_in || 600) * 1000;
      $("#oauthId").value = r.session_id;
      $("#oauthCallback").value = "";
      // 登录链接是可复制的绝对地址：换浏览器 / 换机器打开都能完成授权。
      try {
        $("#oauthLink").value = new URL(r.open_url || ("/login/open?id=" + encodeURIComponent(r.session_id)), location.origin).href;
      } catch {
        $("#oauthLink").value = location.origin + (r.open_url || ("/login/open?id=" + encodeURIComponent(r.session_id)));
      }
      $("#oauthStatus").textContent = "等待浏览器完成登录…";
      $("#oauthPanel").style.display = "block";
      // 目标是一个固定的相对路径 + 随机会话 id，浏览器新标签页打开。
      $("#oauthForm").submit();
      oauthTimer = setInterval(pollOAuth, 2000);
    }).catch(function(e){ toast("发起登录失败：" + e.message, "err"); });
  };
  // 登录链接可复制：绝对地址，能拿到另一个浏览器 / 机器上去打开。
  // 非安全上下文（http + 非 localhost）没有 navigator.clipboard，退回选中 + execCommand。
  function copyText(text){
    if (navigator.clipboard && navigator.clipboard.writeText) return navigator.clipboard.writeText(text);
    return new Promise(function(resolve, reject){
      var ta = document.createElement("textarea");
      ta.value = text;
      ta.setAttribute("readonly", "");
      ta.style.position = "fixed";
      ta.style.top = "-1000px";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.select();
      var ok = false;
      try { ok = document.execCommand("copy"); } catch {}
      document.body.removeChild(ta);
      if (ok) resolve(); else reject(new Error("copy unsupported"));
    });
  }
  $("#oauthCopy").onclick = function(){
    var link = String(($("#oauthLink") || {}).value || "").trim();
    if (!link) { toast("先发起一次登录，再复制链接", "err"); return; }
    copyText(link).then(function(){
      toast("登录链接已复制", "ok");
    }).catch(function(){
      var el = $("#oauthLink");
      el.focus();
      el.select();
      toast("复制失败，已选中链接，请按 Ctrl+C", "err");
    });
  };
  $("#oauthCancel").onclick = function(){ stopOAuth(); $("#oauthPanel").style.display = "none"; };

  // 粘贴回调 URL：Docker / 远程部署时浏览器停在 127.0.0.1:<port> 打不开，
  // 地址栏里的 code/state 仍然有效，交给网关兑换并完成入池。
  $("#oauthPaste").onclick = function(){
    var raw = String(($("#oauthCallback") || {}).value || "").trim();
    if (!raw) { toast("先粘贴回调 URL", "err"); return; }
    if (!oauthSid) { toast("登录会话已过期，请重新发起。", "err"); return; }
    $("#oauthStatus").textContent = "正在用粘贴的回调兑换…";
    api("/admin/api/login/callback", {method:"POST", body:{session_id: oauthSid, callback: raw}}).then(function(r){
      if (r.status === "expired") {
        stopOAuth();
        $("#oauthStatus").textContent = "登录会话已过期，请重新发起。";
        return;
      }
      if (r.status !== "ok") {
        $("#oauthStatus").textContent = "登录失败：" + (r.error || "未知错误");
        return;
      }
      stopOAuth();
      $("#oauthPanel").style.display = "none";
      $("#oauthCallback").value = "";
      toast("登录成功：" + (r.username || "") + (r.is_pro ? "（Pro）" : ""), "ok");
      refresh();
    }).catch(function(e){ $("#oauthStatus").textContent = "兑换失败：" + e.message; });
  };

  try {
    var saved = sessionStorage.getItem("deepaicode2api_pw");
    if (saved) $("#pw").value = saved;
    // 清理旧版本遗留在 localStorage 里的密码。
    localStorage.removeItem("deepaicode2api_pw");
  } catch {}
  $("#pw").addEventListener("keydown", function(e){ if (e.key === "Enter") { $("#savePw").click(); } });

  // --- 主题（白天 / 黑夜）---
  // 优先用上次的选择；没选过就跟随系统 prefers-color-scheme。
  function applyTheme(mode){
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
  (function initTheme(){
    var mode = null;
    try { mode = localStorage.getItem("deepaicode2api_theme"); } catch {}
    if (mode !== "light" && mode !== "dark") {
      mode = (window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches) ? "light" : "dark";
    }
    applyTheme(mode);
  })();
  $("#theme").onclick = function(){
    var next = document.documentElement.getAttribute("data-theme") === "light" ? "dark" : "light";
    applyTheme(next);
    try { localStorage.setItem("deepaicode2api_theme", next); } catch {}
  };

  // --- 账号分页控件 ---
  $("#acctSize").value = String(ui.acctPageSize);
  $("#acctPrev").onclick = function(){ if (ui.acctPage > 0) { ui.acctPage--; loadAccounts(); } };
  $("#acctNext").onclick = function(){ ui.acctPage++; loadAccounts(); };
  $("#acctSize").onchange = function(){
    var v = parseInt(this.value, 10);
    if (v !== 20 && v !== 50 && v !== 100) return;
    ui.acctPageSize = v;
    ui.acctPage = 0;
    try { localStorage.setItem("deepaicode2api_acct_page_size", String(v)); } catch {}
    loadAccounts();
  };

  // --- 账号 / 日志筛选 ---
  function applyAcctFilters(){ ui.acctPage = 0; loadAccounts(); }
  $("#acctFilterApply").onclick = applyAcctFilters;
  $("#acctFilterClear").onclick = function(){
    $("#acctGroup").value = ""; $("#acctStatus").value = ""; $("#acctQuery").value = "";
    applyAcctFilters();
  };
  $("#acctStatus").onchange = applyAcctFilters;
  $("#acctGroup").onkeydown = $("#acctQuery").onkeydown = function(ev){
    if (ev.key === "Enter") applyAcctFilters();
  };
  $("#logFilterApply").onclick = refreshLogs;
  $("#logFilterClear").onclick = function(){
    $("#logEndpoint").value = ""; $("#logAccount").value = ""; $("#logMinStatus").value = "";
    $("#logKeyword").value = ""; $("#logSince").value = "";
    refreshLogs();
  };
  $("#logEndpoint").onchange = refreshLogs;
  $("#logKeyword").onkeydown = $("#logAccount").onkeydown = function(ev){
    if (ev.key === "Enter") refreshLogs();
  };

  // --- 表头排序 ---
  // accounts 走服务端（分页必须如此），其余三张小表在前端排。
  function wireSort(sel, name, onChange){
    document.querySelectorAll(sel + " th[data-sort]").forEach(function(th){
      th.addEventListener("click", function(){
        toggleSort(name, th.getAttribute("data-sort"));
        markSort(sel, sortState[name]);
        onChange();
      });
    });
  }
  wireSort("#acctTable", "accounts", function(){ ui.acctPage = 0; loadAccounts(); });
  wireSort("#modelTable", "models", function(){ if (state.overview) renderOverview(state.overview); });
  wireSort("#keyTable", "keys", function(){ if (state.overview) renderKeys(state.overview); });
  wireSort("#logTable", "logs", function(){ renderLogs(state.logs || []); });

  // --- 语言切换 ---
  $("#lang").onclick = function(){
    setLang(getLang() === "en" ? "zh" : "en");
    try { localStorage.setItem("deepaicode2api_lang", getLang()); } catch { /* ignore */ }
    applyLang();
    refresh();
    if ($("#tab-logs").classList.contains("on")) refreshLogs();
  };

  setReload(refresh);
  applyLang();
  refresh();
  setInterval(loadHealth, 20000);


// 移动端侧边栏抽屉。包成 IIFE 是因为原来那一段是独立 <script>，
// 内部用 `return` 做「元素不齐就整体放弃」的早退；ES module 顶层不允许 return。
(function initDrawer() {
  var toggle = document.getElementById("navToggle");
  var sidebar = document.getElementById("sidebar");
  var scrim = document.getElementById("scrim");
  if (!toggle || !sidebar || !scrim) return;
  function setOpen(open){
    sidebar.classList.toggle("open", open);
    scrim.classList.toggle("on", open);
  }
  toggle.addEventListener("click", function(){ setOpen(!sidebar.classList.contains("open")); });
  scrim.addEventListener("click", function(){ setOpen(false); });
  document.querySelectorAll("nav.nav button").forEach(function(b){
    b.addEventListener("click", function(){ setOpen(false); });
  });
  document.addEventListener("keydown", function(e){ if (e.key === "Escape") setOpen(false); });

  // 深链接：#accounts / #logs … 直接打开对应标签页。
  var want = (location.hash || "").replace(/^#/, "");
  if (want) {
    var target = document.querySelector('nav.nav button[data-tab="' + want + '"]');
    if (target) target.click();
  }
})();
