// 账号列表分页控件：上一页 / 下一页 / 每页条数。
//
// 与渲染分离：分页器只负责「改 ui.acctPage / acctPageSize，然后请 app.js 重新取数」，
// 不自己拼查询串。总数与页码文案由 render/accounts.js 的 updateAcctPager 负责，
// 避免两处各算一份「共 N 页」。

import { $ } from './dom.js';
import { ui } from './state.js';

const SIZE_KEY = "deepaicode2api_acct_page_size";

// initAccountPager 绑定分页控件。
//
// onChange 在页码 / 页长变化后调用（app.js 传 loadAccounts，顺带同步 hash）。
export function initAccountPager(onChange) {
  var notify = onChange || function () {};

  $("#acctSize").value = String(ui.acctPageSize);

  $("#acctPrev").onclick = function () {
    if (ui.acctPage <= 0) return;
    ui.acctPage--;
    notify();
  };

  $("#acctNext").onclick = function () {
    ui.acctPage++;
    notify();
  };

  $("#acctSize").onchange = function () {
    var v = parseInt(this.value, 10);
    if (v !== 20 && v !== 50 && v !== 100) return;
    ui.acctPageSize = v;
    ui.acctPage = 0;
    try { localStorage.setItem(SIZE_KEY, String(v)); } catch { /* 隐私模式忽略 */ }
    notify();
  };
}
