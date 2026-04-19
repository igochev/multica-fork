package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	maxOverseerProductContextLength = 1000
	maxOverseerListValueLength      = 64
)

type OverseerScanFocus string

const (
	OverseerScanFocusSecurity      OverseerScanFocus = "security"
	OverseerScanFocusTestCoverage  OverseerScanFocus = "test_coverage"
	OverseerScanFocusCodeQuality   OverseerScanFocus = "code_quality"
	OverseerScanFocusDocumentation OverseerScanFocus = "documentation"
	OverseerScanFocusArchitecture  OverseerScanFocus = "architecture"
)

var allowedOverseerScanFocus = map[OverseerScanFocus]struct{}{
	OverseerScanFocusSecurity:      {},
	OverseerScanFocusTestCoverage:  {},
	OverseerScanFocusCodeQuality:   {},
	OverseerScanFocusDocumentation: {},
	OverseerScanFocusArchitecture:  {},
}

type OverseerPriorityWeights map[string]float64

type OverseerConfig struct {
	ScanIntervalHours *int32                  `json:"scan_interval_hours,omitempty"`
	ScanFocus         []OverseerScanFocus     `json:"scan_focus,omitempty"`
	ProductContext    *string                 `json:"product_context,omitempty"`
	QualityBar        []string                `json:"quality_bar,omitempty"`
	PriorityWeights   OverseerPriorityWeights `json:"priority_weights,omitempty"`
	MaxIssuesPerRun   *int32                  `json:"max_issues_per_run,omitempty"`
	RequireApproval   *bool                   `json:"require_approval,omitempty"`
}

type ProjectOverseerAutonomyResponse struct {
	AutopilotID string  `json:"autopilot_id"`
	Status      string  `json:"status"`
	TriggerID   *string `json:"trigger_id"`
	NextRunAt   *string `json:"next_run_at"`
}

type ProjectControlResponse struct {
	ProjectID                 string                           `json:"project_id"`
	OverseerAgentID           *string                          `json:"overseer_agent_id"`
	OverseerAutopilotID       *string                          `json:"overseer_autopilot_id"`
	OverseerAutonomyStatus    *string                          `json:"overseer_autonomy_status"`
	OverseerAutonomyTriggerID *string                          `json:"overseer_autonomy_trigger_id"`
	OverseerAutonomyNextRunAt *string                          `json:"overseer_autonomy_next_run_at"`
	OverseerAutonomy          *ProjectOverseerAutonomyResponse `json:"overseer_autonomy"`
	DefaultPipelineID         *string                          `json:"default_pipeline_id"`
	AutomationMode            string                           `json:"automation_mode"`
	AutoTriageEnabled         bool                             `json:"auto_triage_enabled"`
	AutoRouteEnabled          bool                             `json:"auto_route_enabled"`
	AutoEscalateBlocked       bool                             `json:"auto_escalate_blocked"`
	StaleAfterMinutes         int32                            `json:"stale_after_minutes"`
	ReviewPolicy              json.RawMessage                  `json:"review_policy"`
	QualityPolicy             json.RawMessage                  `json:"quality_policy"`
	OverseerConfig            json.RawMessage                  `json:"overseer_config"`
	CreatedAt                 string                           `json:"created_at"`
	UpdatedAt                 string                           `json:"updated_at"`
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
	response := ProjectControlResponse{
		ProjectID:           uuidToString(row.ProjectID),
		OverseerAgentID:     uuidToPtr(row.OverseerAgentID),
		OverseerAutopilotID: uuidToPtr(row.OverseerAutopilotID),
		DefaultPipelineID:   uuidToPtr(row.DefaultPipelineID),
		AutomationMode:      row.AutomationMode,
		AutoTriageEnabled:   row.AutoTriageEnabled,
		AutoRouteEnabled:    row.AutoRouteEnabled,
		AutoEscalateBlocked: row.AutoEscalateBlocked,
		StaleAfterMinutes:   row.StaleAfterMinutes,
		ReviewPolicy:        json.RawMessage(row.ReviewPolicy),
		QualityPolicy:       json.RawMessage(row.QualityPolicy),
		OverseerConfig:      json.RawMessage(row.OverseerConfig),
		CreatedAt:           timestampToString(row.CreatedAt),
		UpdatedAt:           timestampToString(row.UpdatedAt),
	}
	return response
}

