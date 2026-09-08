package server

import (
	"net/http"
	"strconv"
)

// Phase 10.1 — 项目 + 声明式流水线 /api/v1 handler(契约 docs/phase10/design/project-pipeline-foundation.md §3.4)。
// 全部是 service 方法 thin wrapper;写操作 actor = consoleActor(human:console)。

// ---- 项目 ----

func (s *Server) apiListProjects(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListProjects(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

func (s *Server) apiCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		RootPath    string `json:"root_path"`
		Description string `json:"description"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if pathParam(r, "id") == "" || req.Name == "" || req.RootPath == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "company_id, name and root_path are required")
		return
	}
	p, err := s.svc.CreateProjectAs(r.Context(), pathParam(r, "id"), req.Name, req.RootPath, req.Description, consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiCreated(w, p)
}

func (s *Server) apiGetProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.svc.GetProject(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, p)
}

func (s *Server) apiDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	if err := s.svc.DeleteProjectAs(r.Context(), id, consoleActor); err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, map[string]any{"id": id, "deleted": true})
}

// ---- 流水线 ----

func (s *Server) apiListPipelines(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListPipelines(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

func (s *Server) apiCreatePipeline(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Kind        string `json:"kind"`
		Description string `json:"description"`
		Risk        string `json:"risk"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if pathParam(r, "id") == "" || req.Name == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "project_id and name are required")
		return
	}
	p, err := s.svc.CreatePipelineAs(r.Context(), pathParam(r, "id"), req.Name, req.Kind, req.Description, req.Risk, consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiCreated(w, p)
}

func (s *Server) apiGetPipeline(w http.ResponseWriter, r *http.Request) {
	p, err := s.svc.GetPipeline(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, p)
}

func (s *Server) apiDeletePipeline(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	if err := s.svc.DeletePipelineAs(r.Context(), id, consoleActor); err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, map[string]any{"id": id, "deleted": true})
}

// apiListProjectTasks 项目「最近 runs」:GET /projects/{id}/tasks?limit=N(默认 20,上限 100)。
func (s *Server) apiListProjectTasks(w http.ResponseWriter, r *http.Request) {
	limit := int64(20)
	if q := queryParam(r, "limit"); q != "" {
		n, err := strconv.ParseInt(q, 10, 64)
		if err != nil || n < 0 {
			apiErr(w, http.StatusBadRequest, "bad_request", "limit must be a non-negative integer")
			return
		}
		if n > 0 {
			limit = n
		}
		if limit > 100 {
			limit = 100
		}
	}
	list, err := s.svc.ListTasksByProject(r.Context(), pathParam(r, "id"), limit)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, list)
}

func (s *Server) apiRunPipeline(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Request string `json:"request"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	pipelineID := pathParam(r, "id")
	t, err := s.svc.RunPipelineAs(r.Context(), pipelineID, req.Request, consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	// 契约:POST /pipelines/{id}/run → {task_id, project_id, pipeline_id}。附完整 task 供详情即时展示。
	apiOK(w, map[string]any{
		"task_id":     t.ID,
		"project_id":  t.ProjectID,
		"pipeline_id": pipelineID,
		"task":        t,
	})
}
