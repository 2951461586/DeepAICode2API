#!/usr/bin/env python3
"""测量网关「从进程启动到 /health 可服务」的耗时。

用法: python scripts/measure-startup.py <binary> <port> <dataPath> [--upstream-origin URL]

上游指向不可达地址（TEST-NET）时，旧实现会在启动路径上同步拉模型目录、
等满超时才监听；新实现应当立刻可服务。
"""
import argparse
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request

DEFAULT_ORIGIN = "http://192.0.2.1:9"


def parse_args() -> argparse.Namespace:
    """argparse 负责校验参数：缺参数、端口非数字都会给出明确用法提示。"""
    ap = argparse.ArgumentParser(description="测量网关启动到 /health 可服务的耗时")
    ap.add_argument("binary", help="网关可执行文件路径")
    ap.add_argument("port", type=int, help="监听端口（需未被占用）")
    ap.add_argument("data_path", help="状态文件路径（会先删除以做冷启动）")
    ap.add_argument("--upstream-origin", default=DEFAULT_ORIGIN,
                    help=f"上游地址，默认不可达的 TEST-NET: {DEFAULT_ORIGIN}")
    return ap.parse_args()


def remove_stale(path: str) -> None:
    """删掉上次测量的残留状态文件（不存在就跳过）。"""
    try:
        os.remove(path)
    except FileNotFoundError:
        return


def wait_until_ready(url: str, deadline_seconds: float) -> float:
    """轮询 /health，返回首次 200 的秒数；超时返回 -1。"""
    started = time.perf_counter()
    deadline = time.time() + deadline_seconds
    last_error: Exception | None = None
    while time.time() < deadline:
        try:
            # url 恒为 http://127.0.0.1:<int>/health，协议与主机都由本脚本拼定。
            with urllib.request.urlopen(url, timeout=0.5) as resp:  # noqa: S310
                if resp.status == 200:
                    return time.perf_counter() - started
        except (urllib.error.URLError, OSError, TimeoutError) as exc:
            # 服务还没起来时连不上是预期内的，只记下最后一次原因供超时报告使用。
            last_error = exc
        time.sleep(0.05)
    if last_error is not None:
        print(f"last error: {last_error}", file=sys.stderr)
    return -1.0


def main() -> int:
    args = parse_args()
    binary, port, data_path, origin = args.binary, args.port, args.data_path, args.upstream_origin
    remove_stale(data_path)

    env = dict(os.environ)
    env["DEEPAI_API_ORIGIN"] = origin
    env["NO_COLOR"] = "1"

    started = time.perf_counter()
    proc = subprocess.Popen(
        [binary, "-port", str(port), "-no-import", "-data", data_path],
        env=env,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    url = f"http://127.0.0.1:{port}/health"
    try:
        ready = wait_until_ready(url, deadline_seconds=60)
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()

    if ready < 0:
        print("TIMEOUT (60s)")
        return 1

    # started 只用于兜底（wait_until_ready 内部已从它自己的起点计时）。
    _ = time.perf_counter() - started
    print(f"{ready:.2f}s")
    return 0


if __name__ == "__main__":
    # 入口统一兜底：参数错误、二进制不存在、端口被占等都转成一行可读信息，
    # 不让调用方看到裸 traceback。
    try:
        sys.exit(main())
    except (SystemExit, KeyboardInterrupt):
        raise
    except Exception as exc:  # noqa: BLE001 - 顶层兜底
        print(f"measure-startup failed: {exc}", file=sys.stderr)
        sys.exit(1)
