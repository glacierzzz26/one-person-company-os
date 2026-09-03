# Phase 7.2 — 运营控制台 UI(设计定稿)

> 阶段方向级设计,定稿冻结后按它执行。承接 [web-console.md](web-console.md)(7.1 JSON API 契约,冻结不动);
> 本文是控制台的**信息架构 / 视觉语言 / 交互语义 / 契约保真**定稿,React SPA(7.3)按此实现。
> 高保真可交互原型:[console-ui-prototype.html](console-ui-prototype.html)(浏览器直开,mock 数据 = 契约真实形状)。

## 一、定位

- 一人操作台的**驾驶舱**:全景 → 异常 → 决策,三步内闭环;人只做决策,不做搬运。
- 原型阶段以 vanilla 单文件承载(零依赖、可分享、能演交互);7.3 用 React + TypeScript + Ant Design
  重写,复用本文全部设计令牌与交互规格,mock 数据整层替换为 `/api/v1` 真调用(契约不变)。
- **本轮同步补齐两处后端留白**(见 stages/2.md):① 控制台写操作审计 actor = `human:console`
  (service 全套 `*As` 变体,CLI 保持 `human:cli`);② `os server --queue-work` 队列认领循环,
  控制台建的任务可由 server 自动消费,无需手动 `os queue work`。

## 二、信息架构(8 视图,对齐页面×接口矩阵)

| 视图 | 一句话职责 | 数据端点 | 关键写 |
|---|---|---|---|
| 全景总览 | 公司健康度 + 研发部 RD 块(熔断/待审批/子任务/issue 账本)+ 待审批卡片区 | `GET /companies/{id}/overview` | 跳转决策 |
| 待审批 | 人的决策中心:pending 优先,已决只读 | `GET /approvals?status=` | `POST /approvals/{id}/decision` |
| 任务 | 全任务台账:状态/风险/工具/回合筛选,执行时间线抽屉 | `GET /tasks` `GET /tasks/{id}/executions` | `POST /tasks`(建即入队) |
| 决策 | 治理链:kind 筛选 + 溯源(approval:<id> / manual) | `GET /companies/{id}/decisions?kind=` | `POST …/decisions` |
| 记忆 | 知识库:类型筛选 + 全文搜索高亮 | `GET …/memories` `…/memories/search?q=` | `POST …/memories` |
| 审计 | 全系统事件流,实体筛选;actor 即来源血缘 | `GET /audit?entity=` | — |
| 模型端点池 | token 仅密文(enc:v1:),响应无明文;拉模型/选模型/角色 | `GET /companies/{id}/endpoints` 等 | `POST /endpoints` `/{id}/select` `/{id}/models` |
| 研发仓库(通道 B) | issue 账本处置分布 + 立即同步 | `GET /companies/{id}/repos` | `POST /companies/{id}/intake/sync` |

布局骨架:深色侧栏(品牌/公司切换/导航/主题)+ 浅色内容区;每卡片右上角标**数据来源端点徽标**,
原型即联调清单 —— 7.3 逐卡把徽标对应的 mock 换成真 fetch。

## 三、设计令牌(双主题)

- **强调色**:极客蓝 `#2f54eb`(操作/选中/链接),**语义色与其分离**:ok `#1a9e65` / warn `#c9861a` /
  crit `#d43b52`(熔断、驳回、失败一律走语义色,不蹭强调色)。
- **字体**:Noto Sans SC(界面)+ JetBrains Mono(ID/时间/JSON/命令);等宽数字 `tabular-nums`。
- **主题机制**:三态(浅色/深色/跟随系统)。令牌定义于裸 `:root`(浅色全量),
  `@media (prefers-color-scheme: dark)` 守 `:root:not([data-theme="light"])` 只覆盖令牌,
  `:root[data-theme="dark"]` 再覆盖(手选赢系统);`localStorage` 记忆(try/catch,不可用则静默)。
- 中性灰带蓝偏(向强调色靠拢),不用纯灰;卡片按角色分配边/影/圆角,不处处同款。

## 四、交互规格(语义必须与 service 一致)

