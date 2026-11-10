// 网关密钥渲染。

import { $, cell, row, pill, emptyRow, menuToggle, openMenu } from '../dom.js';
import { t, translateDom } from '../i18n.js';
import { fmtTime } from '../format.js';
import { sortCopy, sortState } from '../table.js';
import { api } from '../api.js';
import { keyOp } from '../actions.js';
import { toast } from '../ui.js';

export function keyMenu(k){
  var m = menuToggle("操作");
  m.addEventListener("click", function(ev){
    ev.stopPropagation();
    openMenu(m, [
      { label: k.enabled ? "停用" : "启用", fn: function(){ keyOp("toggle", k.key); } },
      { label: "改绑分组", fn: function(){
        var g = prompt(t("密钥绑定的分组（留空=全部）"), k.group || "");
        if (g === null) return;
        // 列表里是脱敏 Key；服务端 resolveKeyRef 能认出掩码形式。
        api("/admin/api/keys", {method:"POST", body:{action:"set-group", key:k.key, group:String(g).trim()}})
          .then(function(){ toast("操作成功", "ok"); return refresh(); })
          .catch(function(e){ toast("失败：" + e.message, "err"); });
      }},
      { sep: true },
      { label: "删除", danger: true, fn: function(){
        if (confirm(t("确定删除该密钥？"))) keyOp("key", k.key);
      }}
    ]);
  });
  return m;
}

export function renderKeys(d){
  var tb = $("#keys");
  tb.textContent = "";
  var keys = sortCopy(d.keys || [], sortState.keys, {
    key: function(k){ return k.key || ""; },
    name: function(k){ return k.name || ""; },
    group: function(k){ return k.group || ""; },
    enabled: function(k){ return k.enabled ? 1 : 0; },
    requests: function(k){ return k.requests || 0; },
    created: function(k){ return k.created_at || 0; },
    last_used: function(k){ return k.last_used || 0; }
  });
  if (!keys.length) {
    tb.appendChild(emptyRow(8, "暂无密钥（未签发时 /v1 允许无密钥调用）"));
    return;
  }
  keys.forEach(function(k){
    var st = cell("");
    st.appendChild(pill(k.enabled ? "启用" : "停用", k.enabled ? "ok" : ""));
    var ops = cell("");
    ops.appendChild(keyMenu(k));
    tb.appendChild(row([
      cell(k.key, "mono"), cell(k.name), cell(k.group || t("全部"), "mono"), st,
      cell(k.requests, "mono"), cell(fmtTime(k.created_at), "muted"),
      cell(fmtTime(k.last_used), "muted"), ops
    ]));
  });
  translateDom(tb);
}
