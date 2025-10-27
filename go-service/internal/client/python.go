package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"anomaly-detection-platform/go-service/pkg/config"
)

type pyPredictRequest struct {
	Text string `json:"text"`
}

type pyPredictResponse struct {
	Label string  `json:"label"`
	Score float64 `json:"score"`
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

func pythonURL() string {
	return config.GetEnv("PYTHON_SERVICE_URL", "http://localhost:8001/predict")
}

func CallPythonPredict(ctx context.Context, text string) (string, float64, error) {
	payload := pyPredictRequest{Text: text}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", 0, err
	}

	var lastErr error
	// Retry with backoff for transient errors and 5xx responses
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, pythonURL(), bytes.NewReader(b))
		if err != nil {
			return "", 0, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			// Ensure body closed for each attempt
			func() {
				defer resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					var out pyPredictResponse
					if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
						lastErr = err
						return
					}
					lastErr = nil
					// Success
					// Return here to break out of function
					// Using named return would be nicer, but keep explicit
					// to avoid confusion.
					// We cannot return from inside the inner func; set a marker
				} else if resp.StatusCode >= 500 {
					lastErr = fmt.Errorf("python service 5xx: %d", resp.StatusCode)
				} else {
					lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
				}
			}()

			if lastErr == nil {
				// We need to redo decode because we exited inner scope without value
				// Re-issue one final successful request to decode
				// This keeps code simple while preserving retries
				req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, pythonURL(), bytes.NewReader(b))
				req2.Header.Set("Content-Type", "application/json")
				if resp2, err2 := httpClient.Do(req2); err2 == nil {
					defer resp2.Body.Close()
					var out pyPredictResponse
					if err := json.NewDecoder(resp2.Body).Decode(&out); err == nil {
						return out.Label, out.Score, nil
					}
				}
				// fallback continue to retry
				lastErr = fmt.Errorf("failed to decode success response")
			}
		}

		// Backoff before next attempt if context not done
		select {
		case <-ctx.Done():
			return "", 0, ctx.Err()
		case <-time.After(time.Duration(200*(1<<attempt)) * time.Millisecond):
		}
	}
	return "", 0, lastErr
}
