package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

// LogRequest represents JSON payload for logs
// Example: {"text":"Error: connection timed out","metadata":{"source":"api"}}
type LogRequest struct {
	Text     string                 `json:"text" binding:"required"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// LogResponse is the normalized response we return
type LogResponse struct {
	Accepted      bool                   `json:"accepted"`
	Text          string                 `json:"text"`
	ContentType   string                 `json:"content_type"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	ReceivedAtUTC time.Time              `json:"received_at_utc"`
	Label         string                 `json:"label,omitempty"`
	Score         float64                `json:"score,omitempty"`
}

// Python inference DTOs
type pyPredictRequest struct {
	Text string `json:"text"`
}

type pyPredictResponse struct {
	Label string  `json:"label"`
	Score float64 `json:"score"`
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

func pythonURL() string {
	return getEnv("PYTHON_SERVICE_URL", "http://localhost:8001/predict")
}

func callPythonPredict(ctx context.Context, text string) (label string, score float64, err error) {
	payload := pyPredictRequest{Text: text}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pythonURL(), bytes.NewReader(b))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmtErrorStatus(resp.StatusCode)
	}

	var out pyPredictResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", 0, err
	}
	return out.Label, out.Score, nil
}

func main() {
	port := getEnv("PORT", "8080")

	if getEnv("GIN_MODE", "") != "" {
		gin.SetMode(os.Getenv("GIN_MODE"))
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// Health endpoint
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/v1")
	{
		v1.POST("/logs", logsHandler)
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("Go (Gin) service listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	log.Println("server stopped")
}

// logsHandler accepts JSON (object, array, or NDJSON) or raw text.
func logsHandler(c *gin.Context) {
	ct := c.GetHeader("Content-Type")

	if strings.HasPrefix(ct, "application/json") {
		b, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "could not read body"})
			return
		}

		// Try single object
		var single LogRequest
		if err := json.Unmarshal(b, &single); err == nil && single.Text != "" {
			respondBatch(c, "structured", []LogRequest{single})
			return
		}

		// Try array of objects
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

	// Raw text fallback
	b, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read body"})
		return
	}
	text := string(b)
	respondBatch(c, "unstructured", []LogRequest{{Text: text}})
}

// respondBatch handles single or multiple log requests concurrently
func respondBatch(c *gin.Context, contentType string, logs []LogRequest) {
	results := make([]LogResponse, len(logs))
	ctx := c.Request.Context()

	var wg sync.WaitGroup
	wg.Add(len(logs))

	for i, logReq := range logs {
		go func(i int, lr LogRequest) {
			defer wg.Done()

			// Clean/preprocess log text
			cleaned := preprocessLogText(lr.Text)

			resp := LogResponse{
				Accepted:      true,
				Text:          cleaned,
				ContentType:   contentType,
				Metadata:      lr.Metadata,
				ReceivedAtUTC: time.Now().UTC(),
			}

			// Call Python with timeout
			cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
			defer cancel()
			if label, score, err := callPythonPredict(cctx, cleaned); err == nil {
				resp.Label = label
				resp.Score = score
			} else {
				log.Printf("python inference error: %v", err)
			}
			results[i] = resp
		}(i, logReq)
	}

	wg.Wait()

	if len(results) == 1 {
		c.JSON(http.StatusOK, results[0]) // backward compat
	} else {
		c.JSON(http.StatusOK, results)
	}
}

// --- Preprocessing function ---
func preprocessLogText(input string) string {
	// Remove timestamps like [2025-09-29 12:00:00]
	reTime := regexp.MustCompile(`\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\]`)
	cleaned := reTime.ReplaceAllString(input, "")

	// Remove IP addresses
	reIP := regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)
	cleaned = reIP.ReplaceAllString(cleaned, "[REDACTED_IP]")

	// Collapse multiple spaces
	reSpace := regexp.MustCompile(`\s+`)
	cleaned = reSpace.ReplaceAllString(cleaned, " ")

	// Trim leading/trailing spaces
	cleaned = strings.TrimSpace(cleaned)

	// Normalize to lower case
	cleaned = strings.ToLower(cleaned)

	return cleaned
}

// --- Utility helpers ---
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fmtErrorStatus(code int) error {
	return &statusError{Code: code}
}

type statusError struct{ Code int }

func (e *statusError) Error() string {
	return fmt.Sprintf("unexpected status %d: %s", e.Code, http.StatusText(e.Code))
}
