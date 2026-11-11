// 管理接口调用层：统一鉴权头、错误归一、同路径 GET 去重。
//
// 口令来源只有一个：本标签页的 sessionStorage（登录页写入）。控制台不再有
// 顶部口令输入框，因此这里也不再去读 DOM —— 401 一律清口令并回登录页。

import { getLang } from './i18n.js';

const PW_KEY = "deepaicode2api_pw";
const LOGIN_PATH = "/login";

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

// api(path, opts) 调用管理接口。
//
// 超过 timeoutMs 则中止请求（GET 15s / 其它 90s）：没有超时时一个悬挂的
// fetch 会让 UI 永久停在旧状态，而 401 也不会再来触发回登录页。
function apiTimeout(method) {
  return method === "GET" ? 15000 : 90000;
}

export function api(path, opts) {
  opts = opts || {};
  var method = opts.method || "GET";
  // 去重键要带查询串：不同筛选条件的并发 GET 不能共用一个 promise。
  var dedupeKey = method + " " + path;
  if (method === "GET" && inflight[dedupeKey]) return inflight[dedupeKey];

  var h = { "Content-Type": "application/json" };
  var secret = pw();
  if (secret) h["X-Admin-Password"] = secret;

  var ctrl = typeof AbortController === "function" ? new AbortController() : null;
  var timer = ctrl ? setTimeout(function () { ctrl.abort(); }, apiTimeout(method)) : null;

  var p = fetch(path, {
    method: method,
    headers: h,
    signal: ctrl ? ctrl.signal : undefined,
    body: opts.body ? JSON.stringify(opts.body) : undefined
  }).then(function (r) {
    return r.text().then(function (txt) {
      var j = null;
      try { j = JSON.parse(txt); } catch { /* 非 JSON 错误体：下面用原文兜底 */ }
      if (!r.ok) {
        // 口令错 / 会话里没有口令：清掉并回登录页，别让控制台一直空转 401。
        if (r.status === 401) {
          clearPw();
          toLogin();
        }
        var detail = (j && j.error && j.error.message) || txt || ("HTTP " + r.status);
        var type = (j && j.error && j.error.type) || "";
        var msg = detail;
        if (getLang() === "en") {
          msg = (ERROR_EN[type] || "Request failed") + " (HTTP " + r.status + ")";
        }
        var err = new Error(msg);
        err.status = r.status;
        err.detail = detail;
        err.type = type;
        // 英文界面下原文只进控制台：排查时仍能拿到后端的确切中文原因。
        if (msg !== detail) console.warn("[api]", detail);
        throw err;
      }
      return j;
    });
  }).catch(function (e) {
    // AbortError 的 message 是英文且对用户无意义，换成可读文案并保留 status=0。
    if (e && e.name === "AbortError") {
      var te = new Error("请求超时（服务端未在限定时间内响应）");
      te.status = 0;
      throw te;
    }
    throw e;
  }).finally(function () {
    if (timer) clearTimeout(timer);
  });

  if (method === "GET") {
    inflight[dedupeKey] = p;
    var done = function () { delete inflight[dedupeKey]; };
    p.then(done, done);
  }
  return p;
}
