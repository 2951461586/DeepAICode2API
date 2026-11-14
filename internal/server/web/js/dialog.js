// 控制台对话框：用原生 <dialog> 替代 window.confirm / window.prompt。
//
// 为什么换掉：
//   - window.prompt 在部分环境（iOS 独立窗口、部分内嵌浏览器）被禁用或样式不可控；
//   - 原生弹窗无法做 i18n 按钮、危险色、ESC/焦点管理，也不可测试；
//   - 阻断式弹窗会冻结页面（后台轮询、日志实时刷新全停）。
//
// 本模块零依赖（除 i18n），返回 Promise：
//   confirmDialog({...}) -> Promise<boolean>
//   promptDialog({...}) -> Promise<string|null>
//
// 结构固定为 dialog > form(method=dialog)：回车提交、ESC 关闭都走浏览器原生行为，
// 只需在 close 时读取 returnValue。

import { t } from './i18n.js';

let active = null; // 当前打开的 dialog 节点（同一时刻只允许一个）

function mk(tag, cls, text) {
  var el = document.createElement(tag);
  if (cls) el.className = cls;
  if (text !== undefined && text !== null) el.textContent = text;
  return el;
}

function closeActive() {
  if (!active) return;
  var el = active;
  active = null;
  if (el.open) el.close("");
  if (el.parentNode) el.parentNode.removeChild(el);
}

// openDialog 是 confirm / prompt 的公共实现。
function openDialog(opts) {
  closeActive();
  opts = opts || {};

  var dialog = mk("dialog", "dialog");
  var form = mk("form", "dialog-card");
  form.method = "dialog";

  var title = mk("h2", "dialog-title", opts.title || "");
  form.appendChild(title);

  if (opts.message) {
    form.appendChild(mk("p", "dialog-message", opts.message));
  }

  // rows 是「改前 / 改后」这类结构化预览：账号、当前出口、新出口……
  // 塞进 message 里拼句子既不好读、也没法对齐等宽值（代理 host 很长），
  // 更没法在窄屏上换行。label 走 t()，value 原样（已经是脱敏过的展示值）。
  if (opts.rows && opts.rows.length) {
    var list = mk("dl", "dialog-rows");
    opts.rows.forEach(function (r) {
      if (!r) return;
      var item = mk("div", "dialog-row");
      item.appendChild(mk("dt", null, r.label || ""));
      var dd = mk("dd", r.mono === false ? null : "mono", r.value === undefined ? "" : String(r.value));
      if (r.title) dd.title = r.title;
      item.appendChild(dd);
      // append 而不是 appendChild：本仓 lint 规则偏好前者。
      list.append(item);
    });
    form.append(list);
  }

  var input = null;
  if (opts.input) {
    var field = mk("label", "dialog-field");
    if (opts.label) field.appendChild(mk("span", null, opts.label));
    input = mk("input", "dialog-input");
    input.type = "text";
    input.value = opts.value || "";
    input.placeholder = opts.placeholder || "";
    input.autocomplete = "off";
    field.appendChild(input);
    form.appendChild(field);
  }

  var actions = mk("div", "dialog-actions");
  var cancel = mk("button", "btn", opts.cancelText || t("取消"));
  cancel.type = "button";
  var ok = mk("button", "btn " + (opts.danger ? "danger" : "primary"), opts.okText || t("确定"));
  ok.type = "submit";
  actions.appendChild(cancel);
  actions.appendChild(ok);
  form.appendChild(actions);

  dialog.appendChild(form);
  document.body.appendChild(dialog);
  active = dialog;

  return new Promise(function (resolve) {
    var settled = false;
    function finish(value) {
      if (settled) return;
      settled = true;
      closeActive();
      resolve(value);
    }

    cancel.addEventListener("click", function () { finish(opts.input ? null : false); });
    dialog.addEventListener("cancel", function (ev) {
      // 原生 ESC：prompt 返回 null、confirm 返回 false。
      ev.preventDefault();
      finish(opts.input ? null : false);
    });
    form.addEventListener("submit", function (ev) {
      ev.preventDefault();
      finish(opts.input ? String(input.value) : true);
    });
    dialog.showModal();
    if (input) {
      input.focus();
      input.select();
    } else {
      ok.focus();
    }
  });
}

export function confirmDialog(opts) {
  return openDialog(opts);
}

export function promptDialog(opts) {
  return openDialog(Object.assign({}, opts, { input: true }));
}
