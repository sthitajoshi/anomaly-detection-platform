package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"anomaly-detection-platform/go-service/internal/client"
	"anomaly-detection-platform/go-service/internal/preprocessing"
	"anomaly-detection-platform/go-service/pkg/config"
)

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
