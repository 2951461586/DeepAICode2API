// 控制台 i18n 的公开入口：实现与字典都在 ./i18n/ 下。
//
// 这里显式转出函数（而不是 `export { x } from`）：console_imports_test.go
// 用轻量正则解析导出名，export-from 形式会让它误判「没有导出」。显式包装
// 只多一层调用，换来契约测试继续有效。
import * as core from './i18n/core.js';

export function tr(s) { return core.trCore(s); }
export function t(s, vars) { return core.tCore(s, vars); }
export function translateDom(root) { return core.translateDomCore(root); }
export function translateAttrs(root) { return core.translateAttrsCore(root); }
export function applyLang() { return core.applyLangCore(); }
export function setLang(v) { return core.setLangCore(v); }
export function getLang() { return core.getLangCore(); }
