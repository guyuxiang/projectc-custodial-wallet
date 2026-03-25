package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func main() {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(fmt.Errorf("generate ed25519 key pair: %w", err))
	}

	seed := privateKey.Seed()

	fmt.Println("ed25519 key pair generated")
	fmt.Printf("public_key_base64=%s\n", base64.StdEncoding.EncodeToString(publicKey))
	fmt.Printf("private_key_seed_base64=%s\n", base64.StdEncoding.EncodeToString(seed))
	fmt.Printf("private_key_base64=%s\n", base64.StdEncoding.EncodeToString(privateKey))
}
