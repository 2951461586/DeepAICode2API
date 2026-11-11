// 控制台的 DOM 冒烟测试（Node，无第三方依赖）。
//
// 为什么需要它：控制台是纯前端，`go test` 只能断言「字符串存在」，抓不到
// ReferenceError / TypeError。拆模块时最容易出的就是这类断裂——文件都对、
// 语法都对、测试全绿，点一下按钮才炸（本轮就抓到过 `lang` 与 `acctPage`）。
//
// 做法：用一个极简 DOM 桩把 web/js/*.js 跑起来，import 即执行全部顶层接线，
// 然后给四个渲染函数喂固定数据。任何未捕获异常都会让进程退出码非 0。
//
// 用法：node scripts/console-smoke.mjs
import { fileURLToPath, pathToFileURL } from 'node:url';
import { cpSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

const here = path.dirname(fileURLToPath(import.meta.url));
const jsDir = path.join(here, '..', 'internal', 'server', 'web', 'js');

/* ---------------- 极简 DOM 桩 ---------------- */

class ClassList {
  constructor() { this.set = new Set(); }
  add(...v) { v.forEach((x) => this.set.add(x)); }
  remove(...v) { v.forEach((x) => this.set.delete(x)); }
  contains(v) { return this.set.has(v); }
  toggle(v, on) { const want = on === undefined ? !this.set.has(v) : !!on; want ? this.set.add(v) : this.set.delete(v); }
}

class El {
  constructor(tag) {
    this.tagName = String(tag || 'div').toUpperCase();
    this.children = [];
    this.attrs = {};
    this.classList = new ClassList();
    this.style = {};
    this.dataset = {};
    this.textContent = '';
    this.value = '';
    this.checked = false;
    this.disabled = false;
    this.className = '';
    this.parentNode = null;
    this.offsetWidth = 120;
    this.offsetHeight = 40;
  }
  appendChild(child) { if (child) { child.parentNode = this; this.children.push(child); } return child; }
  append(...kids) { kids.forEach((k) => this.appendChild(k)); }
  removeChild(child) { this.children = this.children.filter((c) => c !== child); if (child) child.parentNode = null; return child; }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  getAttribute(k) { return this.attrs[k] ?? null; }
  addEventListener() {}
  removeEventListener() {}
  getBoundingClientRect() { return { top: 0, bottom: 40, left: 0, right: 120 }; }
  querySelector() { return null; }
  querySelectorAll() { return []; }
  focus() {}
  select() {}
  submit() {}
}

const bySelector = new Map();
const el = (sel) => {
  if (!bySelector.has(sel)) bySelector.set(sel, new El('div'));
  return bySelector.get(sel);
};

const document = {
  body: new El('body'),
  documentElement: new El('html'),
  createElement: (tag) => new El(tag),
  createTextNode: (t) => ({ nodeValue: t }),
  querySelector: (sel) => el(sel),
  querySelectorAll: () => [],
  getElementById: (id) => el('#' + id),
  addEventListener() {},
  createTreeWalker: () => ({ nextNode: () => null }),
};

globalThis.document = document;
globalThis.window = {
  innerWidth: 1280, innerHeight: 800,
  matchMedia: () => ({ matches: false, addEventListener() {} }),
  addEventListener() {},
};
// Node 22 的 navigator 是只读 getter，必须用 defineProperty 覆盖
Object.defineProperty(globalThis, 'navigator', { value: { language: 'zh-CN' }, configurable: true });
globalThis.location = { search: '', origin: 'http://127.0.0.1:10087', hash: '' };
globalThis.NodeFilter = { SHOW_TEXT: 4 };
globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} };
globalThis.sessionStorage = { getItem: () => null, setItem() {}, removeItem() {} };
globalThis.setInterval = () => 0;
globalThis.clearInterval = () => {};
// 管理接口一律返回空对象：refresh() 会真的跑一遍渲染路径。
globalThis.fetch = async () => ({ ok: true, status: 200, text: async () => '{}' });

/* ---------------- 以 ESM 方式加载 web/js ---------------- */

// web/js 用 `import` 语法，但目录里没有 package.json，Node 默认按 CJS 解析。
// 复制到临时目录并放一个 {"type":"module"} 是最省事的办法（不动仓库）。
const tmp = mkdtempSync(path.join(tmpdir(), 'console-smoke-'));
const copy = path.join(tmp, 'js');
cpSync(jsDir, copy, { recursive: true });
writeFileSync(path.join(copy, 'package.json'), JSON.stringify({ type: 'module' }));

