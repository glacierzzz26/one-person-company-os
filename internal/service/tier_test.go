package service

// Phase 8.4 契约用例(tier-enforcement.md §五):
//   - C1 默认解析确定性(active+tier/proto 过滤,created_at ASC 取最早)
//   - C2 建单落档 + create audit detail 列默认命中(role=<id>:<model>)
//   - C3 显式端点覆盖默认(给了不解析不改)
//   - C4 判读槽默认档无 active 网关端点 → 建单硬失败(带指引)
//   - C5 writer 宽容(无 cheap+anthropic → 留空,建单成功)
//   - C6 scripted 红线(scripted 建 engineering 全空成功、不走解析)
//   - B1 engEndpointFor 分槽(test → test → reviewer → writer;review 不走 test 槽)
// 端点只落行不触网:默认解析/建单不发起判读,纯路径验证。

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/storage/repository"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// seedTierEndpoint 落一条 active 端点(不触网;baseURL 假地址即可,本组用例不发判读请求)。
func seedTierEndpoint(t *testing.T, st *repository.Store, compID, name, tier, proto, model string, at int64) endpoint.Endpoint {
	t.Helper()
	ep, err := st.CreateEndpoint(context.Background(), endpoint.Endpoint{
		ID: uuid.NewString(), CompanyID: compID, Name: name,
		BaseURL: "http://127.0.0.1:1", Proto: proto, Vendor: "gateway",
		Tier: tier, SelectedModel: model, Role: "pool", Status: "active",
		CreatedAt: at, UpdatedAt: at,
	})
	if err != nil {
		t.Fatalf("seed endpoint %s(tier=%s proto=%s): %v", name, tier, proto, err)
	}
	return ep
}

func createEng(t *testing.T, svc *Service, compID string) (task.Task, error) {
	t.Helper()
	return svc.CreateTaskAs(context.Background(), TaskParams{
		CompanyID: compID, Title: "impl", Description: "make it work",
		ToolName: "engineering", Risk: "low", MaxAttempts: 1, TimeoutSec: 60,
	}, "test")
}

// createAuditDetail 取某任务 create 审计的 detail(默认落档凭证)。
func createAuditDetail(t *testing.T, st *repository.Store, taskID string) string {
	t.Helper()
	audits, err := st.ListAudits(context.Background(), "task")
	if err != nil {
		t.Fatalf("ListAudits: %v", err)
	}
	for _, a := range audits {
		if a.EntityID == taskID && a.Action == "create" {
			return a.Detail
		}
	}
	return ""
}

// C2 + C1:配齐三档端点建单(全空槽)→ 三槽自动落位;review=frontier 最早、test=standard 最早、writer=cheap+anthropic;
// create audit detail 列 reviewer=/test=/writer=(含所选 model)。
func TestDefaultTierResolutionOnCreate(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "live")
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	base := time.Now().Unix()

	// 同档两个端点(created_at 错开)验证确定性:review/test 都取最早那条。
	frontLate := seedTierEndpoint(t, st, compID, "front-2", "frontier", "openai", "frontier-model-2", base+3)
	frontEarly := seedTierEndpoint(t, st, compID, "front-1", "frontier", "openai", "frontier-model-1", base)
	seedTierEndpoint(t, st, compID, "std-2", "standard", "openai", "standard-model-2", base+2)
	stdEarly := seedTierEndpoint(t, st, compID, "std-1", "standard", "openai", "standard-model-1", base)
	cheap := seedTierEndpoint(t, st, compID, "cheap-1", "cheap", "anthropic", "cheap-model", base+1)

	tk, err := createEng(t, svc, compID)
	if err != nil {
		t.Fatalf("create engineering (live, all-empty) should resolve defaults: %v", err)
	}
	if tk.ReviewerEndpointID == nil || *tk.ReviewerEndpointID != frontEarly.ID {
		t.Fatalf("reviewer slot = %v, want earliest frontier %s (not %s)", strOrNil(tk.ReviewerEndpointID), frontEarly.ID, frontLate.ID)
	}
	if tk.TestEndpointID == nil || *tk.TestEndpointID != stdEarly.ID {
		t.Fatalf("test slot = %v, want earliest standard %s", strOrNil(tk.TestEndpointID), stdEarly.ID)
	}
	if tk.WriterEndpointID == nil || *tk.WriterEndpointID != cheap.ID {
		t.Fatalf("writer slot = %v, want cheap+anthropic %s", strOrNil(tk.WriterEndpointID), cheap.ID)
	}

	got, err := st.GetTask(context.Background(), tk.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ReviewerEndpointID == nil || *got.ReviewerEndpointID != frontEarly.ID || got.TestEndpointID == nil || *got.TestEndpointID != stdEarly.ID {
		t.Fatalf("default slots not persisted: reviewer=%v test=%v", strOrNil(got.ReviewerEndpointID), strOrNil(got.TestEndpointID))
	}

	detail := createAuditDetail(t, st, tk.ID)
	for _, want := range []string{
		"reviewer=" + frontEarly.ID + ":frontier-model-1",
		"test=" + stdEarly.ID + ":standard-model-1",
		"writer=" + cheap.ID + ":cheap-model",
	} {
		if !strings.Contains(detail, want) {
			t.Fatalf("create audit detail missing %q, got: %s", want, detail)
		}
	}
}

