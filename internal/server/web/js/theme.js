// 主题与布局：白天 / 黑夜、侧边栏收起、移动端抽屉。
//
// 这里全是纯 DOM 副作用（localStorage + class 切换），不碰取数层，所以能独立成
// 模块：app.js 只负责「按顺序装配」。首帧前的 data-theme 由 theme-boot.js 写入，
// 本模块只管运行期切换，两者共用同一组 key 与选择器约定。

import { $ } from './dom.js';
import { translateDom } from './i18n.js';

const THEME_KEY = "deepaicode2api_theme";
const SIDE_KEY = "deepaicode2api_sidebar";

// applyTheme 切换主题并同步图标 / 文案。
export function applyTheme(mode) {
  document.documentElement.setAttribute("data-theme", mode);
  var moon = $("#themeIconMoon"), sun = $("#themeIconSun"), label = $("#themeLabel");
  // 用 class 而不是 style.display：HTML 里已改成 .hidden 工具类（CSP 禁止内联样式）。
  if (moon) moon.classList.toggle("hidden", mode === "light");
  if (sun) sun.classList.toggle("hidden", mode !== "light");
  if (label) {
    // 文案变了就得让 translateDom 重记中文原文，否则切回英文时会复现上一次的标签。
    label.__zh = undefined;
    label.textContent = mode === "light" ? "浅色" : "深色";
    translateDom(label.parentNode);
  }
}

// initTheme 绑定主题按钮：优先用上次的选择，没选过就跟随系统。
export function initTheme() {
  var mode = null;
  try { mode = localStorage.getItem(THEME_KEY); } catch { /* 隐私模式忽略 */ }
  if (mode !== "light" && mode !== "dark") {
    mode = (window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches) ? "light" : "dark";
  }
  applyTheme(mode);

  $("#theme").onclick = function () {
    var next = document.documentElement.getAttribute("data-theme") === "light" ? "dark" : "light";
    applyTheme(next);
    try { localStorage.setItem(THEME_KEY, next); } catch { /* 隐私模式忽略 */ }
  };
}

// initSidebar 绑定桌面端侧边栏收起 / 展开。
//
// 收起态只改 body 上的一个 class：CSS 里把 --sidebar 换成 --sidebar-min，
// 侧边栏宽度与 .main 的左边距就一起跟着变（220ms 过渡），JS 不需要量尺寸。
export function initSidebar() {
  function applySide(collapsed) {
    document.body.classList.toggle("side-collapsed", !!collapsed);
    var b = $("#sideToggle");
    if (b) b.setAttribute("aria-expanded", collapsed ? "false" : "true");
  }

  $("#sideToggle").onclick = function () {
    var next = !document.body.classList.contains("side-collapsed");
    applySide(next);
    try { localStorage.setItem(SIDE_KEY, next ? "collapsed" : "expanded"); } catch { /* 隐私模式忽略 */ }
  };

  var saved = null;
  try { saved = localStorage.getItem(SIDE_KEY); } catch { /* 隐私模式忽略 */ }
  if (saved === "collapsed") applySide(true);
}

// initDrawer 绑定移动端侧边栏抽屉。
export function initDrawer() {
  var toggle = $("#navToggle");
  var sidebar = $("#sidebar");
  var scrim = $("#scrim");
  if (!toggle || !sidebar || !scrim) return;

  function setOpen(open) {
    sidebar.classList.toggle("open", open);
    scrim.classList.toggle("on", open);
  }
  toggle.addEventListener("click", function () { setOpen(!sidebar.classList.contains("open")); });
  scrim.addEventListener("click", function () { setOpen(false); });
  document.querySelectorAll("nav.nav button").forEach(function (b) {
    b.addEventListener("click", function () { setOpen(false); });
  });
  document.addEventListener("keydown", function (e) { if (e.key === "Escape") setOpen(false); });
  // 深链接（#accounts?…）由 app.js 的 start() 统一处理。
}
