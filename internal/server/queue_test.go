package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glacierzzz26/one-person-company-os/internal/agent"
	"github.com/glacierzzz26/one-person-company-os/internal/permission"
	"github.com/glacierzzz26/one-person-company-os/internal/policy"
	"github.com/glacierzzz26/one-person-company-os/internal/task"
	"github.com/google/uuid"
)

// TestQueueLoopConsumesConsoleTask Phase 7.2「不留白」②端到端:控制台 POST /tasks 建的任务,
// server 队列循环(queueOnce)自己认领并执行完成 —— 不再必须手动 `os queue work`。
// file-write 工具走宿主 FS(temp dir),离线可测;治理链同真:agent + allow policy + permission,
// 默认拒绝不破(无授权的旁路任务应 fail)。
func TestQueueLoopConsumesConsoleTask(t *testing.T) {
	srv, st := newTestServer(t)
	ctx := context.Background()

	comp := seedCompany(t, st, "ACME", "")
	cp := seedCapability(t, st, comp.ID, "ops")
	ag, err := st.CreateAgent(ctx, agent.Agent{
		ID: uuid.NewString(), CapabilityID: cp.ID, Name: "runner", Role: "executor",
		CreatedAt: nowUnix(), UpdatedAt: nowUnix(),
	})
	if err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	pol, err := st.CreatePolicy(ctx, policy.Policy{
		ID: uuid.NewString(), CompanyID: comp.ID, Name: "file-write allow", Kind: "allow",
		Statement: "executor may write files", Enabled: true,
		CreatedAt: nowUnix(), UpdatedAt: nowUnix(),
	})
	if err != nil {
		t.Fatalf("seed policy: %v", err)
	}
	if _, err := st.CreatePermission(ctx, permission.Permission{
		ID: uuid.NewString(), PolicyID: pol.ID, Subject: "executor",
		Action: "write", Resource: "file", CreatedAt: nowUnix(),
	}); err != nil {
		t.Fatalf("seed permission: %v", err)
	}

	ws := t.TempDir()
	body := fmt.Sprintf(`{"company_id":%q,"capability_id":%q,"agent_id":%q,"title":"写个文件","tool_name":"file-write","risk":"low","max_attempts":1,"workspace":%q,"description":%q}`,
		comp.ID, cp.ID, ag.ID, ws, "write hello.txt\nhi from queue loop")
	rec, env := doAPI(t, srv.Handler(), http.MethodPost, "/api/v1/tasks", body)
	if rec.Code != http.StatusCreated || !env.OK {
		t.Fatalf("create task: code=%d env=%+v", rec.Code, env)
	}
	var tk task.Task
	decodeData(t, env, &tk)

	srv.SetQueueWork(50 * time.Millisecond)
	if !srv.QueueWorkEnabled() {
		t.Fatal("QueueWorkEnabled should be true after SetQueueWork")
	}
	// 直接驱动单 tick(等价 QueueLoop 一轮),确定性验证消费 + 执行 + 治理链路。
	srv.queueOnce(ctx)

	got, err := st.GetTask(ctx, tk.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("task status = %q (last_error=%q), want completed via server queue loop", got.Status, got.LastError)
	}
	raw, err := os.ReadFile(filepath.Join(ws, "hello.txt"))
	if err != nil || string(raw) != "hi from queue loop" {
		t.Errorf("workspace file = %q err=%v, want %q", raw, err, "hi from queue loop")
	}
	// 执行审计由 runtime:server 落链(证明是 server 消费,非其他 worker)。
	audits, _ := srv.svc.ListAudits(ctx, "execution")
	runtimeSeen := false
	for _, a := range audits {
		if a.Actor == "runtime:server" && a.Action == "complete" {
			runtimeSeen = true
		}
	}
	if !runtimeSeen {
		t.Errorf("expected runtime:server completion audit, got %+v", audits)
	}

	// QueueLoop 协程:开启后快速 tick,ctx 取消即退出(不死锁、不 panic)。
	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { srv.QueueLoop(loopCtx); close(done) }()
	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("QueueLoop did not stop after ctx cancel")
	}
}

// TestQueueLoopDisabledDefault 默认关闭:未 SetQueueWork 时 QueueLoop 记日志即返回,
// 驱动语义不变(仍走 CLI os queue work)。
func TestQueueLoopDisabledDefault(t *testing.T) {
	srv, _ := newTestServer(t)
	if srv.QueueWorkEnabled() {
		t.Fatal("queue work should default off")
	}
	done := make(chan struct{})
	go func() { srv.QueueLoop(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("QueueLoop should return immediately when disabled")
	}
}