const load = (rel) => import(pathToFileURL(path.join(copy, rel)).href);

const failures = [];
const check = (name, fn) => {
  try {
    fn();
    console.log(`  PASS ${name}`);
  } catch (e) {
    failures.push(`${name}: ${e && e.message}`);
    console.log(`  FAIL ${name}: ${e && e.message}`);
  }
};

console.log('控制台冒烟：');

// 1) 顶层接线：import 即执行（事件绑定、applyLang、refresh 的调用链）
await load('app.js');
check('app.js 顶层执行', () => {});

// 2) 四个渲染函数：喂固定数据，确认不抛异常且真的往表里写了行
const accounts = await load('render/accounts.js');
const keys = await load('render/keys.js');
const logs = await load('render/logs.js');
const settings = await load('render/settings.js');
const format = await load('format.js');
// 从 app.js 拆出去的三块（概览 / 登录面板 / 设置表单）单独装配一次：
// 拆模块最容易出的就是「文件都在、逻辑都对，但没人调用 init*」。
const overview = await load('overview.js');
const oauth = await load('oauth.js');
const settingsForm = await load('settings-form.js');

check('renderAccounts 空列表', () => {
  accounts.renderAccounts([], 0);
  if (el('#accounts').children.length === 0) throw new Error('没有写入空态行');
});
check('renderAccounts 有数据', () => {
  accounts.renderAccounts([{
    id: 'a1', name: 'n1', email: 'e@x.com', username: 'u1', group: 'default',
    source: 'manual', key: 'sk-abc', is_pro: true, plan: 'Pro', email_verified: true,
    theos: { balance: '200' }, status: 'ok', enabled: true, failures: 0, last_used: 0,
  }], 1);
  if (el('#accounts').children.length === 0) throw new Error('没有写入数据行');
});
check('renderKeys', () => {
  keys.renderKeys({ keys: [{ key: 'sk-1', name: 'k1', group: 'default', enabled: true, created_at: 0, last_used: 0, requests: 3 }] });
});
check('renderLogs', () => {
  logs.renderLogs([{ time: 0, endpoint: 'chat', model: 'm', status: 200, account: 'a', prompt_len: 10, output_len: 20, tokens_in: 1, tokens_out: 2, cost: 0.1 }]);
});
check('renderSettings', () => {
  settings.renderSettings({ request_timeout_sec: 120, max_retries: 3, log_keep: 500, cors_origin: '*', strict_model: true });
});
check('fmtCompact 纯函数', () => {
  if (format.fmtCompact(1500) !== '1.5K') throw new Error('fmtCompact(1500) = ' + format.fmtCompact(1500));
});
check('perSecondText 纯函数', () => {
  const { perSecondText } = format;
  if (perSecondText({}) !== '—') throw new Error('空对象应为 —');
  if (perSecondText({ theosPerSecond: {} }) !== '—') throw new Error('空 map 应为 —');
  if (perSecondText({ theosPerSecond: { standard: '0.5' } }) !== '0.5') throw new Error('单档位应只显示数值');
  if (perSecondText({ theosPerSecond: { a: '1', b: '2' } }) !== 'a: 1 · b: 2') throw new Error('多档位格式不对');
});

check('renderOverview 空数据', () => {
  overview.renderOverview({ stats: {}, models: [], keys: [] });
  if (el('#kpis').children.length === 0) throw new Error('KPI 卡片没画出来');
});
check('renderOverview 带模型', () => {
  overview.renderOverview({
    stats: {}, keys: [], models_updated: 0,
    models: [{ id: 'm1', name: 'M1', reasoning: true, input: ['text'], contextWindow: 100, maxTokens: 10, theosPerRequest: '0' }],
  });
  if (el('#models').children.length === 0) throw new Error('模型表没写入行');
});
check('loadHealth', () => {
  overview.loadHealth();
});
check('initOAuth / initSettingsForm 可重复装配', () => {
  oauth.initOAuth({ refresh() {} });
  settingsForm.initSettingsForm({ refresh() {} });
  if (typeof el('#oauthLogin').onclick !== 'function') throw new Error('登录按钮没接线');
  if (typeof el('#saveSettings').onclick !== 'function') throw new Error('设置保存没接线');
});

if (failures.length) {
  console.error(`\n${failures.length} 项失败：`);
  failures.forEach((f) => console.error('  - ' + f));
  process.exit(1);
}
console.log('\n全部通过');