1. **审批决策**(三态分段 + 备注):
   - `approve` → 审批 `approved`,任务回队 `pending/ready`、conflict 清零、round+1;
   - `reject` / `changes` → **均置任务 failed(q_status=dead)**,`last_error` 分别为
     `rejected by human` / `changes requested: <note>`;「重开」= 重新建单,不是返工按钮;
   - 已决审批**只读**(再点 → 提示 `approval already <status>`);审批状态存原始决策串(`changes`);
   - 每次决策联动:RD 计数(by_status/fused/waiting)+ 自动 decision(`kind=approval`,
     `title="审批 <status>: <任务>"`,`source=approval:<id>`)+ 审计一条(`action=<decision>`)
     + toast 回显端点与状态码。
2. **任务创建**:high 风险即时提示将卡审批门(等价 service 行为);入队后**两条真实消费路径** —
   `os queue work` 手动排空,或 server 带 `--queue-work` 自驱(审计 `runtime:server`)。
   API 契约维持不暴露执行(❌,长时阻塞归 CLI/后台)。
3. **端点池**:role 取值 `pool|planner|standby`(planner=拆解规划端点,standby=热备);
   proto 取值 `auto|anthropic|openai`(auto 按域名识别 vendor);「选择模型」弹窗 =
   `POST /endpoints/{id}/select`(model 必填、role 可选),模型缓存 chips 点选回填;
   Token 输入永不回显;「拉取模型」= `POST /endpoints/{id}/models`(真实网络触发)。
4. **actor 血缘**(本轮贯通):凡经控制台发生的写(建任务/决策/沉淀/端点/仓库)actor 一律
   `human:console`;历史 CLI 行保持 `human:cli`。审计视图 actor 列即血缘视图。
5. 危险动作(端点删除/复权)不进 API(契约 ❌);原型不提供假按钮。

## 五、数据保真原则(原型即联调预演)

- mock 字段**逐字**对齐 json tag(snake_case、int64 unix 秒、完整 UUID、`*string→null`);
- 展示文案用业务语言(「放行」「热备」),但实体/动作/actor 值与后端一字不差;
- 不发明契约里没有的枚举(`changes` 不是 `changes_requested`;无 `writer/reviewer` 端点角色 —
  那是任务级 `writer_endpoint_id/reviewer_endpoint_id` 的回合概念)。

## 六、子阶段与验收

- **7.2(本阶段,✅ 冻结)**:actor 贯通 + QueueLoop 代码落地(httptest + 离线真实 server 冒烟
  `create human:console → runtime:server → completed`);设计文档 + 可交互原型。
- **7.3(2026-09-03,✅ 实现完成,归档见 [stage 3](../stages/3.md))**:React 18 + TS + AntD v5 SPA 落地在仓库根
  `web/`(Vite;`go:embed` 单二进制,`os server` 同源托管 —— 方案即上文 7.2 候选的「embed 单二进制」);8 视图全部
  接真 `/api/v1`,字段/枚举与后端 json tag 逐字一致。与本文差异仅记录的实现取舍(非契约变更):
  - 部署 = `internal/console`(stdlib)+ `make ui` 拷 `web/dist` 进 embed 目录;dev = `npm run dev` + vite proxy → 8787。
  - 双主题经 AntD ConfigProvider token + CSS 变量(`data-theme` 三态),不引外部字体/图标字体(离线单二进制)。
  - 部分只在原型出现的 mock 形状在真实契约下不存在/不同,按「数据保真」处理(见上 §五与 stage 3 已知差异表):
    审批中心行标题懒取 `/tasks/{id}`;端点只显 `token_enc` 非空指示;账本/状态/枚举用真键;`overview` 空态
    capabilities/workflows/pending_approvals 为 JSON null → 前端兜底空数组。
  - 分页/CORS 仍按需补(见 web-console.md §一「不做」),未在本阶段引入。
- 验收口径:原型 8 视图全部可点、状态变化可回退刷新;每卡端点徽标在 7.3 即联调清单 —— 7.3 已按此跑通
  tsc/build/go test + 离线 `os server` smoke(SPA + client 路由 fallback + 建公司 → overview RD null)。
