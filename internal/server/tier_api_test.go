package server

// Phase 8.4 C8(端点面):POST /endpoints/{id}/select 带 tier 指派/落库 + 非法 tier 校验;
// POST /tasks 带 test_endpoint_id(显式)→ 201 回读。harness 沿用 api_test.go(同包)。

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
)

func TestAPISelectEndpointTier(t *testing.T) {
	srv, st := newTestServer(t)
	comp := seedCompany(t, st, "ACME", "")
	ctx := context.Background()

	// add 端点(proto=openai;未标档 → standard)
	addBody := fmt.Sprintf(`{"company_id":%q,"name":"gw","base_url":"http://127.0.0.1:1/v1","proto":"openai"}`, comp.ID)
	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/endpoints", addBody)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("add endpoint: code=%d env=%+v", rec.Code, env)
	}
	var ep endpoint.Endpoint
	decodeData(t, env, &ep)
	if ep.Tier != "standard" {
		t.Fatalf("new endpoint tier = %q, want standard (列默认起步)", ep.Tier)
	}

	// select 指派 tier=frontier(role 同时给,验证正交)
	selBody := fmt.Sprintf(`{"model":"opus-model","role":"planner","tier":"frontier"}`)
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/endpoints/"+ep.ID+"/select", selBody)
	if rec.Code != http.StatusOK || !env.OK {
		t.Fatalf("select tier: code=%d env=%+v", rec.Code, env)
	}
	var sel endpoint.Endpoint
	decodeData(t, env, &sel)
	if sel.Tier != "frontier" || sel.Role != "planner" || sel.SelectedModel != "opus-model" {
		t.Fatalf("select result tier/role/model = %q/%q/%q, want frontier/planner/opus-model", sel.Tier, sel.Role, sel.SelectedModel)
	}

	// 非法 tier → 500(internal)+ 校验消息(handleServiceErr 映射)
	badBody := `{"model":"m","tier":"ultra"}`
	rec, env = doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/endpoints/"+ep.ID+"/select", badBody)
	if env.OK || env.Error == nil || !strings.Contains(env.Error.Message, "tier must be frontier|standard|cheap") {
		t.Fatalf("invalid tier should be rejected with validation message, env=%+v", env)
	}

	// 审计可见 select detail 带 tier
	audits, err := st.ListAudits(ctx, "endpoint")
	if err != nil {
		t.Fatalf("ListAudits: %v", err)
	}
	found := false
	for _, a := range audits {
		if a.EntityID == ep.ID && a.Action == "select" && strings.Contains(a.Detail, "tier=frontier") {
			found = true
		}
	}
	if !found {
		t.Fatalf("select audit should carry tier in detail")
	}
}

func TestAPICreateTaskEchoesTestEndpoint(t *testing.T) {
	srv, st := newTestServer(t)
	comp := seedCompany(t, st, "ACME", "")

	// 公司两个判读端点:review=frontier(select 指派)、test=standard(列默认),task 显式给两端点。
	var ids []string
	for _, c := range []string{
		`{"company_id":%q,"name":"review-gw","base_url":"http://127.0.0.1:1/v1","proto":"openai"}`,
		`{"company_id":%q,"name":"test-gw","base_url":"http://127.0.0.1:1/v1","proto":"openai"}`,
	} {
		rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/endpoints", fmt.Sprintf(c, comp.ID))
		if rec.Code != http.StatusCreated || !env.OK {
			t.Fatalf("add endpoint: code=%d env=%+v", rec.Code, env)
		}
		var e endpoint.Endpoint
		decodeData(t, env, &e)
		ids = append(ids, e.ID)
	}
	reviewer, test := ids[0], ids[1]

	// task body:engineering + 显式 reviewer/test + writer 空(cheap+anthropic 无 → 合法)。显式两判读槽 → 无硬失败。
	tkBody := fmt.Sprintf(`{"company_id":%q,"title":"impl","tool_name":"engineering","risk":"low","reviewer_endpoint_id":%q,"test_endpoint_id":%q}`,
		comp.ID, reviewer, test)
	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/tasks", tkBody)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create engineering task with explicit judge slots: code=%d env=%+v", rec.Code, env)
	}
	var tk task.Task
	decodeData(t, env, &tk)
	if tk.TestEndpointID == nil || *tk.TestEndpointID != test {
		t.Fatalf("echoed test_endpoint_id = %v, want %s", tk.TestEndpointID, test)
	}
	if tk.ReviewerEndpointID == nil || *tk.ReviewerEndpointID != reviewer {
		t.Fatalf("echoed reviewer_endpoint_id = %v, want %s", tk.ReviewerEndpointID, reviewer)
	}
	if tk.WriterEndpointID != nil {
		t.Fatalf("writer must stay empty (no cheap+anthropic), got %v", tk.WriterEndpointID)
	}
}
