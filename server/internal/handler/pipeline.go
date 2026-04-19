package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type PipelineResponse struct {
	ID          string                  `json:"id"`
	WorkspaceID string                  `json:"workspace_id"`
	Name        string                  `json:"name"`
	Description *string                 `json:"description"`
	Stages      []PipelineStageResponse `json:"stages"`
}

type PipelineStageResponse struct {
	ID                string  `json:"id"`
	PipelineID        string  `json:"pipeline_id"`
	Name              string  `json:"name"`
	Status            string  `json:"status"`
	AgentID           string  `json:"agent_id"`
	StageInstructions *string `json:"stage_instructions,omitempty"`
	Position          int     `json:"position"`
}

type PipelineStageRequest struct {
	Name              string  `json:"name"`
	Status            string  `json:"status"`
	AgentID           string  `json:"agent_id"`
	StageInstructions *string `json:"stage_instructions,omitempty"`
	Position          int     `json:"position"`
}

type CreatePipelineRequest struct {
	Name        string                 `json:"name"`
	Description *string                `json:"description"`
	Stages      []PipelineStageRequest `json:"stages"`
}

type UpdatePipelineRequest struct {
	Name        *string                `json:"name"`
	Description *string                `json:"description"`
	Stages      []PipelineStageRequest `json:"stages"`
}

func pipelineToResponse(p db.Pipeline, stages []db.PipelineStage) PipelineResponse {
	resp := PipelineResponse{
		ID:          uuidToString(p.ID),
		WorkspaceID: uuidToString(p.WorkspaceID),
		Name:        p.Name,
		Description: textToPtr(p.Description),
		Stages:      make([]PipelineStageResponse, len(stages)),
	}
	for i, stage := range stages {
		resp.Stages[i] = pipelineStageToResponse(stage)
	}
	return resp
}

func pipelineStageToResponse(stage db.PipelineStage) PipelineStageResponse {
	return PipelineStageResponse{
		ID:                uuidToString(stage.ID),
		PipelineID:        uuidToString(stage.PipelineID),
		Name:              stage.Name,
		Status:            stage.Status,
		AgentID:           uuidToString(stage.AgentID),
		StageInstructions: textToPtr(stage.StageInstructions),
		Position:          int(stage.Position),
	}
}

func normalizePipelineStages(stages []PipelineStageRequest) []PipelineStageRequest {
	normalized := append([]PipelineStageRequest(nil), stages...)
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Position < normalized[j].Position
	})
	return normalized
}

func (h *Handler) validatePipelineStages(r *http.Request, workspaceID string, stages []PipelineStageRequest) error {
	if len(stages) == 0 {
		return fmt.Errorf("at least one stage is required")
	}

	positions := make(map[int]struct{}, len(stages))
	for _, stage := range stages {
		if stage.Name == "" {
			return fmt.Errorf("stage name is required")
		}
		if stage.Status == "" {
			return fmt.Errorf("stage status is required")
		}
		if stage.AgentID == "" {
			return fmt.Errorf("stage agent_id is required")
		}
		if stage.Position <= 0 {
			return fmt.Errorf("stage position must be greater than 0")
		}
		if _, exists := positions[stage.Position]; exists {
			return fmt.Errorf("duplicate stage position: %d", stage.Position)
		}
		positions[stage.Position] = struct{}{}

		if _, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
			ID:          parseUUID(stage.AgentID),
			WorkspaceID: parseUUID(workspaceID),
		}); err != nil {
			return fmt.Errorf("stage agent must be a valid agent in this workspace")
		}
	}

	return nil
}

func (h *Handler) ListPipelines(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	pipelines, err := h.Queries.ListPipelines(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list pipelines")
		return
	}

	resp := make([]PipelineResponse, len(pipelines))
	for i, pipeline := range pipelines {
		stages, err := h.Queries.ListPipelineStages(r.Context(), pipeline.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list pipeline stages")
			return
		}
		resp[i] = pipelineToResponse(pipeline, stages)
	}

	writeJSON(w, http.StatusOK, map[string]any{"pipelines": resp, "total": len(resp)})
}

func (h *Handler) GetPipeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	workspaceID := h.resolveWorkspaceID(r)

	pipeline, err := h.Queries.GetPipelineInWorkspace(r.Context(), db.GetPipelineInWorkspaceParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	stages, err := h.Queries.ListPipelineStages(r.Context(), pipeline.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list pipeline stages")
		return
	}

	writeJSON(w, http.StatusOK, pipelineToResponse(pipeline, stages))
}

