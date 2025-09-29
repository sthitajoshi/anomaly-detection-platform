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
		return "", 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var out pyPredictResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", 0, err
	}
	return out.Label, out.Score, nil
}
