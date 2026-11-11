// 管理接口调用层：统一鉴权头、错误归一、同路径 GET 去重。
//
// 口令来源只有一个：本标签页的 sessionStorage（登录页写入）。控制台不再有
// 顶部口令输入框，因此这里也不再去读 DOM —— 401 一律清口令并回登录页。

const PW_KEY = "deepaicode2api_pw";
const LOGIN_PATH = "/login";

const inflight = {};

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

export function api(path, opts) {
  opts = opts || {};
  var method = opts.method || "GET";
  if (method === "GET" && inflight[path]) return inflight[path];

  var h = { "Content-Type": "application/json" };
  var secret = pw();
  if (secret) h["X-Admin-Password"] = secret;
  var p = fetch(path, {
    method: method,
    headers: h,
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
        var m = (j && j.error && j.error.message) || txt || ("HTTP " + r.status);
        var err = new Error(m); err.status = r.status; throw err;
      }
      return j;
    });
  });

  if (method === "GET") {
    inflight[path] = p;
    var done = function () { delete inflight[path]; };
    p.then(done, done);
  }
  return p;
}
