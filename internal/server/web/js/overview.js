// 概览页渲染：KPI 卡片、模型目录表、上游健康卡。
//
// 从 app.js 拆出来的第二块（第一块是 render/*）。拆分的判据是「只管画，
// 不管取数」：这里不碰 api，也不决定什么时候刷新，调用方把数据传进来即可
// —— 于是 console-smoke.mjs 能直接喂假数据跑一遍，不必起网关。
import { t, translateDom } from './i18n.js';
import { fmtTime, fmtNum, fmtCost, fmtCompact, fmtPercent, perSecondText } from './format.js';
import { $, mk, cell, row, emptyRow } from './dom.js';
import { sortState, sortCopy } from './table.js';
import { renderVersion } from './version.js';

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
    var degraded = h.degraded || 0;
    var healthCls = "";
    if (total === 0) healthCls = "";
    else if (ready > 0) healthCls = degraded > 0 ? "warn" : "ok";
    else healthCls = "warn";
    // 只报 ready/total 会把“刚被 429 打了一下”和“真坏了”混成一团，
    // 所以后端另下 degraded（错误还新鲜的账号数），这里显式带出来。
    var text = degraded > 0
      ? t("账号 {a}/{b} 就绪 · {d} 个近期失败 · 模型 {c}", { a: ready, b: total, d: degraded, c: h.models || 0 })
      : t("账号 {a}/{b} 就绪 · 模型 {c}", { a: ready, b: total, c: h.models || 0 });
    setHealth(healthCls, text);
    // 侧边栏收起后只剩一颗小圆点，完整文案放 tooltip 里，不至于丢信息。
    $("#health").title = text;
    renderVersion(h.version);
    $("#upstream").textContent = t("机器指纹 {m} · 默认 {d}",
      { m: h.machine_id || t("不可用"), d: h.default_model || "-" });
  }).catch(function () { setHealth("err", t("不可达")); });
}

// renderOverview 画 KPI + 模型目录表（两块各自成函数：合在一起会超过本仓的
// 复杂度门槛 25，而它们之间本来就没有共享状态）。
export function renderOverview(d) {
  renderKpis(d);
  renderRuntime(d);
  renderModels(d);
  translateDom($("#tab-overview"));
}

// fmtMs 把毫秒分位收敛成整数展示（0 表示还没样本）。
function fmtMs(v) {
  return v > 0 ? String(Math.round(v)) : "—";
}

// runtimeItem 画一张运行健康小卡。
function runtimeItem(label, value, sub, cls) {
  var box = mk("div", "runtime-item" + (cls ? " " + cls : ""));
  box.appendChild(mk("div", "k", label));
  box.appendChild(mk("div", "v", value));
  if (sub) box.appendChild(mk("div", "s", sub));
  return box;
}

// renderRuntime 画「运行健康」：延迟分位、签到、形状兜底与上游错误 Top。
//
// 数据来自 /admin/api/overview 的 latency / checkin / fallback_total /
// upstream_errors（进程内直方图，重启清零）。缺字段时整卡退化为占位，
// 不让旧后端版本把概览页打空。
export function renderRuntime(d) {
  var box = $("#runtimeBody");
  if (!box) return;
  box.textContent = "";
  var lat = d.latency || {};
  var ci = d.checkin || {};
  var errs = d.upstream_errors || [];
  var top = errs.length ? errs[0] : null;
  var checkinCls = (ci.timeouts || 0) > 0 ? "warn" : "";
  var errCls = top && top.status >= 500 ? "err" : "";

  box.appendChild(runtimeItem(
    t("请求延迟 P50 / P95"),
    fmtMs(lat.p50_ms) + " / " + fmtMs(lat.p95_ms) + " ms",
    t("P99 {v} ms", { v: fmtMs(lat.p99_ms) })));

  box.appendChild(runtimeItem(
    t("签到 P95"),
    fmtMs(ci.p95_ms) + " ms",
    t("尝试 {a} · 阻塞 {b} · 超时 {x}", {
      a: fmtNum(ci.attempts), b: fmtNum(ci.blocked), x: fmtNum(ci.timeouts)
    }), checkinCls));

  box.appendChild(runtimeItem(
    t("形状兜底"),
    fmtNum(d.fallback_total || 0),
    t("次「补 system」重试")));

  box.appendChild(runtimeItem(
    t("上游错误 Top"),
    top ? top.code : t("无"),
    top ? (top.path + " ×" + fmtNum(top.count)) : t("暂无样本"), errCls));

  var meta = $("#runtimeMeta");
  if (meta) meta.textContent = t("进程内指标，重启清零");
}

