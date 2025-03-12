package api

import (
	"reacher-cron/api/v1/health"
	"reacher-cron/config"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func StartServer() {
	r := gin.Default()

	// Configuração básica de CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	SetupRoutes(r)

	r.Run(":" + config.AppConfig.Port)
}

func SetupRoutes(r *gin.Engine) {
	v1 := r.Group("/api/v1")
	{

		healthApi := v1.Group("/health")
		{
			healthApi.GET("", health.GetHealthCron)
		}

	}
}
