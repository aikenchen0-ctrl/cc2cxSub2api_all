FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1
WORKDIR /app

COPY server/requirements.txt ./server/requirements.txt
RUN pip install --no-cache-dir -r server/requirements.txt
COPY src ./src
COPY server ./server

ENV PYTHONPATH=/app/server:/app/src
EXPOSE 8080
CMD ["python", "-m", "uvicorn", "app:app", "--app-dir", "/app/server", "--host", "0.0.0.0", "--port", "8080"]
