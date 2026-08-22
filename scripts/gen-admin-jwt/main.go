package main

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
)

const defaultTTL = 1 * time.Hour

func Loadenv() {
	if err := godotenv.Load(); err != nil {
		fmt.Println(err)
	}
}

func main() {
	Loadenv()
	secret := os.Getenv("ADMIN_JWT_SECRET")
	if secret == "" {
		fmt.Fprintln(os.Stderr, "error: ADMIN_JWT_SECRET is not set")
		os.Exit(1)
	}

	subject := os.Getenv("ADMIN_JWT_SUBJECT")
	if subject == "" {
		subject = "blog-admin"
	}

	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   subject,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(defaultTTL)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to sign token: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(signed)
}
