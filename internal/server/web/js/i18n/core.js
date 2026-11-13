// i18n 运行时：语言状态 + 字典合并 + 文本替换。
//
// 字典拆在 ./dict-*.js（按用途）；这里合并成一张表，tr/t/translateDom 的
// 查找语义与拆分前完全一致。对外仍从 ../i18n.js 导出（见那里的 re-export）。
import { SHELL_DICT } from './dict-shell.js';
import { PAGES_DICT } from './dict-pages.js';

var I18N_EN = Object.assign({}, SHELL_DICT, PAGES_DICT);

var lang = "zh";
try {
  // ?lang=en 优先（便于分享/截图固定语言），其次本地偏好，最后跟随浏览器。
  var qs = (location.search.match(/[?&]lang=(en|zh)/) || [])[1];
  var savedLang = localStorage.getItem("deepaicode2api_lang");
  if (qs) lang = qs;
  else if (savedLang === "en" || savedLang === "zh") lang = savedLang;
  else if ((navigator.language || "").toLowerCase().indexOf("zh") !== 0) lang = "en";
} catch { /* localStorage 不可用时按语言环境判断 */ }

// tr 只做精确匹配（渲染后的整段文本）。
export function trCore(s){
  if (lang !== "en") return s;
  return I18N_EN[s] !== undefined ? I18N_EN[s] : s;
}

// t 支持 {name} 插值，用于拼接句子。
export function tCore(s, vars){
  var out = trCore(s);
  if (vars) Object.keys(vars).forEach(function(k){ out = out.split("{" + k + "}").join(String(vars[k])); });
  return out;
}

var SKIP_TAGS = { SCRIPT:1, STYLE:1, TEXTAREA:1 };

// translateDom 把一段 DOM 里的「纯中文叶子文本」翻成当前语言。
//
// 首次遇到某节点时把中文原文记在 node.__zh 上，切回中文时用它还原；
// 这样静态标记与动态渲染都走同一条路径，不必给每个节点加 data-i18n。
export function translateDomCore(root){
  if (!root) return;
  var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, null);
  var nodes = [], n;
  while ((n = walker.nextNode())) nodes.push(n);
  nodes.forEach(function(node){
    var parent = node.parentNode;
    if (parent && SKIP_TAGS[parent.nodeName]) return;
    var raw = node.nodeValue || "";
    var key = raw.trim();
    if (!key) return;
    if (node.__zh === undefined) node.__zh = key;
    var want = node.__zh;
    if (lang === "en" && I18N_EN[node.__zh] !== undefined) want = I18N_EN[node.__zh];
    if (want === key) return;
    var lead = raw.match(/^\s*/)[0], trail = raw.match(/\s*$/)[0];
    node.nodeValue = lead + want + trail;
  });
}

// 输入框占位符与 title 也要跟着切换。
export function translateAttrsCore(root){
  if (!root || !root.querySelectorAll) return;
  root.querySelectorAll("[placeholder]").forEach(function(el){
    if (el.__ph === undefined) el.__ph = el.getAttribute("placeholder");
    el.setAttribute("placeholder", lang === "en" && I18N_EN[el.__ph] !== undefined ? I18N_EN[el.__ph] : el.__ph);
  });
  root.querySelectorAll("[title]").forEach(function(el){
    if (el.__title === undefined) el.__title = el.getAttribute("title");
    el.setAttribute("title", lang === "en" && I18N_EN[el.__title] !== undefined ? I18N_EN[el.__title] : el.__title);
  });
}

export function applyLangCore(){
  translateDomCore(document.body);
  translateAttrsCore(document.body);
  var b = document.getElementById("lang");
  if (b) b.textContent = lang === "en" ? "中" : "EN";
  document.documentElement.setAttribute("lang", lang === "en" ? "en" : "zh-CN");
}

/* ================= 行内操作菜单构造 ================= */
// setLang / getLang 是语言状态的唯一入口。
//
// lang 是模块私有变量：外部只能通过 setLang 改，避免各模块各拿一份副本
// 导致「切了语言但某些文案没跟着变」。
export function setLangCore(v) { lang = v === 'en' ? 'en' : 'zh'; }
export function getLangCore() { return lang; }

