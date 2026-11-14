// 顶栏版本号与「检查更新」。
//
// 版本号来自 /health（公开端点，登录页也能拿到），更新比对走
// /admin/api/version（服务端代查 GitHub 并缓存 6 小时）——前端 CSP 是
// connect-src 'self'，本来就不允许直连 api.github.com。
import { $ } from './dom.js';
import { api } from './api.js';
import { t } from './i18n.js';

// renderVersion 只改文案，不触发网络请求。
//
// 版本号本身可能已经带 v 前缀（git describe --tags / docker 构建参数都是
// `v0.1.4+efa5d24`），再无条件拼一个 v 就变成线上的 `vv0.1.4+efa5d24`。
// 前缀归 JS 管：服务端只回它真实知道的构建版本，不替展示层做格式化。
export function renderVersion(v) {
  var el = $("#version");
  if (!el) return;
  var s = String(v === undefined || v === null || v === "" ? "dev" : v).trim();
  el.textContent = /^v/i.test(s) ? s : "v" + s;
}

// checkUpdate 查一次更新；有新版则把版本号变成可点的高亮徽章。
//
// 跳转目标**写死**成仓库的 releases 页，不转发服务端给的 html_url：
// 那属于外部输入，动态拼进 window.open 就是一个 open redirect 面。
// 最新 release 本来就置顶在 releases 页，少一次跳转精度、换掉一类风险。
//
// 失败静默：离线 / 仓库不可达 / 未登录管理口令都不该在控制台弹错误，
// 版本号本身仍然可读。
export function checkUpdate() {
  var el = $("#version");
  if (!el) return;
  api("/admin/api/version").then(function (r) {
    if (r.update_available) {
      el.classList.add("update");
      // 跳转目标写在标记里的静态 <a href>：JS 只负责改文案与高亮，
      // 不把任何外部输入拼进导航目标（GitHub 返回的 html_url 属于外部输入）。
      el.title = t("发现新版本 {v}，点击查看 Release", { v: r.latest });
      return;
    }
    el.title = r.error ? t("无法检查更新") : t("已是最新版本");
  }).catch(function () {
    el.title = t("无法检查更新");
  });
}
