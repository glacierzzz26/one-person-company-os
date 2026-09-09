package server

import (
	"context"
	"net/http"
	"strconv"

	"github.com/glacierzzz26/one-person-company-os/internal/github"
	"github.com/glacierzzz26/one-person-company-os/internal/project"
	"github.com/glacierzzz26/one-person-company-os/internal/service"
)

// Phase 10.1 — 项目 + 声明式流水线 /api/v1 handler(契约 docs/phase10/design/project-pipeline-foundation.md §3.4)。
// 全部是 service 方法 thin wrapper;写操作 actor = consoleActor(human:console)。

// projectView 是项目视图(加性富化 D7 code_source):原字段逐字保留,外加 code_source(可空)。
type projectView struct {
	project.Project
	CodeSource *codeSourceView `json:"code_source"`
}

type codeSourceView struct {
	RepoID    string `json:"repo_id"`
	RepoURL   string `json:"repo_url"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Bound     bool   `json:"bound"`      // 绑到项目(非 legacy)
	HasGithub bool   `json:"has_github"` // remote 可解析 GitHub owner/repo(通道 B 可路由)
}

// projectViewOf 把一条项目富化成 projectView(读一次代码源;无绑定 → code_source null)。
func (s *Server) projectViewOf(ctx context.Context, p project.Project) (projectView, error) {
	view := projectView{Project: p}
	src, err := s.svc.CodeSourceFor(ctx, p.ID)
	if err != nil {
		return view, err
	}
	if src == nil {
		return view, nil
	}
	cv := &codeSourceView{RepoID: src.ID, RepoURL: src.RepoURL, Bound: src.ProjectID != nil}
	if o, n, ok := github.ParseOwnerRepo(src.RepoURL); ok {
		cv.Owner, cv.Repo, cv.HasGithub = o, n, true
	}
	view.CodeSource = cv
	return view, nil
}

// ---- 项目 ----

func (s *Server) apiListProjects(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.ListProjects(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	views := make([]projectView, 0, len(list))
	for _, p := range list {
		v, verr := s.projectViewOf(r.Context(), p)
		if verr != nil {
			handleServiceErr(w, verr)
			return
		}
		views = append(views, v)
	}
	apiOK(w, views)
}

func (s *Server) apiCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		RootPath    string `json:"root_path"`
		Description string `json:"description"`
		RepoURL     string `json:"repo_url"` // 可空:空目录/不存在 root 时 OS 自动 clone 该 GitHub 地址
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if pathParam(r, "id") == "" || req.Name == "" || req.RootPath == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "company_id, name and root_path are required")
		return
	}
	p, err := s.svc.CreateProjectAs(r.Context(), pathParam(r, "id"), req.Name, req.RootPath, req.Description, req.RepoURL, consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	v, verr := s.projectViewOf(r.Context(), p)
	if verr != nil {
		handleServiceErr(w, verr)
		return
	}
	apiCreated(w, v)
}

func (s *Server) apiGetProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.svc.GetProject(r.Context(), pathParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	v, verr := s.projectViewOf(r.Context(), p)
	if verr != nil {
		handleServiceErr(w, verr)
		return
	}
	apiOK(w, v)
}

// apiRefreshProjectCodeSource 重新认领/刷新项目代码源(建项目后补 remote 或换 remote):
// POST /projects/{id}/code-source/refresh → 无 GitHub remote → 400(ErrInvalid 带指引)。
func (s *Server) apiRefreshProjectCodeSource(w http.ResponseWriter, r *http.Request) {
	repo, err := s.svc.RefreshProjectCodeSourceAs(r.Context(), pathParam(r, "id"), consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	cv := &codeSourceView{RepoID: repo.ID, RepoURL: repo.RepoURL, Bound: repo.ProjectID != nil}
	if o, n, ok := github.ParseOwnerRepo(repo.RepoURL); ok {
		cv.Owner, cv.Repo, cv.HasGithub = o, n, true
	}
	apiOK(w, cv)
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
		Schedule    string `json:"schedule"`    // 10.2:cron 五段;空/off = 不调度
		PlanPolicy  string `json:"plan_policy"` // 10.4:adaptive(缺省)| synthesize;空 → adaptive
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if pathParam(r, "id") == "" || req.Name == "" {
		apiErr(w, http.StatusBadRequest, "bad_request", "project_id and name are required")
		return
	}
	p, err := s.svc.CreatePipelineAs(r.Context(), pathParam(r, "id"), req.Name, req.Kind, req.Description, req.Risk, req.Schedule, consoleActor,
		service.PipelinePlanPolicy(req.PlanPolicy))
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
