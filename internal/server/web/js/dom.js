// DOM 构建助手与行内菜单。
//
// 全程 createElement + textContent，**绝不拼接 HTML**（账号名、错误串、
// 模型名都是外部输入）。要写 innerHTML 前先想清楚这里为什么没有。

import { tr, t } from './i18n.js';

export const $ = (s) => document.querySelector(s);

// menuEl 是当前打开的行内菜单节点（null = 没有菜单）。
//
// 必须在本模块里声明：模块是严格模式，引用未声明的变量会直接抛
// ReferenceError —— 拆分前它在单文件 IIFE 的闭包里，拆到 dom.js 时
// 忘了带上声明，结果点「操作」整个菜单都出不来（先调的 closeMenu 就炸了）。
let menuEl = null;
// menuAnchor 是打开菜单的按钮：关闭时要把 aria-expanded 复位。
let menuAnchor = null;

export function menuToggle(title){
  var m = mk("button", "menu-toggle", "⋯");
  m.type = "button";
  m.title = t(title);
  return m;
}

export function closeMenu(){
  if (menuEl && menuEl.parentNode) menuEl.parentNode.removeChild(menuEl);
  menuEl = null;
  if (menuAnchor && menuAnchor.setAttribute) {
    menuAnchor.setAttribute("aria-expanded", "false");
    menuAnchor = null;
  }
}

export function openMenu(anchor, items){
  closeMenu();
  var m = mk("div", "menu");
  m.setAttribute("role", "menu");
  var buttons = [];
  items.forEach(function(it){
    if (it.sep){ m.appendChild(mk("div", "menu-sep")); return; }
    var b = mk("button", "menu-item" + (it.danger ? " danger" : ""), tr(it.label));
    b.type = "button";
    b.setAttribute("role", "menuitem");
    b.addEventListener("click", function(ev){ ev.stopPropagation(); closeMenu(); it.fn(); });
    buttons.push(b);
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
  menuAnchor = anchor;
  if (anchor && anchor.setAttribute) {
    anchor.setAttribute("aria-haspopup", "menu");
    anchor.setAttribute("aria-expanded", "true");
  }
  // 键盘导航：↑↓ / Home / End 移动，Esc 关闭（全局已绑定），Enter 原生触发。
  m.addEventListener("keydown", function(ev){
    if (!buttons.length) return;
    var i = buttons.indexOf(document.activeElement);
    if (ev.key === "ArrowDown") { ev.preventDefault(); buttons[(i + 1 + buttons.length) % buttons.length].focus(); }
    else if (ev.key === "ArrowUp") { ev.preventDefault(); buttons[(i - 1 + buttons.length) % buttons.length].focus(); }
    else if (ev.key === "Home") { ev.preventDefault(); buttons[0].focus(); }
    else if (ev.key === "End") { ev.preventDefault(); buttons[buttons.length - 1].focus(); }
    else if (ev.key === "Tab") { closeMenu(); }
  });
  // 打开即把焦点放进菜单：纯键盘用户不需要先 Tab 一圈才进来。
  if (buttons.length) buttons[0].focus();
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

// emptyRow 画「空态」行。
//
// 文本过一遍 t()：空态行是**唯一**一种不经过 translateDom 的单元格（渲染函数
// 在追加空态行后就 return 了，日志页更是连 translateDom 都不调），英文界面
// 因此会直接显示中文——而空态恰恰是新装用户的默认视图。
export function emptyRow(cols, text){
  var c = cell(t(text), "muted");
  c.colSpan = cols;
  var r = mk("tr");
  r.appendChild(c);
  return r;
}
