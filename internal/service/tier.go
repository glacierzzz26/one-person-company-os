package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/glacierzzz26/one-person-company-os/internal/endpoint"
)

// 档位分层强制(Phase 8.4,方向 model-runtime §三/§十)。tier 与 role(pool|planner|standby)正交:
// endpoint 带档,角色默认档映射在 engineering 建单(live)未显式给端点时解析落槽;显式端点永远覆盖默认。
// tier 中文标注(方向 §十 1:frontier=高智、standard=均衡、cheap=经济)属展示面,落在各自层:
// CLI internal/cli/endpoint.go tierCN、控制台 web/src/pages/Endpoints.tsx 的 tierLabel;service 只持常量与解析。
const (
	tierFrontier = "frontier"
	tierStandard = "standard"
	tierCheap    = "cheap"
)

// isScriptedEngine 是否离线确定性模式(与 engCall 分界同源:scripted 永不落委派/网关,
// 故默认解析与硬失败也只在 live 语义下生效,scripted 建 engineering 任务沿用现语义)。
func isScriptedEngine() bool {
	return strings.EqualFold(os.Getenv("OS_ENGINE_MODE"), "scripted")
}

// applyEndpointDefaults 8.4 契约:engineering(live)建单把**空**角色槽按默认档确定性解析落位。
// 显式给的槽不解析、不硬失败。返回最终三槽指针 + 默认命中明细(供 create audit detail)。
//
// 契约(方向 §三表 + 定稿门 ①~③):
//
//	reviewer 默认 frontier、test 默认 standard —— 判读槽都要 proto=openai(判读走网关),解析不到 → **硬错误**;
//	writer 默认 cheap 且只收 proto=anthropic(可作 claude 委派后端;OpenAI 方言网关不能作 claude 后端,
//	见 delegateEnv 头注),解析不到 → **留空**(claude 自带鉴权/模型 = 合法态),不硬失败。
//
// 确定性:active + 匹配 tier/proto 中按 created_at ASC、id ASC 取最早。
func (s *Service) applyEndpointDefaults(ctx context.Context, companyID string, writer, reviewer, test *string) (w, r, t *string, detail []string, err error) {
	w, r, t = writer, reviewer, test
	if companyID == "" {
		return w, r, t, nil, nil // 无公司上下文(异常建单)不做默认解析
	}
	list, lerr := s.store.ListEndpoints(ctx, companyID)
	if lerr != nil {
		return nil, nil, nil, nil, lerr
	}
	ptr := func(v string) *string { return &v }
	// pick 取 active 且 tier/proto 匹配中最早的一条(created_at ASC、id ASC 决胜,确定性)。
	pick := func(tier, proto string) *endpoint.Endpoint {
		var best *endpoint.Endpoint
		for i := range list {
			e := &list[i]
			if e.Status != "active" || e.Tier != tier {
				continue
			}
			if proto != "" && e.Proto != proto {
				continue
			}
			if best == nil || e.CreatedAt < best.CreatedAt || (e.CreatedAt == best.CreatedAt && e.ID < best.ID) {
				best = e
			}
		}
		return best
	}
	hit := func(role string, e *endpoint.Endpoint) {
		if e == nil {
			return
		}
		detail = append(detail, fmt.Sprintf("%s=%s:%s", role, e.ID, e.SelectedModel))
	}
	judgeErr := func(role, tier, flag string) error {
		return fmt.Errorf("engineering task default: company %s has no active tier=%s gateway (proto=openai) endpoint for %s; add one (os endpoint add --proto openai …  + os endpoint select <id> --tier %s), or pass --%s explicitly", short8(companyID), tier, role, tier, flag)
	}
	if isNilOrEmpty(w) {
		if e := pick(tierCheap, "anthropic"); e != nil {
			w = ptr(e.ID)
			hit("writer", e)
		} // 无 cheap+anthropic → 留空:claude 委派走自带(合法,非错误)
	}
	if isNilOrEmpty(r) {
		e := pick(tierFrontier, "openai")
		if e == nil {
			return nil, nil, nil, nil, judgeErr("reviewer", tierFrontier, "reviewer-endpoint")
		}
		r = ptr(e.ID)
		hit("reviewer", e)
	}
	if isNilOrEmpty(t) {
		e := pick(tierStandard, "openai")
		if e == nil {
			return nil, nil, nil, nil, judgeErr("test", tierStandard, "test-endpoint")
		}
		t = ptr(e.ID)
		hit("test", e)
	}
	return w, r, t, detail, nil
}

// isNilOrEmpty 指针为空或空串都视为「未显式给」(空串 = 显式清空,等同没给)。
func isNilOrEmpty(p *string) bool { return p == nil || *p == "" }
