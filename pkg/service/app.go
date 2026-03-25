package service

import (
	"net/http"
	"time"

	"github.com/guyuxiang/projectc-custodial-wallet/pkg/config"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/store"
	"gorm.io/gorm"
)

type App struct {
	Wallet       WalletService
	SignatureKey SignatureKeyService
	CallbackMQ   *callbackConsumer
}

var app *App

func InitApp(cfg *config.Config, db *gorm.DB) error {
	st := store.New(db)
	if err := st.AutoMigrate(); err != nil {
		return err
	}

	timeout := 30 * time.Second
	if cfg != nil && cfg.Callback != nil && cfg.Callback.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.Callback.TimeoutSeconds) * time.Second
	}
	httpClient := &http.Client{Timeout: timeout}

	svc := NewWalletService(cfg, st, httpClient)
	signatureSvc := NewSignatureKeyService(st)
	if err := svc.EnsureWalletNetworks(); err != nil {
		return err
	}
	if err := svc.SyncSubscriptions(); err != nil {
		return err
	}

	var consumer *callbackConsumer
	if shouldUseRabbitMQ(cfg.RabbitMQ) {
		consumer = newCallbackConsumer(cfg.RabbitMQ, svc)
		if err := consumer.Start(); err != nil {
			return err
		}
	}

	app = &App{Wallet: svc, SignatureKey: signatureSvc, CallbackMQ: consumer}
	return nil
}

func GetApp() *App {
	return app
}
