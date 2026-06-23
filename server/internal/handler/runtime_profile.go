package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/pkg/agent"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// ---------------------------------------------------------------------------
// Custom Runtime Profiles (MUL-3284)
//
// A runtime_profile is a workspace-level, team-shared definition of a custom
// runtime — e.g. an in-house Codex wrapper. Daemons pull the enabled profiles
// for their workspace, resolve command_name on PATH, and register an
// agent_runtime instance carrying the profile_id. The profile only changes how
// a runtime is launched/displayed; the underlying protocol_family must be a
// backend Multica officially supports (validated against agent.SupportedTypes).
//
// Iron rule: a profile carries NO generic per-agent args. Per-agent launch args
// stay on agent.custom_args. The only args field is fixed_args — args every
// agent on this runtime must inherit to enter a compatible mode.
// ---------------------------------------------------------------------------

type RuntimeProfileResponse struct {
	ID             string   `json:"id"`
	WorkspaceID    string   `json:"workspace_id"`
	DisplayName    string   `json:"display_name"`
	ProtocolFamily string   `json:"protocol_family"`
	CommandName    string   `json:"command_name"`
	Description    *string  `json:"description"`
	FixedArgs      []string `json:"fixed_args"`
	Visibility     string   `json:"visibility"`
	CreatedBy      *string  `json:"created_by"`
	Enabled        bool     `json:"enabled"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

func runtimeProfileToResponse(p db.RuntimeProfile) RuntimeProfileResponse {
	args := []string{}
	if len(p.FixedArgs) > 0 {
		_ = json.Unmarshal(p.FixedArgs, &args)
		if args == nil {
			args = []string{}
		}
	}
	return RuntimeProfileResponse{
		ID:             uuidToString(p.ID),
		WorkspaceID:    uuidToString(p.WorkspaceID),
		DisplayName:    p.DisplayName,
		ProtocolFamily: p.ProtocolFamily,
		CommandName:    p.CommandName,
		Description:    textToPtr(p.Description),
		FixedArgs:      args,
		Visibility:     p.Visibility,
		CreatedBy:      uuidToPtr(p.CreatedBy),
		Enabled:        p.Enabled,
		CreatedAt:      timestampToString(p.CreatedAt),
		UpdatedAt:      timestampToString(p.UpdatedAt),
	}
}

// NOTE: runtime_profile.visibility is intentionally NOT user-settable in v1.
// The column exists and the API still returns it, but creation always forces
// 'workspace': the daemon-pull, DaemonRegister and ListRuntimeProfiles read
// paths do not yet enforce 'private', so accepting 'private' from a client
// would silently leak a "private" profile's name/command to other members and
// let other machines' daemons register it (lateral data leak). Re-expose a
// visibility control only once those read paths enforce creator visibility.
// Follow-up: MUL-3308.
const runtimeProfileDefaultVisibility = "workspace"

// marshalFixedArgs validates and JSON-encodes the fixed_args list. Each entry
// must be a non-empty string; the column defaults to an empty array.
func marshalFixedArgs(args []string) ([]byte, error) {
	if len(args) == 0 {
		return []byte("[]"), nil
	}
	clean := make([]string, 0, len(args))
	for _, a := range args {
		// fixed_args are launch flags inherited by every agent on the runtime;
		// blank entries are always a client mistake.
		if strings.TrimSpace(a) == "" {
			return nil, errors.New("fixed_args entries must be non-empty")
		}
		clean = append(clean, a)
	}
	return json.Marshal(clean)
}

type createRuntimeProfileRequest struct {
	DisplayName    string   `json:"display_name"`
	ProtocolFamily string   `json:"protocol_family"`
	CommandName    string   `json:"command_name"`
	Description    *string  `json:"description"`
	FixedArgs      []string `json:"fixed_args"`
	Enabled        *bool    `json:"enabled"`
}

// CreateRuntimeProfile creates a workspace runtime profile. Admin-gated by the
// router. protocol_family is validated against the agent backend whitelist.
func (h *Handler) CreateRuntimeProfile(w http.ResponseWriter, r *http.Request) {
	wsID := strings.TrimSpace(chi.URLParam(r, "id"))
	member, ok := h.requireWorkspaceMember(w, r, wsID, "workspace not found")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, wsID, "workspace id")
	if !ok {
		return
	}

	var req createRuntimeProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.ProtocolFamily = strings.TrimSpace(req.ProtocolFamily)
	req.CommandName = strings.TrimSpace(req.CommandName)

	if req.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "display_name is required")
		return
	}
	if !agent.IsSupportedType(req.ProtocolFamily) {
		writeError(w, http.StatusBadRequest, "unsupported protocol_family: must be one of "+strings.Join(agent.SupportedTypes, ", "))
		return
	}
	if req.CommandName == "" {
		writeError(w, http.StatusBadRequest, "command_name is required")
		return
	}
	fixedArgs, err := marshalFixedArgs(req.FixedArgs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	profile, err := h.Queries.CreateRuntimeProfile(r.Context(), db.CreateRuntimeProfileParams{
		WorkspaceID:    wsUUID,
		DisplayName:    req.DisplayName,
		ProtocolFamily: req.ProtocolFamily,
		CommandName:    req.CommandName,
		Description:    ptrToText(req.Description),
		FixedArgs:      fixedArgs,
		Visibility:     runtimeProfileDefaultVisibility,
		CreatedBy:      member.UserID,
		Enabled:        enabled,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "a runtime profile with this display_name already exists")
			return
		}
		slog.Error("CreateRuntimeProfile failed", "error", err, "workspace_id", wsID)
		writeError(w, http.StatusInternalServerError, "failed to create runtime profile")
		return
	}

	profileID := uuidToString(profile.ID)
	h.requestDaemonRuntimeProfileRefresh(wsID, profileID)
	h.publish(protocol.EventDaemonRegister, wsID, "member", uuidToString(member.UserID), map[string]any{
		"runtime_profile_id": profileID,
	})

	writeJSON(w, http.StatusCreated, runtimeProfileToResponse(profile))
}

// ListRuntimeProfiles returns every runtime profile in the workspace.
// Member-gated by the router.
func (h *Handler) ListRuntimeProfiles(w http.ResponseWriter, r *http.Request) {
	wsID := strings.TrimSpace(chi.URLParam(r, "id"))
	if _, ok := h.requireWorkspaceMember(w, r, wsID, "workspace not found"); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, wsID, "workspace id")
	if !ok {
		return
	}

	profiles, err := h.Queries.ListRuntimeProfiles(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runtime profiles")
		return
	}
	resp := make([]RuntimeProfileResponse, len(profiles))
	for i, p := range profiles {
		resp[i] = runtimeProfileToResponse(p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"runtime_profiles": resp})
}

// GetRuntimeProfile returns one runtime profile. Member-gated by the router.
func (h *Handler) GetRuntimeProfile(w http.ResponseWriter, r *http.Request) {
	wsID := strings.TrimSpace(chi.URLParam(r, "id"))
	if _, ok := h.requireWorkspaceMember(w, r, wsID, "workspace not found"); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, wsID, "workspace id")
	if !ok {
		return
	}
	profileUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "profileId"), "profile id")
	if !ok {
		return
	}

	profile, err := h.Queries.GetRuntimeProfileForWorkspace(r.Context(), db.GetRuntimeProfileForWorkspaceParams{
		ID:          profileUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "runtime profile not found")
		return
	}
	writeJSON(w, http.StatusOK, runtimeProfileToResponse(profile))
}

type updateRuntimeProfileRequest struct {
	DisplayName *string   `json:"display_name"`
	CommandName *string   `json:"command_name"`
	Description *string   `json:"description"`
	FixedArgs   *[]string `json:"fixed_args"`
	Enabled     *bool     `json:"enabled"`
}

// UpdateRuntimeProfile applies a partial update. protocol_family is immutable
// (changing it would silently repoint bound agents onto a different backend).
// Admin-gated by the router.
func (h *Handler) UpdateRuntimeProfile(w http.ResponseWriter, r *http.Request) {
	wsID := strings.TrimSpace(chi.URLParam(r, "id"))
	member, ok := h.requireWorkspaceMember(w, r, wsID, "workspace not found")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, wsID, "workspace id")
	if !ok {
		return
	}
	profileUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "profileId"), "profile id")
	if !ok {
		return
	}

	var req updateRuntimeProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if _, err := h.Queries.GetRuntimeProfileForWorkspace(r.Context(), db.GetRuntimeProfileForWorkspaceParams{
		ID:          profileUUID,
		WorkspaceID: wsUUID,
	}); err != nil {
		writeError(w, http.StatusNotFound, "runtime profile not found")
		return
	}
	if runtimeProfileOperationalChange(req) {
		runtimeIDs, err := h.Queries.ListAgentRuntimeIDsByProfile(r.Context(), profileUUID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to enumerate profile runtimes")
			return
		}
		if len(runtimeIDs) > 0 {
			writeError(w, http.StatusConflict, "cannot update runtime profile: registered runtime instances still exist")
			return
		}
	}

	params := db.UpdateRuntimeProfileParams{ID: profileUUID, WorkspaceID: wsUUID}
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if name == "" {
			writeError(w, http.StatusBadRequest, "display_name cannot be empty")
			return
		}
		params.DisplayName = strToText(name)
	}
	if req.CommandName != nil {
		cmd := strings.TrimSpace(*req.CommandName)
		if cmd == "" {
			writeError(w, http.StatusBadRequest, "command_name cannot be empty")
			return
		}
		params.CommandName = strToText(cmd)
	}
	if req.Description != nil {
		params.Description = ptrToText(req.Description)
	}
	if req.FixedArgs != nil {
		fixedArgs, err := marshalFixedArgs(*req.FixedArgs)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.FixedArgs = fixedArgs
	}
	if req.Enabled != nil {
		params.Enabled = pgtype.Bool{Bool: *req.Enabled, Valid: true}
	}

	profile, err := h.Queries.UpdateRuntimeProfile(r.Context(), params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "runtime profile not found")
			return
		}
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "a runtime profile with this display_name already exists")
			return
		}
		slog.Error("UpdateRuntimeProfile failed", "error", err, "profile_id", uuidToString(profileUUID))
		writeError(w, http.StatusInternalServerError, "failed to update runtime profile")
		return
	}

	profileID := uuidToString(profile.ID)
	h.requestDaemonRuntimeProfileRefresh(wsID, profileID)
	h.publish(protocol.EventDaemonRegister, wsID, "member", uuidToString(member.UserID), map[string]any{
		"runtime_profile_id": profileID,
	})

	writeJSON(w, http.StatusOK, runtimeProfileToResponse(profile))
}

func runtimeProfileOperationalChange(req updateRuntimeProfileRequest) bool {
	return req.CommandName != nil || req.FixedArgs != nil || req.Enabled != nil
}

// DeleteRuntimeProfile removes a workspace-level runtime profile.
// Runtime instances are owner-only resources, so profile deletion must not
// cascade into agent_runtime rows owned by other members. Disable the profile
// first to stop new daemon registrations, then delete it only after all runtime
// owners have removed their registered instances.
func (h *Handler) DeleteRuntimeProfile(w http.ResponseWriter, r *http.Request) {
	wsID := strings.TrimSpace(chi.URLParam(r, "id"))
	member, ok := h.requireWorkspaceMember(w, r, wsID, "workspace not found")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, wsID, "workspace id")
	if !ok {
		return
	}
	profileUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "profileId"), "profile id")
	if !ok {
		return
	}

	// Confirm the profile exists in this workspace before mutating anything.
	if _, err := h.Queries.GetRuntimeProfileForWorkspace(r.Context(), db.GetRuntimeProfileForWorkspaceParams{
		ID:          profileUUID,
		WorkspaceID: wsUUID,
	}); err != nil {
		writeError(w, http.StatusNotFound, "runtime profile not found")
		return
	}

	// Runtime instances are private to their owner. A workspace admin may
	// manage the shared profile definition, but cannot indirectly delete
	// another member's runtime by deleting the profile it registered from.
	runtimeIDs, err := h.Queries.ListAgentRuntimeIDsByProfile(r.Context(), profileUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enumerate profile runtimes")
		return
	}
	if len(runtimeIDs) > 0 {
		writeError(w, http.StatusConflict, "cannot delete runtime profile: registered runtime instances still exist")
		return
	}

	agentCount, err := h.Queries.CountAgentsByProfile(r.Context(), profileUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check profile usage")
		return
	}
	if agentCount > 0 {
		writeError(w, http.StatusConflict, "cannot delete runtime profile: active agents are still bound to its runtimes")
		return
	}

	if err := h.Queries.DeleteRuntimeProfile(r.Context(), db.DeleteRuntimeProfileParams{
		ID:          profileUUID,
		WorkspaceID: wsUUID,
	}); err != nil {
		slog.Error("DeleteRuntimeProfile failed", "error", err, "profile_id", uuidToString(profileUUID))
		writeError(w, http.StatusInternalServerError, "failed to delete runtime profile")
		return
	}

	profileID := uuidToString(profileUUID)
	h.requestDaemonRuntimeProfileRefresh(wsID, profileID)
	h.publish(protocol.EventDaemonRegister, wsID, "member", uuidToString(member.UserID), map[string]any{
		"deleted_runtime_profile_id": profileID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// DaemonListRuntimeProfiles serves the enabled runtime profiles for a workspace
// to a daemon. The daemon resolves each profile's command_name on PATH and
// registers an agent_runtime instance per profile it can run. Daemon-token
// gated by the router.
func (h *Handler) DaemonListRuntimeProfiles(w http.ResponseWriter, r *http.Request) {
	workspaceID := strings.TrimSpace(chi.URLParam(r, "workspaceId"))
	if !h.requireDaemonWorkspaceAccess(w, r, workspaceID) {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}

	profiles, err := h.Queries.ListEnabledRuntimeProfilesForWorkspace(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runtime profiles")
		return
	}
	resp := make([]RuntimeProfileResponse, len(profiles))
	for i, p := range profiles {
		resp[i] = runtimeProfileToResponse(p)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id":     workspaceID,
		"runtime_profiles": resp,
	})
}

func (h *Handler) requestDaemonRuntimeProfileRefresh(workspaceID, profileID string) {
	if h.DaemonProfileRefresh == nil {
		return
	}
	h.DaemonProfileRefresh.NotifyRuntimeProfilesChanged(workspaceID, profileID)
}
