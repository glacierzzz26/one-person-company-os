package service

import "testing"

// 判读结构化单元用例(Phase 8.3 A):JSON 主契约 + fence/prose 容忍 + 旧标记兜底 + 坏输入不崩。

// A1 JSON 主解析(各角色)。
func TestParseJudgeJSON(t *testing.T) {
	// test
	if pass, sum := parseTest(`{"pass":false,"summary":"failed on edge x"}`); pass || sum != "failed on edge x" {
		t.Fatalf("parseTest json = (%v,%q)", pass, sum)
	}
	if pass, sum := parseTest(`{"pass":true,"summary":"ok"}`); !pass || sum != "ok" {
		t.Fatalf("parseTest json pass = (%v,%q)", pass, sum)
	}
	// review
	if v, r := parseReview(`{"verdict":"needs_changes","reason":"extract this first"}`); v != "needs_changes" || r != "extract this first" {
		t.Fatalf("parseReview json = (%q,%q)", v, r)
	}
	if v, _ := parseReview(`{"verdict":"approve"}`); v != "approve" {
		t.Fatalf("parseReview approve = %q", v)
	}
	// triage
	if d, n := parseDisposition(`{"disposition":"direct_work"}`); d != engDispDirectWork || n != "" {
		t.Fatalf("parseDisposition json = (%q,%q)", d, n)
	}
	if d, n := parseDisposition(`{"disposition":"merge","note":"umbrella"}`); d != engDispMerge || n != "umbrella" {
		t.Fatalf("parseDisposition merge = (%q,%q)", d, n)
	}
}

// fence / prose 前后缀容忍(统一解码)。
func TestParseJudgeFencedAndProse(t *testing.T) {
	// ```json 围栏
	if pass, _ := parseTest("```json\n{\"pass\":false,\"summary\":\"boom\"}\n```"); pass {
		t.Fatalf("fenced json should parse")
	}
	// prose 夹带
	if v, r := parseReview("Sure, here you go:\n{\"verdict\":\"needs_changes\",\"reason\":\"concern\"}\nHope that helps"); v != "needs_changes" || r != "concern" {
		t.Fatalf("prose-wrapped review = (%q,%q)", v, r)
	}
	if pass, _ := parseTest("Summary of run:\n{\"pass\":true,\"summary\":\"all good\"}\nEnd"); !pass {
		t.Fatalf("prose-wrapped test should parse pass")
	}
}

// A2 旧标记兜底(仅兜底,防解析崩)。
func TestParseJudgeLegacyFallback(t *testing.T) {
	// test:显式 FAIL → 不过;OK → 过;无信号 → 过(8.2 启发式)
	if pass, _ := parseTest("TEST FAIL: missing import"); pass {
		t.Fatalf("TEST FAIL should not pass")
	}
	if pass, _ := parseTest("TEST OK"); !pass {
		t.Fatalf("TEST OK should pass")
	}
	if pass, _ := parseTest("ran the suite, nothing else"); !pass {
		t.Fatalf("no failure signal should default to pass (8.2 heuristic)")
	}
	// review:VERDICT 单行标记
	if v, r := parseReview("VERDICT: needs_changes: the API shape is wrong"); v != "needs_changes" || r == "" {
		t.Fatalf("legacy VERDICT = (%q,%q)", v, r)
	}
	if v, _ := parseReview("VERDICT: approve"); v != "approve" {
		t.Fatalf("legacy VERDICT approve = %q", v)
	}
	// triage:DISPOSITION 单行标记
	if d, _ := parseDisposition("DISPOSITION: skip: duplicate of #9"); d != engDispSkip {
		t.Fatalf("legacy DISPOSITION = %q", d)
	}
	// 垃圾文本不崩,按角色语义不可判/默认过
	if pass, _ := parseTest("%%% not json"); !pass {
		t.Fatalf("garbage test text should default to pass without crashing")
	}
	if v, _ := parseReview("no verdict here"); v != "" {
		t.Fatalf("garbage review should be unparseable (v=%q)", v)
	}
	if d, _ := parseDisposition("no disposition"); d != "" {
		t.Fatalf("garbage triage should be unparseable (d=%q)", d)
	}
}

// 合法 JSON 但未知 verdict → 不可判(空),driver 沿既有报错路径。
func TestParseReviewUnknownVerdict(t *testing.T) {
	if v, _ := parseReview(`{"verdict":"ship_it"}`); v != "" {
		t.Fatalf("unknown verdict should be unparseable, got %q", v)
	}
	if v, _ := parseReview(`{"pass":true}`); v != "" {
		t.Fatalf("missing verdict should be unparseable, got %q", v)
	}
	// 归一:approved / rejected 均归主契约值
	if v, _ := parseReview(`{"verdict":"rejected","reason":"no"}`); v != "needs_changes" {
		t.Fatalf("rejected should normalize to needs_changes, got %q", v)
	}
}
