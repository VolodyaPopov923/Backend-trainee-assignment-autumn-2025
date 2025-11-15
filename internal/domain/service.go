package domain

import (
	"database/sql"
	"errors"
	"sort"
)

type Repo interface {
	CreateTeam(tx *sql.Tx, teamName string) error
	TeamExists(tx *sql.Tx, teamName string) (bool, error)
	UpsertUser(tx *sql.Tx, u User) error
	GetTeamMembers(teamName string) ([]TeamMember, error)

	SetUserActive(uID string, active bool) (*User, error)
	GetUser(uID string) (*User, error)

	CreatePR(tx *sql.Tx, pr PullRequest) error
	GetPR(prID string) (*PullRequest, error)
	SetPRMerged(tx *sql.Tx, prID string) (*PullRequest, error)

	GetAuthorTeam(authorID string) (string, error)
	PickReviewersFromTeam(prID, team string, exclude []string, limit int) ([]string, error)

	GetAssignedReviewers(prID string) ([]string, error)
	AssignReviewers(tx *sql.Tx, prID string, userIDs []string) error
	ReplaceReviewer(tx *sql.Tx, prID, oldUser, newUser string) error
	DeleteReviewer(tx *sql.Tx, prID, userID string) error

	ListUserPRs(uID string) ([]PullRequestShort, error)

	StatsAssignmentsByUser() (map[string]int, error)
	StatsAssignmentsByPR() (map[string]int, error)

	BulkDeactivateUsers(team string, userIDs []string) ([]string, error)
	ListOpenAssignmentsByUsers(userIDs []string) ([]OpenAssignment, error)

	WithTx(fn func(tx *sql.Tx) error) error
}

type AssignmentStats struct {
	ByUser map[string]int `json:"by_user,omitempty"`
	ByPR   map[string]int `json:"by_pr,omitempty"`
}

type OpenAssignment struct {
	PRID        string
	AuthorID    string
	OldUserID   string
	OldUserTeam string
}

type BulkDeactivateResult struct {
	Team          string                `json:"team_name"`
	Deactivated   []string              `json:"deactivated_user_ids"`
	Reassignments []BulkReassignOutcome `json:"reassignments"`
}
type BulkReassignOutcome struct {
	PRID       string  `json:"pr_id"`
	OldUserID  string  `json:"old_user_id"`
	Action     string  `json:"action"`
	ReplacedBy *string `json:"replaced_by"`
}

type Service struct {
	repo Repo
}

func NewService(r Repo) *Service { return &Service{repo: r} }

