package server

import (
	"time"
)

// admissionWait / admissionPoll 是「池里暂时没有空槽」时的排队上限与轮询间隔。
//
// 以前槽位不够就立刻 429（见 docs/performance.md §14：14 号 / 28 并发时
// 成功率只有 48~70%）。短暂排队能把突发流量摊平，又不至于把调用方挂死。
const (
	admissionWait = 2 * time.Second
	admissionPoll = 50 * time.Millisecond
)
