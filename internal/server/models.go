package server

import (
	"deepaicode2api/internal/deepai"
	"deepaicode2api/internal/registry"
	"fmt"
	"strings"
)

// catalog 返回当前模型目录；目录为空时回落到内置兜底模型，
// 便于首次启动（尚未拉到上游目录）时仍能应答。
func (s *Server) catalog() []deepai.Model {
	catalog := s.Store.Models()
	if len(catalog) == 0 {
		return []deepai.Model{deepai.DefaultModelEntry()}
	}
	return catalog
}

// resolveModel 用当前目录 + 别名解析请求模型（宽松：解析不到时回落，永不失败）。
func (s *Server) resolveModel(name string) deepai.Model {
	m, _ := s.resolveModelOK(name)
	return m
}

// resolveModelOK 解析模型名，并报告它是否“有依据”地命中目录。
//
// 顺序：精确 ID → 自定义别名 → 内置别名 → 模型名称 → （非严格模式）子串
// → 默认模型 / 目录首项。ok=false 只会在严格模式下出现。
//
// 严格模式（Settings.StrictModel）的存在意义：宽松回落会把拼写错误
// 静默换成一个完全不同的模型（例如 gpt-6-sol → glm-5.3-flash），
// 调用方会以为自己测的是 A，实际跑的是 B。
func (s *Server) resolveModelOK(name string) (deepai.Model, bool) {
	catalog := s.catalog()
	settings := s.Store.Settings()
	def := s.defaultModelID()
	if m, ok := registry.ResolveStrict(catalog, settings.ModelAliases, name, def); ok {
		return m, true
	}
	if settings.StrictModel {
		return deepai.Model{}, false
	}
	return registry.Resolve(catalog, settings.ModelAliases, name, def), true
}

// defaultModelID 返回回落目标：设置里的兜底模型优先，否则目录默认。
//
// 目录默认空串时会再退一步取目录首项：否则「默认模型」会显示为空，
// 而实际推理已经回落到目录首项（registry.Resolve 的第 37 行），两者不一致。
func (s *Server) defaultModelID() string {
	catalog := s.catalog()
	settings := s.Store.Settings()
	def := strings.TrimSpace(s.Store.DefaultModel())
	if def == "" && len(catalog) > 0 {
		def = catalog[0].ID
	}
	if settings.FallbackModel == "" {
		return def
	}
	if m, ok := registry.Find(catalog, settings.FallbackModel); ok {
		return m.ID
	}
	return settings.FallbackModel
}

// unknownModelMessage 生成严格模式下的可执行错误信息。
//
// 只靠一句“模型不存在”会让调用方无从下手；带上近似候选（gpt-6-sol 附近
// 就是 gpt-6.1-sol）才能一眼看出是拼写还是渠道不支持。
func (s *Server) unknownModelMessage(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "(空)"
	}
	msg := fmt.Sprintf("模型 %q 不在上游目录中（严格模式已开启，不做静默回落）", name)
	if picks := registry.Suggest(s.catalog(), name, 6); len(picks) > 0 {
		msg += "；相近的模型：" + strings.Join(picks, "、")
	}
	msg += "。可用模型见 GET /v1/models；也可在控制台「模型别名」里把该名称映射到目录模型。"
	return msg
}

// adminModelViews 把目录条目转成控制台下发的视图。
//
// 比裸 deepai.Model 只多两件事：
//   - 补 free：与 Model.Free() **同源**。前端自己算一套口径，
//     一旦两边判断不一致，控制台就会说免费而实际计费（或反之）；
//   - 显式带出 theosPerSecond：它的 JSON tag 是 omitempty，为空时前端拿不到键。
func adminModelViews(models []deepai.Model) []map[string]any {
	out := make([]map[string]any, 0, len(models))
	for _, m := range models {
		out = append(out, map[string]any{
			"id":              m.ID,
			"name":            m.Name,
			"reasoning":       m.Reasoning,
			"input":           m.Input,
			"contextWindow":   m.ContextWindow,
			"maxTokens":       m.MaxTokens,
			"premier":         m.Premier,
			"api":             m.API,
			"theosPerRequest": m.TheosPerRequest,
			"theosPerSecond":  m.TheosPerSecond,
			"free":            m.Free(),
		})
	}
	return out
}
