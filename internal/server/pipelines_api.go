package server

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/service"
)

// Phase 10.2 — 流水线调度 + 巡检报告读端点(契约 docs/phase10/design/ops-patrol-schedule.md §3.8)。
// 均为 service 方法 thin wrapper;写操作 actor = consoleActor;信封/鉴权沿用。

// apiUpdatePipelineSchedule PUT /pipelines/{id} — 改流水线调度(Web 作者面 / HTTP 唯一写口)。
// body {"schedule":"0 9 * * *"|""};空/off = 停调度。非法 cron → 400 bad_request;不存在 → 404。
func (s *Server) apiUpdatePipelineSchedule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Schedule string `json:"schedule"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	pl, err := s.svc.UpdatePipelineScheduleAs(r.Context(), pathParam(r, "id"), req.Schedule, consoleActor)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	apiOK(w, pl)
}

// apiGetPatrolReport GET /projects/{projectID}/patrol/{taskID} — 巡检报告正文读端点(定稿门④:正文内嵌、
// 受控读)。服务端按 taskID 推导 patrol/<taskID>.md(join task.WorkspacePath),**无客户端路径入参**
// → 无穿越面(路径仅在 GetTask 命中真实 task 后由其 UUID 构造)。校验:task 归属 project、已完成且
// result 以 "patrol:" 前缀开头(确系巡检产物)。文件限长 PatrolReportCap 截断带 truncated 标记。
// 越权(project 不匹配)→ 404(不泄漏存在性);非巡检/未完成 → 400;文件缺失 → 404。
func (s *Server) apiGetPatrolReport(w http.ResponseWriter, r *http.Request) {
	projectID := pathParam(r, "projectID")
	taskID := pathParam(r, "taskID")
	t, err := s.svc.GetTask(r.Context(), taskID)
	if err != nil {
		handleServiceErr(w, err) // 无此 task → 404
		return
	}
	if t.ProjectID == nil || *t.ProjectID != projectID {
		apiErr(w, http.StatusNotFound, "not_found", "no patrol report: task is not under this project")
		return
	}
	if t.Status != "completed" || !strings.HasPrefix(t.Result, service.PatrolResultTag) {
		apiErr(w, http.StatusBadRequest, "bad_request",
			"task is not a completed patrol run — patrol reports serve finished ops_patrol runs only")
		return
	}
	rel := filepath.Join(service.PatrolDirName, t.ID+".md")
	content, truncated, err := readReportCappedHTTP(filepath.Join(t.WorkspacePath, rel))
	if err != nil {
		apiErr(w, http.StatusNotFound, "not_found", "patrol report file missing: "+filepath.ToSlash(rel))
		return
	}
	apiOK(w, map[string]any{
		"task_id":   t.ID,
		"path":      filepath.ToSlash(rel),
		"truncated": truncated,
		"content":   content,
	})
}

// readReportCappedHTTP 读报告正文(限长 PatrolReportCap 截断带标记;与 service readReportCapped 同源常量)。
func readReportCappedHTTP(abs string) (string, bool, error) {
	f, err := os.Open(abs)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, service.PatrolReportCap+1))
	if err != nil {
		return "", false, err
	}
	truncated := len(b) > service.PatrolReportCap
	if truncated {
		b = b[:service.PatrolReportCap]
	}
	return string(b), truncated, nil
}
