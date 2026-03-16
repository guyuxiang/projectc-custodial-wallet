package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/config"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/signature"
)

func RequestSignatureMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.GetConfig()
		if cfg == nil || cfg.Signature == nil || cfg.Signature.APIKey == "" || cfg.Signature.PublicKey == "" {
			c.Next()
			return
		}

		apiKey := c.GetHeader("X-API-Key")
		timestamp := c.GetHeader("X-Timestamp")
		signatureValue := c.GetHeader("X-Signature")
		if apiKey == "" || timestamp == "" || signatureValue == "" {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSignatureInvalid, Message: "missing signature headers", Data: struct{}{}})
			return
		}
		if apiKey != cfg.Signature.APIKey {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodePermissionDenied, Message: "invalid api key", Data: struct{}{}})
			return
		}

		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSignatureInvalid, Message: "invalid timestamp", Data: struct{}{}})
			return
		}
		maxSkew := cfg.Signature.MaxSkewMillis
		if maxSkew <= 0 {
			maxSkew = 300000
		}
		if absDuration(time.Now().UnixMilli()-ts) > maxSkew {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSignatureInvalid, Message: "timestamp expired", Data: struct{}{}})
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSystemBusy, Message: err.Error(), Data: struct{}{}})
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		if err := signature.VerifyBase64(
			cfg.Signature.PublicKey,
			signatureValue,
			c.Request.Method,
			c.Request.URL.Path,
			c.Request.URL.RawQuery,
			body,
			timestamp,
		); err != nil {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSignatureInvalid, Message: "invalid signature", Data: struct{}{}})
			return
		}
		c.Next()
	}
}

func absDuration(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
