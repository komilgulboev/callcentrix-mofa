package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Sub      int    `json:"sub"`
	Username string `json:"username"`
	TenantID *int   `json:"tenantId"`
	UserType int    `json:"userType"`
	Role     int    `json:"role"`
	jwt.RegisteredClaims
}

var ErrInvalid = errors.New("invalid token")

func Generate(secret string, ttlMinutes int, userID int, username string, tenantID *int, userType, role int) (string, error) {
	claims := Claims{
		Sub:      userID,
		Username: username,
		TenantID: tenantID,
		UserType: userType,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(ttlMinutes) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func Parse(secret, tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalid
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalid
	}
	c, ok := token.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalid
	}
	return c, nil
}
