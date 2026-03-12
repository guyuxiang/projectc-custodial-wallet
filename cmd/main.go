package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/guyuxiang/projectc-wallet/pkg/config"
	"github.com/guyuxiang/projectc-wallet/pkg/log"
	"github.com/guyuxiang/projectc-wallet/pkg/mysql"
	"github.com/guyuxiang/projectc-wallet/pkg/rabbitmq"
	"github.com/guyuxiang/projectc-wallet/pkg/route"
	"github.com/guyuxiang/projectc-wallet/pkg/util"
)

func main() {
	util.SetupSigusr1Trap()

	if _, err := mysql.Init(config.GetConfig().MySQL); err != nil {
		log.Fatalf("init mysql failed: %v", err)
	}
	defer func() {
		if err := mysql.Close(); err != nil {
			log.Errorf("close mysql failed: %v", err)
		}
	}()

	if _, err := rabbitmq.Init(config.GetConfig().RabbitMQ); err != nil {
		log.Fatalf("init rabbitmq failed: %v", err)
	}
	defer func() {
		if err := rabbitmq.Close(); err != nil {
			log.Errorf("close rabbitmq failed: %v", err)
		}
	}()

	r := gin.Default()
	m := config.GetString(config.FLAG_KEY_GIN_MODE)
	gin.SetMode(m)

	route.InstallRoutes(r)
	serverBindAddr := fmt.Sprintf("%s:%d", config.GetString(config.FLAG_KEY_SERVER_HOST), config.GetInt(config.FLAG_KEY_SERVER_PORT))
	log.Infof("mysql initialized successfully")
	log.Infof("rabbitmq initialized successfully")
	log.Infof("Run server at %s", serverBindAddr)
	r.Run(serverBindAddr) // listen and serve
}
