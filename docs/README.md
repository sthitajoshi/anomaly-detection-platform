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
