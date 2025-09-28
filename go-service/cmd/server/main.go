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
	"strings"
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
// Extend this later to call the Python AI service
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
// POST /predict expects: {"text":"..."}
// Responds: {"label":"...","score":0.97}
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

	// Set Gin mode from ENV if needed (release/debug)
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

	// Build HTTP server to support graceful shutdown
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

// logsHandler accepts either application/json with {text, metadata}
// (single object, array, or NDJSON lines) or raw text bodies.
func logsHandler(c *gin.Context) {
	ct := c.GetHeader("Content-Type")

	if strings.HasPrefix(ct, "application/json") {
		// Read once for multiple parsing strategies
		b, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "could not read body"})
			return
		}

		// Try single JSON object
		var single LogRequest
		if err := json.Unmarshal(b, &single); err == nil && single.Text != "" {
			respondWithPrediction(c, ct, single.Text, single.Metadata)
			return
		}

		// Try JSON array of objects
		var arr []LogRequest
		if err := json.Unmarshal(b, &arr); err == nil && len(arr) > 0 {
			first := arr[0]
			if first.Text == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "first array item missing 'text'"})
				return
			}
			respondWithPrediction(c, ct, first.Text, first.Metadata)
			return
		}

		// Try NDJSON (json_stream)
		for _, ln := range strings.Split(string(b), "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			var it LogRequest
			if err := json.Unmarshal([]byte(ln), &it); err == nil && it.Text != "" {
				respondWithPrediction(c, ct, it.Text, it.Metadata)
				return
			}
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
	respondWithPrediction(c, ct, text, nil)
}

// Helper to call Python and build response
func respondWithPrediction(c *gin.Context, contentType, text string, meta map[string]interface{}) {
	resp := LogResponse{
		Text:          text,
		ContentType:   firstNonEmpty(contentType, "text/plain"),
		Metadata:      meta,
		ReceivedAtUTC: time.Now().UTC(),
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
	label, score, err := callPythonPredict(ctx, text)
	cancel()
	if err == nil {
		resp.Label = label
		resp.Score = score
	} else {
		log.Printf("python inference error: %v", err)
	}
	c.JSON(http.StatusOK, resp)
}

// Utility helpers
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

// helper to format non-2xx status as error
func fmtErrorStatus(code int) error {
	return &statusError{Code: code}
}

type statusError struct{ Code int }

func (e *statusError) Error() string {
	return fmt.Sprintf("unexpected status %d: %s", e.Code, http.StatusText(e.Code))
}
