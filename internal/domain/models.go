package domain

import "time"

type PRStatus string

const (
	StatusOPEN   PRStatus = "OPEN"
	StatusMERGED PRStatus = "MERGED"
)

type ErrorCode string

const (
	ErrTeamExists  ErrorCode = "TEAM_EXISTS"
	ErrPRExists    ErrorCode = "PR_EXISTS"
	ErrPRMerged    ErrorCode = "PR_MERGED"
	ErrNotAssigned ErrorCode = "NOT_ASSIGNED"
	ErrNoCandidate ErrorCode = "NO_CANDIDATE"
	ErrNotFound    ErrorCode = "NOT_FOUND"
)

type TeamMember struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

type Team struct {
	TeamName string       `json:"team_name"`
	Members  []TeamMember `json:"members"`
}

type User struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	TeamName string `json:"team_name"`
	IsActive bool   `json:"is_active"`
}

type PullRequest struct {
	ID                string     `json:"pull_request_id"`
	Name              string     `json:"pull_request_name"`
	AuthorID          string     `json:"author_id"`
	Status            PRStatus   `json:"status"`
	AssignedReviewers []string   `json:"assigned_reviewers"`
	CreatedAt         *time.Time `json:"createdAt,omitempty"`
	MergedAt          *time.Time `json:"mergedAt,omitempty"`
}

type PullRequestShort struct {
	ID       string   `json:"pull_request_id"`
	Name     string   `json:"pull_request_name"`
	AuthorID string   `json:"author_id"`
	Status   PRStatus `json:"status"`
}
