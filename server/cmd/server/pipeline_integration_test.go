package main

import (
	"net/http"
	"testing"
)

func TestPipelineRoutes(t *testing.T) {
	resp := authRequest(t, "GET", "/api/pipelines?workspace_id="+testWorkspaceID, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
