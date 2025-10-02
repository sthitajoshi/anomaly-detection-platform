package elastic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// Client wraps the Elasticsearch client with our custom methods
type Client struct {
	es *elasticsearch.Client
}

// LogDocument represents a log entry stored in Elasticsearch
type LogDocument struct {
	ID        string    `json:"id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	LogText   string    `json:"log_text"`
	IsAnomaly bool      `json:"is_anomaly"`
}

// NewClient creates a new Elasticsearch client
func NewClient(addresses []string) (*Client, error) {
	cfg := elasticsearch.Config{
		Addresses: addresses,
	}

	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Elasticsearch client: %w", err)
	}

	client := &Client{es: es}

	// Test connection
	_, err = client.es.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Elasticsearch: %w", err)
	}

	return client, nil
}

// IndexLog stores a log document in Elasticsearch
func (c *Client) IndexLog(ctx context.Context, doc *LogDocument) error {
	docBytes, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	req := esapi.IndexRequest{
		Index:      "logs",
		DocumentID: doc.ID,
		Body:       bytes.NewReader(docBytes),
		Refresh:    "true",
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("Elasticsearch error: %s", res.String())
	}

	return nil
}

// SearchLogs searches for logs with optional filters
func (c *Client) SearchLogs(ctx context.Context, query map[string]interface{}) ([]LogDocument, error) {
	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	req := esapi.SearchRequest{
		Index: []string{"logs"},
		Body:  bytes.NewReader(queryBytes),
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("Elasticsearch search error: %s", res.String())
	}

	var searchResponse struct {
		Hits struct {
			Hits []struct {
				Source LogDocument `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&searchResponse); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	documents := make([]LogDocument, len(searchResponse.Hits.Hits))
	for i, hit := range searchResponse.Hits.Hits {
		documents[i] = hit.Source
	}

	return documents, nil
}

// GetAnomalies retrieves all logs flagged as anomalies
func (c *Client) GetAnomalies(ctx context.Context, from, size int) ([]LogDocument, error) {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"is_anomaly": true,
						},
					},
				},
			},
		},
		"sort": []map[string]interface{}{
			{
				"timestamp": map[string]interface{}{
					"order": "desc",
				},
			},
		},
		"from": from,
		"size": size,
	}

	return c.SearchLogs(ctx, query)
}

// GetLogsByTimeRange retrieves logs within a time range
func (c *Client) GetLogsByTimeRange(ctx context.Context, start, end time.Time, from, size int) ([]LogDocument, error) {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"range": map[string]interface{}{
				"timestamp": map[string]interface{}{
					"gte": start.Format(time.RFC3339),
					"lte": end.Format(time.RFC3339),
				},
			},
		},
		"sort": []map[string]interface{}{
			{
				"timestamp": map[string]interface{}{
					"order": "desc",
				},
			},
		},
		"from": from,
		"size": size,
	}

	return c.SearchLogs(ctx, query)
}

// CreateIndex creates the logs index with proper mapping
func (c *Client) CreateIndex(ctx context.Context) error {
	mapping := `{
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
	}`

	req := esapi.IndicesCreateRequest{
		Index: "logs",
		Body:  strings.NewReader(mapping),
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		// Check if index already exists
		if strings.Contains(res.String(), "resource_already_exists_exception") {
			return nil
		}
		return fmt.Errorf("failed to create index: %s", res.String())
	}

	return nil
}

// SearchAnomaliesByText searches for anomalies containing specific text
func (c *Client) SearchAnomaliesByText(ctx context.Context, searchText string, from, size int) ([]LogDocument, error) {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"is_anomaly": true,
						},
					},
					{
						"match": map[string]interface{}{
							"log_text": searchText,
						},
					},
				},
			},
		},
		"sort": []map[string]interface{}{
			{
				"timestamp": map[string]interface{}{
					"order": "desc",
				},
			},
		},
		"from": from,
		"size": size,
	}

	return c.SearchLogs(ctx, query)
}

// SearchLogsByText searches for logs containing specific text
func (c *Client) SearchLogsByText(ctx context.Context, searchText string, from, size int) ([]LogDocument, error) {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"match": map[string]interface{}{
				"log_text": searchText,
			},
		},
		"sort": []map[string]interface{}{
			{
				"timestamp": map[string]interface{}{
					"order": "desc",
				},
			},
		},
		"from": from,
		"size": size,
	}

	return c.SearchLogs(ctx, query)
}

