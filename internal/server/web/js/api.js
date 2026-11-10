// 管理接口调用层：统一鉴权头、错误归一、同路径 GET 去重。

import { $ } from './dom.js';

const inflight = {};

export function pw(){ return $("#pw").value.trim(); }

export function api(path, opts){
  opts = opts || {};
  var method = opts.method || "GET";
  if (method === "GET" && inflight[path]) return inflight[path];

  var h = { "Content-Type":"application/json" };
  if (pw()) h["X-Admin-Password"] = pw();
  var p = fetch(path, {
    method: method,
    headers: h,
    body: opts.body ? JSON.stringify(opts.body) : undefined
  }).then(function(r){
    return r.text().then(function(txt){
      var j = null;
      try { j = JSON.parse(txt); } catch {}
      if (!r.ok) {
        var m = (j && j.error && j.error.message) || txt || ("HTTP " + r.status);
        var err = new Error(m); err.status = r.status; throw err;
      }
      return j;
    });
  });

  if (method === "GET") {
    inflight[path] = p;
    var done = function(){ delete inflight[path]; };
    p.then(done, done);
  }
  return p;
}
