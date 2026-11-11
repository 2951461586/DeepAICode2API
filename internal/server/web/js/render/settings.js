// 设置页表单回填。

import { $ } from '../dom.js';

export function renderSettings(s){
  $("#sTimeout").value = s.request_timeout_sec || "";
  $("#sRetries").value = s.max_retries || "";
  $("#sLogKeep").value = s.log_keep || "";
  $("#sCors").value = s.cors_origin || "";
  $("#sFallback").value = s.fallback_model || "";
  $("#sStrict").checked = !!s.strict_model;
  // 限流参数：0 是合法值（关闭），不能用 `|| ""` 吞掉。
  $("#sRps").value = s.rate_limit_rps || "";
  $("#sBurst").value = s.rate_limit_burst || "";
  $("#sConcurrency").value = s.key_concurrency || "";
  $("#sAliases").value = JSON.stringify(s.model_aliases || {}, null, 2);
  $("#sPw").placeholder = s.admin_password_set ? "已设置，留空则不修改" : "未设置";
}
