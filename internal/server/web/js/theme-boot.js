// 主题启动脚本（经典脚本，非 module）。
//
// 为什么单独一个文件：主题必须在首次绘制之前落到 <html data-theme>，否则浅色
// 用户每次刷新都会先闪一下深色（FOUC）。内联脚本能做到最早，但 CSP 已禁止
// 内联脚本；放在 head 里的同步外部脚本同样在渲染前执行，且仍属 'self'。
//
// 失败一律静默：这段脚本只影响首帧颜色，任何异常都不该影响控制台可用性。
(function () {
  try {
    var mode = localStorage.getItem("deepaicode2api_theme");
    if (mode !== "light" && mode !== "dark") {
      mode = (window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches) ? "light" : "dark";
    }
    document.documentElement.setAttribute("data-theme", mode);
  } catch { /* 隐私模式下 localStorage 不可用：保留 HTML 上的默认值 */ }
})();
