// 请求日志渲染。

import { $, cell, row, pill, emptyRow } from '../dom.js';
import { t, translateDom } from '../i18n.js';
import { fmtTime, fmtNum, fmtCost, fmtCompact } from '../format.js';
import { sortCopy, sortState } from '../table.js';

export function renderLogs(logs){
  var tb = $("#logs");
  tb.textContent = "";
  var list = logs || [];
  if (!list.length) { tb.appendChild(emptyRow(11, "暂无日志")); return; }
  if (sortState.logs.key) {
    list = sortCopy(list, sortState.logs, {
      time: function(l){ return l.time || 0; },
      endpoint: function(l){ return l.endpoint || ""; },
      model: function(l){ return l.model || ""; },
      stream: function(l){ return l.stream ? 1 : 0; },
      status: function(l){ return l.status || 0; },
      latency: function(l){ return l.latency_ms || 0; },
      chars: function(l){ return (l.prompt_len || 0) + (l.output_len || 0); },
      tokens: function(l){ return (l.tokens_in || 0) + (l.tokens_out || 0); },
      cost: function(l){ return l.cost || 0; },
      account: function(l){ return l.account || ""; },
      err: function(l){ return l.err || ""; }
    });
  } else {
    list = list.slice().reverse(); // 默认最新在前
  }
  list.forEach(function(l){
    var st = cell("");
    st.appendChild(pill(l.status, l.status >= 400 ? "err" : "ok"));
    var errCell = cell(l.err || "", "muted trunc");
    if (l.err) errCell.title = l.err;
    // 字符 / Token 用 K/M 压缩，精确值放 title。
    var charsCell = cell(fmtCompact(l.prompt_len) + " / " + fmtCompact(l.output_len), "mono");
    charsCell.title = fmtNum(l.prompt_len) + " / " + fmtNum(l.output_len);
    var tokCell = cell(fmtCompact(l.tokens_in) + " / " + fmtCompact(l.tokens_out), "mono");
    tokCell.title = fmtNum(l.tokens_in) + " / " + fmtNum(l.tokens_out);
    tb.appendChild(row([
      cell(fmtTime(l.time), "muted"), cell(l.endpoint || "-", "mono muted"), cell(l.model, "mono"),
      cell(t(l.stream ? "是" : "否"), "mono"), st, cell(l.latency_ms + "ms", "mono"),
      charsCell, tokCell,
      cell(fmtCost(l.cost), "mono"),
      cell(l.account, "mono"), errCell
    ]));
  });
  translateDom(tb);
}
