// 轻量提示条（toast）。
//
// 独立成模块是因为「渲染模块」和「操作模块」都要用它；放在 app.js 会让
// render/* → app.js 形成循环导入。

import { $ } from './dom.js';
import { translateDom } from './i18n.js';

export function toast(msg, kind){
  var el = $("#toast");
  el.textContent = msg;
  el.className = "toast show " + (kind||"");
  clearTimeout(toast._t);
  toast._t = setTimeout(function(){ el.className = "toast"; }, 4600);
  translateDom(el);
}