func projectControlResponseFromModel(row db.ProjectControlSetting) ProjectControlResponse {
	response := ProjectControlResponse{
		ProjectID:           uuidToString(row.ProjectID),
		OverseerAgentID:     uuidToPtr(row.OverseerAgentID),
		OverseerAutopilotID: uuidToPtr(row.OverseerAutopilotID),
		DefaultPipelineID:   uuidToPtr(row.DefaultPipelineID),
		AutomationMode:      row.AutomationMode,
		AutoTriageEnabled:   row.AutoTriageEnabled,
		AutoRouteEnabled:    row.AutoRouteEnabled,
		AutoEscalateBlocked: row.AutoEscalateBlocked,
		StaleAfterMinutes:   row.StaleAfterMinutes,
		ReviewPolicy:        json.RawMessage(row.ReviewPolicy),
		QualityPolicy:       json.RawMessage(row.QualityPolicy),
		OverseerConfig:      json.RawMessage(row.OverseerConfig),
		CreatedAt:           timestampToString(row.CreatedAt),
		UpdatedAt:           timestampToString(row.UpdatedAt),
	}
	return response
}

func applyProjectOverseerAutonomySyncResult(response *ProjectControlResponse, syncResult *service.ProjectOverseerAutonomySyncResult) {
	if response == nil || syncResult == nil {
		return
	}

	autopilotID := uuidToString(syncResult.AutopilotID)
	if autopilotID != "" {
		response.OverseerAutopilotID = &autopilotID
	}
	status := string(syncResult.Status)
	if status != "" {
		response.OverseerAutonomyStatus = &status
	}
	response.OverseerAutonomyTriggerID = uuidPtrToStringPtr(syncResult.TriggerID)
	response.OverseerAutonomyNextRunAt = timestamptzPtrToStringPtr(syncResult.NextRunAt)

	if response.OverseerAutopilotID == nil && response.OverseerAutonomyStatus == nil && response.OverseerAutonomyTriggerID == nil && response.OverseerAutonomyNextRunAt == nil {
		response.OverseerAutonomy = nil
		return
	}

	autonomy := &ProjectOverseerAutonomyResponse{}
	if response.OverseerAutopilotID != nil {
		autonomy.AutopilotID = *response.OverseerAutopilotID
	}
	if response.OverseerAutonomyStatus != nil {
		autonomy.Status = *response.OverseerAutonomyStatus
	}
	autonomy.TriggerID = response.OverseerAutonomyTriggerID
	autonomy.NextRunAt = response.OverseerAutonomyNextRunAt
	response.OverseerAutonomy = autonomy
}

func uuidPtrToStringPtr(value *pgtype.UUID) *string {
	if value == nil {
		return nil
	}
	return uuidToPtr(*value)
}

