// 设置表单的保存动作。
//
// 从 app.js 拆出来的第四块：只处理「读表单 → POST /admin/api/settings →
// 回填 + 让调用方刷新」。读取与渲染仍由 render/settings.js 负责，这里不重复。
import { $ } from './dom.js';
import { toast } from './ui.js';
import { api, setPw } from './api.js';
import { renderSettings } from './render/settings.js';

// initSettingsForm 绑定「保存设置」按钮。
//
// 保存成功后要做三件事：清空密码框、按服务端回显重画表单、让调用方刷新
// 概览（管理密码 / 兜底模型可能影响别的面板）。
export function initSettingsForm(opts) {
  var refresh = (opts && opts.refresh) || function () {};

  $("#saveSettings").onclick = function () {
    var aliases;
    try { aliases = JSON.parse($("#sAliases").value || "{}"); }
    catch { toast("模型别名不是合法 JSON", "err"); return; }
    var body = {
      model_aliases: aliases,
      request_timeout_sec: parseInt($("#sTimeout").value, 10) || 0,
      max_retries: parseInt($("#sRetries").value, 10) || 0,
      log_keep: parseInt($("#sLogKeep").value, 10) || 0,
      cors_origin: $("#sCors").value.trim(),
      fallback_model: $("#sFallback").value.trim(),
      strict_model: $("#sStrict").checked,
      // 限流：0 / 空都表示关闭（后端按“显式 0”处理，不会回落到默认值）。
      rate_limit_rps: parseFloat($("#sRps").value) || 0,
      rate_limit_burst: parseInt($("#sBurst").value, 10) || 0,
      key_concurrency: parseInt($("#sConcurrency").value, 10) || 0,
      // 0 = 用后端默认（10s），是合法值。
      checkin_block_ms: parseInt($("#sCheckinBlock").value, 10) || 0,
      preset_desktop_shape: $("#sPresetShape").checked
    };
    var np = $("#sPw").value;
    if (np !== "") body.admin_password = np;
    api("/admin/api/settings", { method: "POST", body: body }).then(function (r) {
      $("#sPw").value = "";
      toast("设置已保存", "ok");
      renderSettings(r.settings || {});
      // 改了管理口令就得同步本标签页记住的那份，否则下一次请求立刻 401。
      if (np !== "") setPw(np);
      return refresh();
    }).catch(function (e) { toast("保存失败：" + e.message, "err"); });
  };
}
