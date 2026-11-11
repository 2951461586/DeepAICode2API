// 表头排序的共享状态与纯函数。
//
// accounts 走服务端分页，排序必须下推到后端；其余小表数据量小，前端排即可。

export const sortState = {
  accounts: { key: "", desc: false },
  models:   { key: "", desc: false },
  keys:     { key: "", desc: false },
  logs:     { key: "", desc: false }
};

export function markSort(tableSel, st){
  document.querySelectorAll(tableSel + " th[data-sort]").forEach(function(th){
    var on = th.getAttribute("data-sort") === st.key;
    th.classList.toggle("asc", on && !st.desc);
    th.classList.toggle("desc", on && st.desc);
    // 屏幕阅读器只认 aria-sort，看不到 class。
    if (on) th.setAttribute("aria-sort", st.desc ? "descending" : "ascending");
    else th.removeAttribute("aria-sort");
  });
}

export function toggleSort(name, key){
  var st = sortState[name];
  if (st.key === key) st.desc = !st.desc;
  else { st.key = key; st.desc = false; }
  return st;
}

export function cmpVal(a, b){
  if (a < b) return -1;
  if (a > b) return 1;
  return 0;
}

export function sortCopy(list, st, getters){
  var g = getters[st.key];
  if (!g || !st.key) return list.slice();
  return list.slice().sort(function(x, y){
    var c = cmpVal(g(x), g(y));
    return st.desc ? -c : c;
  });
}
