package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type ProjectControlResponse struct {
	ProjectID           string          `json:"project_id"`
	OverseerAgentID     *string         `json:"overseer_agent_id"`
	DefaultPipelineID   *string         `json:"default_pipeline_id"`
	AutomationMode      string          `json:"automation_mode"`
	AutoTriageEnabled   bool            `json:"auto_triage_enabled"`
	AutoRouteEnabled    bool            `json:"auto_route_enabled"`
	AutoEscalateBlocked bool            `json:"auto_escalate_blocked"`
	StaleAfterMinutes   int32           `json:"stale_after_minutes"`
	ReviewPolicy        json.RawMessage `json:"review_policy"`
	QualityPolicy       json.RawMessage `json:"quality_policy"`
	CreatedAt           string          `json:"created_at"`
	UpdatedAt           string          `json:"updated_at"`
}

type UpdateProjectControlRequest struct {
	OverseerAgentID     *string `json:"overseer_agent_id"`
	DefaultPipelineID   *string `json:"default_pipeline_id"`
	AutomationMode      *string `json:"automation_mode"`
	AutoTriageEnabled   *bool   `json:"auto_triage_enabled"`
	AutoRouteEnabled    *bool   `json:"auto_route_enabled"`
	AutoEscalateBlocked *bool   `json:"auto_escalate_blocked"`
	StaleAfterMinutes   *int32  `json:"stale_after_minutes"`
}

func projectControlResponseFromRow(row db.GetProjectControlSettingsRow) ProjectControlResponse {
	return ProjectControlResponse{
		ProjectID:           uuidToString(row.ProjectID),
		OverseerAgentID:     uuidToPtr(row.OverseerAgentID),
		DefaultPipelineID:   uuidToPtr(row.DefaultPipelineID),
		AutomationMode:      row.AutomationMode,
		AutoTriageEnabled:   row.AutoTriageEnabled,
		AutoRouteEnabled:    row.AutoRouteEnabled,
		AutoEscalateBlocked: row.AutoEscalateBlocked,
		StaleAfterMinutes:   row.StaleAfterMinutes,
		ReviewPolicy:        json.RawMessage(row.ReviewPolicy),
		QualityPolicy:       json.RawMessage(row.QualityPolicy),
		CreatedAt:           timestampToString(row.CreatedAt),
		UpdatedAt:           timestampToString(row.UpdatedAt),
	}
}

func projectControlResponseFromModel(row db.ProjectControlSetting) ProjectControlResponse {
	return ProjectControlResponse{
		ProjectID:           uuidToString(row.ProjectID),
		OverseerAgentID:     uuidToPtr(row.OverseerAgentID),
		DefaultPipelineID:   uuidToPtr(row.DefaultPipelineID),
		AutomationMode:      row.AutomationMode,
		AutoTriageEnabled:   row.AutoTriageEnabled,
		AutoRouteEnabled:    row.AutoRouteEnabled,
		AutoEscalateBlocked: row.AutoEscalateBlocked,
		StaleAfterMinutes:   row.StaleAfterMinutes,
		ReviewPolicy:        json.RawMessage(row.ReviewPolicy),
		QualityPolicy:       json.RawMessage(row.QualityPolicy),
		CreatedAt:           timestampToString(row.CreatedAt),
		UpdatedAt:           timestampToString(row.UpdatedAt),
	}
}

func isValidAutomationMode(mode string) bool {
	return mode == "manual" || mode == "assisted" || mode == "autonomous"
}

func isJSONObject(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}

	_, ok := value.(map[string]any)
	return ok
}

func (h *Handler) GetProjectControl(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	workspaceID := h.resolveWorkspaceID(r)

	if _, err := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(workspaceID),
	}); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	settings, err := h.Queries.GetProjectControlSettings(r.Context(), parseUUID(id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get project control settings")
		return
	}

	writeJSON(w, http.StatusOK, projectControlResponseFromRow(settings))
}

