package server

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestConsoleSmoke 跑 scripts/console-smoke.mjs：用极简 DOM 桩把 web/js/*.js
// 真正执行一遍（import 即触发全部顶层接线），并给四个渲染函数喂固定数据。
//
// 存在的理由：控制台是纯前端，Go 测试只能断言「字符串存在」。拆模块时最容易
// 出的是引用类断裂——文件都在、语法都对、`node --check` 也过（它按 CJS 解析，
// 会放过 ESM 的顶层 return），点一下按钮才炸。本轮拆分中它抓到了三个这种问题：
//   - app.js 里残留 `lang = …`（模块私有变量被直接赋值）；
//   - 重复 import 同一个符号；
//   - 抽屉段删掉 IIFE 外壳后留下顶层 `return`。
//
// 未安装 node 时跳过：CI（ubuntu-latest 自带 Node）会真跑，本地按需装。
func TestConsoleSmoke(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("未安装 node，跳过控制台冒烟测试")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("解析仓库根目录: %v", err)
	}
	cmd := exec.Command(node, filepath.Join("scripts", "console-smoke.mjs"))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("控制台冒烟失败：%v\n%s", err, out)
	}
	t.Logf("控制台冒烟通过：\n%s", out)
}
