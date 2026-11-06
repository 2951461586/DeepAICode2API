package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errBoom = errors.New("boom")

func alwaysRetryable(error) bool { return true }

func TestDoSucceedsFirstTry(t *testing.T) {
	calls := 0
	err := Do(t.Context(), []time.Duration{time.Second}, nil, func(context.Context) error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d，期望一次成功", err, calls)
	}
}

func TestDoRetriesThenSucceeds(t *testing.T) {
	calls := 0
	err := Do(t.Context(), []time.Duration{0, 0}, alwaysRetryable, func(context.Context) error {
		calls++
		if calls < 3 {
			return errBoom
		}
		return nil
	})
	if err != nil {
		t.Fatalf("第 3 次应成功，实际 %v", err)
	}
	if calls != 3 {
		t.Fatalf("调用次数 = %d，期望 3", calls)
	}
}

// 不可重试的错误必须立刻返回，不能白等退避。
func TestDoStopsOnNonRetryable(t *testing.T) {
	calls := 0
	start := time.Now()
	err := Do(t.Context(), []time.Duration{time.Hour}, func(error) bool { return false },
		func(context.Context) error {
			calls++
			return errBoom
		})
	if !errors.Is(err, errBoom) {
		t.Fatalf("错误应原样返回，实际 %v", err)
	}
	if calls != 1 {
		t.Fatalf("调用次数 = %d，期望 1", calls)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("不应等待退避，实际耗时 %v", elapsed)
	}
}

// 退避用尽后返回最后一次错误。
func TestDoReturnsLastErrorAfterExhausted(t *testing.T) {
	calls := 0
	err := Do(t.Context(), []time.Duration{0, 0}, alwaysRetryable, func(context.Context) error {
		calls++
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("应返回最后一次错误，实际 %v", err)
	}
	if calls != 3 {
		t.Fatalf("1 次首发 + 2 次重试 = 3，实际 %d", calls)
	}
}

// ctx 已取消时不得再发起新尝试，并返回已有错误。
//
// 用 1 小时的退避而不是 sleep 竞态：ctx 先行取消后，首次尝试仍会执行，
// 随后的退避必然被 ctx 打断，结果是确定性的。
func TestDoAbortsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	calls := 0
	err := Do(ctx, []time.Duration{time.Hour}, alwaysRetryable, func(context.Context) error {
		calls++
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("取消后应返回已有错误，实际 %v", err)
	}
	if calls != 1 {
		t.Fatalf("取消后不应再尝试，实际 %d 次", calls)
	}
}

// delays 为空表示只尝试一次。
func TestDoSingleAttemptWithoutDelays(t *testing.T) {
	calls := 0
	err := Do(t.Context(), nil, alwaysRetryable, func(context.Context) error {
		calls++
		return errBoom
	})
	if !errors.Is(err, errBoom) || calls != 1 {
		t.Fatalf("err=%v calls=%d，期望只尝试一次", err, calls)
	}
}
