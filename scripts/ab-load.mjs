// 固定并发压测：输出一行 JSON（区分 200 / 429 容量拒绝 / 其它错误）。
//
// 用法: BASE=http://127.0.0.1:10087 KEY=sk-... CONC=28 TOTAL=56 node scripts/ab-load.mjs
// 说明: 与 docs/performance.md §14.5 的同名片段一致，改动时两边同步。
const BASE = process.env.BASE || 'http://127.0.0.1:10087';
const KEY = process.env.KEY;
const CONC = Number(process.env.CONC || 28);
const TOTAL = Number(process.env.TOTAL || 56);
const MODEL = process.env.MODEL || 'glm-5.3-flash';

const body = JSON.stringify({
  model: MODEL, stream: false, max_tokens: 16, temperature: 0,
  messages: [
    { role: 'system', content: 'You are a helpful AI assistant.' },
    { role: 'user', content: 'Reply with exactly: OK' },
  ],
});

async function one() {
  const t0 = performance.now();
  try {
    const r = await fetch(BASE + '/v1/chat/completions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + KEY },
      body, signal: AbortSignal.timeout(180000),
    });
    const text = await r.text();
    let j = null; try { j = JSON.parse(text); } catch {}
    return { status: r.status, ms: performance.now() - t0, type: j?.error?.type || '' };
  } catch {
    return { status: 0, ms: performance.now() - t0, type: 'client_error' };
  }
}

const t0 = performance.now();
let next = 0;
const out = [];
const workers = [];
for (let w = 0; w < Math.min(CONC, TOTAL); w++) {
  workers.push((async () => {
    for (;;) {
      const i = next++;
      if (i >= TOTAL) return;
      out.push(await one());
    }
  })());
}
await Promise.all(workers);
const wall = performance.now() - t0;

const ms = out.map(r => r.ms).sort((a, b) => a - b);
const q = p => ms.length ? Math.round(ms[Math.min(ms.length - 1, Math.floor(ms.length * p))]) : 0;
const byStatus = {}, byType = {};
for (const r of out) {
  byStatus[r.status] = (byStatus[r.status] || 0) + 1;
  if (r.status !== 200) byType[`${r.status} ${r.type || '-'}`] = (byType[`${r.status} ${r.type || '-'}`] || 0) + 1;
}
console.log(JSON.stringify({
  conc: CONC, total: TOTAL, wall_s: +(wall / 1000).toFixed(1),
  rps: +(out.length / (wall / 1000)).toFixed(2), ok: byStatus[200] || 0,
  byStatus, p50: q(0.5), p95: q(0.95), p99: q(0.99), errors: byType,
}));
