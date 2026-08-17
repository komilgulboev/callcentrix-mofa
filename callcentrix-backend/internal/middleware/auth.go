package middleware

import (
	"context"
	"net/http"
	"strings"

	"callcentrix/internal/jwt"
)

type contextKey string

const ClaimsKey contextKey = "claims"

func Auth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			claims, err := jwt.Parse(secret, token)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetClaims(r *http.Request) *jwt.Claims {
	c, _ := r.Context().Value(ClaimsKey).(*jwt.Claims)
	return c
}

func RequireRole(minType int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := GetClaims(r)
			if c == nil || c.UserType > minType {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireTenantAdmin allows only TenantAdmin (userType == 1).
func RequireTenantAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := GetClaims(r)
		if c == nil || c.UserType != 1 {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractToken(r *http.Request) string {
	// Header: Authorization: Bearer <token>
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	// Query param: ?token=...  (used by WebSocket)
	return r.URL.Query().Get("token")
}
