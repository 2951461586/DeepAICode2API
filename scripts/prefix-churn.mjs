// 前缀抖动体检：量化「客户端到底有没有固定前缀」。
//
// 背景：缓存命中率只有 60–70%，光看 cached/prompt 分不清两种完全不同的原因——
//   A. 客户端每轮重写 system / tools（前缀变了，缓存必然 miss）；
//   B. 前缀稳定但换了账号 / 空闲超过上游 TTL（缓存分区换了）。
// 网关从 v0.1.29 起在请求日志里记了 `affinity`（会话粘性标识）与 `prefix`
// （可缓存前缀的短哈希），把两者按 affinity 分组就能把 A 与 B 分开：
//   - 同一 affinity 下出现多个 prefix  → A（前缀在动）
//   - 同一 affinity 下只有 1 个 prefix  → 前缀稳定，0 命中属 B
//
// 用法：
//   ADMIN_PW=... node scripts/prefix-churn.mjs
//   GW=https://deepai-gw.liziai.cloud ADMIN_PW=... LIMIT=500 node scripts/prefix-churn.mjs
//
// 注意：日志里这两列要**部署包含该改动的版本之后**才会出现；老版本一律没有。

const GW = (process.env.GW || 'https://deepai-gw.liziai.cloud').replace(/\/+$/, '');
const PW = process.env.ADMIN_PW || process.env.DEEPAI2API_ADMIN_PASSWORD || '';
const LIMIT = Number(process.env.LIMIT || 500);

if (!PW) {
  console.error('缺少 ADMIN_PW（或 DEEPAI2API_ADMIN_PASSWORD）');
  process.exit(2);
}

const res = await fetch(`${GW}/admin/api/logs?limit=${LIMIT}`, {
  headers: { 'X-Admin-Password': PW },
});
if (!res.ok) {
  console.error(`拉取日志失败: HTTP ${res.status} ${await res.text()}`);
  process.exit(1);
}
const body = await res.json();
const logs = body.logs || [];

const pct = (n, d) => (d > 0 ? `${((100 * n) / d).toFixed(1)}%` : '—');
const compact = (n) => (n >= 1e9 ? `${(n / 1e9).toFixed(2)}B` : n >= 1e6 ? `${(n / 1e6).toFixed(2)}M` : n >= 1e3 ? `${(n / 1e3).toFixed(1)}K` : String(n));

// 只看真正用掉了 prompt token 的 200 请求：其它响应没有 usage，算比例没有意义。
const ok = logs.filter((l) => l.status === 200 && (l.tokens_in || 0) > 0);

const withAffinity = ok.filter((l) => l.affinity);
const withPrefix = ok.filter((l) => l.prefix);
const withAffinityOnly = withAffinity;
if (withAffinity.length === 0) {
  console.log(`样本 ${ok.length} 条 200，但没有任何 affinity 字段。`);
  console.log('→ 说明线上跑的还是「未包含 §94 改动」的版本，先部署再来。');
  process.exit(0);
}

// 按 affinity 分组：请求数、前缀数、各前缀的请求数、读命中 token。
//
// 只统计**带 affinity 字段**的请求：部署前的老日志没有这两个字段，混进来会被
// 当成一个巨大的「前缀稳定」会话，把结论带偏（实测会把 churn 直接压成 0）。
const groups = new Map();
for (const l of withAffinityOnly) {
  const key = l.affinity || '(无)';
  let g = groups.get(key);
  if (!g) {
    g = { key, reqs: 0, prefixes: new Map(), tokIn: 0, cached: 0 };
    groups.set(key, g);
  }
  g.reqs += 1;
  g.tokIn += l.tokens_in || 0;
  g.cached += l.cached_tokens || 0;
  const p = l.prefix || '(无)';
  g.prefixes.set(p, (g.prefixes.get(p) || 0) + 1);
}

const rows = [...groups.values()].map((g) => {
  const top = Math.max(...g.prefixes.values());
  return {
    key: g.key,
    reqs: g.reqs,
    prefixes: g.prefixes.size,
    // churn = 1 − 最常见前缀的占比：0 表示前缀从头到尾没变过。
    churn: 1 - top / g.reqs,
    tokIn: g.tokIn,
    hit: g.tokIn > 0 ? g.cached / g.tokIn : 0,
  };
});

const multi = rows.filter((r) => r.prefixes > 1);
const multiReqs = multi.reduce((a, r) => a + r.reqs, 0);
const multiTok = multi.reduce((a, r) => a + r.tokIn, 0);
const multiCached = multi.reduce((a, r) => a + Math.round(r.hit * r.tokIn), 0);
const stable = rows.filter((r) => r.prefixes === 1);
const stableTok = stable.reduce((a, r) => a + r.tokIn, 0);
const stableCached = stable.reduce((a, r) => a + Math.round(r.hit * r.tokIn), 0);

const totalTok = ok.reduce((a, l) => a + (l.tokens_in || 0), 0);
const totalCached = ok.reduce((a, l) => a + (l.cached_tokens || 0), 0);
const legacy = ok.length - withAffinity.length;

console.log(`样本窗口: ${new Date(logs[0]?.time * 1000 || 0).toISOString()} ~ ${new Date(logs.at(-1)?.time * 1000 || 0).toISOString()}`);
console.log(`200 且带 usage: ${ok.length} 条 · 带 affinity ${withAffinity.length} · 带 prefix ${withPrefix.length}`);
console.log(`整体命中率: ${pct(totalCached, totalTok)}（${compact(totalCached)} / ${compact(totalTok)}）`);
if (legacy > 0) console.log(`（其中 ${legacy} 条是部署前的旧日志，没有指纹字段，已从分组统计里剔除）`);
console.log('');

console.log(`参与分组的请求: ${withAffinityOnly.length} 条（${withAffinityOnly.reduce((a, l) => a + (l.tokens_in || 0), 0) > 0 ? compact(withAffinityOnly.reduce((a, l) => a + (l.tokens_in || 0), 0)) : 0} token）`);
console.log(`前缀稳定的会话: ${stable.length} 个，${stable.reduce((a, r) => a + r.reqs, 0)} 条请求，命中 ${pct(stableCached, stableTok)}`);
console.log(`前缀在变的会话: ${multi.length} 个，${multiReqs} 条请求（${pct(multiReqs, ok.length)}），命中 ${pct(multiCached, multiTok)}`);
console.log('');

if (multi.length) {
  console.log('前缀抖动最严重的会话（按请求数）:');
  console.log('  churn   请求   前缀数  命中率   affinity');
  for (const r of multi.sort((a, b) => b.reqs - a.reqs).slice(0, 10)) {
    console.log(`  ${pct(r.churn * 100, 100).padStart(6)}  ${String(r.reqs).padStart(4)}  ${String(r.prefixes).padStart(6)}  ${pct(r.hit * 100, 100).padStart(6)}   ${r.key}`);
  }
  console.log('');
  console.log('判读: churn 高 → 客户端每轮在改 system / tools（客户端侧修）；');
  console.log('      前缀只有 1 个但命中率仍然低 → 换号或上游 TTL 过期（网关/网络侧）。');
} else {
  console.log('没有发现前缀抖动：同一 affinity 下只有一个 prefix。');
  console.log('那么命中率的缺口来自「冷启动 / TTL 过期 / 换号」，不是客户端重写前缀。');
}