func (s *Service) AddTeam(team Team) (*Team, error) {
	returnTeam := &Team{TeamName: team.TeamName}
	err := s.repo.WithTx(func(tx *sql.Tx) error {
		exists, err := s.repo.TeamExists(tx, team.TeamName)
		if err != nil {
			return err
		}
		if exists {
			return wrapCode(ErrTeamExists, "team_name already exists")
		}
		if err := s.repo.CreateTeam(tx, team.TeamName); err != nil {
			return err
		}
		for _, m := range team.Members {
			if err := s.repo.UpsertUser(tx, User{
				UserID:   m.UserID,
				Username: m.Username,
				TeamName: team.TeamName,
				IsActive: m.IsActive,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	members, err := s.repo.GetTeamMembers(team.TeamName)
	if err != nil {
		return nil, err
	}
	sort.Slice(members, func(i, j int) bool { return members[i].UserID < members[j].UserID })
	returnTeam.Members = members
	return returnTeam, nil
}

func (s *Service) GetTeam(teamName string) (*Team, error) {
	members, err := s.repo.GetTeamMembers(teamName)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, wrapCode(ErrNotFound, "team not found")
	}
	return &Team{TeamName: teamName, Members: members}, nil
}

func (s *Service) SetIsActive(userID string, active bool) (*User, error) {
	u, err := s.repo.SetUserActive(userID, active)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Service) CreatePR(prID, name, authorID string) (*PullRequest, error) {
	var out *PullRequest
	err := s.repo.WithTx(func(tx *sql.Tx) error {
		if _, err := s.repo.GetPR(prID); err == nil {
			return wrapCode(ErrPRExists, "PR id already exists")
		}
		author, err := s.repo.GetUser(authorID)
		if err != nil {
			return err
		}
		team := author.TeamName
		pr := PullRequest{ID: prID, Name: name, AuthorID: authorID, Status: StatusOPEN}
		if err := s.repo.CreatePR(tx, pr); err != nil {
			return err
		}
		cands, err := s.repo.PickReviewersFromTeam(prID, team, []string{authorID}, 2)
		if err != nil {
			return err
		}
		if err := s.repo.AssignReviewers(tx, prID, cands); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	pr, err := s.repo.GetPR(prID)
	if err != nil {
		return nil, err
	}
	revs, _ := s.repo.GetAssignedReviewers(prID)
	pr.AssignedReviewers = revs
	out = pr
	return out, nil
}

func (s *Service) MergePR(prID string) (*PullRequest, error) {
	var out *PullRequest
	err := s.repo.WithTx(func(tx *sql.Tx) error {
		pr, err := s.repo.GetPR(prID)
		if err != nil {
			return err
		}
		if pr.Status == StatusMERGED {
			out = pr
			return nil
		}
		pr, err = s.repo.SetPRMerged(tx, prID)
		if err != nil {
			return err
		}
		out = pr
		return nil
	})
	if err != nil {
		return nil, err
	}
	revs, _ := s.repo.GetAssignedReviewers(prID)
	out.AssignedReviewers = revs
	return out, nil
}

func (s *Service) Reassign(prID, oldUserID string) (*PullRequest, string, error) {
	var out *PullRequest
	var replacedBy string
	err := s.repo.WithTx(func(tx *sql.Tx) error {
		pr, err := s.repo.GetPR(prID)
		if err != nil {
			return err
		}
		if pr.Status == StatusMERGED {
			return wrapCode(ErrPRMerged, "cannot reassign on merged PR")
		}
		assigned, err := s.repo.GetAssignedReviewers(prID)
		if err != nil {
			return err
		}
		found := false
		for _, a := range assigned {
			if a == oldUserID {
				found = true
				break
			}
		}
		if !found {
			return wrapCode(ErrNotAssigned, "reviewer is not assigned to this PR")
		}
		oldUser, err := s.repo.GetUser(oldUserID)
		if err != nil {
			return err
		}
		excl := append(assigned, pr.AuthorID)
		cands, err := s.repo.PickReviewersFromTeam(prID, oldUser.TeamName, excl, 1)
		if err != nil {
			return err
		}
		if len(cands) == 0 {
			return wrapCode(ErrNoCandidate, "no active replacement candidate in team")
		}
		if err := s.repo.ReplaceReviewer(tx, prID, oldUserID, cands[0]); err != nil {
			return err
		}
		replacedBy = cands[0]
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	pr, err := s.repo.GetPR(prID)
	if err != nil {
		return nil, "", err
	}
	revs, _ := s.repo.GetAssignedReviewers(prID)
	pr.AssignedReviewers = revs
	out = pr
	return out, replacedBy, nil
}

func (s *Service) ListUserPRs(userID string) ([]PullRequestShort, error) {
	return s.repo.ListUserPRs(userID)
}

func (s *Service) StatsAssignments(groupBy string) (*AssignmentStats, error) {
	stats := &AssignmentStats{}
	switch groupBy {
	case "user":
		m, err := s.repo.StatsAssignmentsByUser()
		if err != nil {
			return nil, err
		}
		stats.ByUser = m
	case "pr":
		m, err := s.repo.StatsAssignmentsByPR()
		if err != nil {
			return nil, err
		}
		stats.ByPR = m
	default:
		mu, err := s.repo.StatsAssignmentsByUser()
		if err != nil {
			return nil, err
		}
		mp, err := s.repo.StatsAssignmentsByPR()
		if err != nil {
			return nil, err
		}
		stats.ByUser, stats.ByPR = mu, mp
	}
	return stats, nil
}

func (s *Service) BulkDeactivateAndReassign(team string, userIDs []string) (*BulkDeactivateResult, error) {
	res := &BulkDeactivateResult{Team: team}

	err := s.repo.WithTx(func(tx *sql.Tx) error {
		deactivated, err := s.repo.BulkDeactivateUsers(team, userIDs)
		if err != nil {
			return err
		}
		res.Deactivated = deactivated
		if len(deactivated) == 0 {
			return nil
		}

		open, err := s.repo.ListOpenAssignmentsByUsers(deactivated)
		if err != nil {
			return err
		}

		for _, item := range open {
			assigned, err := s.repo.GetAssignedReviewers(item.PRID)
			if err != nil {
				return err
			}
			excl := append(append([]string{}, assigned...), item.AuthorID)
			cands, err := s.repo.PickReviewersFromTeam(item.PRID, item.OldUserTeam, excl, 1)
			if err != nil {
				return err
			}
			if len(cands) > 0 {
				if err := s.repo.ReplaceReviewer(tx, item.PRID, item.OldUserID, cands[0]); err != nil {
					return err
				}
				r := cands[0]
				res.Reassignments = append(res.Reassignments, BulkReassignOutcome{
					PRID: item.PRID, OldUserID: item.OldUserID, Action: "replaced", ReplacedBy: &r,
				})
			} else {
				if err := s.repo.DeleteReviewer(tx, item.PRID, item.OldUserID); err != nil {
					return err
				}
				res.Reassignments = append(res.Reassignments, BulkReassignOutcome{
					PRID: item.PRID, OldUserID: item.OldUserID, Action: "removed", ReplacedBy: nil,
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func wrapCode(code ErrorCode, msg string) error {
	return errors.New(string(code) + ":" + msg)
}

func ParseErrorCode(err error) (ErrorCode, string) {
	if err == nil {
		return "", ""
	}
	s := err.Error()
	for _, c := range []ErrorCode{ErrTeamExists, ErrPRExists, ErrPRMerged, ErrNotAssigned, ErrNoCandidate, ErrNotFound} {
		prefix := string(c) + ":"
		if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
			return c, s[len(prefix):]
		}
	}
	return "", s
}
