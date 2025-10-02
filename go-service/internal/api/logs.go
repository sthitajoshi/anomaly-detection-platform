package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"anomaly-detection-platform/go-service/internal/client"
	"anomaly-detection-platform/go-service/internal/elastic"
	"anomaly-detection-platform/go-service/internal/preprocessing"
	"anomaly-detection-platform/go-service/pkg/config"
)

// Global Elasticsearch client - should be initialized in main.go
var ESClient *elastic.Client

// Request/response types can stay here or move to pkg/models
type LogRequest struct {
	Text     string                 `json:"text" binding:"required"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type LogResponse struct {
	Accepted      bool                   `json:"accepted"`
	Text          string                 `json:"text"`
	ContentType   string                 `json:"content_type"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	ReceivedAtUTC time.Time              `json:"received_at_utc"`
	Label         string                 `json:"label,omitempty"`
	Score         float64                `json:"score,omitempty"`
}

func LogsHandler(c *gin.Context) {
	ct := c.GetHeader("Content-Type")

	if strings.HasPrefix(ct, "application/json") {
		b, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "could not read body"})
			return
		}

		// Try single
		var single LogRequest
		if err := json.Unmarshal(b, &single); err == nil && single.Text != "" {
			respondBatch(c, "structured", []LogRequest{single})
			return
		}

		// Try array
		var arr []LogRequest
		if err := json.Unmarshal(b, &arr); err == nil && len(arr) > 0 {
			respondBatch(c, "structured", arr)
			return
		}

		// Try NDJSON
		var ndjson []LogRequest
		for _, ln := range strings.Split(string(b), "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			var it LogRequest
			if err := json.Unmarshal([]byte(ln), &it); err == nil && it.Text != "" {
				ndjson = append(ndjson, it)
			}
		}
		if len(ndjson) > 0 {
			respondBatch(c, "structured", ndjson)
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON or missing 'text'"})
		return
	}

	// Fallback raw text
	b, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read body"})
		return
	}
	respondBatch(c, "unstructured", []LogRequest{{Text: string(b)}})
}

func respondBatch(c *gin.Context, contentType string, logs []LogRequest) {
	results := make([]LogResponse, len(logs))
	ctx := c.Request.Context()

	var wg sync.WaitGroup
	wg.Add(len(logs))

	for i, logReq := range logs {
		go func(i int, lr LogRequest) {
			defer wg.Done()

			cleaned := preprocessing.PreprocessLogText(lr.Text)

			resp := LogResponse{
				Accepted:      true,
				Text:          cleaned,
				ContentType:   contentType,
				Metadata:      lr.Metadata,
				ReceivedAtUTC: time.Now().UTC(),
			}

			cctx, cancel := config.WithTimeout(ctx, 4*time.Second)
			defer cancel()
			if label, score, err := client.CallPythonPredict(cctx, cleaned); err == nil {
				resp.Label = label
				resp.Score = score
			}

			// Store in Elasticsearch if client is available
			if ESClient != nil {
				// Determine if this is an anomaly based on ML prediction
				isAnomaly := resp.Label == "anomaly" || resp.Score > 0.5

				doc := &elastic.LogDocument{
					ID:        fmt.Sprintf("%d", time.Now().UnixNano()+int64(i)), // Simple ID generation
					Timestamp: resp.ReceivedAtUTC,
					LogText:   cleaned,
					IsAnomaly: isAnomaly,
				}

				if err := ESClient.IndexLog(cctx, doc); err != nil {
					// Log error but don't fail the request
					fmt.Printf("Failed to index log in Elasticsearch: %v\n", err)
				}
			}

			results[i] = resp
		}(i, logReq)
	}

	wg.Wait()
	if len(results) == 1 {
		c.JSON(http.StatusOK, results[0])
	} else {
		c.JSON(http.StatusOK, results)
	}
}

// GetAnomaliesHandler retrieves all logs flagged as anomalies
func GetAnomaliesHandler(c *gin.Context) {
	if ESClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Elasticsearch not available"})
		return
	}

	// Parse query parameters
	from := 0
	size := 20

	if fromStr := c.Query("from"); fromStr != "" {
		if f, err := fmt.Sscanf(fromStr, "%d", &from); err != nil || f != 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'from' parameter"})
			return
		}
	}

	if sizeStr := c.Query("size"); sizeStr != "" {
		if s, err := fmt.Sscanf(sizeStr, "%d", &size); err != nil || s != 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'size' parameter"})
			return
		}
	}

	// Limit size to prevent abuse
	if size > 100 {
		size = 100
	}

	anomalies, err := ESClient.GetAnomalies(c.Request.Context(), from, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to retrieve anomalies: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"anomalies": anomalies,
		"total":     len(anomalies),
		"from":      from,
		"size":      size,
	})
}

