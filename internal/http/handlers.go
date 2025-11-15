package http

import (
	"encoding/json"
	"net/http"

	domain "prsrv/internal/domain"
)

type Handlers struct {
	Svc  *domain.Service
	Auth Auth
}

func NewHandlers(s *domain.Service, admin, user string) *Handlers {
	return &Handlers{
		Svc:  s,
		Auth: Auth{AdminToken: admin, UserToken: user},
	}
}

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("/health", Require(RoleNone, h.Auth, h.handleHealth))

	mux.HandleFunc("/team/add", Require(RoleAdmin, h.Auth, h.handleTeamAdd))
	mux.HandleFunc("/team/get", Require(RoleUser, h.Auth, h.handleTeamGet))

	mux.HandleFunc("/users/setIsActive", Require(RoleAdmin, h.Auth, h.handleSetIsActive))
	mux.HandleFunc("/users/getReview", Require(RoleUser, h.Auth, h.handleUsersGetReview))
	mux.HandleFunc("/users/bulkDeactivate", Require(RoleAdmin, h.Auth, h.handleUsersBulkDeactivate))

	mux.HandleFunc("/pullRequest/create", Require(RoleAdmin, h.Auth, h.handlePRCreate))
	mux.HandleFunc("/pullRequest/merge", Require(RoleAdmin, h.Auth, h.handlePRMerge))
	mux.HandleFunc("/pullRequest/reassign", Require(RoleAdmin, h.Auth, h.handlePRReassign))

	mux.HandleFunc("/stats/assignments", Require(RoleUser, h.Auth, h.handleStatsAssignments))
}

func (h *Handlers) handleHealth(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handlers) handleTeamAdd(w http.ResponseWriter, r *http.Request) {
	var req domain.Team
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, string(domain.ErrNotFound), "invalid json")
		return
	}
	if req.TeamName == "" {
		writeError(w, http.StatusBadRequest, string(domain.ErrNotFound), "team_name is required")
		return
	}
	team, err := h.Svc.AddTeam(req)
	if err != nil {
		code, msg := domain.ParseErrorCode(err)
		if code == domain.ErrTeamExists {
			writeError(w, http.StatusBadRequest, string(code), msg)
			return
		}
		writeError(w, http.StatusInternalServerError, string(domain.ErrNotFound), err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"team": team})
}

func (h *Handlers) handleTeamGet(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("team_name")
	if name == "" {
		writeError(w, 400, string(domain.ErrNotFound), "team_name is required")
		return
	}
	team, err := h.Svc.GetTeam(name)
	if err != nil {
		code, msg := domain.ParseErrorCode(err)
		if code == domain.ErrNotFound {
			writeError(w, 404, string(code), msg)
			return
		}
		writeError(w, 500, string(domain.ErrNotFound), err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(team)
}

func (h *Handlers) handleSetIsActive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID   string `json:"user_id"`
		IsActive bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, string(domain.ErrNotFound), "invalid json")
		return
	}
	u, err := h.Svc.SetIsActive(req.UserID, req.IsActive)
	if err != nil {
		code, msg := domain.ParseErrorCode(err)
		if code == domain.ErrNotFound {
			writeError(w, 404, string(code), msg)
			return
		}
		writeError(w, 500, string(domain.ErrNotFound), err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"user": u})
}

func (h *Handlers) handleUsersGetReview(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("user_id")
	prs, err := h.Svc.ListUserPRs(uid)
	if err != nil {
		writeError(w, 500, string(domain.ErrNotFound), err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"user_id":       uid,
		"pull_requests": prs,
	})
}

func (h *Handlers) handleUsersBulkDeactivate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TeamName string   `json:"team_name"`
		UserIDs  []string `json:"user_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, string(domain.ErrNotFound), "invalid json")
		return
	}
	if req.TeamName == "" || len(req.UserIDs) == 0 {
		writeError(w, 400, string(domain.ErrNotFound), "team_name and user_ids are required")
		return
	}
	res, err := h.Svc.BulkDeactivateAndReassign(req.TeamName, req.UserIDs)
	if err != nil {
		writeError(w, 500, string(domain.ErrNotFound), err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(res)
}

func (h *Handlers) handlePRCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"pull_request_id"`
		Name     string `json:"pull_request_name"`
		AuthorID string `json:"author_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, string(domain.ErrNotFound), "invalid json")
		return
	}
	pr, err := h.Svc.CreatePR(req.ID, req.Name, req.AuthorID)
	if err != nil {
		code, msg := domain.ParseErrorCode(err)
		if code == domain.ErrPRExists {
			writeError(w, 409, string(code), msg)
			return
		}
		if code == domain.ErrNotFound {
			writeError(w, 404, string(code), msg)
			return
		}
		writeError(w, 500, string(domain.ErrNotFound), err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"pr": pr})
}

func (h *Handlers) handlePRMerge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"pull_request_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, string(domain.ErrNotFound), "invalid json")
		return
	}
	pr, err := h.Svc.MergePR(req.ID)
	if err != nil {
		code, msg := domain.ParseErrorCode(err)
		if code == domain.ErrNotFound {
			writeError(w, 404, string(code), msg)
			return
		}
		writeError(w, 500, string(domain.ErrNotFound), err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"pr": pr})
}

func (h *Handlers) handlePRReassign(w http.ResponseWriter, r *http.Request) {
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, 400, string(domain.ErrNotFound), "invalid json")
		return
	}
	prID, _ := raw["pull_request_id"].(string)
	old, _ := raw["old_user_id"].(string)
	if old == "" {
		old, _ = raw["old_reviewer_id"].(string)
	}
	pr, replacedBy, err := h.Svc.Reassign(prID, old)
	if err != nil {
		code, msg := domain.ParseErrorCode(err)
		switch code {
		case domain.ErrPRMerged, domain.ErrNotAssigned, domain.ErrNoCandidate:
			writeError(w, 409, string(code), msg)
		case domain.ErrNotFound:
			writeError(w, 404, string(code), msg)
		default:
			writeError(w, 500, string(domain.ErrNotFound), err.Error())
		}
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"pr": pr, "replaced_by": replacedBy})
}

func (h *Handlers) handleStatsAssignments(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group_by")
	if group == "" {
		group = "all"
	}
	stats, err := h.Svc.StatsAssignments(group)
	if err != nil {
		writeError(w, 500, string(domain.ErrNotFound), err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(stats)
}
