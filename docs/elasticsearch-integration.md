# Elasticsearch Integration

This document describes the Elasticsearch integration for the anomaly detection platform.

## Overview

The platform now stores all processed logs in Elasticsearch with anomaly flags, enabling:
- Persistent storage of all log entries
- Anomaly detection results storage
- Time-based log queries
- Anomaly-specific searches
- Full-text search capabilities

## Architecture

```
Log Input → Go Service → Python ML Service → Elasticsearch
                ↓
            Anomaly Detection
                ↓
            Elasticsearch Storage
```

## Data Model

### LogDocument Structure

```go
type LogDocument struct {
    ID        string    `json:"id,omitempty"`
    Timestamp time.Time `json:"timestamp"`
    LogText   string    `json:"log_text"`
    IsAnomaly bool      `json:"is_anomaly"`
}
```

## API Endpoints

### Log Ingestion & Retrieval
- **POST** `/v1/logs` - Store new log entries (existing endpoint, now with ES storage)
- **GET** `/v1/logs` - Retrieve stored logs
  - Query parameters:
    - `from` (int): Pagination offset (default: 0)
    - `size` (int): Number of results (default: 20, max: 100)
    - `start_time` (RFC3339): Start time filter
    - `end_time` (RFC3339): End time filter
- **GET** `/v1/anomalies` - Retrieve logs flagged as anomalies
  - Query parameters:
    - `from` (int): Pagination offset (default: 0)
    - `size` (int): Number of results (default: 20, max: 100)

### Search Endpoints
- **GET** `/v1/search/logs` - Search logs containing specific text
  - Query parameters:
    - `q` (string): Search text (required)
    - `from` (int): Pagination offset (default: 0)
    - `size` (int): Number of results (default: 20, max: 100)
- **GET** `/v1/search/anomalies` - Search anomalies containing specific text
  - Query parameters:
    - `q` (string): Search text (required)
    - `from` (int): Pagination offset (default: 0)
    - `size` (int): Number of results (default: 20, max: 100)

### Statistics Endpoints
- **GET** `/v1/stats/logs` - Get general log statistics
  - Query parameters:
    - `start_time` (RFC3339): Start time (required)
    - `end_time` (RFC3339): End time (required)
- **GET** `/v1/stats/anomalies` - Get anomaly statistics
  - Query parameters:
    - `start_time` (RFC3339): Start time (required)
    - `end_time` (RFC3339): End time (required)

### Detection Result Endpoints
- **POST** `/v1/detection` - Push single detection result
  - Body: `{"log_text": "string", "is_anomaly": boolean, "metadata": {}}`
- **POST** `/v1/detection/bulk` - Push multiple detection results
  - Body: `{"results": [{"id": "string", "timestamp": "RFC3339", "log_text": "string", "is_anomaly": boolean}]}`

## Configuration

### Environment Variables

- `ELASTICSEARCH_URLS`: Comma-separated list of Elasticsearch URLs (default: "http://localhost:9200")

### Docker Compose

The `deploy/docker-compose.yml` includes:
- Elasticsearch 8.11.0
- Kibana 8.11.0 (for data visualization)
- Proper networking between services

## Index Mapping

The Elasticsearch index is created with the following simplified mapping:

```json
{
  "mappings": {
    "properties": {
      "timestamp": {
        "type": "date"
      },
      "log_text": {
        "type": "text",
        "analyzer": "standard"
      },
      "is_anomaly": {
        "type": "boolean"
      }
    }
  }
}
```

## Usage Examples

### Store a Log Entry
```bash
curl -X POST http://localhost:8080/v1/logs \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Error: Database connection failed"
  }'
```

### Retrieve Recent Logs
```bash
curl "http://localhost:8080/v1/logs?size=10"
```

### Retrieve Anomalies
```bash
curl "http://localhost:8080/v1/anomalies?size=5"
```

### Retrieve Logs by Time Range
```bash
curl "http://localhost:8080/v1/logs?start_time=2024-01-01T00:00:00Z&end_time=2024-01-02T00:00:00Z"
```

### Search Logs by Text
```bash
curl "http://localhost:8080/v1/search/logs?q=error&size=10"
```

### Search Anomalies by Text
```bash
curl "http://localhost:8080/v1/search/anomalies?q=database&size=5"
```

### Get Log Statistics
```bash
curl "http://localhost:8080/v1/stats/logs?start_time=2024-01-01T00:00:00Z&end_time=2024-01-02T00:00:00Z"
```

### Get Anomaly Statistics
```bash
curl "http://localhost:8080/v1/stats/anomalies?start_time=2024-01-01T00:00:00Z&end_time=2024-01-02T00:00:00Z"
```

### Push Detection Result
```bash
curl -X POST http://localhost:8080/v1/detection \
  -H "Content-Type: application/json" \
  -d '{
    "log_text": "Critical system failure detected",
    "is_anomaly": true,
    "metadata": {
      "severity": "critical",
      "component": "database"
    }
  }'
```

### Bulk Push Detection Results
```bash
curl -X POST http://localhost:8080/v1/detection/bulk \
  -H "Content-Type: application/json" \
  -d '{
    "results": [
      {
        "id": "result-1",
        "timestamp": "2024-01-01T10:00:00Z",
        "log_text": "Normal operation",
        "is_anomaly": false
      },
      {
        "id": "result-2", 
        "timestamp": "2024-01-01T10:01:00Z",
        "log_text": "Anomalous behavior detected",
        "is_anomaly": true
      }
    ]
  }'
```

## Kibana Integration

Access Kibana at `http://localhost:5601` to:
- Visualize log data
- Create dashboards
- Set up alerts
- Analyze anomaly patterns

## Error Handling

- If Elasticsearch is unavailable, the service continues to function but logs are not stored
- Failed index operations are logged but don't affect the API response
- Graceful degradation ensures the anomaly detection pipeline remains functional

## Performance Considerations

- Index operations are asynchronous to avoid blocking API responses
- Pagination is enforced to prevent large result sets
- Time-based queries are optimized with proper date field mapping
- Elasticsearch connection pooling is handled by the official Go client

## Monitoring

Monitor the integration through:
- Application logs for connection status
- Elasticsearch cluster health
- Kibana dashboards for data visualization
- Prometheus metrics (if configured)

