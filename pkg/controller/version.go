package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/guyuxiang/projectc-wallet/pkg/config"
)

func Version(c *gin.Context) {
	c.JSON(200, config.FLAG_KEY_SERVER_VERSION)
}
