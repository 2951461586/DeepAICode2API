// 概览页渲染：KPI 卡片、模型目录表、上游健康卡。
//
// 从 app.js 拆出来的第二块（第一块是 render/*）。拆分的判据是「只管画，
// 不管取数」：这里不碰 api，也不决定什么时候刷新，调用方把数据传进来即可
// —— 于是 console-smoke.mjs 能直接喂假数据跑一遍，不必起网关。
import { t, translateDom } from './i18n.js';
import { fmtTime, fmtNum, fmtCost, fmtCompact, perSecondText } from './format.js';
import { $, mk, cell, row, emptyRow } from './dom.js';
import { sortState, sortCopy } from './table.js';

// setHealth 画顶部那颗状态胶囊（就绪账号数 / 上游是否可达）。
function setHealth(cls, text) {
  var el = $("#health");
  el.className = "badge " + cls;
  el.textContent = "";
  el.appendChild(mk("i", "dot"));
  el.appendChild(mk("span", null, text));
}

// loadHealth 拉 /health 并刷新状态胶囊与机器指纹行。
//
// 这是唯一保留在本模块里的请求：它是纯展示数据，失败也只是把胶囊画成
// 「不可达」，不参与任何业务状态。
export function loadHealth() {
  fetch("/health").then(function (r) { return r.json(); }).then(function (h) {
    var ready = h.ready || 0, total = h.accounts || 0;
    var healthCls = "";
    if (total === 0) healthCls = "";
    else if (ready > 0) healthCls = "ok";
    else healthCls = "warn";
    setHealth(healthCls, t("账号 {a}/{b} 就绪 · 模型 {c}", { a: ready, b: total, c: h.models || 0 }));
    $("#upstream").textContent = t("机器指纹 {m} · 默认 {d}",
      { m: h.machine_id || t("不可用"), d: h.default_model || "-" });
  }).catch(function () { setHealth("err", t("不可达")); });
}

// renderOverview 画 KPI + 模型目录表。
export function renderOverview(d) {
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
    ["密钥", (d.keys || []).length],
    ["统计自", fmtTime(s.reset_at || s.started_at)]
  ];
  var box = $("#kpis");
  box.textContent = "";
  kpis.forEach(function (k) {
    var c = mk("div", "kpi");
    c.appendChild(mk("div", "k", k[0]));
    c.appendChild(mk("div", "v", k[1]));
    box.appendChild(c);
  });

  $("#modelsMeta").textContent = t("更新于 {t} · 默认模型 {m}",
    { t: fmtTime(d.models_updated), m: d.default_model || "-" });
  var tb = $("#models");
  tb.textContent = "";
  var models = sortCopy(d.models || [], sortState.models, {
    id: function (m) { return m.id || ""; },
    name: function (m) { return m.name || m.id || ""; },
    billing: function (m) { return m.premier ? 1 : 0; },
    theos: function (m) { return Number(m.theosPerRequest || 0); },
    theos2: function (m) { return perSecondText(m); },
    context: function (m) { return Number(m.contextWindow || 0); },
    max: function (m) { return Number(m.maxTokens || 0); },
    api: function (m) { return m.api || "chat"; },
    input: function (m) { return (m.input || []).join(",") + (m.reasoning ? ",r" : ""); }
  });
  if (!models.length) { tb.appendChild(emptyRow(9, "无")); return; }
  models.forEach(function (m) {
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