func timestamptzPtrToStringPtr(value *pgtype.Timestamptz) *string {
	if value == nil {
		return nil
	}
	return timestampToPtr(*value)
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

func validateOverseerConfig(raw json.RawMessage) (OverseerConfig, error) {
	if !isJSONObject(raw) {
		return OverseerConfig{}, fmt.Errorf("overseer_config must be a JSON object")
	}

	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawFields); err != nil {
		return OverseerConfig{}, fmt.Errorf("overseer_config must be a JSON object")
	}

	for field, rawValue := range rawFields {
		switch field {
		case "scan_interval_hours":
			var value int32
			if err := json.Unmarshal(rawValue, &value); err != nil || value < 6 || value > 24 {
				return OverseerConfig{}, fmt.Errorf("overseer_config.scan_interval_hours must be an integer between 6 and 24")
			}
		case "scan_focus":
			var values []OverseerScanFocus
			if err := json.Unmarshal(rawValue, &values); err != nil {
				return OverseerConfig{}, fmt.Errorf("overseer_config.scan_focus must be an array of allowed values")
			}
			for _, value := range values {
				if _, ok := allowedOverseerScanFocus[value]; !ok {
					return OverseerConfig{}, fmt.Errorf("overseer_config.scan_focus contains unsupported value %q", value)
				}
			}
		case "product_context":
			var value string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				return OverseerConfig{}, fmt.Errorf("overseer_config.product_context must be a string")
			}
			if utf8.RuneCountInString(value) > maxOverseerProductContextLength {
				return OverseerConfig{}, fmt.Errorf("overseer_config.product_context must be at most %d characters", maxOverseerProductContextLength)
			}
		case "quality_bar":
			var values []string
			if err := json.Unmarshal(rawValue, &values); err != nil {
				return OverseerConfig{}, fmt.Errorf("overseer_config.quality_bar must be an array of strings")
			}
			for _, value := range values {
				trimmed := strings.TrimSpace(value)
				if trimmed == "" || utf8.RuneCountInString(trimmed) > maxOverseerListValueLength {
					return OverseerConfig{}, fmt.Errorf("overseer_config.quality_bar must contain non-empty strings up to %d characters", maxOverseerListValueLength)
				}
			}
		case "priority_weights":
			var values OverseerPriorityWeights
			if err := json.Unmarshal(rawValue, &values); err != nil {
				return OverseerConfig{}, fmt.Errorf("overseer_config.priority_weights must be an object with numeric values")
			}
			for key := range values {
				trimmed := strings.TrimSpace(key)
				if trimmed == "" || utf8.RuneCountInString(trimmed) > maxOverseerListValueLength {
					return OverseerConfig{}, fmt.Errorf("overseer_config.priority_weights must use non-empty keys up to %d characters", maxOverseerListValueLength)
				}
			}
		case "max_issues_per_run":
			var value int32
			if err := json.Unmarshal(rawValue, &value); err != nil || value < 1 || value > 3 {
				return OverseerConfig{}, fmt.Errorf("overseer_config.max_issues_per_run must be an integer between 1 and 3")
			}
		case "require_approval":
			var value bool
			if err := json.Unmarshal(rawValue, &value); err != nil {
				return OverseerConfig{}, fmt.Errorf("overseer_config.require_approval must be a boolean")
			}
		}
	}

	var config OverseerConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return OverseerConfig{}, fmt.Errorf("overseer_config must have a valid internal shape")
	}

	return config, nil
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

	response := projectControlResponseFromRow(settings)
	response = h.hydrateProjectControlAutonomyResponse(r.Context(), parseUUID(id), response)
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) hydrateProjectControlAutonomyResponse(ctx context.Context, projectID pgtype.UUID, response ProjectControlResponse) ProjectControlResponse {
	if response.OverseerAutopilotID == nil {
		return response
	}

	autopilot, err := h.Queries.GetProjectLinkedOverseerAutopilot(ctx, projectID)
	if err != nil {
		return response
	}

	status := autopilot.Status
	response.OverseerAutonomyStatus = &status

	triggers, err := h.Queries.ListAutopilotTriggers(ctx, autopilot.ID)
	if err != nil {
		return response
	}

	for _, trigger := range triggers {
		if trigger.Kind != "schedule" {
			continue
		}
		response.OverseerAutonomyTriggerID = uuidToPtr(trigger.ID)
		response.OverseerAutonomyNextRunAt = timestampToPtr(trigger.NextRunAt)
		break
	}

	autonomy := &ProjectOverseerAutonomyResponse{
		AutopilotID: *response.OverseerAutopilotID,
		Status:      status,
		TriggerID:   response.OverseerAutonomyTriggerID,
		NextRunAt:   response.OverseerAutonomyNextRunAt,
	}
	response.OverseerAutonomy = autonomy
	return response
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
		OverseerConfig:      current.OverseerConfig,
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

	if rawConfig, ok := rawFields["overseer_config"]; ok {
		if _, err := validateOverseerConfig(rawConfig); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.OverseerConfig = rawConfig
	}

	settings, err := h.Queries.UpsertProjectControlSettings(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to upsert project control settings")
		return
	}

	response := projectControlResponseFromModel(settings)
	if h.ProjectOverseerAutonomyService != nil {
		syncResult, err := h.ProjectOverseerAutonomyService.SyncProject(r.Context(), project.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to sync project overseer autonomy")
			return
		}
		applyProjectOverseerAutonomySyncResult(&response, syncResult)
	}

	writeJSON(w, http.StatusOK, response)
}
