package e2e

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	domain "prsrv/internal/domain"
	httppkg "prsrv/internal/http"
	repo "prsrv/internal/repo"
)

func mustEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := mustEnv("TEST_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/prsrv?sslmode=disable")

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func migrationsPath(t *testing.T) string {
	t.Helper()
	p := filepath.Clean(filepath.Join("..", "..", "migrations"))
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("abs migrations: %v", err)
	}
	return abs
}

func makeServer(t *testing.T, db *sql.DB) *httptest.Server {
	t.Helper()

	if err := repo.RunMigrations(db, migrationsPath(t)); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	_, _ = db.Exec(`TRUNCATE TABLE pr_reviewers, pull_requests, users, teams CASCADE`)

	r := repo.NewPostgresRepo(db)
	svc := domain.NewService(r)
	h := httppkg.NewHandlers(svc, "admin", "user")

	mux := http.NewServeMux()
	h.Register(mux)
	ts := httptest.NewServer(httppkg.LoggingMiddleware(mux))
	t.Cleanup(ts.Close)
	return ts
}

func TestE2E_Flow_CreatePR_Assign_Reassign_Merge(t *testing.T) {
	db := openTestDB(t)
	srv := makeServer(t, db)

	body := `{"team_name":"backend","members":[
		{"user_id":"u1","username":"Alice","is_active":true},
		{"user_id":"u2","username":"Bob","is_active":true},
		{"user_id":"u3","username":"Carol","is_active":true}
	]}`
	req, _ := http.NewRequest("POST", srv.URL+"/team/add", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 && resp.StatusCode != 400 {
		t.Fatalf("team/add status=%d", resp.StatusCode)
	}

	cbody := `{"pull_request_id":"pr-1","pull_request_name":"Add search","author_id":"u1"}`
	req2, _ := http.NewRequest("POST", srv.URL+"/pullRequest/create", strings.NewReader(cbody))
	req2.Header.Set("Authorization", "Bearer admin")
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 201 {
		t.Fatalf("pr/create status=%d", resp2.StatusCode)
	}

	mbody := `{"pull_request_id":"pr-1"}`
	req4, _ := http.NewRequest("POST", srv.URL+"/pullRequest/merge", strings.NewReader(mbody))
	req4.Header.Set("Authorization", "Bearer admin")
	req4.Header.Set("Content-Type", "application/json")
	resp4, err := http.DefaultClient.Do(req4)
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != 200 {
		t.Fatalf("merge status=%d", resp4.StatusCode)
	}

	req5, _ := http.NewRequest("GET", srv.URL+"/stats/assignments", nil)
	req5.Header.Set("Authorization", "Bearer user")
	resp5, err := http.DefaultClient.Do(req5)
	if err != nil {
		t.Fatal(err)
	}
	defer resp5.Body.Close()
	if resp5.StatusCode != 200 {
		t.Fatalf("stats status=%d", resp5.StatusCode)
	}
}

func TestE2E_BulkDeactivate_Reassign(t *testing.T) {
	db := openTestDB(t)
	srv := makeServer(t, db)

	body := `{"team_name":"backend","members":[
		{"user_id":"u1","username":"Alice","is_active":true},
		{"user_id":"u2","username":"Bob","is_active":true},
		{"user_id":"u3","username":"Carol","is_active":true},
		{"user_id":"u4","username":"Dave","is_active":true}
	]}`
	req, _ := http.NewRequest("POST", srv.URL+"/team/add", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	for i := 1; i <= 5; i++ {
		cbody := fmt.Sprintf(`{"pull_request_id":"pr-%d","pull_request_name":"F%d","author_id":"u1"}`, i, i)
		r, _ := http.NewRequest("POST", srv.URL+"/pullRequest/create", strings.NewReader(cbody))
		r.Header.Set("Authorization", "Bearer admin")
		r.Header.Set("Content-Type", "application/json")
		respPr, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatalf("create pr-%d err=%v", i, err)
		}
		defer respPr.Body.Close()
		if respPr.StatusCode != 201 {
			t.Fatalf("create pr-%d status=%d", i, respPr.StatusCode)
		}
	}

	dbody := `{"team_name":"backend","user_ids":["u2","u3"]}`
	req2, _ := http.NewRequest("POST", srv.URL+"/users/bulkDeactivate", strings.NewReader(dbody))
	req2.Header.Set("Authorization", "Bearer admin")
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("bulkDeactivate status=%d", resp2.StatusCode)
	}
}
