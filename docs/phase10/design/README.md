# Phase 10 设计索引

> 该阶段定稿设计登记处。设计定稿后在此登记,此后按它执行。
> 状态:🔶 方向定稿 + 10.1/10.2/**10.3/10.4 完结**(2026-09-08;10.4 高智运行时合成 = plan_policy 列触发 + frontier 合成落 upfront 账本 + 先审后干 + 平行驱动 runSynthesized,归档 stages/4.md)+ **增补:项目 ⇄ GitHub 自动代码获取 ✅(2026-09-09;建时 clone + run 前自动拉 = D7 code 源绑定补成实取代码半环,零迁移;归档 stages/5.md)** + **10.5 项目级 GitHub token + issue→分支→PR 闭环 ✅(2026-09-09;通道 B 补回写半环:每项目 github_token → 同步 issue / 建 bugfix·feature 分支 / 收尾发 PR,人在 GitHub 合入;迁移 0019;归档 stages/6.md)** + **10.6 OS 机械扩 build·test ✅(2026-09-09;合成 upfront 计划里 os-accept 相位显式声明 verify → OS 真跑 go build ./… / go test ./… 作 D4 真裁判;触发面仅合成+显式声明,默认 adaptive 不真跑;零迁移 + 零 Web/CLI/API diff;归档 stages/7.md)**。Phase 10 完结后候选(10.6+):例行模板化数据化 / 凭据注入·主机 allowlist(方向 §八;原「10.5」指代因本回环取号 10.5 冻结移后)。

| 文档 | 状态 | 定稿日期 |
|---|---|---|
| [declarative-pipelines.md](declarative-pipelines.md) | ✅ 定稿(方向;D7=A 项目=company 下新实体 / D6 三类形态 / D3 例行模板化先行合成 10.4 / knowledge 文件为准+Memory 索引) | 2026-09-07 |
| [project-pipeline-foundation.md](project-pipeline-foundation.md) | ✅ 定稿(10.1 实施契约;run 复用 engineering driver / 整项目一个 git 仓库 / 契约定稿门:root 空目录 OS git init + 受控 DELETE) | 2026-09-08 |
| [ops-patrol-schedule.md](ops-patrol-schedule.md) | ✅ 定稿(10.2 实施契约;cron 五段调度 / ops_patrol 巡检真形态 / 契约定稿门:cron 语法 + 凭据后移 + fix+high 自动链拉 + 独立调度开关 & 报告正文内嵌读) | 2026-09-08 |
| [phase-plan-contract.md](phase-plan-contract.md) | ✅ 定稿(10.3 实施契约;run 计划账本 task_plan/phase + 双形态 patrol upfront 预铺·engineering grow 记账 + 计划审批=增强既有 risk 门 + 逐阶段 I/O 验收 & patrol OS 机械预检 D4 首落点) | 2026-09-08 |
| [run-synthesis.md](run-synthesis.md) | ✅ 定稿并实施完结(10.4 实施契约;高智运行时合成 — plan_policy 列触发 + frontier 合成计划落 upfront 账本 + 先审后干 + 平行驱动 runSynthesized 逐阶段 do/accept/dispose + OS 机械只读允许清单;定稿门:预算不做[外部控制]/列触发/机械只读/平行驱动;归档 [stages/4.md](../stages/4.md)) | 2026-09-08 |
| [project-github-code.md](project-github-code.md) | ✅ 定稿并实施完结(10.4 后增补方向+实施契约;项目绑 GitHub 地址,本地目录可空/不存在 → **建时 OS 自动 clone** + 工程 run 前**首个 fresh 认领自动 pull**;零迁移[origin=代码源单一来源];契约定稿门=本会话计划通过 2026-09-09:建时 clone + run 前拉 + dogfood 公开仓库;归档 [stages/5.md](../stages/5.md)) | 2026-09-09 |
| [github-roundtrip-pr.md](github-roundtrip-pr.md) | ✅ 定稿并实施完结(10.5 方向+实施契约;**取回半环(上条)补上回写**:每项目一个 GitHub token,OS 回拉 issue / 建 bugfix·feature 分支 / 分支上提交 / 收尾发 PR,审批由人在 GitHub 页面合入 — 项目级机密 project_secret(github_token 白名单,enc:v2: seal)+ token 双层解析(写=仅项目 / 读=项目→公司回退)+ SyncProject 项目级同步 + UpdateProjectAs 已有项目编辑 + finishRun 统一发 PR 三完成点 + POST /tasks/{id}/publish-pr 人工重试;契约定稿门=本会话 ExitPlanMode 计划通过(用户原话逐条保留入 §引言);归档 [stages/6.md](../stages/6.md)) | 2026-09-09 |
| [os-mechanical-verify.md](os-mechanical-verify.md) | ✅ 定稿并实施完结(10.6 实施契约;**OS 机械扩 build·test — D4 真裁判**;重开 10.4 门③;合成 upfront 计划里 os-accept 相位**显式声明 verify** → OS 真跑封闭命令集 go build ./… / go test ./… 作机械验收判据;触发面 = 仅合成 + 相位显式声明,默认 adaptive 与无 verify 相位永不真跑 go → 600+ 既有用例零 body 改;命令封闭双键固定 argv 无 shell;网络策略零改[继承宿主 go env];残留对账只回收新增 untracked·改动 tracked → fail;契约定稿门=本会话 ExitPlanMode 计划通过[触发面/双键/继承 env/回收 四问全按推荐];零迁移 + **零 Web/CLI/API diff**;归档 [stages/7.md](../stages/7.md)) | 2026-09-09 |

