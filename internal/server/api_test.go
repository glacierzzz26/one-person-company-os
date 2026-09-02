package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/approval"
	"github.com/glacierzzz26/one-person-company-os/internal/capability"
	"github.com/glacierzzz26/one-person-company-os/internal/company"
	"github.com/glacierzzz26/one-person-company-os/internal/decision"
	"github.com/glacierzzz26/one-person-company-os/internal/service"
	"github.com/glacierzzz26/one-person-company-os/internal/storage"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// newTestServer 起真 svc(temp sqlite,打开即迁移)+ 同 db 的 Store(test seed 用)。
func newTestServer(t *testing.T) (*Server, *repository.Store) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "os-test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	st := repository.NewStore(db)
	return New(service.New(st), 0), st
}

func nowUnix() int64 { return time.Now().Unix() }

func seedCompany(t *testing.T, st *repository.Store, name, vision string) company.Company {
	t.Helper()
	c, err := st.CreateCompany(context.Background(), company.Company{
		ID: uuid.NewString(), Name: name, Vision: vision, CreatedAt: nowUnix(), UpdatedAt: nowUnix(),
	})
	if err != nil {
		t.Fatalf("seed company: %v", err)
	}
	return c
}

func seedCapability(t *testing.T, st *repository.Store, compID, code string) capability.Capability {
	t.Helper()
	c, err := st.CreateCapability(context.Background(), capability.Capability{
		ID: uuid.NewString(), CompanyID: compID, Code: code, Name: code,
		CreatedAt: nowUnix(), UpdatedAt: nowUnix(),
	})
	if err != nil {
		t.Fatalf("seed capability: %v", err)
	}
	return c
}

func seedTask(t *testing.T, st *repository.Store, compID string, mod func(*task.Task)) task.Task {
	t.Helper()
	now := nowUnix()
	tk := task.Task{
		ID: uuid.NewString(), CompanyID: compID, Title: "seed task",
		ToolName: "shell", Status: "pending", Risk: "low", QStatus: "ready",
		CreatedAt: now, UpdatedAt: now,
	}
	if mod != nil {
		mod(&tk)
	}
	got, err := st.CreateTask(context.Background(), tk)
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	return got
}

// apiEnvelope 对应 /api/v1 响应信封。
type apiEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func doAPI(t *testing.T, h http.Handler, method, path, body string) (*httptest.ResponseRecorder, apiEnvelope) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var env apiEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec, env
}

// decodeData 把信封 data 解到目标结构。
func decodeData(t *testing.T, env apiEnvelope, dst any) {
	t.Helper()
	if err := json.Unmarshal(env.Data, dst); err != nil {
		t.Fatalf("decode data %s: %v", string(env.Data), err)
	}
}

