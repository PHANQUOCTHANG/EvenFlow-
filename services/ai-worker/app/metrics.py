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
# EV-159: job chet im lang la te nhat -- dem va do de alert (deploy/observability/alerts.yml).
DLQ_DEPTH = Gauge("ai_dlq_depth", "So message dang nam trong ai.dlq")
DEAD_LETTERED = Counter(
    "ai_dead_lettered_total",
    "So message da chuyen vao DLQ theo ly do (permanent: loi khong the tu het, exhausted: het luot)",
    ["queue", "reason"],
)
# EV-171: API tro ly phia khach.
ASSISTANT_REQUESTS = Counter(
    "ai_assistant_requests_total",
    "Yeu cau toi API tro ly theo ket qua (queued, quota_exceeded, unavailable, invalid, ...)",
    ["outcome"],
)
ASSISTANT_ACCEPT_SECONDS = Histogram(
    "ai_assistant_accept_seconds",
    "Thoi gian API tro ly nhan va xep hang mot cau hoi (AC: < 50 ms)",
    buckets=(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1),
)
CACHE_HIT = Gauge("ai_semantic_cache_hit_ratio", "Ti le hit cache ngu nghia")
RATE_LIMITED = Counter("ai_gemini_429_total", "So lan bi Gemini tu choi vi rate-limit")
PERMIT_DENIED = Counter("ai_permit_denied_total", "So lan limiter tu choi cap phep")
MODE = Gauge("ai_degradation_mode", "Nac suy bien (0=NORMAL..3=OFF)")
