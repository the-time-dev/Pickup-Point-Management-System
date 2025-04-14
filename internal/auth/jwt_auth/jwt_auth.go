package jwt_auth

import (
	"avito_intr/internal/auth"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"time"
)

type JwtAuth struct {
	secretKey []byte
}

func NewJwtAuth(secretKey string) *JwtAuth {
	return &JwtAuth{secretKey: []byte(secretKey)}
}

func (gen *JwtAuth) Generate(id, role string) (string, error) {
	claims := jwt.MapClaims{
		"id":   id,
		"role": role,
		"iat":  time.Now().Unix(), // время выпуска
		"exp":  time.Now().Add(12 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signedToken, err := token.SignedString(gen.secretKey)
	if err != nil {
		return "", err
	}
	return signedToken, nil
}

func (gen *JwtAuth) Validate(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return gen.secretKey, nil
	})
	if err != nil {
		return "", err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		if exp, ok := claims["exp"].(float64); ok {
			if int64(exp) < time.Now().Unix() {
				return "", auth.TokenExpired{}
			}
		}
		return claims["id"].(string), nil
	} else {
		return "", errors.New("invalid token")
	}
}
