# syntax=docker/dockerfile:1
FROM python:3.12-slim AS base
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
WORKDIR /app
COPY services/ai-worker/pyproject.toml ./
RUN pip install --no-cache-dir uv && uv pip install --system --no-cache .
COPY services/ai-worker/app ./app
# Worker tu khai bao topology AI luc khoi dong (EV-157): RabbitMQ chi mount file nay, khong nap.
COPY deploy/rabbitmq/definitions.json ./topology/definitions.json
ENV AI_TOPOLOGY_FILE=/app/topology/definitions.json
RUN useradd -m -u 10001 worker
USER worker
EXPOSE 8090
# --log-config: log JSON tu dong dau tien (truoc lifespan), khong chi tu luc app khoi dong.
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8090", "--log-config", "app/logging.json"]
