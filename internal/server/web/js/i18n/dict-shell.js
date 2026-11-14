// i18n 字典：外壳（导航 / 顶栏 / 页头 / 面板 / 按钮 / 筛选 / 输入 / 下拉）。
//
// 键是中文原文；拆分只为可读性，运行期由 core.js 合并成一张 I18N_EN。
// 缺词条由 console_i18n_test.go 的覆盖率棘轮兜底。
export const SHELL_DICT = {
  // 导航 / 品牌
  "概览":"Overview", "账号池":"Accounts", "密钥":"API Keys",
  "日志":"Logs", "设置":"Settings", "控制台":"Console",
  // 顶栏
  "刷新":"Refresh", "退出":"Sign out",
  "收起 / 展开侧边栏":"Collapse / expand sidebar",
  "退出登录（清除本标签页的口令）":"Sign out (clears this tab's password)",
  // 页头
  "号池健康、用量统计与上游模型目录":"Pool health, usage and upstream model catalog",
  "刷新模型目录":"Refresh models", "每秒计费":"Per second",
  "上游账号、配额与调度状态":"Upstream accounts, quota and scheduling state",
  "导入本机桌面端":"Import desktop creds",
  "浏览器登录":"Browser sign-in",
  "刷新全部配额":"Refresh all quota",
  "网关对外签发的调用凭据，可绑定号池分组":"Gateway client keys, optionally bound to a pool group",
  "只记录元信息，不落对话内容":"Metadata only, no conversation content",
  "清空":"Clear",
  "运行参数、模型别名与管理密码":"Runtime options, model aliases and admin password",
  "保存设置":"Save",
  "重置统计":"Reset stats",
  // 面板
  "模型目录":"Model catalog", "手动添加":"Add manually", "账号列表":"Account list",
  "密钥列表":"Key list", "新建密钥":"New key", "运行参数":"Runtime options",
  // 按钮
  "添加 API Key":"Add API key", "创建密钥":"Create key", "上一页":"Prev", "下一页":"Next",
  "取消":"Cancel", "打开 DeepAI 登录页":"Open DeepAI sign-in",
  "配额":"Quota", "停用":"Disable", "启用":"Enable", "清错":"Clear errors", "删除":"Delete",
  "操作":"Actions", "配额刷新":"Quota refresh",
  // 筛选 / 行内治理
  "筛选":"Filter", "重置":"Reset", "分组（留空=全部）":"Group (empty = all)",
  "全部状态":"All statuses", "搜索邮箱 / 名称 / Key / 备注":"Search email / name / key / note",
  "全部端点":"All endpoints", "状态 ≥":"Status ≥", "关键字（错误 / 模型）":"Keyword (error / model)",
  "起始时间":"Since",
  "补绑代理":"Assign proxy", "重绑代理":"Reassign proxy", "清除代理":"Clear proxy",
  "分配机器指纹":"Assign machine ID", "改分组":"Change group", "标记已验证":"Mark verified",
  "新分组名":"New group name", "分组不能为空":"Group cannot be empty",
  "密钥绑定的分组（留空=全部）":"Key group (empty = all)",
  // 代理 / 机器指纹弹窗的结构化字段（“改前 → 改后”）
  "当前出口":"Current exit", "新出口":"New exit", "直连":"Direct",
  "代理池随机分配":"Random pick from proxy pool", "重新分配":"Reassign", "分配":"Assign",
  "当前机器":"Current machine", "新机器":"New machine", "生成独立指纹":"Generate dedicated ID",
  "换出口后，该账号的后续请求都会走新代理（旧出口不再使用）。":"Once switched, this account's requests use the new proxy (the previous exit is dropped).",
  "分配后，该账号的后续请求都会走代理。":"After assigning, this account's requests go through the proxy.",
  "清除后该账号回到直连，不再经过代理池。":"After clearing, this account connects directly and no longer uses the proxy pool.",
  "只补空值，不覆盖已有指纹。":"Only fills empty values; existing IDs are never overwritten.",
  "把 {name} 标记为邮箱已验证？":"Mark {name} as email-verified?",
  // 输入占位 / 标签
  "账号名称（可选）":"Account name (optional)",
  "API Key（DeepAI 账号密钥）":"API key (DeepAI account key)",
  "密钥备注（可选）":"Key note (optional)", "分组（默认 default）":"Group (default)",
  "每页":"Per page",
  "号池分组":"Pool group",
  "请求超时（秒）":"Request timeout (s)", "最大重试次数":"Max retries",
  "日志保留条数":"Log keep", "CORS 允许来源（逗号分隔，留空=仅本机）":"CORS origins (comma separated; empty = loopback only)",
  "兜底模型（无法识别的模型名回落到它，留空=目录默认）":"Fallback model (empty = catalog default)",
  "管理密码（留空则保持；设置后管理接口必须携带）":"Admin password (empty = keep current)",
  "不修改请留空":"Leave empty to keep",
  // 下拉项
  "自动":"Auto",
  "最近 50":"Last 50", "最近 200":"Last 200", "全部":"All",
};
