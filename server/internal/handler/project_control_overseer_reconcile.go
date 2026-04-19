package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type ProjectControlOverseerReconcileResponse struct {
	ProjectID          string                           `json:"project_id"`
	Autonomy           *ProjectOverseerAutonomyResponse `json:"autonomy"`
	CreatedAutopilot   bool                             `json:"created_autopilot"`
	UpdatedAutopilot   bool                             `json:"updated_autopilot"`
	CreatedTrigger     bool                             `json:"created_trigger"`
	UpdatedTrigger     bool                             `json:"updated_trigger"`
	UpdatedProjectLink bool                             `json:"updated_project_link"`
}

func (h *Handler) ReconcileProjectControlOverseer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	workspaceID := h.resolveWorkspaceID(r)
	projectID := parseUUID(id)

	if _, err := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{
		ID:          projectID,
		WorkspaceID: parseUUID(workspaceID),
	}); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	settings, err := h.Queries.GetProjectControlSettings(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get project control settings")
		return
	}
	if _, err := validateOverseerConfig(settings.OverseerConfig); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if h.ProjectOverseerAutonomyService == nil {
		writeError(w, http.StatusInternalServerError, "project overseer autonomy service unavailable")
		return
	}

	syncResult, err := h.ProjectOverseerAutonomyService.SyncProject(r.Context(), projectID)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "project control settings not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to reconcile project overseer autonomy")
		return
	}

	response := ProjectControlOverseerReconcileResponse{
		ProjectID:          id,
		CreatedAutopilot:   syncResult.CreatedAutopilot,
		UpdatedAutopilot:   syncResult.UpdatedAutopilot,
		CreatedTrigger:     syncResult.CreatedTrigger,
		UpdatedTrigger:     syncResult.UpdatedTrigger,
		UpdatedProjectLink: syncResult.UpdatedProjectLink,
	}
	if syncResult != nil {
		response.Autonomy = &ProjectOverseerAutonomyResponse{
			AutopilotID: uuidToString(syncResult.AutopilotID),
			Status:      string(syncResult.Status),
			TriggerID:   uuidPtrToStringPtr(syncResult.TriggerID),
			NextRunAt:   timestamptzPtrToStringPtr(syncResult.NextRunAt),
		}
	}

	writeJSON(w, http.StatusOK, response)
}
