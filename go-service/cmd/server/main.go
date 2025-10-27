package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"anomaly-detection-platform/go-service/internal/api"
	"anomaly-detection-platform/go-service/internal/metrics"
	"anomaly-detection-platform/go-service/internal/elastic"
	"anomaly-detection-platform/go-service/pkg/config"
)

func main() {
	port := config.GetEnv("PORT", "8080")

	if mode := config.GetEnv("GIN_MODE", ""); mode != "" {
		gin.SetMode(mode)
	}

	// Initialize Elasticsearch client
	esAddresses := strings.Split(config.GetEnv("ELASTICSEARCH_URLS", "http://localhost:9200"), ",")
	esClient, err := elastic.NewClient(esAddresses)
	if err != nil {
		log.Printf("Warning: Failed to connect to Elasticsearch: %v", err)
		log.Println("Continuing without Elasticsearch - logs will not be stored")
	} else {
		log.Println("Connected to Elasticsearch successfully")

		// Create index if it doesn't exist
		if err := esClient.CreateIndex(context.Background()); err != nil {
			log.Printf("Warning: Failed to create Elasticsearch index: %v", err)
		} else {
			log.Println("Elasticsearch index created/verified successfully")
		}

		// Set global client
		api.ESClient = esClient
	}

	// Initialize Prometheus metrics
	metrics.Init()

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), api.LoggingMiddleware())

	// Expose Prometheus metrics endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Register API routes
	api.RegisterRoutes(r)

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
