package http

import (
	"log"
	"net/http"
	"strings"
	"time"
)

type Role int

const (
	RoleNone Role = iota
	RoleUser
	RoleAdmin
)

type Auth struct {
	AdminToken string
	UserToken  string
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		d := time.Since(start)
		log.Printf("%s %s %s", r.Method, r.URL.Path, d)
	})
}

func (a Auth) RoleFrom(r *http.Request) Role {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		t := strings.TrimPrefix(auth, "Bearer ")
		if t == a.AdminToken && t != "" {
			return RoleAdmin
		}
		if t == a.UserToken && t != "" {
			return RoleUser
		}
	}
	return RoleNone
}

func Require(role Role, a Auth, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if role == RoleNone {
			h(w, r)
			return
		}
		if a.RoleFrom(r) < role {
			writeError(w, http.StatusUnauthorized, "NOT_FOUND", "unauthorized")
			return
		}
		h(w, r)
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":{"code":"` + code + `","message":"` + msg + `"}}`))
}