// C3:显式给的槽不解析不改(即便档不匹配默认档);只对空槽做默认解析。
func TestExplicitEndpointSkipsDefaultResolution(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "live")
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	base := time.Now().Unix()
	cheap := seedTierEndpoint(t, st, compID, "cheap", "cheap", "anthropic", "cheap-model", base)
	seedTierEndpoint(t, st, compID, "front", "frontier", "openai", "front-model", base+1)
	std := seedTierEndpoint(t, st, compID, "std", "standard", "openai", "std-model", base+2)

	// reviewer 显式指到 cheap 端点(档不匹配也不被覆盖);writer/test 显式给 → 全显式,零解析。
	refW, refR, refT := cheap.ID, cheap.ID, std.ID
	tk, err := svc.CreateTaskAs(context.Background(), TaskParams{
		CompanyID: compID, Title: "impl", Description: "x",
		ToolName: "engineering", Risk: "low", MaxAttempts: 1, TimeoutSec: 60,
		WriterEndpointID: &refW, ReviewerEndpointID: &refR, TestEndpointID: &refT,
	}, "test")
	if err != nil {
		t.Fatalf("create with explicit endpoints: %v", err)
	}
	if tk.WriterEndpointID == nil || *tk.WriterEndpointID != cheap.ID ||
		tk.ReviewerEndpointID == nil || *tk.ReviewerEndpointID != cheap.ID ||
		tk.TestEndpointID == nil || *tk.TestEndpointID != std.ID {
		t.Fatalf("explicit endpoints must be kept verbatim: w=%v r=%v t=%v",
			strOrNil(tk.WriterEndpointID), strOrNil(tk.ReviewerEndpointID), strOrNil(tk.TestEndpointID))
	}
	if d := createAuditDetail(t, st, tk.ID); d != "" {
		t.Fatalf("all-explicit create should have empty detail (nothing default-resolved), got: %q", d)
	}

	// 只给 reviewer(test 空)→ test 走默认解析落 standard;reviewer 原样。
	tk2, err := svc.CreateTaskAs(context.Background(), TaskParams{
		CompanyID: compID, Title: "impl2", Description: "x",
		ToolName: "engineering", Risk: "low", MaxAttempts: 1, TimeoutSec: 60,
		ReviewerEndpointID: &refR,
	}, "test")
	if err != nil {
		t.Fatalf("create with only reviewer explicit: %v", err)
	}
	if tk2.ReviewerEndpointID == nil || *tk2.ReviewerEndpointID != cheap.ID {
		t.Fatalf("explicit reviewer must survive default pass, got %v", strOrNil(tk2.ReviewerEndpointID))
	}
	if tk2.TestEndpointID == nil || *tk2.TestEndpointID != std.ID {
		t.Fatalf("empty test slot should default-resolve to standard, got %v", strOrNil(tk2.TestEndpointID))
	}
}

// C4:判读槽(review/test)默认档无 active 网关端点 → 建单硬失败,消息含档位 + 指引。
func TestJudgeSlotHardFailsWhenTierUnmet(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "live")
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	base := time.Now().Unix()

	// 只有 standard+openai(无 frontier)→ reviewer 槽硬失败。
	seedTierEndpoint(t, st, compID, "std", "standard", "openai", "std-model", base)
	if _, err := createEng(t, svc, compID); err == nil {
		t.Fatalf("reviewer empty + no frontier gateway should hard-fail create")
	} else {
		for _, part := range []string{"tier=frontier", "reviewer", "os endpoint", "--reviewer-endpoint"} {
			if !strings.Contains(err.Error(), part) {
				t.Fatalf("reviewer hard-fail message missing %q: %v", part, err)
			}
		}
	}

	// 新公司:只有 frontier+openai(无 standard)→ test 槽硬失败。
	compID2 := seedCompanyID(t, st)
	seedTierEndpoint(t, st, compID2, "front", "frontier", "openai", "front-model", base)
	if _, err := createEng(t, svc, compID2); err == nil {
		t.Fatalf("test empty + no standard gateway should hard-fail create")
	} else {
		for _, part := range []string{"tier=standard", "test", "--test-endpoint"} {
			if !strings.Contains(err.Error(), part) {
				t.Fatalf("test hard-fail message missing %q: %v", part, err)
			}
		}
	}
}

