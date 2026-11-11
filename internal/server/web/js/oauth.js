// 浏览器登录面板（Google / Apple / 邮箱密码 → 回调 URL 入池）。
//
// 从 app.js 拆出来的第三块。这块代码自带定时轮询与超时状态机，和「刷新数据」
// 只是弱耦合（登录成功后调一次 refresh），因此用 initOAuth({refresh}) 注入，
// 避免 oauth.js ⇄ app.js 循环导入。
import { $ } from './dom.js';
import { toast } from './ui.js';
import { api } from './api.js';

var timer = null, sid = null, deadline = 0;

function stop() {
  if (timer) { clearInterval(timer); timer = null; }
  sid = null;
}

function poll(refresh) {
  if (!sid) return;
  if (Date.now() > deadline) {
    stop();
    $("#oauthStatus").textContent = "登录会话已过期，请重新发起。";
    return;
  }
  api("/admin/api/login/poll", { method: "POST", body: { session_id: sid } }).then(function (r) {
    if (r.status === "ok") {
      stop();
      $("#oauthPanel").style.display = "none";
      toast("登录成功：" + (r.username || "") + (r.is_pro ? "（Pro）" : ""), "ok");
      refresh();
    } else if (r.status === "error") {
      stop();
      $("#oauthStatus").textContent = "登录失败：" + (r.error || "未知错误");
    } else if (r.status === "expired") {
      stop();
      $("#oauthStatus").textContent = "登录会话已过期，请重新发起。";
    } else {
      $("#oauthStatus").textContent = "等待浏览器完成登录…";
    }
  }).catch(function (e) {
    stop();
    $("#oauthStatus").textContent = "轮询失败：" + e.message;
  });
}

// copyText 剪贴板兜底：非安全上下文（http + 非 localhost）没有
// navigator.clipboard，退回「选中 + execCommand」。
function copyText(text) {
  if (navigator.clipboard && navigator.clipboard.writeText) return navigator.clipboard.writeText(text);
  return new Promise(function (resolve, reject) {
    var ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.top = "-1000px";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    var ok = false;
    try { ok = document.execCommand("copy"); } catch { /* 老浏览器直接走失败分支 */ }
    document.body.removeChild(ta);
    if (ok) resolve(); else reject(new Error("copy unsupported"));
  });
}

// initOAuth 绑定登录面板上的四个按钮。
export function initOAuth(opts) {
  var refresh = (opts && opts.refresh) || function () {};

  $("#oauthLogin").onclick = function () {
    api("/admin/api/login/start", { method: "POST", body: {} }).then(function (r) {
      stop();
      sid = r.session_id;
      deadline = Date.now() + (r.expires_in || 600) * 1000;
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
      timer = setInterval(function () { poll(refresh); }, 2000);
    }).catch(function (e) { toast("发起登录失败：" + e.message, "err"); });
  };

  $("#oauthCopy").onclick = function () {
    var link = String(($("#oauthLink") || {}).value || "").trim();
    if (!link) { toast("先发起一次登录，再复制链接", "err"); return; }
    copyText(link).then(function () {
      toast("登录链接已复制", "ok");
    }).catch(function () {
      var el = $("#oauthLink");
      el.focus();
      el.select();
      toast("复制失败，已选中链接，请按 Ctrl+C", "err");
    });
  };

  $("#oauthCancel").onclick = function () { stop(); $("#oauthPanel").style.display = "none"; };

  // 粘贴回调 URL：Docker / 远程部署时浏览器停在 127.0.0.1:<port> 打不开，
  // 地址栏里的 code/state 仍然有效，交给网关兑换并完成入池。
  $("#oauthPaste").onclick = function () {
    var raw = String(($("#oauthCallback") || {}).value || "").trim();
    if (!raw) { toast("先粘贴回调 URL", "err"); return; }
    if (!sid) { toast("登录会话已过期，请重新发起。", "err"); return; }
    $("#oauthStatus").textContent = "正在用粘贴的回调兑换…";
    api("/admin/api/login/callback", { method: "POST", body: { session_id: sid, callback: raw } }).then(function (r) {
      if (r.status === "expired") {
        stop();
        $("#oauthStatus").textContent = "登录会话已过期，请重新发起。";
        return;
      }
      if (r.status !== "ok") {
        $("#oauthStatus").textContent = "登录失败：" + (r.error || "未知错误");
        return;
      }
      stop();
      $("#oauthPanel").style.display = "none";
      $("#oauthCallback").value = "";
      toast("登录成功：" + (r.username || "") + (r.is_pro ? "（Pro）" : ""), "ok");
      refresh();
    }).catch(function (e) { $("#oauthStatus").textContent = "兑换失败：" + e.message; });
  };
}