// GetLogsHandler retrieves logs with optional time range filtering
func GetLogsHandler(c *gin.Context) {
	if ESClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Elasticsearch not available"})
		return
	}

	// Parse query parameters
	from := 0
	size := 20

	if fromStr := c.Query("from"); fromStr != "" {
		if f, err := fmt.Sscanf(fromStr, "%d", &from); err != nil || f != 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'from' parameter"})
			return
		}
	}

	if sizeStr := c.Query("size"); sizeStr != "" {
		if s, err := fmt.Sscanf(sizeStr, "%d", &size); err != nil || s != 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'size' parameter"})
			return
		}
	}

	// Limit size to prevent abuse
	if size > 100 {
		size = 100
	}

	var logs []elastic.LogDocument
	var err error

	// Check for time range parameters
	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")

	if startTimeStr != "" && endTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'start_time' format, use RFC3339"})
			return
		}

		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'end_time' format, use RFC3339"})
			return
		}

		logs, err = ESClient.GetLogsByTimeRange(c.Request.Context(), startTime, endTime, from, size)
	} else {
		// Get all logs
		query := map[string]interface{}{
			"query": map[string]interface{}{
				"match_all": map[string]interface{}{},
			},
			"sort": []map[string]interface{}{
				{
					"received_at_utc": map[string]interface{}{
						"order": "desc",
					},
				},
			},
			"from": from,
			"size": size,
		}

		logs, err = ESClient.SearchLogs(c.Request.Context(), query)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to retrieve logs: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": len(logs),
		"from":  from,
		"size":  size,
	})
}

// SearchAnomaliesHandler searches for anomalies containing specific text
func SearchAnomaliesHandler(c *gin.Context) {
	if ESClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Elasticsearch not available"})
		return
	}

	searchText := c.Query("q")
	if searchText == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	// Parse pagination parameters
	from := 0
	size := 20

	if fromStr := c.Query("from"); fromStr != "" {
		if f, err := fmt.Sscanf(fromStr, "%d", &from); err != nil || f != 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'from' parameter"})
			return
		}
	}

	if sizeStr := c.Query("size"); sizeStr != "" {
		if s, err := fmt.Sscanf(sizeStr, "%d", &size); err != nil || s != 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'size' parameter"})
			return
		}
	}

	if size > 100 {
		size = 100
	}

	anomalies, err := ESClient.SearchAnomaliesByText(c.Request.Context(), searchText, from, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to search anomalies: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"anomalies": anomalies,
		"total":     len(anomalies),
		"from":      from,
		"size":      size,
		"query":     searchText,
	})
}

// SearchLogsHandler searches for logs containing specific text
func SearchLogsHandler(c *gin.Context) {
	if ESClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Elasticsearch not available"})
		return
	}

	searchText := c.Query("q")
	if searchText == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	// Parse pagination parameters
	from := 0
	size := 20

	if fromStr := c.Query("from"); fromStr != "" {
		if f, err := fmt.Sscanf(fromStr, "%d", &from); err != nil || f != 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'from' parameter"})
			return
		}
	}

	if sizeStr := c.Query("size"); sizeStr != "" {
		if s, err := fmt.Sscanf(sizeStr, "%d", &size); err != nil || s != 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'size' parameter"})
			return
		}
	}

	if size > 100 {
		size = 100
	}

	logs, err := ESClient.SearchLogsByText(c.Request.Context(), searchText, from, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to search logs: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": len(logs),
		"from":  from,
		"size":  size,
		"query": searchText,
	})
}

// GetAnomalyStatsHandler retrieves anomaly statistics
func GetAnomalyStatsHandler(c *gin.Context) {
	if ESClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Elasticsearch not available"})
		return
	}

	// Parse time range parameters
	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")

	if startTimeStr == "" || endTimeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "both 'start_time' and 'end_time' parameters are required"})
		return
	}

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'start_time' format, use RFC3339"})
		return
	}

	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'end_time' format, use RFC3339"})
		return
	}

	stats, err := ESClient.GetAnomalyStats(c.Request.Context(), startTime, endTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get anomaly stats: %v", err)})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetLogStatsHandler retrieves general log statistics
func GetLogStatsHandler(c *gin.Context) {
	if ESClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Elasticsearch not available"})
		return
	}

	// Parse time range parameters
	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")

	if startTimeStr == "" || endTimeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "both 'start_time' and 'end_time' parameters are required"})
		return
	}

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'start_time' format, use RFC3339"})
		return
	}

	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'end_time' format, use RFC3339"})
		return
	}

	stats, err := ESClient.GetLogStats(c.Request.Context(), startTime, endTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get log stats: %v", err)})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// PushDetectionResultHandler pushes a detection result directly to Elasticsearch
func PushDetectionResultHandler(c *gin.Context) {
	if ESClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Elasticsearch not available"})
		return
	}

	var request struct {
		LogText   string                 `json:"log_text" binding:"required"`
		IsAnomaly bool                   `json:"is_anomaly"`
		Metadata  map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := ESClient.PushDetectionResult(c.Request.Context(), request.LogText, request.IsAnomaly, request.Metadata)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to push detection result: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Detection result pushed successfully",
		"log_text":   request.LogText,
		"is_anomaly": request.IsAnomaly,
	})
}

// BulkPushDetectionResultsHandler pushes multiple detection results
func BulkPushDetectionResultsHandler(c *gin.Context) {
	if ESClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Elasticsearch not available"})
		return
	}

	var request struct {
		Results []elastic.DetectionResult `json:"results" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(request.Results) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "results array cannot be empty"})
		return
	}

	if len(request.Results) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "maximum 1000 results allowed per request"})
		return
	}

	err := ESClient.BulkPushDetectionResults(c.Request.Context(), request.Results)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to bulk push detection results: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Detection results pushed successfully",
		"count":   len(request.Results),
	})
}