// TestAPIOverviewRD 旗舰读:overview 响应含 RD 研发部块(熔断 fused / 待审批 / 子任务 / 账本字段),字段 snake_case。
func TestAPIOverviewRD(t *testing.T) {
	srv, st := newTestServer(t)
	ctx := context.Background()

	comp := seedCompany(t, st, "ACME", "one-person os")
	eng := seedCapability(t, st, comp.ID, "engineering")

	seedTask(t, st, comp.ID, func(tk *task.Task) {
		tk.Title = "顶层工程请求"
		tk.ToolName = "engineering"
		tk.CapabilityID = &eng.ID
		tk.Status = "completed"
		tk.QStatus = "completed"
	})
	// 拆解子任务(挂父,计入 subtask_count)。
	parent := seedTask(t, st, comp.ID, func(tk *task.Task) {
		tk.Title = "父任务"
		tk.ToolName = "engineering"
		tk.CapabilityID = &eng.ID
		tk.Status = "completed"
		tk.QStatus = "completed"
	})
	seedTask(t, st, comp.ID, func(tk *task.Task) {
		tk.Title = "子任务"
		tk.ToolName = "engineering"
		tk.CapabilityID = &eng.ID
		tk.ParentTaskID = &parent.ID
		tk.Status = "completed"
		tk.QStatus = "completed"
	})
	// 熔断任务:conflict=3 + pending approval(reason=engineering fuse)→ RD.fused。
	fused := seedTask(t, st, comp.ID, func(tk *task.Task) {
		tk.Title = "flaky auth refactor"
		tk.ToolName = "engineering"
		tk.CapabilityID = &eng.ID
		tk.Status = "waiting_approval"
		tk.QStatus = "waiting_approval"
		tk.Risk = "high"
		tk.ConflictCount = 3
		tk.RoundNo = 2
	})
	if _, err := st.CreateApproval(ctx, approval.Approval{
		ID: uuid.NewString(), TaskID: fused.ID, Risk: "high", Reason: "engineering fuse",
		Status: "pending", CreatedAt: nowUnix(),
	}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	rec, env := doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/companies/"+comp.ID+"/overview", "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("overview: code=%d ok=%v err=%+v body=%s", rec.Code, env.OK, env.Error, rec.Body.String())
	}
	var ov service.Overview
	decodeData(t, env, &ov)

	if ov.Company.Name != "ACME" {
		t.Errorf("company.name = %q, want ACME", ov.Company.Name)
	}
	if ov.RD == nil {
		t.Fatal("rd should be non-nil for company with engineering capability")
	}
	if ov.RD.CapabilityID != eng.ID {
		t.Errorf("rd.capability_id = %q, want %q", ov.RD.CapabilityID, eng.ID)
	}
	if ov.RD.Total != 4 {
		t.Errorf("rd.total = %d, want 4 (3 top + 1 subtask)", ov.RD.Total)
	}
	if ov.RD.SubtaskCount != 1 {
		t.Errorf("rd.subtask_count = %d, want 1", ov.RD.SubtaskCount)
	}
	if len(ov.RD.Fused) != 1 || ov.RD.Fused[0].Title != "flaky auth refactor" || ov.RD.Fused[0].Conflict != 3 {
		t.Errorf("rd.fused = %+v, want 1 fused(conflict=3)", ov.RD.Fused)
	}
	// 字段名 snake_case 抽查(契约)。
	raw := string(env.Data)
	for _, key := range []string{`"capability_id"`, `"subtask_count"`, `"by_status"`, `"fused"`, `"waiting"`, `"ledger"`, `"ledger_seen"`, `"parent_task_id"`, `"recent_tasks"`, `"pending_approvals"`} {
		if !bytes.Contains([]byte(raw), []byte(key)) {
			t.Errorf("overview JSON missing %s (snake_case contract)", key)
		}
	}
}

// TestAPIDecideApproval 审批核心写:POST decision approve → 审批 approved + 任务回队 + decision 自动落库。
func TestAPIDecideApproval(t *testing.T) {
	srv, st := newTestServer(t)
	ctx := context.Background()

	comp := seedCompany(t, st, "ACME", "")
	tk := seedTask(t, st, comp.ID, func(tt *task.Task) {
		tt.Title = "billing migration"
		tt.ToolName = "engineering"
		tt.Risk = "high"
		tt.Status = "waiting_approval"
		tt.QStatus = "waiting_approval"
		tt.ConflictCount = 3
	})
	ap, err := st.CreateApproval(ctx, approval.Approval{
		ID: uuid.NewString(), TaskID: tk.ID, Risk: "high", Reason: "engineering fuse",
		Status: "pending", CreatedAt: nowUnix(),
	})
	if err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/approvals/"+ap.ID+"/decision", `{"decision":"approve","note":"OK 放行"}`)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("decision: code=%d ok=%v err=%+v body=%s", rec.Code, env.OK, env.Error, rec.Body.String())
	}
	var got approval.Approval
	decodeData(t, env, &got)
	if got.Status != "approved" {
		t.Errorf("approval.status = %q, want approved", got.Status)
	}

	// 任务回队(重新可认领执行)。
	tt, err := st.GetTask(ctx, tk.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if tt.Status != "pending" || tt.QStatus != "ready" {
		t.Errorf("task after approve = status=%q q_status=%q, want pending/ready", tt.Status, tt.QStatus)
	}
	// 治理链:自动落 decision(kind=approval)。
	ds, err := st.ListDecisions(ctx, comp.ID, "approval")
	if err != nil || len(ds) != 1 {
		t.Fatalf("decisions = %+v err=%v, want 1 approval decision", ds, err)
	}
	if ds[0].Source != "approval:"+ap.ID {
		t.Errorf("decision.source = %q, want approval:<id>", ds[0].Source)
	}

	// 非法 decision → 400。
	rec2, env2 := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/approvals/"+ap.ID+"/decision", `{"decision":"burn"}`)
	if rec2.Code != http.StatusBadRequest || env2.OK {
		t.Errorf("bad decision: code=%d ok=%v, want 400", rec2.Code, env2.OK)
	}
}

