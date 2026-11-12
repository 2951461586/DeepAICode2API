// 用 CDP 驱动无头 Chrome，验证控制台的真实交互（不是只看截图）：
//   - 页面有没有 JS 异常 / console.error
//   - 账号表渲染出行
//   - 点「⋯」能弹出操作菜单
//   - 点表头会带上 sort/order 参数重新请求
//   - ?lang=en 会切换文案
// 用法：node scripts/console-e2e.mjs <baseURL> <shotDir>
const BASE = process.argv[2] || "http://127.0.0.1:10087";
const SHOT = process.argv[3] || "bin/shots";
const DEBUG = "http://127.0.0.1:9222";

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function targets() {
  const r = await fetch(DEBUG + "/json/list");
  return r.json();
}

function connect(wsUrl) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(wsUrl);
    ws.onopen = () => resolve(ws);
    ws.onerror = (e) => reject(e);
  });
}

let seq = 0;
function send(ws, method, params) {
  const id = ++seq;
  return new Promise((resolve) => {
    const onMsg = (ev) => {
      let m;
      try { m = JSON.parse(ev.data); } catch { return; }  // 非法帧直接忽略
      if (m.id === id) { ws.removeEventListener("message", onMsg); resolve(m.result); }
    };
    ws.addEventListener("message", onMsg);
    ws.send(JSON.stringify({ id, method, params: params || {} }));
  });
}

async function evaluate(ws, expr) {
  const r = await send(ws, "Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true });
  if (r && r.exceptionDetails) throw new Error(r.exceptionDetails.text || "eval error");
  return r && r.result ? r.result.value : undefined;
}

async function shot(ws, name) {
  const r = await send(ws, "Page.captureScreenshot", { format: "png" });
  const fs = await import("node:fs");
  fs.writeFileSync(`${SHOT}/${name}.png`, Buffer.from(r.data, "base64"));
  console.log("  shot:", name);
}

async function navigate(ws, url) {
  await send(ws, "Page.enable");
  const bust = (url.indexOf("?") >= 0 ? "&" : "?") + "cb=" + Date.now();
  await send(ws, "Page.navigate", { url: url.replace("#", bust + "#") });
  await sleep(2500);
}

const problems = [];
const requests = [];

(async () => {
  const list = await targets();
  const page = list.find((t) => t.type === "page");
  if (!page) throw new Error("没有可用的 page target");
  const ws = await connect(page.webSocketDebuggerUrl);

  ws.addEventListener("message", (ev) => {
    let m;
    try { m = JSON.parse(ev.data); } catch { return; }  // 非法帧直接忽略
    if (m.method === "Runtime.exceptionThrown") {
      const d = m.params.exceptionDetails;
      problems.push("exception: " + (d.exception?.description || d.text) +
        " @" + (d.url || "inline") + ":" + d.lineNumber + ":" + d.columnNumber);
    }
    if (m.method === "Runtime.consoleAPICalled" && m.params.type === "error") {
      problems.push("console.error: " + m.params.args.map((a) => a.value ?? a.description).join(" "));
    }
    if (m.method === "Network.requestWillBeSent" && m.params.request.url.includes("/admin/api/accounts")) {
      requests.push(m.params.request.url);
    }
  });

  await send(ws, "Runtime.enable");
  await send(ws, "Network.enable");
  await send(ws, "Network.setCacheDisabled", { cacheDisabled: true });
  await send(ws, "Log.enable");
  await send(ws, "Emulation.setDeviceMetricsOverride", { width: 1440, height: 900, deviceScaleFactor: 1, mobile: false });

  // ---- 中文页 ----
  await navigate(ws, BASE + "/#accounts");
  const rows = await evaluate(ws, `document.querySelectorAll("#accounts tr").length`);
  const navLabel = await evaluate(ws, `document.querySelector('nav.nav button[data-tab=overview]').textContent.trim()`);
  console.log("  rows:", rows, "| nav:", navLabel);

  // 打开第一行菜单
  const hasToggle = await evaluate(ws, `!!document.querySelector("#accounts .menu-toggle")`);
  if (!hasToggle) problems.push("账号行没有渲染出 ⋯ 菜单按钮");
  else await evaluate(ws, `document.querySelector("#accounts .menu-toggle").click()`);
  await sleep(300);
  const menuItems = await evaluate(ws, `Array.from(document.querySelectorAll(".menu .menu-item")).map(function(b){return b.textContent})`);
  console.log("  menu:", JSON.stringify(menuItems));
  if (!menuItems || menuItems.length === 0) problems.push("行内菜单没有渲染出条目");
  await shot(ws, "cdp-menu-zh");

  // 关闭菜单后点表头排序
  await evaluate(ws, `document.body.click()`);
  requests.length = 0;
  const hasTh = await evaluate(ws, `!!document.querySelector('#acctTable th[data-sort=theos]')`);
  if (!hasTh) problems.push("找不到 theos 排序表头");
  else await evaluate(ws, `document.querySelector('#acctTable th[data-sort=theos]').click()`);
  await sleep(1200);
  const sorted = requests.filter((u) => u.includes("sort=theos"));
  console.log("  sort request:", sorted.length ? sorted[sorted.length - 1].split("?")[1] : "(none)");
  if (!sorted.length) problems.push("点击表头没有发出带 sort=theos 的请求");
  const arrow = await evaluate(ws, `document.querySelector('#acctTable th[data-sort=theos]').className`);
  if (!String(arrow).includes("asc")) problems.push("表头排序指示未更新: " + arrow);

  // ---- 英文页 ----
  await navigate(ws, BASE + "/?lang=en#accounts");
  const enNav = await evaluate(ws, `document.querySelector('nav.nav button[data-tab=overview]').textContent.trim()`);
  const enTh = await evaluate(ws, `document.querySelector('#acctTable th[data-sort=account]').textContent.trim()`);
  const enKpi = await evaluate(ws, `document.querySelector("#kpis .k").textContent.trim()`);
  console.log("  en nav:", enNav, "| en th:", enTh, "| en kpi:", enKpi);
  if (enNav !== "Overview") problems.push("英文导航未生效: " + enNav);
  if (enTh !== "Account") problems.push("英文表头未生效: " + enTh);
  // KPI 标签是 JS 渲染出来的，最容易漏翻（静态检查看不出来）。
  if (enKpi !== "Total requests") problems.push("英文 KPI 标签未翻译: " + enKpi);
  await shot(ws, "cdp-accounts-en");

  // 英文概览
  await navigate(ws, BASE + "/?lang=en#overview");
  await shot(ws, "cdp-overview-en");

  ws.close();

  if (problems.length) {
    console.log("\n❌ 问题:");
    problems.forEach((p) => console.log("  -", p));
    process.exitCode = 1;
  } else {
    console.log("\n✅ 交互检查全部通过");
  }
})().catch((e) => { console.error("FAILED:", e.message); process.exitCode = 1; });
