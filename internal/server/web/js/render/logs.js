// 请求日志渲染。

import { $, cell, row, pill, emptyRow } from '../dom.js';
import { t, translateDom } from '../i18n.js';
import { fmtTime, fmtNum, fmtCost, fmtCompact } from '../format.js';
import { sortCopy, sortState } from '../table.js';

// logSortGetters 是日志表的排序取值（键必须与表头 data-sort 一致）。
var logSortGetters = {
  time: function(l){ return l.time || 0; },
  endpoint: function(l){ return l.endpoint || ""; },
  model: function(l){ return l.model || ""; },
  stream: function(l){ return l.stream ? 1 : 0; },
  status: function(l){ return l.status || 0; },
  latency: function(l){ return l.latency_ms || 0; },
  messages: function(l){ return l.messages || 0; },
  chars: function(l){ return (l.prompt_len || 0) + (l.output_len || 0); },
  tokens: function(l){ return (l.tokens_in || 0) + (l.tokens_out || 0); },
  cost: function(l){ return l.cost || 0; },
  account: function(l){ return l.account || ""; },
  err: function(l){ return l.err || ""; }
};

export function renderLogs(logs){
  var tb = $("#logs");
  tb.textContent = "";
  var list = logs || [];
  if (!list.length) { tb.appendChild(emptyRow(12, "暂无日志")); return; }
  if (sortState.logs.key) {
    list = sortCopy(list, sortState.logs, logSortGetters);
  } else {
    list = list.slice().reverse(); // 默认最新在前
  }
  list.forEach(function(l){ tb.appendChild(logRow(l)); });
  translateDom(tb);
}

// statusCell 画状态胶囊。
//
// 499 = 客户端先走了（后端 store.StatusClientGone）——既不是成功也不是上游
// 失败，用 warn 色区分，别把「用户关标签页」看成网关报错。
function statusCell(l){
  var statusCls = l.status >= 400 ? "err" : "ok";
  if (l.status === 499) statusCls = "warn";
  var st = cell("");
  st.appendChild(pill(l.status, statusCls));
  return st;
}

// historyCell 画「回喂消息数」（带 sys 后缀 = 请求里带了系统提示）。
//
// 这个数列是上下文/失忆类 issue（#5）的第一现场：条数为 1 往往说明调用方只
// 发了本轮（典型是依赖 previous_response_id 续接，却走了无状态的 chat 合成
// 路径）；反之客户端发了 N 条而这里显示 1 条，就是网关把历史弄丢了。
function historyCell(l){
  var n = l.messages || 0;
  var c = cell(String(n) + (l.system ? " · sys" : ""), "mono muted");
  c.title = t("回喂消息数：{n}（{s}）", { n: fmtNum(n), s: l.system ? t("带系统提示") : t("无系统提示") });
  return c;
}

// countCell 画「入 / 出」两个计数（字符或 Token），精确值放 title。
function countCell(a, b){
  var c = cell(fmtCompact(a) + " / " + fmtCompact(b), "mono");
  c.title = fmtNum(a) + " / " + fmtNum(b);
  return c;
}

function logRow(l){
  var errCell = cell(l.err || "", "muted trunc");
  if (l.err) errCell.title = l.err;
  return row([
    cell(fmtTime(l.time), "muted"), cell(l.endpoint || "-", "mono muted"), cell(l.model, "mono"),
    cell(t(l.stream ? "是" : "否"), "mono"), statusCell(l), cell((l.latency_ms || 0) + "ms", "mono"),
    historyCell(l), countCell(l.prompt_len, l.output_len), countCell(l.tokens_in, l.tokens_out),
    cell(fmtCost(l.cost), "mono"),
    cell(l.account, "mono"), errCell
  ]);
}