// TestAPIAuthToken bearer 中间件:配 token 后无头 401 / 带头 200;healthz 不受影响。
func TestAPIAuthToken(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.SetAPIToken("sekret-token")

	rec, _ := doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/companies", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: code=%d, want 401", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/companies", nil)
	req.Header.Set("Authorization", "Bearer sekret-token")
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Errorf("with token: code=%d, want 200 (body=%s)", rec2.Code, rec2.Body.String())
	}

	// healthz 不在 /api/v1 组,免认证。
	rec3, _ := doAPI(t, srv.Handler(), http.MethodGet, "/healthz", "")
	if rec3.Code != http.StatusOK {
		t.Errorf("healthz: code=%d, want 200", rec3.Code)
	}
}

// TestAPIGetNotFoundAndBadRequest 错误映射:不存在 id → 404 not_found;缺必填 → 400。
func TestAPIGetNotFoundAndBadRequest(t *testing.T) {
	srv, st := newTestServer(t)
	comp := seedCompany(t, st, "ACME", "")

	rec, env := doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/tasks/"+uuid.NewString(), "")
	if rec.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != "not_found" {
		t.Errorf("missing task: code=%d env=%+v, want 404 not_found", rec.Code, env)
	}

	rec2, env2 := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/tasks", `{"company_id":"`+comp.ID+`"}`)
	if rec2.Code != http.StatusBadRequest || env2.Error == nil || env2.Error.Code != "bad_request" {
		t.Errorf("missing title: code=%d env=%+v, want 400 bad_request", rec2.Code, env2)
	}
}

// TestAPICreateAndList 写端点 + 组织读:POST task/decision/memory 建出 → 列表可见。
func TestAPICreateAndList(t *testing.T) {
	srv, st := newTestServer(t)
	comp := seedCompany(t, st, "ACME", "")

	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/tasks",
		`{"company_id":"`+comp.ID+`","title":"写方案","tool_name":"file-write","risk":"medium"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create task: code=%d ok=%v err=%+v", rec.Code, env.OK, env.Error)
	}
	var tk task.Task
	decodeData(t, env, &tk)
	if tk.ID == "" || tk.Status != "pending" || tk.QStatus != "ready" {
		t.Errorf("created task = %+v", tk)
	}

	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/companies/"+comp.ID+"/decisions",
		`{"kind":"strategy","title":"Q3 主打国内","body":"…"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create decision: code=%d env=%+v", rec.Code, env)
	}
	var dc decision.Decision
	decodeData(t, env, &dc)
	if dc.Status != "made" {
		t.Errorf("decision.status = %q, want default made", dc.Status)
	}

	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/companies/"+comp.ID+"/memories",
		`{"type":"lesson","title":"发布教训","content":"审批门前置"}`)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create memory: code=%d env=%+v", rec.Code, env)
	}

	rec, env = doAPI(t, srv.Handler(), http.MethodGet, "/api/v1/tasks?company="+comp.ID+"&status=pending", "")
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("list tasks: code=%d env=%+v", rec.Code, env)
	}
	var tasks []task.Task
	decodeData(t, env, &tasks)
	if len(tasks) != 1 {
		t.Errorf("pending tasks = %d, want 1", len(tasks))
	}
}
