package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetHealthCron(c *gin.Context) {

	c.JSON(http.StatusOK, gin.H{
		"status":  http.StatusOK,
		"message": "ok",
	})
}
