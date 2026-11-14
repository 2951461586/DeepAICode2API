// 缓存命中率下钻：把「今日命中率」拆成按模型 / 端点 / 小时 / 密钥的分布。
//
// 为什么要这个脚本：控制台只有一个汇总数（day_cached / day_tokens_in）。
// 一个数走低可能是三件完全不同的事，处置方式也完全不同：
//   1. 上游没报缓存字段（cached 恒为 0）—— 换模型 / 换端点才会变；
//   2. 前缀不再复用（多租户各自拼 prompt）—— 这是流量形态，不是 bug；
//   3. 某条链路把 usage 丢了（tokens_in 有值、cached 没值）—— 这才是要修的点。
// 只看汇总数分不出这三种，所以按维度拆开看「无缓存上报的占比」。
//
// 数据源：/admin/api/logs（服务端解密后的 LogEntry，含 tokens_in / cached_tokens）。
// 用法：
//   node scripts/cache-hit.mjs --selftest
//   DEEPAI2API_ADMIN_PASSWORD=xxx node scripts/cache-hit.mjs               # 今天
//   node scripts/cache-hit.mjs --base http://127.0.0.1:10087 --hours 24
//
// 注意：跨天的界是**网关本地日期**（容器里默认 UTC），所以北京时间 08:00
// 才算第一天开始 —— “今天”这个数在北京时间早上天然只有几十分钟样本。

import { pathToFileURL } from 'node:url';

const DEFAULT_BASE = process.env.DEEPAI2API_BASE || 'http://127.0.0.1:10087';

// percent 把 cached/total 格式化成一位小数；无样本时给 —（别画成 0%）。
export function percent(cached, total) {
  if (!(total > 0)) return '—';
  return ((cached / total) * 100).toFixed(1) + '%';
}

// bucket 是一组日志的聚合结果。
function bucket() {
  return { entries: 0, used: 0, tokensIn: 0, cached: 0, noCache: 0, zeroUsage: 0 };
}

function add(b, e) {
  b.entries++;
  const tin = Number(e.tokens_in) || 0;
  const cached = Number(e.cached_tokens) || 0;
  if (tin <= 0) {
    // 上游没回 usage（流式尾部帧丢了 / 上游省略）：既不在分子也不在分母里。
    b.zeroUsage++;
    return;
  }
  b.used++;
  b.tokensIn += tin;
  b.cached += Math.min(cached, tin);
  if (cached <= 0) b.noCache++;
}

// aggregate 把日志数组聚合成总览 + 四个维度的分桶（纯函数，可单测）。
//
// entries 只需要 time / model / endpoint / key / tokens_in / cached_tokens。
export function aggregate(entries, opts) {
  const o = opts || {};
  const hoursOf = o.hours || 0;
  const total = bucket();
  const dims = { model: new Map(), endpoint: new Map(), hour: new Map(), key: new Map() };
  const now = o.now || Date.now();

  for (const e of entries || []) {
    if (!e) continue;
    // 只统计窗口内的样本：日志接口本身也有 since，但跨天/时区口径要在这里兜住。
    if (hoursOf > 0 && (now - (Number(e.time) || 0) * 1000) / 3600000 > hoursOf) continue;
    add(total, e);
    const hourKey = hourLabel(e.time);
    const pairs = [
      ['model', e.model || '-'],
      ['endpoint', e.endpoint || '-'],
      ['hour', hourKey],
      ['key', e.key || '-'],
    ];
    for (const [dim, k] of pairs) {
      const m = dims[dim];
      if (!m.has(k)) m.set(k, bucket());
      add(m.get(k), e);
    }
  }
  return { total: total, model: dims.model, endpoint: dims.endpoint, hour: dims.hour, key: dims.key };
}

