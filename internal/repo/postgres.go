package repo

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	domain "prsrv/internal/domain"
)

type PostgresRepo struct {
	db *sql.DB
}

func NewPostgresRepo(db *sql.DB) *PostgresRepo { return &PostgresRepo{db: db} }

func (r *PostgresRepo) WithTx(fn func(tx *sql.Tx) error) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	err = fn(tx)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepo) CreateTeam(tx *sql.Tx, teamName string) error {
	_, err := tx.Exec(`insert into teams(team_name) values ($1)`, teamName)
	return err
}

func (r *PostgresRepo) TeamExists(tx *sql.Tx, teamName string) (bool, error) {
	var exists bool
	err := tx.QueryRow(`select exists(select 1 from teams where team_name=$1)`, teamName).Scan(&exists)
	return exists, err
}

func (r *PostgresRepo) UpsertUser(tx *sql.Tx, u domain.User) error {
	_, err := tx.Exec(`
		insert into users(user_id, username, team_name, is_active)
		values ($1,$2,$3,$4)
		on conflict (user_id)
		do update set username=excluded.username,
		             team_name=excluded.team_name,
		             is_active=excluded.is_active
	`, u.UserID, u.Username, u.TeamName, u.IsActive)
	return err
}

func (r *PostgresRepo) GetTeamMembers(teamName string) ([]domain.TeamMember, error) {
	rows, err := r.db.Query(`select user_id, username, is_active from users where team_name=$1 order by user_id`, teamName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TeamMember
	for rows.Next() {
		var m domain.TeamMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.IsActive); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *PostgresRepo) SetUserActive(uID string, active bool) (*domain.User, error) {
	res, err := r.db.Exec(`update users set is_active=$1 where user_id=$2`, active, uID)
	if err != nil {
		return nil, err
	}
	a, _ := res.RowsAffected()
	if a == 0 {
		return nil, errors.New(string(domain.ErrNotFound) + ":user not found")
	}
	return r.GetUser(uID)
}

func (r *PostgresRepo) GetUser(uID string) (*domain.User, error) {
	u := &domain.User{}
	err := r.db.QueryRow(`select user_id, username, team_name, is_active from users where user_id=$1`, uID).
		Scan(&u.UserID, &u.Username, &u.TeamName, &u.IsActive)
	if err == sql.ErrNoRows {
		return nil, errors.New(string(domain.ErrNotFound) + ":user not found")
	}
	return u, err
}

func (r *PostgresRepo) CreatePR(tx *sql.Tx, pr domain.PullRequest) error {
	_, err := tx.Exec(`insert into pull_requests(pr_id, pr_name, author_id, status, created_at)
		values ($1,$2,$3,'OPEN', now())`, pr.ID, pr.Name, pr.AuthorID)
	return err
}

func (r *PostgresRepo) GetPR(prID string) (*domain.PullRequest, error) {
	row := r.db.QueryRow(`select pr_id, pr_name, author_id, status, created_at, merged_at from pull_requests where pr_id=$1`, prID)
	var pr domain.PullRequest
	var createdAt, mergedAt sql.NullTime
	if err := row.Scan(&pr.ID, &pr.Name, &pr.AuthorID, &pr.Status, &createdAt, &mergedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New(string(domain.ErrNotFound) + ":PR not found")
		}
		return nil, err
	}
	if createdAt.Valid {
		t := createdAt.Time.UTC()
		pr.CreatedAt = &t
	}
	if mergedAt.Valid {
		t := mergedAt.Time.UTC()
		pr.MergedAt = &t
	}
	rev, _ := r.GetAssignedReviewers(prID)
	pr.AssignedReviewers = rev
	return &pr, nil
}

func (r *PostgresRepo) SetPRMerged(tx *sql.Tx, prID string) (*domain.PullRequest, error) {
	_, err := tx.Exec(`update pull_requests set status='MERGED', merged_at=now() where pr_id=$1`, prID)
	if err != nil {
		return nil, err
	}
	return r.GetPR(prID)
}

func (r *PostgresRepo) GetAuthorTeam(authorID string) (string, error) {
	var team string
	err := r.db.QueryRow(`select team_name from users where user_id=$1`, authorID).Scan(&team)
	if err == sql.ErrNoRows {
		return "", errors.New(string(domain.ErrNotFound) + ":author not found")
	}
	return team, err
}