// hitRate 返回 0~1 的命中率；没有样本时返回 NaN（交给 fmtPercent 显示 —），
// 避免把「今天还没有请求」画成 0%（看起来像缓存完全没生效）。
function hitRate(cached, total) {
  return total > 0 ? cached / total : NaN;
}

// renderKpis 画 KPI 卡片。
//
// 每项是 [标签, 值, 悬停说明?]：标题栏里只放一个数，口径（累计 / 今日）放
// tooltip。缓存命中率以前算的是「累计缓存 / 累计入」，既看着像今天的、又永远
// 不随当天变化；现在主值取今日，累计值退到 tooltip。
function renderKpis(d) {
  var s = d.stats || {};
  var kpis = [
    ["总请求", fmtNum(s.total_requests)],
    ["失败请求", fmtNum(s.failed_requests)],
    ["输出字符", fmtCompact(s.total_chars_out)],
    ["总 Token 入 / 出", fmtCompact(s.total_tokens_in) + " / " + fmtCompact(s.total_tokens_out)],
    ["上游成本", fmtCost(s.total_cost)],
    ["账号（就绪/总）", (d.ready || 0) + " / " + (d.accounts || 0)],
    ["今日 Token 入 / 出", fmtCompact(s.day_tokens_in) + " / " + fmtCompact(s.day_tokens_out)],
    ["缓存命中率（今日）", fmtPercent(hitRate(s.day_cached_tokens, s.day_tokens_in), s.day_tokens_in > 0),
      t("累计 {r} · 命中 {c} / 入 {i}", {
        r: fmtPercent(hitRate(s.total_cached_tokens, s.total_tokens_in), s.total_tokens_in > 0),
        c: fmtCompact(s.total_cached_tokens),
        i: fmtCompact(s.total_tokens_in)
      })]
  ];
  var box = $("#kpis");
  box.textContent = "";
  kpis.forEach(function (k) {
    var c = mk("div", "kpi");
    c.appendChild(mk("div", "k", k[0]));
    c.appendChild(mk("div", "v", k[1]));
    if (k[2]) c.title = k[2];
    box.appendChild(c);
  });
}

// modelSortGetters 是模型表的排序取值（键必须与表头 data-sort 一致）。
var modelSortGetters = {
  id: function (m) { return m.id || ""; },
  name: function (m) { return m.name || m.id || ""; },
  billing: function (m) { return m.premier ? 1 : 0; },
  theos: function (m) { return Number(m.theosPerRequest || 0); },
  theos2: function (m) { return perSecondText(m); },
  context: function (m) { return Number(m.contextWindow || 0); },
  max: function (m) { return Number(m.maxTokens || 0); },
  api: function (m) { return m.api || "chat"; },
  input: function (m) { return (m.input || []).join(",") + (m.reasoning ? ",r" : ""); }
};

// modelRow 画一行模型（字段是 camelCase：theosPerRequest / contextWindow / …）。
function modelRow(m) {
  var price = (m.theosPerRequest === undefined || m.theosPerRequest === null) ? "—" : m.theosPerRequest;
  // free 由后端 Model.Free() 下发（单一口径）；旧响应没有这个字段时才回退本地判断。
  var free = (m.free === undefined)
    ? (!m.premier && (price === "—" || String(price) === "0"))
    : Boolean(m.free);
  var input = (m.input || []).join(" + ");
  if (m.reasoning) input = input ? input + " · " + t("推理") : t("推理");
  return row([
    cell(m.id, "mono"),
    cell(m.name || m.id),
    cell(free ? "free" : "premier", free ? "muted" : ""),
    cell(price, "mono"),
    cell(perSecondText(m), "mono muted"),
    cell(fmtNum(m.contextWindow), "mono"),
    cell(fmtNum(m.maxTokens), "mono"),
    cell(m.api || "chat", "mono muted"),
    cell(input, "muted")
  ]);
}

// renderModels 画模型目录（元信息 + 表格）。
function renderModels(d) {
  $("#modelsMeta").textContent = t("更新于 {t} · 默认模型 {m}",
    { t: fmtTime(d.models_updated), m: d.default_model || "-" });
  var tb = $("#models");
  tb.textContent = "";
  var models = sortCopy(d.models || [], sortState.models, modelSortGetters);
  if (!models.length) { tb.appendChild(emptyRow(9, "无")); return; }
  models.forEach(function (m) { tb.appendChild(modelRow(m)); });
}
