// 设置页表单回填。

import { $ } from '../dom.js';

export function renderSettings(s){
  // 数字字段必须区分「未设置」（undefined/null）与「显式 0」：
  // 0 是合法值（限流关闭），用 `|| ""` 会把它显示成空框，看上去像没设置过。
  var num = function(v){ return (v === undefined || v === null) ? "" : String(v); };
  $("#sTimeout").value = num(s.request_timeout_sec);
  $("#sRetries").value = num(s.max_retries);
  $("#sLogKeep").value = num(s.log_keep);
  $("#sCors").value = s.cors_origin || "";
  $("#sFallback").value = s.fallback_model || "";
  $("#sStrict").checked = !!s.strict_model;
  $("#sRps").value = num(s.rate_limit_rps);
  $("#sBurst").value = num(s.rate_limit_burst);
  $("#sConcurrency").value = num(s.key_concurrency);
  $("#sCheckinBlock").value = num(s.checkin_block_ms);
  $("#sPresetShape").checked = !!s.preset_desktop_shape;
  $("#sAliases").value = JSON.stringify(s.model_aliases || {}, null, 2);
  $("#sPw").placeholder = s.admin_password_set ? "已设置，留空则不修改" : "未设置";
}
