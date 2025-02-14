package routes

import (
	"database/sql"
	"rtsp_streamer/controllers"
	"rtsp_streamer/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, db *sql.DB) {
	r.GET("/streams/*filepath", func(c *gin.Context) {
		// r.GET("/streams/*filepath", middleware.HLSAuthMiddleware(), func(c *gin.Context) {
		filepath := c.Param("filepath")
		fullPath := "./streams" + filepath

		c.File(fullPath)
	})

	cameraRoutes := r.Group("/")
	{
		cameraRoutes.POST("/start/:cameraID", middleware.AuthMiddleware(), func(c *gin.Context) {
			controllers.StartCameraStream(c, db)
		})
		cameraRoutes.POST("/stop/:cameraID", middleware.AuthMiddleware(), func(c *gin.Context) {
			controllers.StopCameraStream(c, db)
		})
	}
}
