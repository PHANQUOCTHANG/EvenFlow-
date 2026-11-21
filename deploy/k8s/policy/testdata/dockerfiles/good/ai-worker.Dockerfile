FROM python:3.12-slim
RUN useradd --uid 10001 worker
COPY app /app
USER 10001
CMD ["uvicorn", "app.main:app"]