// hourLabel 按网关本地时区的小时归档（脚本与网关同机跑时口径一致）。
function hourLabel(sec) {
  const d = new Date((Number(sec) || 0) * 1000);
  const two = (n) => String(n).padStart(2, '0');
  return `${two(d.getMonth() + 1)}-${two(d.getDate())} ${two(d.getHours())}:00`;
}

// topRows 把 Map 变成按 tokensIn 降序的前 n 行（外加一个“其它”汇总）。
function topRows(map, n) {
  const rows = [...map.entries()].sort((a, b) => b[1].tokensIn - a[1].tokensIn);
  if (rows.length <= n) return rows;
  const head = rows.slice(0, n);
  const rest = bucket();
  for (const [, b] of rows.slice(n)) {
    rest.entries += b.entries;
    rest.used += b.used;
    rest.tokensIn += b.tokensIn;
    rest.cached += b.cached;
    rest.noCache += b.noCache;
    rest.zeroUsage += b.zeroUsage;
  }
  head.push([`其它（${rows.length - n} 项）`, rest]);
  return head;
}

function pad(s, n) {
  s = String(s);
  // 中文按两个字符宽计算，让表格在终端里对得齐。
  const width = [...s].reduce((w, ch) => w + (ch.charCodeAt(0) > 0x2e80 ? 2 : 1), 0);
  return s + ' '.repeat(Math.max(0, n - width));
}

function printTable(title, rows) {
  console.log(`\n${title}`);
  console.log(`  ${pad('项', 26)}${pad('请求', 8)}${pad('Token 入', 12)}${pad('命中率', 9)}无缓存占比`);
  for (const [k, b] of rows) {
    const noCache = b.used > 0 ? percent(b.noCache, b.used) : '—';
    console.log(
      `  ${pad(k, 26)}${pad(b.entries, 8)}${pad(fmt(b.tokensIn), 12)}` +
      `${pad(percent(b.cached, b.tokensIn), 9)}${noCache}`
    );
  }
}

function fmt(n) {
  if (n >= 1e6) return (n / 1e6).toFixed(2) + 'M';
  if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
  return String(n);
}

function summary(agg) {
  const t = agg.total;
  console.log(`总览：命中率 ${percent(t.cached, t.tokensIn)}` +
    `（命中 ${fmt(t.cached)} / 入 ${fmt(t.tokensIn)}）`);
  console.log(`  日志 ${t.entries} 条：有用量 ${t.used} 条，无用量 ${t.zeroUsage} 条；` +
    `有用量里「cached=0」${t.noCache} 条（${percent(t.noCache, t.used)}）`);
  if (t.noCache / Math.max(t.used, 1) > 0.5) {
    console.log('  ⚠ 过半样本 cached=0：先看下面按模型的分布，多半是上游本来就不报缓存字段。');
  }
}

function report(agg) {
  summary(agg);
  printTable('按模型（Top 10，按 Token 入排序）', topRows(agg.model, 10));
  printTable('按端点', topRows(agg.endpoint, 10));
  printTable('按密钥', topRows(agg.key, 10));
  printTable('按小时（最近 12 个）', [...agg.hour.entries()].sort().slice(-12));
}

/* ---------------- 自检 ---------------- */