// GetAnomalyStats retrieves statistics about anomalies
func (c *Client) GetAnomalyStats(ctx context.Context, startTime, endTime time.Time) (map[string]interface{}, error) {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"is_anomaly": true,
						},
					},
					{
						"range": map[string]interface{}{
							"timestamp": map[string]interface{}{
								"gte": startTime.Format(time.RFC3339),
								"lte": endTime.Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
		"aggs": map[string]interface{}{
			"total_anomalies": map[string]interface{}{
				"value_count": map[string]interface{}{
					"field": "is_anomaly",
				},
			},
			"anomalies_over_time": map[string]interface{}{
				"date_histogram": map[string]interface{}{
					"field":    "timestamp",
					"interval": "1h",
				},
			},
		},
		"size": 0,
	}

	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	req := esapi.SearchRequest{
		Index: []string{"logs"},
		Body:  bytes.NewReader(queryBytes),
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("Elasticsearch search error: %s", res.String())
	}

	var searchResponse struct {
		Aggregations struct {
			TotalAnomalies struct {
				Value int `json:"value"`
			} `json:"total_anomalies"`
			AnomaliesOverTime struct {
				Buckets []struct {
					KeyAsString string `json:"key_as_string"`
					DocCount    int    `json:"doc_count"`
				} `json:"buckets"`
			} `json:"anomalies_over_time"`
		} `json:"aggregations"`
	}

	if err := json.NewDecoder(res.Body).Decode(&searchResponse); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	stats := map[string]interface{}{
		"total_anomalies":     searchResponse.Aggregations.TotalAnomalies.Value,
		"anomalies_over_time": searchResponse.Aggregations.AnomaliesOverTime.Buckets,
		"time_range": map[string]string{
			"start": startTime.Format(time.RFC3339),
			"end":   endTime.Format(time.RFC3339),
		},
	}

	return stats, nil
}

// GetLogStats retrieves general log statistics
func (c *Client) GetLogStats(ctx context.Context, startTime, endTime time.Time) (map[string]interface{}, error) {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"range": map[string]interface{}{
				"timestamp": map[string]interface{}{
					"gte": startTime.Format(time.RFC3339),
					"lte": endTime.Format(time.RFC3339),
				},
			},
		},
		"aggs": map[string]interface{}{
			"total_logs": map[string]interface{}{
				"value_count": map[string]interface{}{
					"field": "log_text",
				},
			},
			"anomaly_count": map[string]interface{}{
				"filter": map[string]interface{}{
					"term": map[string]interface{}{
						"is_anomaly": true,
					},
				},
			},
			"logs_over_time": map[string]interface{}{
				"date_histogram": map[string]interface{}{
					"field":    "timestamp",
					"interval": "1h",
				},
			},
		},
		"size": 0,
	}

	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	req := esapi.SearchRequest{
		Index: []string{"logs"},
		Body:  bytes.NewReader(queryBytes),
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("Elasticsearch search error: %s", res.String())
	}

	var searchResponse struct {
		Aggregations struct {
			TotalLogs struct {
				Value int `json:"value"`
			} `json:"total_logs"`
			AnomalyCount struct {
				DocCount int `json:"doc_count"`
			} `json:"anomaly_count"`
			LogsOverTime struct {
				Buckets []struct {
					KeyAsString string `json:"key_as_string"`
					DocCount    int    `json:"doc_count"`
				} `json:"buckets"`
			} `json:"logs_over_time"`
		} `json:"aggregations"`
	}

	if err := json.NewDecoder(res.Body).Decode(&searchResponse); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	stats := map[string]interface{}{
		"total_logs":     searchResponse.Aggregations.TotalLogs.Value,
		"anomaly_count":  searchResponse.Aggregations.AnomalyCount.DocCount,
		"normal_count":   searchResponse.Aggregations.TotalLogs.Value - searchResponse.Aggregations.AnomalyCount.DocCount,
		"logs_over_time": searchResponse.Aggregations.LogsOverTime.Buckets,
		"anomaly_rate":   float64(searchResponse.Aggregations.AnomalyCount.DocCount) / float64(searchResponse.Aggregations.TotalLogs.Value),
		"time_range": map[string]string{
			"start": startTime.Format(time.RFC3339),
			"end":   endTime.Format(time.RFC3339),
		},
	}

	return stats, nil
}

// PushDetectionResult pushes a detection result directly to Elasticsearch
func (c *Client) PushDetectionResult(ctx context.Context, logText string, isAnomaly bool, metadata map[string]interface{}) error {
	doc := &LogDocument{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Timestamp: time.Now().UTC(),
		LogText:   logText,
		IsAnomaly: isAnomaly,
	}

	return c.IndexLog(ctx, doc)
}

// BulkPushDetectionResults pushes multiple detection results in a single operation
func (c *Client) BulkPushDetectionResults(ctx context.Context, results []DetectionResult) error {
	if len(results) == 0 {
		return nil
	}

	var bulkBody strings.Builder

	for _, result := range results {
		// Index action
		indexAction := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": "logs",
				"_id":    result.ID,
			},
		}

		// Document
		doc := LogDocument{
			ID:        result.ID,
			Timestamp: result.Timestamp,
			LogText:   result.LogText,
			IsAnomaly: result.IsAnomaly,
		}

		// Add to bulk body
		indexBytes, _ := json.Marshal(indexAction)
		docBytes, _ := json.Marshal(doc)

		bulkBody.Write(indexBytes)
		bulkBody.WriteString("\n")
		bulkBody.Write(docBytes)
		bulkBody.WriteString("\n")
	}

	req := esapi.BulkRequest{
		Index:   "logs",
		Body:    strings.NewReader(bulkBody.String()),
		Refresh: "true",
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return fmt.Errorf("failed to bulk index documents: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("Elasticsearch bulk error: %s", res.String())
	}

	return nil
}

// DetectionResult represents a detection result for bulk operations
type DetectionResult struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	LogText   string    `json:"log_text"`
	IsAnomaly bool      `json:"is_anomaly"`
}

// Close closes the Elasticsearch client
func (c *Client) Close() error {
	// The Elasticsearch client doesn't need explicit closing
	return nil
}
