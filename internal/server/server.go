package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/github"
	osrepo "github.com/glacierzzz26/one-person-company-os/internal/repo"
	"github.com/glacierzzz26/one-person-company-os/internal/service"
	"github.com/go-chi/chi/v5"
)

// Server 是 OS 首次长驻 HTTP 服务(Phase 6.3,设计 §8)。研发 Intake 通道 B 入口:
//
//	GET  /healthz                健康检查
//	POST /api/webhook/github     GitHub issues webhook(实时优先)→ IntakeIssues 单条
//
// 后台轮询 = GitHub 轮询兜底(设计 ≥5 分钟,默认 5;smoke 用 --poll 1)。
// 通知(飞书)属 6.5。webhook 与轮询都写同一 issue_sync 账本(UNIQUE) → 天然去重。
type Server struct {
	svc  *service.Service
	poll time.Duration
}

func New(svc *service.Service, pollMinutes int) *Server {
	if pollMinutes <= 0 {
		pollMinutes = 5
	}
	return &Server{svc: svc, poll: time.Duration(pollMinutes) * time.Minute}
}

// Handler 组装 chi 路由。
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Post("/api/webhook/github", s.handleGitHubWebhook)
	return r
}

// PollLoop 后台轮询:启动即兜底同步一次,随后每 poll 间隔 SyncRepos 一次,直到 ctx 结束。
// 单次失败只记日志不退出(server 常驻不因网络抖动自尽)。
func (s *Server) PollLoop(ctx context.Context) {
	if s.poll <= 0 {
		return
	}
	s.syncOnce(ctx)
	t := time.NewTicker(s.poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("server poll loop stopped")
			return
		case <-t.C:
			s.syncOnce(ctx)
		}
	}
}

func (s *Server) syncOnce(ctx context.Context) {
	results, err := s.svc.SyncRepos(ctx, "")
	if err != nil {
		log.Printf("poll: %v", err)
		return
	}
	for _, r := range results {
		log.Printf("poll repo %s: %d issue(s) seen, %d task(s) created", r.Repo, r.IssuesSeen, len(r.CreatedTasks))
	}
}

// handleGitHubWebhook 处理 issues webhook:opened/reopened/edited 触发单条 intake,
// closed 与 PR 忽略。issue 归属仓库查不到 → 404(不重试语义交给 caller)。
func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	wh, err := github.DecodeWebhook(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch wh.Action {
	case "opened", "reopened", "edited":
	default:
		writeJSON(w, http.StatusOK, map[string]any{"received": true, "ignored": wh.Action})
		return
	}
	it, ok := wh.ToIssue()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"received": true, "ignored": "pull_request"})
		return
	}
	repo, err := s.findRepoByOwner(r.Context(), it.RepoOwner, it.RepoName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	if _, err := s.svc.IntakeIssues(r.Context(), repo, []github.Issue{it}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"received": true, "repo": repo.Name, "issue": it.Number, "title": it.Title,
	})
}

func (s *Server) findRepoByOwner(ctx context.Context, owner, name string) (osrepo.Repo, error) {
	repos, err := s.svc.ListAllRepos(ctx)
	if err != nil {
		return osrepo.Repo{}, err
	}
	for _, r := range repos {
		o, n, ok := github.ParseOwnerRepo(r.RepoURL)
		if ok && o == owner && n == name {
			return r, nil
		}
	}
	return osrepo.Repo{}, fmt.Errorf("no registered repo for %s/%s", owner, name)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