func (h *Handler) CreatePipeline(w http.ResponseWriter, r *http.Request) {
	var req CreatePipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	workspaceID := h.resolveWorkspaceID(r)
	stages := normalizePipelineStages(req.Stages)
	if err := h.validatePipelineStages(r, workspaceID, stages); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(r.Context())

	qtx := h.Queries.WithTx(tx)
	pipeline, err := qtx.CreatePipeline(r.Context(), db.CreatePipelineParams{
		WorkspaceID: parseUUID(workspaceID),
		Name:        req.Name,
		Description: ptrToText(req.Description),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create pipeline")
		return
	}

	createdStages := make([]db.PipelineStage, 0, len(stages))
	for _, stage := range stages {
		createdStage, err := qtx.CreatePipelineStage(r.Context(), db.CreatePipelineStageParams{
			PipelineID:        pipeline.ID,
			Name:              stage.Name,
			Status:            stage.Status,
			AgentID:           parseUUID(stage.AgentID),
			StageInstructions: ptrToText(stage.StageInstructions),
			Position:          int32(stage.Position),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create pipeline stage")
			return
		}
		createdStages = append(createdStages, createdStage)
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit")
		return
	}

	writeJSON(w, http.StatusCreated, pipelineToResponse(pipeline, createdStages))
}

func (h *Handler) UpdatePipeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	workspaceID := h.resolveWorkspaceID(r)

	pipeline, err := h.Queries.GetPipelineInWorkspace(r.Context(), db.GetPipelineInWorkspaceParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var req UpdatePipelineRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var rawFields map[string]json.RawMessage
	_ = json.Unmarshal(bodyBytes, &rawFields)

	params := db.UpdatePipelineParams{
		ID:          pipeline.ID,
		Description: pipeline.Description,
	}
	if _, ok := rawFields["name"]; ok {
		if req.Name == nil || *req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		params.Name = strToText(*req.Name)
	}
	if _, ok := rawFields["description"]; ok {
		params.Description = ptrToText(req.Description)
	}

	existingStages, err := h.Queries.ListPipelineStages(r.Context(), pipeline.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list pipeline stages")
		return
	}

	stagesToCreate := make([]db.CreatePipelineStageParams, 0, len(existingStages))
	for _, stage := range existingStages {
		stagesToCreate = append(stagesToCreate, db.CreatePipelineStageParams{
			PipelineID:        pipeline.ID,
			Name:              stage.Name,
			Status:            stage.Status,
			AgentID:           stage.AgentID,
			StageInstructions: stage.StageInstructions,
			Position:          stage.Position,
		})
	}
	if _, ok := rawFields["stages"]; ok {
		normalizedStages := normalizePipelineStages(req.Stages)
		if err := h.validatePipelineStages(r, workspaceID, normalizedStages); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		stagesToCreate = make([]db.CreatePipelineStageParams, 0, len(normalizedStages))
		for _, stage := range normalizedStages {
			stagesToCreate = append(stagesToCreate, db.CreatePipelineStageParams{
				PipelineID:        pipeline.ID,
				Name:              stage.Name,
				Status:            stage.Status,
				AgentID:           parseUUID(stage.AgentID),
				StageInstructions: ptrToText(stage.StageInstructions),
				Position:          int32(stage.Position),
			})
		}
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(r.Context())

	qtx := h.Queries.WithTx(tx)
	updatedPipeline, err := qtx.UpdatePipeline(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update pipeline")
		return
	}

	for _, stage := range existingStages {
		if err := qtx.DeletePipelineStage(r.Context(), stage.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to replace pipeline stages")
			return
		}
	}

	updatedStages := make([]db.PipelineStage, 0, len(stagesToCreate))
	for _, stage := range stagesToCreate {
		updatedStage, err := qtx.CreatePipelineStage(r.Context(), stage)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create pipeline stage")
			return
		}
		updatedStages = append(updatedStages, updatedStage)
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit")
		return
	}

	writeJSON(w, http.StatusOK, pipelineToResponse(updatedPipeline, updatedStages))
}

func (h *Handler) DeletePipeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	workspaceID := h.resolveWorkspaceID(r)

	if _, err := h.Queries.GetPipelineInWorkspace(r.Context(), db.GetPipelineInWorkspaceParams{
		ID:          parseUUID(id),
		WorkspaceID: parseUUID(workspaceID),
	}); err != nil {
		writeError(w, http.StatusNotFound, "pipeline not found")
		return
	}

	if err := h.Queries.DeletePipeline(r.Context(), parseUUID(id)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete pipeline")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
