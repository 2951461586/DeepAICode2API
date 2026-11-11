// 登录页脚本：验一次管理口令，成功就把口令留在本标签页并进控制台。
//
// 与控制台共用同一个 sessionStorage 键（deepaicode2api_pw），所以 api.js
// 不需要知道登录页存在 —— 它只读这个键。页面文案全部来自 <body data-msg-*>，
// 本文件不含中文，避免和 i18n 字典棘轮纠缠。

const KEY = "deepaicode2api_pw";

const form = document.getElementById("loginForm");
const input = document.getElementById("loginPw");
const errBox = document.getElementById("loginErr");
const goBtn = document.getElementById("loginGo");
const msg = document.body.dataset;

function showError(text) {
  errBox.textContent = text;
  errBox.hidden = false;
  input.focus();
  input.select();
}

function clearError() {
  errBox.hidden = true;
  errBox.textContent = "";
}

// 主题跟着控制台走：同一份 localStorage，没选过就跟随系统。
(function initTheme() {
  let mode = null;
  try { mode = localStorage.getItem("deepaicode2api_theme"); } catch { /* 隐私模式忽略 */ }
  if (mode !== "light" && mode !== "dark") {
    mode = (window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches) ? "light" : "dark";
  }
  document.documentElement.setAttribute("data-theme", mode);
})();

// verify 用一次 /admin/api/auth 校验口令（该接口走管理鉴权，本身不返回敏感信息）。
async function verify(password) {
  const res = await fetch("/admin/api/auth", {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Admin-Password": password },
    body: "{}",
  });
  return res.ok;
}

async function submit(ev) {
  ev.preventDefault();
  const password = input.value;
  if (!password.trim()) {
    showError(msg.msgEmpty || "required");
    return;
  }
  clearError();
  goBtn.disabled = true;
  let ok = false;
  try {
    ok = await verify(password);
  } catch {
    goBtn.disabled = false;
    showError(msg.msgNet || "network");
    return;
  }
  goBtn.disabled = false;
  if (!ok) {
    showError(msg.msgBad || "invalid");
    return;
  }
  try { sessionStorage.setItem(KEY, password); } catch { /* 隐私模式：本次仍可用，只是刷新后要重登 */ }
  location.replace("/");
}

// 已经登录过（同标签页内刷新）：先验一次，省得把错误口令一直带着。
(function resume() {
  let saved = "";
  try { saved = sessionStorage.getItem(KEY) || ""; } catch { /* 隐私模式忽略 */ }
  if (!saved) return;
  verify(saved).then(function (ok) {
    if (ok) location.replace("/");
    else { try { sessionStorage.removeItem(KEY); } catch { /* 同上 */ } }
  }).catch(function () { /* 网关没起来：就留在登录页 */ });
})();

form.addEventListener("submit", submit);
input.addEventListener("input", clearError);
