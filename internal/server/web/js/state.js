// 控制台共享的可变 UI 状态。
//
// 只放「多个模块都要读写」的少数字段：账号分页游标、页大小与脱敏开关。
// 账号数据本身不在这里——它由服务端分页返回，渲染时作为参数传入。

export const ui = { acctPage: 0, acctPageSize: 20, mask: false };

// 分页偏好记在 localStorage；取值只接受白名单，避免手改坏值把列表打空。
try {
  const ps = parseInt(localStorage.getItem('deepaicode2api_acct_page_size'), 10);
  if (ps === 20 || ps === 50 || ps === 100) ui.acctPageSize = ps;
} catch { /* localStorage 不可用时用默认值 */ }

// 脱敏开关同理：只在显式记住过启用时为 true（默认关闭，不影响开箱可读性）。
try {
  ui.mask = localStorage.getItem('deepaicode2api_acct_mask') === '1';
} catch { /* localStorage 不可用时保持默认 */ }
