// 账号 / 密钥的变更操作与配额探测。
//
// 这些操作成功后要让控制台重新拉数据，但「刷新」属于 app 的职责：
// 这里用 setReload 注入回调，避免 actions.js ⇄ app.js 循环导入。

import { api } from './api.js';
import { toast } from './ui.js';

// 由 app.js 在启动时注入（通常是 refresh）。
//
// 默认值必须是「可调用的空实现」而不是裸 identifiers：拆分前这里写的是裸
// `refresh()`，模块里根本没有这个绑定，任何操作成功后的收尾都会抛
// `refresh is not a function`（被 .catch 吃掉，错误提示还会伪装成「失败」）。
let reload = function () { return Promise.resolve(); };
export function setReload(fn) { if (typeof fn === "function") reload = fn; }

export function accountOp(action, extra){
  var body = { action: action };
  if (extra) for (var k in extra) body[k] = extra[k];
  return api("/admin/api/accounts", {method:"POST", body:body}).then(function(){
    toast("操作成功", "ok");
    return reload();
  }).catch(function(e){ toast("失败：" + e.message, "err"); });
}

export function keyOp(kind, key){
  return api("/admin/api/keys?" + kind + "=" + encodeURIComponent(key), {method:"DELETE"})
    .then(function(){ toast("操作成功","ok"); return reload(); })
    .catch(function(e){
      toast("失败：" + e.message, "err");
      // 404 = 这把密钥已经不在了（另一个标签页删过 / 引用已失效）。重拉一次让列表
      // 和真实状态对齐，而不是留一行怎么点都 404 的幽灵行。
      if (e && e.status === 404) return reload();
    });
}

// keySetGroup 是「改绑分组」：同样是变更 + 重拉，放在这里让 render 层不必
// 自己发请求（render/*.js 是纯绘制层，且拿不到注入的 reload）。
export function keySetGroup(key, group){
  return api("/admin/api/keys", {method:"POST", body:{action:"set-group", key:key, group:group}})
    .then(function(){ toast("操作成功", "ok"); return reload(); })
    .catch(function(e){ toast("失败：" + e.message, "err"); });
}

export function checkin(id){
  if (id) {
    toast("正在查询上游配额…");
    return api("/admin/api/checkin", {method:"POST", body:{id:id}}).then(function(r){
      return finishCheckin(r.probed || 0, r.results || []);
    }).catch(function(e){ toast("查询失败：" + e.message, "err"); });
  }
  toast("正在查询上游配额…");
  var done = 0, bad = [], rounds = 0;
  function step(){
    rounds++;
    return api("/admin/api/checkin", {method:"POST", body:{limit:100}}).then(function(r){
      done += r.probed || 0;
      bad = bad.concat((r.results || []).filter(function(x){ return !x.ok; }));
      // remaining 归零或到了轮数上限就停；上限防上游全挂时无限循环。
      if (r.remaining > 0 && rounds < 30) return step();
      return finishCheckin(done, bad);
    });
  }
  return step().catch(function(e){ toast("查询失败：" + e.message, "err"); });
}

export function finishCheckin(done, bad){
  toast("配额刷新完成：" + done + " 个账号，失败 " + bad.length +
    (bad.length ? "\n" + bad.slice(0, 5).map(function(x){ return x.name + ": " + x.err; }).join("\n") : ""),
    bad.length ? "err" : "ok");
  return reload();
}
