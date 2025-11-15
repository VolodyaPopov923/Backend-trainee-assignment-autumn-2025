package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"

	servicepkg "prsrv/internal/domain"
	handlerspkg "prsrv/internal/http"
	repopg "prsrv/internal/repo"
)

func main() {
	addr := getenv("ADDR", ":8080")
	dsn := getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/prsrv?sslmode=disable")
	admin := getenv("ADMIN_TOKEN", "admin")
	user := getenv("USER_TOKEN", "user")

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	if err := repopg.RunMigrations(db, getenv("MIGRATIONS_DIR", "./migrations")); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	repo := repopg.NewPostgresRepo(db)
	service := servicepkg.NewService(repo)
	h := handlerspkg.NewHandlers(service, admin, user)

	mux := http.NewServeMux()
	h.Register(mux)

	srv := &http.Server{
		Addr:    addr,
		Handler: handlerspkg.LoggingMiddleware(mux),
	}

	log.Printf("listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