func (h *Handler) UpsertProjectControl(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	workspaceID := h.resolveWorkspaceID(r)

	project, err := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var req UpdateProjectControlRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var rawFields map[string]json.RawMessage
	_ = json.Unmarshal(bodyBytes, &rawFields)

	current, err := h.Queries.GetProjectControlSettings(r.Context(), project.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get project control settings")
		return
	}

	params := db.UpsertProjectControlSettingsParams{
		ProjectID:           project.ID,
		OverseerAgentID:     current.OverseerAgentID,
		DefaultPipelineID:   current.DefaultPipelineID,
		AutomationMode:      current.AutomationMode,
		AutoTriageEnabled:   current.AutoTriageEnabled,
		AutoRouteEnabled:    current.AutoRouteEnabled,
		AutoEscalateBlocked: current.AutoEscalateBlocked,
		StaleAfterMinutes:   current.StaleAfterMinutes,
		ReviewPolicy:        current.ReviewPolicy,
		QualityPolicy:       current.QualityPolicy,
	}

	if _, ok := rawFields["overseer_agent_id"]; ok {
		if req.OverseerAgentID != nil {
			if _, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
				ID:          parseUUID(*req.OverseerAgentID),
				WorkspaceID: parseUUID(workspaceID),
			}); err != nil {
				writeError(w, http.StatusBadRequest, "overseer_agent_id must be a valid agent in this workspace")
				return
			}
			params.OverseerAgentID = parseUUID(*req.OverseerAgentID)
		} else {
			params.OverseerAgentID = db.UpsertProjectControlSettingsParams{}.OverseerAgentID
		}
	}

	if _, ok := rawFields["default_pipeline_id"]; ok {
		if req.DefaultPipelineID != nil {
			if _, err := h.Queries.GetPipelineInWorkspace(r.Context(), db.GetPipelineInWorkspaceParams{
				ID:          parseUUID(*req.DefaultPipelineID),
				WorkspaceID: parseUUID(workspaceID),
			}); err != nil {
				writeError(w, http.StatusBadRequest, "default_pipeline_id must be a valid pipeline in this workspace")
				return
			}
			params.DefaultPipelineID = parseUUID(*req.DefaultPipelineID)
		} else {
			params.DefaultPipelineID = db.UpsertProjectControlSettingsParams{}.DefaultPipelineID
		}
	}

	if _, ok := rawFields["automation_mode"]; ok {
		if req.AutomationMode == nil || !isValidAutomationMode(*req.AutomationMode) {
			writeError(w, http.StatusBadRequest, "automation_mode must be manual, assisted, or autonomous")
			return
		}
		params.AutomationMode = *req.AutomationMode
	}

	if _, ok := rawFields["auto_triage_enabled"]; ok {
		if req.AutoTriageEnabled == nil {
			writeError(w, http.StatusBadRequest, "auto_triage_enabled must be a boolean")
			return
		}
		params.AutoTriageEnabled = *req.AutoTriageEnabled
	}

	if _, ok := rawFields["auto_route_enabled"]; ok {
		if req.AutoRouteEnabled == nil {
			writeError(w, http.StatusBadRequest, "auto_route_enabled must be a boolean")
			return
		}
		params.AutoRouteEnabled = *req.AutoRouteEnabled
	}

	if _, ok := rawFields["auto_escalate_blocked"]; ok {
		if req.AutoEscalateBlocked == nil {
			writeError(w, http.StatusBadRequest, "auto_escalate_blocked must be a boolean")
			return
		}
		params.AutoEscalateBlocked = *req.AutoEscalateBlocked
	}

	if _, ok := rawFields["stale_after_minutes"]; ok {
		if req.StaleAfterMinutes == nil || *req.StaleAfterMinutes <= 0 {
			writeError(w, http.StatusBadRequest, "stale_after_minutes must be greater than 0")
			return
		}
		params.StaleAfterMinutes = *req.StaleAfterMinutes
	}

	if rawPolicy, ok := rawFields["review_policy"]; ok {
		if !isJSONObject(rawPolicy) {
			writeError(w, http.StatusBadRequest, "review_policy must be a JSON object")
			return
		}
		params.ReviewPolicy = rawPolicy
	}

	if rawPolicy, ok := rawFields["quality_policy"]; ok {
		if !isJSONObject(rawPolicy) {
			writeError(w, http.StatusBadRequest, "quality_policy must be a JSON object")
			return
		}
		params.QualityPolicy = rawPolicy
	}

	settings, err := h.Queries.UpsertProjectControlSettings(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to upsert project control settings")
		return
	}

	writeJSON(w, http.StatusOK, projectControlResponseFromModel(settings))
}
