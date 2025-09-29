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
		v1.POST("/logs", LogsHandler)
	}
}
