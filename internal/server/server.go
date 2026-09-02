package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
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
// 后台任务:
//   - 轮询 = GitHub 轮询兜底(设计 ≥5 分钟,默认 5;smoke 用 --poll 1)。
//   - 每日摘要(6.5,设计 §8「定时 | 每日摘要 9:00」):DigestLoop 到点发昨日摘要(单协程 tick)。
//
// 通知(飞书)由 svc 内 notifier 承担(事件点即时 / 摘要定时)。webhook 与轮询都写同一
// issue_sync 账本(UNIQUE) → 天然去重。
type Server struct {
	svc  *service.Service
	poll time.Duration

	digestEnabled    bool
	digestH, digestM int // 每日摘要时刻(HH:MM,本地时区)

	apiToken string // /api/v1 bearer 校验(OS_API_TOKEN);空 = 开放
}

// SetAPIToken 配置 /api/v1 访问令牌(空 = 不校验)。用于注入 OS_API_TOKEN(CLI)或测试直构。
func (s *Server) SetAPIToken(tok string) { s.apiToken = tok }

// APITokenSet /api/v1 认证是否开启(日志/启动提示用,不泄漏 token)。
func (s *Server) APITokenSet() bool { return s.apiToken != "" }

func New(svc *service.Service, pollMinutes int) *Server {
	if pollMinutes <= 0 {
		pollMinutes = 5
	}
	return &Server{svc: svc, poll: time.Duration(pollMinutes) * time.Minute}
}

// SetDigestTime 配置每日摘要时刻("HH:MM");传 "off"(或空)关闭定时摘要。
func (s *Server) SetDigestTime(hhmm string) error {
	hhmm = strings.TrimSpace(hhmm)
	if hhmm == "" || strings.EqualFold(hhmm, "off") {
		s.digestEnabled = false
		return nil
	}
	parts := strings.Split(hhmm, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid digest time %q (want HH:MM)", hhmm)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("invalid digest hour %q: %w", parts[0], err)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid digest minute %q: %w", parts[1], err)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return fmt.Errorf("digest time %q out of range (HH 0-23, MM 0-59)", hhmm)
	}
	s.digestEnabled = true
	s.digestH, s.digestM = h, m
	return nil
}

// DigestEnabled 摘要定时是否开启。
func (s *Server) DigestEnabled() bool { return s.digestEnabled }

// DigestTime 摘要时刻文本("HH:MM" 或 "off")。
func (s *Server) DigestTime() string {
	if !s.digestEnabled {
		return "off"
	}
	return fmt.Sprintf("%02d:%02d", s.digestH, s.digestM)
}

// DigestLoop 每日摘要调度(单协程 tick,设计 §8「首版单协程 tick」):到本地 digestH:digestM
// 触发一次 svc.SendDailyDigest,然后重排到次日;失败只记日志不退出。
func (s *Server) DigestLoop(ctx context.Context) {
	if !s.digestEnabled {
		log.Printf("daily digest disabled (digest=%s)", s.DigestTime())
		return
	}
	log.Printf("daily digest scheduled at %02d:%02d (local)", s.digestH, s.digestM)
	for {
		next := nextDigestFire(time.Now(), s.digestH, s.digestM)
		select {
		case <-ctx.Done():
			log.Printf("server digest loop stopped")
			return
		case <-time.After(time.Until(next)):
		}
		if ctx.Err() != nil {
			return
		}
		if err := s.svc.SendDailyDigest(ctx); err != nil {
			log.Printf("daily digest: %v", err)
		}
	}
}

// RunDigestNow 立即发送一次昨日摘要(冒烟/运维触发;等价 --digest-now)。仅当 server 配了摘要开关。
func (s *Server) RunDigestNow(ctx context.Context) error {
	if err := s.svc.SendDailyDigest(ctx); err != nil {
		return err
	}
	return nil
}

// nextDigestFire 计算下一次摘要触发时刻:今日 HH:MM 未过(含 2 分钟宽限)→ 今日,否则次日。
// 纯函数,跨午夜由 Add(24h) 自然处理,供单测断言。
func nextDigestFire(now time.Time, h, m int) time.Time {
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, loc)
	if now.After(today.Add(2 * time.Minute)) {
		return today.Add(24 * time.Hour)
	}
	return today
}

// Handler 组装 chi 路由。
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Post("/api/webhook/github", s.handleGitHubWebhook)
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.apiAuth)
		s.registerAPIRoutes(r)
	})
	return r
}

// apiAuth /api/v1 bearer 校验(Phase 7.1):配了 apiToken 时要求
// `Authorization: Bearer <token>`(constant-time);未配 → 开放(单算子本地姿态,暴露到公网务必配)。
func (s *Server) apiAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.apiToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		got := ""
		if h := r.Header.Get("Authorization"); len(h) > 7 && h[:7] == "Bearer " {
			got = h[7:]
		}
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.apiToken)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": map[string]string{"code": "unauthorized", "message": "invalid or missing bearer token"}})
			return
		}
		next.ServeHTTP(w, r)
	})
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
