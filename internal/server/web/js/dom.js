// DOM 构建助手与行内菜单。
//
// 全程 createElement + textContent，**绝不拼接 HTML**（账号名、错误串、
// 模型名都是外部输入）。要写 innerHTML 前先想清楚这里为什么没有。

import { tr, t } from './i18n.js';

export const $ = (s) => document.querySelector(s);

export function menuToggle(title){
  var m = mk("button", "menu-toggle", "⋯");
  m.type = "button";
  m.title = t(title);
  return m;
}

export function closeMenu(){
  if (menuEl && menuEl.parentNode) menuEl.parentNode.removeChild(menuEl);
  menuEl = null;
}

export function openMenu(anchor, items){
  closeMenu();
  var m = mk("div", "menu");
  items.forEach(function(it){
    if (it.sep){ m.appendChild(mk("div", "menu-sep")); return; }
    var b = mk("button", "menu-item" + (it.danger ? " danger" : ""), tr(it.label));
    b.type = "button";
    b.addEventListener("click", function(ev){ ev.stopPropagation(); closeMenu(); it.fn(); });
    m.appendChild(b);
  });
  document.body.appendChild(m);
  var r = anchor.getBoundingClientRect();
  var mw = m.offsetWidth, mh = m.offsetHeight;
  var left = Math.max(8, Math.min(r.right - mw, window.innerWidth - mw - 8));
  var top = r.bottom + 6;
  if (top + mh > window.innerHeight - 8) top = Math.max(8, r.top - mh - 6);
  m.style.left = left + "px";
  m.style.top = top + "px";
  menuEl = m;
}

export function mk(tag, cls, text){
  var n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text !== undefined && text !== null) n.textContent = String(text);
  return n;
}

export function cell(text, cls){ return mk("td", cls, text); }

export function row(cells){
  var r = mk("tr");
  for (var i = 0; i < cells.length; i++) r.appendChild(cells[i]);
  return r;
}

export function pill(text, cls){ return mk("span", "badge " + (cls||""), text); }

export function emptyRow(cols, text){
  var c = cell(text, "muted");
  c.colSpan = cols;
  var r = mk("tr");
  r.appendChild(c);
  return r;
}
