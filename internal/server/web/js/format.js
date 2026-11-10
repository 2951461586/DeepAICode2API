// 纯格式化函数：时间 / 数字 / 成本 / 量级压缩 / 配额文本。
//
// 这一组不碰 DOM、不读全局状态，输入字符串或数字，输出可显示文本，
// 因此可以脱离浏览器单独测（把函数体复制进 node 就能跑）。

export function fmtTime(s){
  if (!s) return "—";
  var d = new Date(s * 1000);
  var p = function(n){ return (n<10?"0":"")+n; };
  return d.getFullYear()+"-"+p(d.getMonth()+1)+"-"+p(d.getDate())+" "+p(d.getHours())+":"+p(d.getMinutes())+":"+p(d.getSeconds());
}
export function fmtNum(n){
  if (n === undefined || n === null) return "0";
  return Number(n).toLocaleString("en-US");
}
export function fmtCost(n){
  if (!n) return "$0";
  return "$" + Number(n).toFixed(6);
}
// fmtCompact 把大数字压成 K/M/B（3 位有效数字），用于 Token / 字符这类
// 量级很大、不需要逐位精确的列。精确值放 title 里随悬停可见。
export function fmtCompact(n){
  var v = Number(n);
  if (!isFinite(v)) return "0";
  var abs = Math.abs(v);
  if (abs < 1000) return String(v);
  var units = [[1e9,"B"],[1e6,"M"],[1e3,"K"]];
  for (var i = 0; i < units.length; i++){
    var d = units[i][0];
    if (abs < d) continue;
    var x = v / d;
    // 3 位有效数字；进位到 1000 时升一级单位（999999 → 1M，而不是 1000K）。
    if (i > 0 && Math.abs(Number(x.toPrecision(3))) >= 1000) {
      d = units[i-1][0];
      return Number((v / d).toPrecision(3)).toString() + units[i-1][1];
    }
    return Number(x.toPrecision(3)).toString() + units[i][1];
  }
  return String(v);
}
export function theosText(al){
  if (!al) return "—";
  var v = al.balance || al.granted || "0";
  return v;
}

// perSecondText 把上游的「按秒计费」对象压成一行可读文本。
//
// 形态是 map（键是计费档位，值是每秒 Theos）。空对象返回 "—"，
// 与其它空列一致；只有一个档位时只显示数值，避免 "standard: 0.5" 这种噪声。
export function perSecondText(m) {
  const sec = m && m.theosPerSecond;
  if (!sec) return '—';
  const keys = Object.keys(sec);
  if (keys.length === 0) return '—';
  if (keys.length === 1) return String(sec[keys[0]]);
  return keys.map((k) => k + ': ' + sec[k]).join(' · ');
}
