// 管理接口调用层：统一鉴权头、错误归一、同路径 GET 去重。
//
// 口令来源只有一个：本标签页的 sessionStorage（登录页写入）。控制台不再有
// 顶部口令输入框，因此这里也不再去读 DOM —— 401 一律清口令并回登录页。
//
// 结构：fetch 一次 → decodeResponse 归一 → api() 只负责去重 / 超时 / 重试。
// 首次请求与重试共用同一条归一链路，避免两处错误文案再次分叉。

import { getLang } from './i18n.js';

const PW_KEY = "deepaicode2api_pw";
const LOGIN_PATH = "/login";
const RETRY_DELAY_MS = 300;

const inflight = {};

// ERROR_EN 是英文界面下的错误分类文案。
//
// 后端的 message 只有中文（写死在 writeErr 里），而把 80 多条中文塞进前端字典
// 会被 i18n 的「死词条」棘轮当成无人使用的词条。这里改用 error.type 这个
// 稳定的机器字段做分类，原文保留在 err.detail（toast 的 title 可悬停查看）：
// 英文用户看到的是正确的错误类别 + HTTP 状态码，不会突然冒出一句中文。
const ERROR_EN = {
  setup_required: "Gateway has no API keys yet — create one in the console first",
  invalid_request_error: "Request rejected by the gateway",
  upstream_error: "Upstream request failed",
  server_error: "Internal gateway error",
  rate_limit_error: "Rate limited — please retry later",
  email_unverified: "All accounts need email verification upstream",
  social_login_required: "Upstream now requires Google/Apple sign-in"
};

// pw 返回当前标签页记住的口令（没有就是空串）。
export function pw() {
  try { return (sessionStorage.getItem(PW_KEY) || "").trim(); } catch { return ""; }
}

// setPw 记住口令（登录页 / 未来的“改口令”成功后调用）。
export function setPw(v) {
  try { sessionStorage.setItem(PW_KEY, String(v)); } catch { /* 隐私模式：只影响下次刷新 */ }
}

// clearPw 忘掉口令。
export function clearPw() {
  try { sessionStorage.removeItem(PW_KEY); } catch { /* 隐私模式忽略 */ }
}

// toLogin 回登录页（已经是登录页就不再跳，避免刷新循环）。
function toLogin() {
  if (location.pathname !== LOGIN_PATH) location.replace(LOGIN_PATH);
}

// apiTimeout 超时上限：GET 15s / 其它 90s。
//
// 没有超时时一个悬挂的 fetch 会让 UI 永久停在旧状态，而 401 也不会再来
// 触发回登录页。
function apiTimeout(method) {
  return method === "GET" ? 15000 : 90000;
}

// makeError 把一次非 2xx 响应归一成前端 Error（带 status / type / detail）。
//
// detail 永远保留后端原文（中文）：英文界面下它只进控制台，方便排查。
function makeError(status, payload, raw) {
  var detail = (payload && payload.error && payload.error.message) || raw || ("HTTP " + status);
  var type = (payload && payload.error && payload.error.type) || "";
  var msg = detail;
  if (getLang() === "en") {
    msg = (ERROR_EN[type] || "Request failed") + " (HTTP " + status + ")";
    if (msg !== detail) console.warn("[api]", detail);
  }
  var err = new Error(msg);
  err.status = status;
  err.detail = detail;
  err.type = type;
  return err;
}

// timeoutError 是 AbortError 的可读替身（保留 status=0 供重试判定）。
function timeoutError() {
  var e = new Error("请求超时（服务端未在限定时间内响应）");
  e.status = 0;
  return e;
}

// decodeResponse 解析响应体；非 2xx 抛归一后的错误。
//
// 401 在这里统一处理：清口令并回登录页，别让控制台一直空转。重试路径也走
// 这里，因此重试拿到 401 同样会跳登录页（旧实现只在首发路径处理）。
function decodeResponse(r) {
  return r.text().then(function (txt) {
    var j = null;
    try { j = JSON.parse(txt); } catch { /* 非 JSON 错误体：走原文兜底 */ }
    if (!r.ok) {
      if (r.status === 401) {
        clearPw();
        toLogin();
      }
      throw makeError(r.status, j, txt);
    }
    return j;
  });
}

// fetchJSON 发一次请求并归一结果（不含重试）。
function fetchJSON(path, method, headers, signal, body) {
  return fetch(path, {
    method: method,
    headers: headers,
    signal: signal,
    body: body
  }).then(decodeResponse).catch(function (e) {
    if (e && e.name === "AbortError") throw timeoutError();
    throw e;
  });
}

// isTransient 判断是否值得重试：网络错误（status=0）与 5xx。
//
// 401 不在此列 —— decodeResponse 已经清口令并跳登录页。
function isTransient(e) {
  return !e || e.status === 0 || e.status >= 500;
}

function sleep(ms) {
  return new Promise(function (resolve) { setTimeout(resolve, ms); });
}

export function api(path, opts) {
  opts = opts || {};
  var method = opts.method || "GET";
  // 去重键要带查询串：不同筛选条件的并发 GET 不能共用一个 promise。
  var dedupeKey = method + " " + path;
  if (method === "GET" && inflight[dedupeKey]) return inflight[dedupeKey];

  var headers = { "Content-Type": "application/json" };
  var secret = pw();
  if (secret) headers["X-Admin-Password"] = secret;
  var body = opts.body ? JSON.stringify(opts.body) : undefined;

  // 一次逻辑请求只有一个 AbortController：重试复用同一个中止点。
  var ctrl = typeof AbortController === "function" ? new AbortController() : null;
  var signal = ctrl ? ctrl.signal : undefined;
  var timer = null;

  function arm() {
    if (!ctrl) return;
    if (timer) clearTimeout(timer);
    timer = setTimeout(function () { ctrl.abort(); }, apiTimeout(method));
  }
  function disarm() {
    if (timer) { clearTimeout(timer); timer = null; }
  }

  arm();
  var p = fetchJSON(path, method, headers, signal, body).then(function (v) {
    disarm();
    return v;
  }, function (e) {
    disarm();
    // GET 幂等：网络错误 / 5xx 自动重试一次（300ms 退避）。
    // POST 不重试：创建密钥、清空日志这类操作重放会出问题。
    if (method !== "GET" || !isTransient(e)) throw e;
    return sleep(RETRY_DELAY_MS).then(function () {
      arm();
      // 重试不带 body：只有 GET 会走到这里。
      return fetchJSON(path, method, headers, signal).then(function (v) {
        disarm();
        return v;
      }, function (e2) {
        disarm();
        throw e2;
      });
    });
  });

  if (method === "GET") {
    inflight[dedupeKey] = p;
    var done = function () { delete inflight[dedupeKey]; };
    p.then(done, done);
  }
  return p;
}
