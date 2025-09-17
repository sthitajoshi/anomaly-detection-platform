package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	return getEnv("PYTHON_SERVICE_URL", "http://localhost:8000/predict")
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

	// Graceful shutdown
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
// or raw text in the request body (text/plain or any other content-type)
func logsHandler(c *gin.Context) {
	ct := c.GetHeader("Content-Type")

	if hasPrefix(ct, "application/json") {
		var req LogRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON or missing 'text'"})
			return
		}
		resp := LogResponse{
			Accepted:      true,
			Text:          req.Text,
			ContentType:   "application/json",
			Metadata:      req.Metadata,
			ReceivedAtUTC: time.Now().UTC(),
		}
		// Call Python inference (best-effort)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
		label, score, err := callPythonPredict(ctx, req.Text)
		cancel()
		if err == nil {
			resp.Label = label
			resp.Score = score
		} else {
			log.Printf("python inference error: %v", err)
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	// Treat as raw text for any non-JSON content-type
	b, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read body"})
		return
	}
	text := string(b)
	resp := LogResponse{
		Accepted:      true,
		Text:          text,
		ContentType:   firstNonEmpty(ct, "text/plain"),
		ReceivedAtUTC: time.Now().UTC(),
	}
	// Call Python inference (best-effort)
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

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

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

func (e *statusError) Error() string { return "unexpected status: " + http.StatusText(e.Code) }
