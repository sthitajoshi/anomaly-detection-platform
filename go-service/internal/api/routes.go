package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Register all routes
func RegisterRoutes(r *gin.Engine) {
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/v1")
	{
		// Log ingestion and retrieval
		v1.POST("/logs", LogsHandler)
		v1.GET("/logs", GetLogsHandler)
		v1.GET("/anomalies", GetAnomaliesHandler)

		// Search endpoints
		v1.GET("/search/anomalies", SearchAnomaliesHandler)
		v1.GET("/search/logs", SearchLogsHandler)

		// Statistics endpoints
		v1.GET("/stats/anomalies", GetAnomalyStatsHandler)
		v1.GET("/stats/logs", GetLogStatsHandler)

		// Detection result endpoints
		v1.POST("/detection", PushDetectionResultHandler)
		v1.POST("/detection/bulk", BulkPushDetectionResultsHandler)
	}
}
