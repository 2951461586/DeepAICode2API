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
  toggle(v, on) {
    // Boolean() 而不是 !!：与仓内既有写法一致（no-extra-boolean-cast-js）。
    const want = on === undefined ? !this.set.has(v) : Boolean(on);
    if (want) this.set.add(v); else this.set.delete(v);
  }
}

class El {
  constructor(tag) {
    this.tagName = String(tag || 'div').toUpperCase();
    this.children = [];
    this.attrs = {};
    this.classList = new ClassList();
    this.style = {};
    this.dataset = {};
    this._text = '';
    this.value = '';
    this.checked = false;
    this.disabled = false;
    this.className = '';
    this.parentNode = null;
    this.offsetWidth = 120;
    this.offsetHeight = 40;
  }
  appendChild(child) { if (child) { child.parentNode = this; this.children.push(child); } return child; }  append(...kids) { kids.forEach((k) => this.appendChild(k)); }
  removeChild(child) { this.children = this.children.filter((c) => c !== child); if (child) child.parentNode = null; return child; }
  // textContent 必须像真实 DOM 一样「赋值即清空子节点」：
  // 渲染函数清表格用的就是 tb.textContent = ""，桩里不清会把上一轮的行留着，
  // 断言就会看到已经不存在于页面上的旧内容。
  get textContent() { return this._text; }
  set textContent(v) { this._text = String(v == null ? '' : v); this.children = []; }

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
  // <dialog> 的两个原生方法：桩里只需要不抛异常，断言看结构。
  showModal() { this.open = true; }
  close() { this.open = false; }
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
// checkAsync 同 check，但等异步链路跑完：变更类操作的收尾是 Promise，
// 裸写 check 会在 catch 真正执行前就判定通过（这正是「refresh is not a
// function」能被藏住的原因）。
const checkAsync = async (name, fn) => {
  try {
    await fn();
    console.log(`  PASS ${name}`);
  } catch (e) {
    failures.push(`${name}: ${e && e.message}`);
    console.log(`  FAIL ${name}: ${e && e.message}`);
  }
};

console.log('控制台冒烟：');

// 1) 顶层接线：import 即执行（事件绑定、applyLang、refresh 的调用链）
//
// 先把日志页标成「当前页」：app.js 的 start() 会按这个 class 决定要不要跑
// refreshLogs()，否则「取数 + 渲染」那条链在冒烟里一次都走不到。
// 2026-11-11 的真实回归：app.js 拆分时漏了 `import { renderLogs }`，未绑定
// 标识符只在执行到那一行时才抛，又被 .catch 包成「日志加载失败」（其实接口 200）。
el('#tab-logs').classList.add('on');
await load('app.js');
await new Promise((r) => setTimeout(r, 0)); // 等 app.js 里的异步链跑完
check('app.js 顶层执行', () => {});
check('app.js 启动即刷新日志页', () => {
  const txt = String(el('#toast').textContent || '');
  if (txt.includes('日志加载失败') || txt.includes('日志渲染失败')) {
    throw new Error('日志页取数/渲染报错：' + txt);
  }
  if (el('#logs').children.length === 0) throw new Error('日志表没有写入行');
});

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
const dom = await load('dom.js');
// 语言状态是模块级单例：切到 en 再渲染，能直接看出哪些文案走了 t()、哪些没走。
const i18n = await load('i18n.js');

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
// 行内菜单：「操作」按钮点开是账号治理的唯一入口，一旦炸掉整张表就只剩查看。
// 2026-11-11 的真实回归：menuEl 拆到 dom.js 时漏了声明，严格模式下
// openMenu 第一行 closeMenu() 就抛 ReferenceError，点击完全无反应。
check('行内菜单 open / close', () => {
  const anchor = el('#menu-anchor');
  dom.openMenu(anchor, [
    { label: '配额', fn() {} },
    { sep: true },
    { label: '删除', danger: true, fn() {} },
  ]);
  if (document.body.children.length !== 1) throw new Error('菜单没挂到 body 上');
  dom.closeMenu();
  if (document.body.children.length !== 0) throw new Error('closeMenu 没清掉菜单');
  dom.closeMenu(); // 幂等：重复关闭不应抛异常
});

// 侧边栏伸缩：只靠 body 上的 class + CSS 变量，JS 一旦没接线按钮就死掉。
check('侧边栏收起 / 展开', () => {
  const btn = el('#sideToggle');
  if (typeof btn.onclick !== 'function') throw new Error('侧边栏开关没接线');
  btn.onclick();
  if (!document.body.classList.contains('side-collapsed')) throw new Error('没切到收起态');
  btn.onclick();
  if (document.body.classList.contains('side-collapsed')) throw new Error('没切回展开态');
});

check('fmtCompact 纯函数', () => {
  if (format.fmtCompact(1500) !== '1.5K') throw new Error('fmtCompact(1500) = ' + format.fmtCompact(1500));
});
// 顶栏版本号：git describe / docker 构建参数本身带 v 前缀，接口也照实返回，
// 展示层再拼一个 v 就成了线上的 `vv0.1.4+efa5d24`。
await checkAsync('版本号只补一个 v 前缀', async () => {
  const version = await load('version.js');
  const cases = [
    ['v0.1.4+efa5d24', 'v0.1.4+efa5d24'],
    ['0.1.4', 'v0.1.4'],
    ['V1.0.0', 'V1.0.0'],
    ['', 'vdev'],
    [undefined, 'vdev'],
  ];
  for (const [input, want] of cases) {
    version.renderVersion(input);
    const got = String(el('#version').textContent || '');
    if (got !== want) throw new Error(`renderVersion(${JSON.stringify(input)}) = ${got}，期望 ${want}`);
  }
});
check('renderSettings 保留显式 0', () => {
  // 0 是「限流关闭」的合法值：曾经用 `|| ""`，结果设置被吞成空框、看上去像没生效。
  settings.renderSettings({ request_timeout_sec: 0, max_retries: 0, log_keep: 0, rate_limit_rps: 0, rate_limit_burst: 0, key_concurrency: 0 });
  const zeros = ['#sRps', '#sBurst', '#sConcurrency'].filter((s) => el(s).value !== '0');
  if (zeros.length) throw new Error('显式 0 被吞成空值：' + zeros.join(','));
});
check('英文界面空态不落中文', () => {
  // 空态行是唯一不经过 translateDom 的单元格，英文界面曾经直接显示中文
  // （而空态正是新装用户的默认视图）。
  i18n.setLang('en');
  const cjk = /[\u4e00-\u9fff]/;
  logs.renderLogs([]);
  keys.renderKeys({ keys: [] });
  accounts.renderAccounts([], 0);
  overview.renderOverview({ stats: {}, models: [], keys: [] });
  const where = [];
  ['#logs', '#keys', '#accounts', '#models'].forEach((sel) => {
    const text = el(sel).children
      .map((r) => (r.children || []).map((c) => String(c.textContent || '')).join(' '))
      .join(' ');
    if (cjk.test(text)) where.push(sel + ': ' + text);
  });
  i18n.setLang('zh');
  if (where.length) throw new Error('英文空态仍是中文：' + where.join(' | '));
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
// 缓存命中率的口径：主值取今日，累计退到 tooltip。
// 2026-11-12 的线上现象：卡片显示 15.1%（累计 228.2M/1.51G），
// 而当天实际是 94%（228.2M/242.4M）——同一张卡片里混了两个口径。
check('缓存命中率用今日口径，累计进 tooltip', () => {
  overview.renderOverview({
    stats: { total_tokens_in: 1000, total_cached_tokens: 100, day_tokens_in: 200, day_cached_tokens: 150 },
    models: [], keys: [],
  });
  const cards = el('#kpis').children.map((c) => ({
    k: String((c.children[0] || {}).textContent || ''),
    v: String((c.children[1] || {}).textContent || ''),
    title: String(c.title || ''),
  }));
  const hit = cards.find((c) => c.k.includes('缓存命中率'));
  if (!hit) throw new Error('没有缓存命中率卡片');
  if (hit.v !== '75.0%') throw new Error('今日命中率 = ' + hit.v + '（期望 75.0%）');
  if (!hit.title.includes('10.0%')) throw new Error('tooltip 缺累计命中率：' + hit.title);
});
// 499 = 客户端先走了：既不是成功也不是上游失败，日志里用 warn 色区分。
check('日志 499 用 warn 色', () => {
  logs.renderLogs([{
    time: 1, endpoint: 'chat', model: 'm', status: 499, latency_ms: 5, err: 'context canceled',
  }]);
  const badge = el('#logs').children[0].children[4].children[0];
  if (!String(badge.className || '').includes('badge')) throw new Error('状态列不是 badge');
  if (!String(badge.className || '').includes('warn')) throw new Error('499 没用 warn 色：' + badge.className);
});
check('initOAuth / initSettingsForm 可重复装配', () => {
  oauth.initOAuth({ refresh() {} });
  settingsForm.initSettingsForm({ refresh() {} });
  if (typeof el('#oauthLogin').onclick !== 'function') throw new Error('登录按钮没接线');
  if (typeof el('#saveSettings').onclick !== 'function') throw new Error('设置保存没接线');
});

// 变更类操作的收尾：拆分后「刷新」只能经 setReload 注入，这里盯住注入值真的被
// 调到。2026-11-11 的真实回归：accountOp / keyOp / finishCheckin / 改绑分组 4 处
// 还写着裸 `refresh()`，收尾抛 TypeError 又被 .catch 吃成「失败：refresh is not
// a function」——看起来像接口挂了。
await checkAsync('变更操作收尾走注入的 reload', async () => {
  const actions = await load('actions.js');
  let reloads = 0;
  actions.setReload(() => { reloads++; });
  // 先跑拆分前就存在的三条：它们就是当初留着裸 refresh() 的地方。
  await actions.accountOp('toggle', { id: 1 });
  await actions.keyOp('toggle', 'sk-x');
  await actions.finishCheckin(2, []);
  if (reloads !== 3) throw new Error(`3 条收尾只触发 reload ${reloads} 次（期望 3）`);
  if (typeof actions.keySetGroup !== 'function') {
    throw new Error('actions.js 没导出 keySetGroup（改绑分组必须走「变更 + 重拉」）');
  }
  await actions.keySetGroup('sk-x', 'grp');
  if (reloads !== 4) throw new Error(`改绑分组后 reload 共 ${reloads} 次（期望 4）`);
  const txt = String(el('#toast').textContent || '');
  if (txt.includes('refresh is not')) throw new Error('收尾仍引用裸 refresh：' + txt);
  // 只认错误前缀：finishCheckin 的成功文案里也含「失败 0」。
  if (txt.includes('失败：')) throw new Error('收尾报错：' + txt);
});

// 密钥 404 的自愈：目标已经不在了（另一个标签页删过）时要重拉一次，
// 否则列表会留一行怎么点都 404 的幽灵行。
await checkAsync('密钥操作遇到 404 仍然重拉', async () => {
  const actions = await load('actions.js');
  let reloads = 0;
  actions.setReload(() => { reloads++; });
  const saved = globalThis.fetch;
  globalThis.fetch = async () => ({
    ok: false,
    status: 404,
    text: async () => JSON.stringify({ error: { message: '密钥不存在', type: 'not_found_error' } }),
  });
  try {
    await actions.keyOp('toggle', 'sk-gone');
  } finally {
    globalThis.fetch = saved;
  }
  if (reloads !== 1) throw new Error(`404 后 reload ${reloads} 次（期望 1）`);
  if (!String(el('#toast').textContent || '').includes('失败')) throw new Error('404 应仍然报错');
});

// 代理 / 机器指纹弹窗：改前 → 改后的字段要成结构化列表，危险操作用 danger 按钮。
// 以前这是一句拼接文案，长代理 host 会把句子扭成一团，也看不出“会改成什么”。
await checkAsync('改出口弹窗是结构化字段', async () => {
  const dialog = await load('dialog.js');
  const rows = [
    { label: '账号', value: 'u***@x.com', mono: false },
    { label: '当前出口', value: 'eu.gw.proxylane.dev:10001' },
    { label: '新出口', value: '代理池随机分配' },
  ];

  // 不 await：监听器在桩里是空实现，Promise 永远不会落定（真实 DOM 里由按钮兜）。
  dialog.confirmDialog({ title: '重绑代理', rows: rows, message: '换出口后……', okText: '重新分配' });
  const card = document.body.children.at(-1).children[0];
  const list = card.children.find((c) => c.className === 'dialog-rows');
  if (!list) throw new Error('没有 dialog-rows 列表');
  if (list.children.length !== rows.length) {
    throw new Error(`字段数 = ${list.children.length}，期望 ${rows.length}`);
  }
  const first = list.children[0];
  if (String(first.children[0].textContent) !== '账号') throw new Error('label 错了：' + first.children[0].textContent);
  if (String(first.children[1].textContent) !== 'u***@x.com') throw new Error('value 错了：' + first.children[1].textContent);
  if (String(first.children[1].className || '') !== '') throw new Error('mono:false 不应加等宽类');
  if (String(list.children[1].children[1].className || '') !== 'mono') throw new Error('代理 host 应为等宽字体');
  const actions2 = card.children.find((c) => c.className === 'dialog-actions');
  const okBtn = actions2.children.at(-1);
  if (!String(okBtn.className || '').includes('primary')) throw new Error('确认键不是 primary：' + okBtn.className);
  if (String(okBtn.textContent) !== '重新分配') throw new Error('确认键文案 = ' + okBtn.textContent);

  // 危险操作（清除代理 / 删除）：同一个入口，按钮要变 danger。
  dialog.confirmDialog({ title: '清除代理', rows: rows, danger: true, okText: '清除代理' });
  const card2 = document.body.children.at(-1).children[0];
  const actions3 = card2.children.find((c) => c.className === 'dialog-actions');
  const okBtn2 = actions3.children.at(-1);
  if (!String(okBtn2.className || '').includes('danger')) throw new Error('危险操作没用 danger 按钮：' + okBtn2.className);
});

if (failures.length) {
  console.error(`\n${failures.length} 项失败：`);
  failures.forEach((f) => console.error('  - ' + f));
  process.exit(1);
}
console.log('\n全部通过');
