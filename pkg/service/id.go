package service

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync/atomic"
	"time"
)

var idSeq uint64

func generateID(prefix string) string {
	seq := atomic.AddUint64(&idSeq, 1) % 10000
	return fmt.Sprintf("%s%d%04d", prefix, time.Now().UnixMilli(), seq)
}

func generateWalletNo() string {
	return generateID("W")
}

func generatePassword() string {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return generateID("P")
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
