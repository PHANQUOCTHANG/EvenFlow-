# syntax=docker/dockerfile:1
FROM python:3.12-slim AS base
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
WORKDIR /app
COPY services/ai-worker/pyproject.toml ./
RUN pip install --no-cache-dir uv && uv pip install --system --no-cache .
COPY services/ai-worker/app ./app
RUN useradd -m -u 10001 worker
USER worker
EXPOSE 8090
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8090"]
