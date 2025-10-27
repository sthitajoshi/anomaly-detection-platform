          ┌───────────────┐
            │   Log Source   │
            │ (apps, files)  │
            └───────┬───────┘
                    │
                    ▼
             ┌───────────────┐
             │  Fluent Bit    │
             │ (log shipper)  │
             └───────┬───────┘
                     │
                     ▼
             ┌───────────────┐
             │   Go Service   │
             │  (Ingestion +  │
             │  Preprocessing │
             │ + Metrics)     │ 
             └───────┬────────┘
                     │ REST/gRPC
                     ▼
           ┌───────────────────┐
           │ Python Inference   │
           │ (FastAPI + HF ML)  │
           └─────────┬─────────┘
                     │
                     ▼
           ┌───────────────────┐
           │ Elasticsearch      │
           │ (logs + anomalies) │
           └─────────┬─────────┘
                     │
            ┌────────▼─────────┐
            │   Prometheus      │
            │ (metrics scrape)  │
            └────────┬─────────┘
                     │
                     ▼
              ┌───────────────┐
              │   Grafana      │
              │  (dashboard)   │
              └───────────────┘

# Anomaly Detection Platform

- Go service: Ingests logs, cleans text, calls Python inference, stores docs in Elasticsearch, exposes Prometheus metrics at `/metrics` and health at `/healthz`.
- Python service: FastAPI + Hugging Face transformers model for text anomaly classification.
- Elasticsearch + Kibana: Stores and explores logs/anomalies.
- Prometheus + Grafana: Scrapes and visualizes metrics.
- Fluent Bit: Demo shipper sending a dummy log to Go `/v1/logs`.

## Quickstart (Docker Compose)

Prereqs: Docker Desktop 4+, ~6 GB free RAM.

```bash
# from repo root
docker compose -f deploy/docker-compose.yml up -d --build

# check health
curl http://localhost:8080/healthz
curl http://localhost:8080/metrics

# send a sample log directly
curl -s -X POST http://localhost:8080/v1/logs \
  -H 'content-type: application/json' \
  -d '{"text":"CRITICAL: kernel panic, system halted"}'
```

Services once up:
- Go API: http://localhost:8080
- Python Inference: http://localhost:8001/docs
- Elasticsearch: http://localhost:9200
- Kibana: http://localhost:5601
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin/admin)

## Configuration

Key env vars (see `deploy/docker-compose.yml`):
- `ELASTICSEARCH_URLS`: `http://elasticsearch:9200`
- `PYTHON_SERVICE_URL`: `http://python-service:8001/predict`

Prometheus scrapes `go-service:8080/metrics` via `deploy/prometheus.yml`.

## Development

- Go run locally:
```bash
cd go-service
ELASTICSEARCH_URLS=http://localhost:9200 \
PYTHON_SERVICE_URL=http://localhost:8001/predict \
go run ./cmd/server
```

- Python run locally:
```bash
cd python-service
pip install -r requirements.txt
uvicorn app.main:app --reload --port 8001
```

## CI/CD

GitHub Actions workflow `.github/workflows/ci.yml` builds, tests, and pushes images to GHCR. Optional Docker Hub push if `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` secrets are set.

## Demo flow

- View Go logs and metrics in Prometheus/Grafana.
- Send test logs via Fluent Bit (already configured) or curl.
- Observe anomalies via API `/v1/anomalies` and on Grafana dashboards.