function selftest() {
  const now = Date.UTC(2026, 10, 14, 3, 0, 0);
  const t = (h) => Math.floor((now - h * 3600000) / 1000);
  const entries = [
    { time: t(0.5), model: 'm1', endpoint: 'responses', key: 'k1', tokens_in: 1000, cached_tokens: 800 },
    { time: t(0.4), model: 'm1', endpoint: 'responses', key: 'k1', tokens_in: 1000, cached_tokens: 0 },
    { time: t(0.3), model: 'm2', endpoint: 'chat', key: 'k2', tokens_in: 0, cached_tokens: 0 },
    { time: t(0.2), model: 'm2', endpoint: 'chat', key: 'k2', tokens_in: 500, cached_tokens: 500 },
    { time: t(0.1), model: 'm2', endpoint: 'chat', key: 'k2', tokens_in: 500 },
  ];
  const agg = aggregate(entries, { now });
  const checks = [
    ['总 token 入', agg.total.tokensIn, 3000],
    ['总命中', agg.total.cached, 1300],
    ['有用量样本', agg.total.used, 4],
    ['无用量样本', agg.total.zeroUsage, 1],
    ['cached=0 样本', agg.total.noCache, 2],
    ['m1 命中率', percent(agg.model.get('m1').cached, agg.model.get('m1').tokensIn), '40.0%'],
    ['m2 命中率', percent(agg.model.get('m2').cached, agg.model.get('m2').tokensIn), '50.0%'],
    ['无样本时显示', percent(0, 0), '—'],
    ['窗口过滤', aggregate(entries, { now, hours: 0.35 }).total.entries, 3],
  ];
  let bad = 0;
  for (const [name, got, want] of checks) {
    if (got !== want) {
      bad++;
      console.error(`  FAIL ${name}: ${got} != ${want}`);
    }
  }
  if (bad) process.exit(1);
  console.log(`自检通过（${checks.length} 项）`);
  report(agg);
}

/* ---------------- CLI ---------------- */

function parseArgs(argv) {
  const out = { base: DEFAULT_BASE, hours: 0, limit: 20000, selftest: false };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--selftest') out.selftest = true;
    else if (a === '--base') out.base = argv[++i];
    else if (a === '--hours') out.hours = Number(argv[++i]);
    else if (a === '--limit') out.limit = Number(argv[++i]);
    else if (a === '--help' || a === '-h') {
      console.log('用法：node scripts/cache-hit.mjs [--base URL] [--hours N] [--limit N] [--selftest]');
      console.log('管理口令从 DEEPAI2API_ADMIN_PASSWORD 读取（回环访问可留空）。');
      process.exit(0);
    }
  }
  return out;
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.selftest) return selftest();

  // --hours 换算成 since：接口只认 Unix 秒，时区口径交给网关自己。
  const since = args.hours > 0 ? Math.floor(Date.now() / 1000 - args.hours * 3600) : 0;
  const url = `${args.base.replace(/\/$/, '')}/admin/api/logs?limit=${args.limit}` +
    (since ? `&since=${since}` : '');
  const headers = {};
  if (process.env.DEEPAI2API_ADMIN_PASSWORD) {
    headers['X-Admin-Password'] = process.env.DEEPAI2API_ADMIN_PASSWORD;
  }

  let payload;
  try {
    const res = await fetch(url, { headers });
    if (!res.ok) {
      console.error(`请求 ${url} 失败：HTTP ${res.status}`);
      const hint = res.status === 401 ? '（管理口令缺失或错误，设 DEEPAI2API_ADMIN_PASSWORD）' : '';
      console.error(hint);
      process.exit(2);
    }
    payload = await res.json();
  } catch (err) {
    console.error(`请求 ${url} 失败：${err && err.message}`);
    process.exit(2);
  }

  const entries = (payload && payload.logs) || [];
  if (!entries.length) {
    console.error('没有日志样本（窗口内无请求，或数据文件是空的）');
    process.exit(3);
  }
  console.log(`${args.base} · 样本 ${entries.length} 条` +
    (args.hours ? ` · 最近 ${args.hours} 小时` : ' · 全部'));
  report(aggregate(entries, { hours: args.hours }));
}

// 只有直接执行才跑 CLI（被 import 时只导出纯函数，便于单测）。
//
// pathToFileURL 对非路径输入会抛 TypeError（Windows 下相对路径也不会自动补齐），
// 所以包一层：判断失败就当“不是入口”，比让脚本在 import 场景炸掉安全。
function isEntryPoint() {
  if (!process.argv[1]) return false;
  try {
    return import.meta.url === pathToFileURL(process.argv[1]).href;
  } catch {
    return false;
  }
}

if (isEntryPoint()) {
  await main();
}