func (r *PostgresRepo) PickReviewersFromTeam(prID, team string, exclude []string, limit int) ([]string, error) {
	q := `
		select u.user_id
		from users u
		where u.team_name=$1
		  and u.is_active=true
		  and (array_length($2::text[], 1) is null or u.user_id <> all($2::text[]))
		order by md5($3 || u.user_id)
		limit $4
	`
	rows, err := r.db.Query(q, team, pqStringArray(exclude), prID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (r *PostgresRepo) GetAssignedReviewers(prID string) ([]string, error) {
	rows, err := r.db.Query(`select user_id from pr_reviewers where pr_id=$1 order by user_id`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (r *PostgresRepo) AssignReviewers(tx *sql.Tx, prID string, userIDs []string) error {
	for _, id := range userIDs {
		if _, err := tx.Exec(`insert into pr_reviewers(pr_id, user_id)
			values ($1,$2) on conflict do nothing`, prID, id); err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresRepo) ReplaceReviewer(tx *sql.Tx, prID, oldUser, newUser string) error {
	if _, err := tx.Exec(`delete from pr_reviewers where pr_id=$1 and user_id=$2`, prID, oldUser); err != nil {
		return err
	}
	_, err := tx.Exec(`insert into pr_reviewers(pr_id, user_id)
		values ($1,$2) on conflict do nothing`, prID, newUser)
	return err
}

func (r *PostgresRepo) DeleteReviewer(tx *sql.Tx, prID, userID string) error {
	_, err := tx.Exec(`delete from pr_reviewers where pr_id=$1 and user_id=$2`, prID, userID)
	return err
}

func (r *PostgresRepo) ListUserPRs(uID string) ([]domain.PullRequestShort, error) {
	rows, err := r.db.Query(`
		select p.pr_id, p.pr_name, p.author_id, p.status
		from pull_requests p
		join pr_reviewers r using(pr_id)
		where r.user_id=$1
		order by p.pr_id`, uID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PullRequestShort
	for rows.Next() {
		var s domain.PullRequestShort
		if err := rows.Scan(&s.ID, &s.Name, &s.AuthorID, &s.Status); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *PostgresRepo) StatsAssignmentsByUser() (map[string]int, error) {
	rows, err := r.db.Query(`select user_id, count(*) from pr_reviewers group by user_id order by user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var id string
		var cnt int
		if err := rows.Scan(&id, &cnt); err != nil {
			return nil, err
		}
		out[id] = cnt
	}
	return out, nil
}

func (r *PostgresRepo) StatsAssignmentsByPR() (map[string]int, error) {
	rows, err := r.db.Query(`select pr_id, count(*) from pr_reviewers group by pr_id order by pr_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var id string
		var cnt int
		if err := rows.Scan(&id, &cnt); err != nil {
			return nil, err
		}
		out[id] = cnt
	}
	return out, nil
}

func (r *PostgresRepo) BulkDeactivateUsers(team string, userIDs []string) ([]string, error) {
	rows, err := r.db.Query(`select user_id from users where team_name=$1 and user_id = any($2::text[])`, team, pqStringArray(userIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var target []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		target = append(target, id)
	}
	if len(target) == 0 {
		return []string{}, nil
	}

	_, err = r.db.Exec(`update users set is_active=false where team_name=$1 and user_id = any($2::text[])`, team, pqStringArray(target))
	if err != nil {
		return nil, err
	}
	return target, nil
}

func (r *PostgresRepo) ListOpenAssignmentsByUsers(userIDs []string) ([]domain.OpenAssignment, error) {
	q := `
		select pr.pr_id, pr.author_id, u.user_id, u.team_name
		from pr_reviewers r
		join pull_requests pr on pr.pr_id = r.pr_id
		join users u on u.user_id = r.user_id
		where pr.status='OPEN'
		  and r.user_id = any($1::text[])
		order by pr.pr_id
	`
	rows, err := r.db.Query(q, pqStringArray(userIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.OpenAssignment
	for rows.Next() {
		var item domain.OpenAssignment
		if err := rows.Scan(&item.PRID, &item.AuthorID, &item.OldUserID, &item.OldUserTeam); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func RunMigrations(db *sql.DB, dir string) error {
	files := []string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".up.sql") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(b)); err != nil {
			return fmt.Errorf("migration %s: %w", f, err)
		}
	}
	return nil
}

func pqStringArray(a []string) string {
	if len(a) == 0 {
		return "{}"
	}
	return "{" + strings.Join(a, ",") + "}"
}
