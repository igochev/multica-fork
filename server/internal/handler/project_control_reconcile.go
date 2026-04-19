package handler

import (
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func (h *Handler) ReconcileProjectControl(w http.ResponseWriter, r *http.Request) {
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

	issues, err := h.Queries.ListOpenIssues(r.Context(), db.ListOpenIssuesParams{
		WorkspaceID: project.WorkspaceID,
		ProjectID:   project.ID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list issues")
		return
	}

	escalatedIssueIDs := make([]string, 0)
	for _, row := range issues {
		issue := db.Issue{
			ID:          row.ID,
			WorkspaceID: row.WorkspaceID,
			Title:       row.Title,
			Status:      row.Status,
			Priority:    row.Priority,
			ProjectID:   row.ProjectID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			Number:      row.Number,
		}

		result, err := h.OverseerService.MaybeEscalateStaleIssue(r.Context(), issue)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to reconcile project control")
			return
		}
		if result != nil && result.Requested {
			escalatedIssueIDs = append(escalatedIssueIDs, uuidToString(issue.ID))
		}
	}

	sort.Strings(escalatedIssueIDs)
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id":          uuidToString(project.ID),
		"escalated_issue_ids": escalatedIssueIDs,
		"escalated_count":     len(escalatedIssueIDs),
	})
}