// C5 + C6:无 cheap+anthropic → writer 留空、建单成功(live);scripted 全空 → 成功且不走解析。
func TestWriterTolerantAndScriptedSkip(t *testing.T) {
	t.Setenv("OS_ENGINE_MODE", "live")
	svc, st := newSvc(t)
	compID := seedCompanyID(t, st)
	base := time.Now().Unix()
	seedTierEndpoint(t, st, compID, "front", "frontier", "openai", "front-model", base)
	seedTierEndpoint(t, st, compID, "std", "standard", "openai", "std-model", base+1)

	tk, err := createEng(t, svc, compID)
	if err != nil {
		t.Fatalf("live create with judge tiers but no cheap+anthropic should succeed: %v", err)
	}
	if tk.WriterEndpointID != nil {
		t.Fatalf("writer must stay empty (no cheap+anthropic → claude 自带合法态), got %v", strOrNil(tk.WriterEndpointID))
	}
	if tk.ReviewerEndpointID == nil || tk.TestEndpointID == nil {
		t.Fatalf("judge slots still default-resolve: r=%v t=%v", strOrNil(tk.ReviewerEndpointID), strOrNil(tk.TestEndpointID))
	}

	// scripted:全空建单成功、端点槽全部空(不解析、不硬失败) —— 离线红线。
	t.Setenv("OS_ENGINE_MODE", "scripted")
	compID2 := seedCompanyID(t, st)
	tk2, err := createEng(t, svc, compID2)
	if err != nil {
		t.Fatalf("scripted create (no endpoints) must succeed untouched: %v", err)
	}
	if tk2.WriterEndpointID != nil || tk2.ReviewerEndpointID != nil || tk2.TestEndpointID != nil {
		t.Fatalf("scripted must skip default resolution entirely: w=%v r=%v t=%v",
			strOrNil(tk2.WriterEndpointID), strOrNil(tk2.ReviewerEndpointID), strOrNil(tk2.TestEndpointID))
	}
}

// B1:engEndpointFor 分槽 —— test 优先 test_endpoint_id → 回退 reviewer → writer;review 走 reviewer → writer(不用 test 槽)。
func TestEngineEndpointForTestSlot(t *testing.T) {
	w, r, tt := "writer-ep", "reviewer-ep", "test-ep"
	ref := func(s string) *string { return &s }

	// test 槽命中 → 用 test(即便 reviewer 也在)。
	tk := task.Task{WriterEndpointID: ref(w), ReviewerEndpointID: ref(r), TestEndpointID: ref(tt)}
	if got := (&Service{}).engEndpointFor(tk, engRoleTest); got != tt {
		t.Fatalf("test role should use test_endpoint_id, got %q", got)
	}
	// test 槽空 → 回退 reviewer(writer 兜底语义保持)。
	tk2 := task.Task{WriterEndpointID: ref(w), ReviewerEndpointID: ref(r), TestEndpointID: nil}
	if got := (&Service{}).engEndpointFor(tk2, engRoleTest); got != r {
		t.Fatalf("test role w/o test slot should fall back to reviewer, got %q", got)
	}
	tk3 := task.Task{WriterEndpointID: ref(w), ReviewerEndpointID: nil, TestEndpointID: nil}
	if got := (&Service{}).engEndpointFor(tk3, engRoleTest); got != w {
		t.Fatalf("test role w/o test+reviewer should fall back to writer, got %q", got)
	}
	// review 不受 test 槽影响。
	tk4 := task.Task{WriterEndpointID: ref(w), ReviewerEndpointID: ref(r), TestEndpointID: ref(tt)}
	if got := (&Service{}).engEndpointFor(tk4, engRoleReview); got != r {
		t.Fatalf("review role must use reviewer slot (never test), got %q", got)
	}
	tk5 := task.Task{WriterEndpointID: ref(w), ReviewerEndpointID: nil, TestEndpointID: ref(tt)}
	if got := (&Service{}).engEndpointFor(tk5, engRoleReview); got != w {
		t.Fatalf("review role w/o reviewer should fall back to writer (not test), got %q", got)
	}
	// 全空 → ""
	if got := (&Service{}).engEndpointFor(task.Task{}, engRoleTest); got != "" {
		t.Fatalf("all-empty should return empty, got %q", got)
	}
}

// A2 补充:CreateEndpoint 未给 tier → 落列默认 standard(mapper 归一,INSERT 显式列不再绕过 DEFAULT)。
func TestEndpointTierDefaultToStandardOnCreate(t *testing.T) {
	_, st := newSvc(t)
	compID := seedCompanyID(t, st)
	now := time.Now().Unix()
	ep, err := st.CreateEndpoint(context.Background(), endpoint.Endpoint{
		ID: uuid.NewString(), CompanyID: compID, Name: "legacy",
		BaseURL: "http://127.0.0.1:1", Proto: "openai", Vendor: "gateway",
		Status: "active", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateEndpoint: %v", err)
	}
	if ep.Tier != "standard" {
		t.Fatalf("tier empty at create must normalize to standard, got %q", ep.Tier)
	}
	got, err := st.GetEndpoint(context.Background(), ep.ID)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if got.Tier != "standard" {
		t.Fatalf("persisted tier = %q, want standard", got.Tier)
	}
}

func strOrNil(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}
