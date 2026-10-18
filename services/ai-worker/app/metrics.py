"""Metric nghiep vu cua ai-worker -- nhung con so Ops nhin trong war room.

Gom mot cho de runtime, Gemini client va main.py dung chung mot registry, va de ten
metric khong troi khoi deploy/observability/alerts.yml.
"""

from __future__ import annotations

from prometheus_client import Counter, Gauge, Histogram

QUEUE_LAG = Gauge(
    "ai_queue_lag_seconds",
    "Tuoi cua message vua nhan tren hang doi AI (0 khi hang trong va khong co viec dang chay)",
    ["queue"],
)
QUEUE_DEPTH = Gauge("ai_queue_depth", "So message dang cho trong hang doi AI", ["queue"])
JOBS = Counter(
    "ai_jobs_total",
    "So message da xu ly theo ket qua (ack, retry, dead_letter, expired, requeue)",
    ["queue", "outcome"],
)
JOB_DURATION = Histogram(
    "ai_job_duration_seconds",
    "Thoi gian xu ly mot message",
    ["queue"],
    buckets=(0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8, 15, 30, 60, 120),
)
CACHE_HIT = Gauge("ai_semantic_cache_hit_ratio", "Ti le hit cache ngu nghia")
RATE_LIMITED = Counter("ai_gemini_429_total", "So lan bi Gemini tu choi vi rate-limit")
PERMIT_DENIED = Counter("ai_permit_denied_total", "So lan limiter tu choi cap phep")
MODE = Gauge("ai_degradation_mode", "Nac suy bien (0=NORMAL..3=OFF)")
