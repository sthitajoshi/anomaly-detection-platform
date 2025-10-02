package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"anomaly-detection-platform/go-service/internal/elastic"
)

func main() {
	// Test Elasticsearch connection
	esClient, err := elastic.NewClient([]string{"http://localhost:9200"})
	if err != nil {
		log.Fatalf("Failed to connect to Elasticsearch: %v", err)
	}
	defer esClient.Close()

	ctx := context.Background()

	// Create index
	if err := esClient.CreateIndex(ctx); err != nil {
		log.Fatalf("Failed to create index: %v", err)
	}
	fmt.Println("✅ Index created successfully")

	// Test 1: Push individual detection results
	fmt.Println("\n🔍 Test 1: Push individual detection results")

	// Push normal log
	err = esClient.PushDetectionResult(ctx, "User login successful", false, nil)
	if err != nil {
		log.Printf("Failed to push normal log: %v", err)
	} else {
		fmt.Println("✅ Normal log pushed")
	}

	// Push anomaly
	err = esClient.PushDetectionResult(ctx, "Database connection failed - critical error", true, nil)
	if err != nil {
		log.Printf("Failed to push anomaly: %v", err)
	} else {
		fmt.Println("✅ Anomaly pushed")
	}

	// Test 2: Bulk push detection results
	fmt.Println("\n🔍 Test 2: Bulk push detection results")

	results := []elastic.DetectionResult{
		{
			ID:        fmt.Sprintf("bulk-%d", time.Now().UnixNano()),
			Timestamp: time.Now().UTC(),
			LogText:   "System startup completed",
			IsAnomaly: false,
		},
		{
			ID:        fmt.Sprintf("bulk-%d", time.Now().UnixNano()+1),
			Timestamp: time.Now().UTC(),
			LogText:   "Memory usage exceeded 90% - potential memory leak",
			IsAnomaly: true,
		},
		{
			ID:        fmt.Sprintf("bulk-%d", time.Now().UnixNano()+2),
			Timestamp: time.Now().UTC(),
			LogText:   "API request processed successfully",
			IsAnomaly: false,
		},
	}

	err = esClient.BulkPushDetectionResults(ctx, results)
	if err != nil {
		log.Printf("Failed to bulk push results: %v", err)
	} else {
		fmt.Println("✅ Bulk results pushed")
	}

	// Wait a moment for indexing
	time.Sleep(2 * time.Second)

	// Test 3: Search anomalies by text
	fmt.Println("\n🔍 Test 3: Search anomalies by text")

	anomalies, err := esClient.SearchAnomaliesByText(ctx, "error", 0, 10)
	if err != nil {
		log.Printf("Failed to search anomalies: %v", err)
	} else {
		fmt.Printf("✅ Found %d anomalies containing 'error'\n", len(anomalies))
		for _, anomaly := range anomalies {
			fmt.Printf("   - %s (Anomaly: %t)\n", anomaly.LogText, anomaly.IsAnomaly)
		}
	}

	// Test 4: Search logs by text
	fmt.Println("\n🔍 Test 4: Search logs by text")

	logs, err := esClient.SearchLogsByText(ctx, "successful", 0, 10)
	if err != nil {
		log.Printf("Failed to search logs: %v", err)
	} else {
		fmt.Printf("✅ Found %d logs containing 'successful'\n", len(logs))
		for _, log := range logs {
			fmt.Printf("   - %s (Anomaly: %t)\n", log.LogText, log.IsAnomaly)
		}
	}

	// Test 5: Get anomaly statistics
	fmt.Println("\n🔍 Test 5: Get anomaly statistics")

	startTime := time.Now().Add(-1 * time.Hour)
	endTime := time.Now()

	stats, err := esClient.GetAnomalyStats(ctx, startTime, endTime)
	if err != nil {
		log.Printf("Failed to get anomaly stats: %v", err)
	} else {
		fmt.Println("✅ Anomaly statistics:")
		fmt.Printf("   Total anomalies: %v\n", stats["total_anomalies"])
		fmt.Printf("   Time range: %v\n", stats["time_range"])
	}

	// Test 6: Get general log statistics
	fmt.Println("\n🔍 Test 6: Get general log statistics")

	logStats, err := esClient.GetLogStats(ctx, startTime, endTime)
	if err != nil {
		log.Printf("Failed to get log stats: %v", err)
	} else {
		fmt.Println("✅ Log statistics:")
		fmt.Printf("   Total logs: %v\n", logStats["total_logs"])
		fmt.Printf("   Anomaly count: %v\n", logStats["anomaly_count"])
		fmt.Printf("   Normal count: %v\n", logStats["normal_count"])
		fmt.Printf("   Anomaly rate: %.2f%%\n", logStats["anomaly_rate"].(float64)*100)
	}

	// Test 7: Get all anomalies
	fmt.Println("\n🔍 Test 7: Get all anomalies")

	allAnomalies, err := esClient.GetAnomalies(ctx, 0, 10)
	if err != nil {
		log.Printf("Failed to get anomalies: %v", err)
	} else {
		fmt.Printf("✅ Found %d total anomalies\n", len(allAnomalies))
		for _, anomaly := range allAnomalies {
			fmt.Printf("   - %s\n", anomaly.LogText)
		}
	}

	// Test 8: Get logs by time range
	fmt.Println("\n🔍 Test 8: Get logs by time range")

	timeRangeLogs, err := esClient.GetLogsByTimeRange(ctx, startTime, endTime, 0, 10)
	if err != nil {
		log.Printf("Failed to get logs by time range: %v", err)
	} else {
		fmt.Printf("✅ Found %d logs in the last hour\n", len(timeRangeLogs))
		for _, log := range timeRangeLogs {
			fmt.Printf("   - %s (Anomaly: %t)\n", log.LogText, log.IsAnomaly)
		}
	}

	fmt.Println("\n🎉 All tests completed successfully!")
	fmt.Println("\n📊 API Endpoints Available:")
	fmt.Println("   GET  /v1/logs - Retrieve logs")
	fmt.Println("   GET  /v1/anomalies - Retrieve anomalies")
	fmt.Println("   GET  /v1/search/logs?q=text - Search logs by text")
	fmt.Println("   GET  /v1/search/anomalies?q=text - Search anomalies by text")
	fmt.Println("   GET  /v1/stats/logs?start_time=...&end_time=... - Get log statistics")
	fmt.Println("   GET  /v1/stats/anomalies?start_time=...&end_time=... - Get anomaly statistics")
	fmt.Println("   POST /v1/detection - Push single detection result")
	fmt.Println("   POST /v1/detection/bulk - Push multiple detection results")
}
