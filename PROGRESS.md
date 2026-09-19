# 项目进度备忘录 — zhonghuawenhua\_backend

> 最后更新: 2026-08-21 (防御性编程全库复审：24 条问题清单，修复 4 条高危 + 5 条中优先级 #5-9)
> 本文件置于项目最外层，供随时查阅。

***

## 〇、最近更新

### 2026-08-21 修复问题清单中优先级 #5-9（错误码语义 + 内部细节泄露 + AI 会话越权）
- **#5 respondError 降级 400**（`handler/helpers.go`）: 原先 403/409 一律 `BadRequest`(400)，`BizError.Status` 语义失效。新增 `pkg/httpresp` 的 `Forbidden`(403)/`Conflict`(409) helper，`respondError` 按 `be.Status` 原状返回 401/403/409/501，其余 4xx 兜底 400
- **#6 500 泄露 err.Error()**（`handler/helpers.go:53`、`server.go:94/129`、`upload.go:26`）: `respondError` 未知错误改 `InternalError(c,"internal error","")` 不再带内部细节；AI Chat/GetMessages handler 改走 `respondError`（已知 BizError 正确映射、未知错误不泄露）；upload `FormFile` 错误去掉 `err.Error()`
- **#7 AI 会话裸 fmt.Errorf**（`service/ai_chat.go`）: 会话不存在/越权改用已定义的 `ErrAISessionNotFound`(400)/`ErrAISessionAccessDenied`(403)，不再 500；Chat 与 GetMessages 两处均已替换
- **#8 GetSources IDOR 残留**（`service/moment.go:227`）: `GetSources` 补 `userID` 入参 + `EnsureClassMember` 班级校验，handler `GetAPIStuClassClassIDMomentsSources` 从 JWT 取 userID 传入
- **#9 user_id 为 NULL 越权被跳过**（`service/ai_chat.go`）: 归属校验 `sess.UserID != nil && *sess.UserID != userID` 在 user_id 为 NULL 时被跳过；改为 `sess.UserID == nil || *sess.UserID != userID`，NULL 默认拒绝（防御性）
- **验证**: `go build ./...`、`go vet ./...` 通过（exit 0）；gofmt 仅剩 CRLF 历史遗留标记（helpers.go/server.go 为整文件行尾差异，非本轮引入，未做整文件换行改动）

### 2026-08-21 防御性编程全代码库专项复审（24 条问题清单 + 高危修复）
- **范围/方法**: 5 层全量审查（handler / middleware / service / repository+model / pkg + config + main），4 个分层代理并行通读 + 主线程逐行核实 + 2 个独立验证代理二次交叉复核；**24 条候选问题全部确认存在、无 false positive**
- **现有防御基线（已达标，无需改动）**:
  - 身份来源：userID 一律取自 JWT（`c.Get("user_id")`），不信任路径/请求体；API Key 鉴权不注入 user_id，业务接口一律 401
  - 越权防护：6 处 `GetSubmission` 全做 NodeID+NodeType+UserID 三重归属校验（越权返回 `ErrQuestionNotFound`）；class-scoped 方法统一 `EnsureClassMember`；朋友圈 `getOwnedMoment` 校验 ClassID
  - 并发：`UserSubmitLock`（非阻塞/幂等/槽回收）+ `RunInTx` + `attempt_count` ON CONFLICT 原子自增 + `draft_json` jsonb_set 原子合并 + 综合评价单飞锁 + 朋友圈 `MomentLock`
  - 第三方：AI 客户端状态码校验/BaseURL 去斜杠/60s 超时；主判定 AI 失败不静默成功、反馈类失败优雅降级；回写 `updateSubmissionWithRetry` 重试
  - 错误处理：`BizError` 稳定业务码 + Tip 透出；httpresp `UseNumber` 保 int64 精度、序列化失败转 500 杜绝假 200
  - 数据库：全参数化 SQL 零注入面；JSONB NOT NULL 空值兜底（learning.go）；`pgx.ErrNoRows` 区分未找到
  - 运行时：`http.Server` 四类超时（M13）+ `gin.Recovery()` + `toUserBaseInfo` nil 判空
  - 确认无问题：SQL 注入、synclock 死锁/内存回收、httpresp 精度与假 200、accesslog、`ComputeSegmentDurations` 负数/越界
- **问题清单（24 条，含状态）**——状态 = **已修复**（本轮落地）/ **待办**（后续处理）：

  | # | 级别 | 问题 | 位置 | 状态 |
  | - | - | - | - | - |
  | 1 | 高 | 提交 `Type` 未校验枚举：`type="foo"` 绕过空输入/错误上限/幂等，无限刷 AI | write_thoughts.go / zhaozhouqiao_submit.go / wenmingzhongwai_submit.go 的 submit 分支 | **已修复**（入口校验 `Type ∈ {submit,get-answer}`） |
  | 2 | 高 | `PostAPIAiSession` 传空 Content 命中 `ErrEmptyInput` 恒 500，"创建会话"不可用 | handler/server.go:145 + service/ai_chat.go:42 | **已修复**（新增独立建会话路径） |
  | 3 | 高 | heritage `judgeByAI` 成功后 `coachByAI` 失败整体报错，合格判定丢失且白耗提交名额 | service/heritage_cultural.go:187-190 | **已修复**（辅导失败降级为引导话术，保留判定） |
  | 4 | 高 | `latestRevisedExample` 倒序遍历（`ListByUserNode` 为 DESC，最新在前），返回最旧范文 | service/heritage_cultural.go:553-565 | **已修复**（改为正序取最新） |
  | 5 | 中 | `respondError` 把 403/409 全降级成 400，`BizError.Status` 语义失效 | handler/helpers.go:43-50 | **已修复**（新增 httpresp.Forbidden/Conflict，按 Status 原状返回 401/403/409/501） |
  | 6 | 中 | 500 响应泄露 `err.Error()`（SQL/路径/AI 原始错误），与注释"避免泄露内部细节"相悖 | handler/helpers.go:53、server.go:94/129/147、upload.go:26 | **已修复**（500 不再带 err.Error；Chat/GetMessages 改走 respondError；upload.go 去掉 err.Error()） |
  | 7 | 中 | AI 会话不存在/越权返回裸 `fmt.Errorf`（已定义的 `ErrAISessionNotFound/AccessDenied` 未用），返回 500 而非 400/403 | service/ai_chat.go:67/72/221/226 | **已修复**（改用 `ErrAISessionNotFound`/`ErrAISessionAccessDenied`） |
  | 8 | 中 | `MomentService.GetSources` 无班级校验且签名无 userID，可枚举任意班级来源（IDOR 残留） | service/moment.go:227-236 | **已修复**（补 userID 入参 + `EnsureClassMember`，handler 同步） |
  | 9 | 中 | AI 会话归属 `sess.UserID != nil && *sess.UserID != userID`，user_id 为 NULL 时校验被跳过 | service/ai_chat.go:71/225 | **已修复**（NULL 视为越权，改 `sess.UserID == nil || *sess.UserID != userID`） |
  | 10 | 低 | 文明中外全对却反馈"回答错误！"（feedbacks 为空被填错误文案） | service/wenmingzhongwai_submit.go:263-265 | 待办 |
  | 11 | 低 | 百度三 SDK HTTP 辅助层（OCR post/STT do/TTS post）未校验 HTTP 状态码（TTS 主链路已校验） | pkg/ocr、pkg/stt、pkg/tts | 待办 |
  | 12 | 低 | Anthropic 响应无 text 块时静默空成功（与 openai.go 不对称） | pkg/ai/anthropic.go:125-131 | 待办 |
  | 13 | 低 | `Chat` 会话 `nextSeq := len(allMsgs)+1` 非原子，并发请求消息乱序 | service/ai_chat.go:83-97 | 待办 |
  | 14 | 低 | `Moment.Create` 未加 `MomentLock`、无幂等，连点重复发布 | service/moment.go:73-96 | 待办 |
  | 15 | 低 | `SubmitQuickSelect` 插提交+更进度无事务，中途失败不一致 | service/cultural_style.go:664-695 | 待办 |
  | 16 | 低 | `UnlikeMoment` 无条件 DELETE 后无条件扣计数（依赖调用方预检查） | repository/moment.go:358-370 | 待办 |
  | 17 | 低 | `GradeTextSimple` 空参考答案时空输入判通过 | pkg/grading/grading.go:28 | 待办 |
  | 18 | 低 | `toAccountUser`/Login 解引用前未判 nil（潜在 panic 点） | service/account.go:66/107-133 | 待办 |
  | 19 | 低 | 初步感悟 Submit 无空输入校验，空作答入库计错 | service/initial_insight.go:318-375 | 待办 |
  | 20 | 低 | `GetDragSortSubmission` 缺 NodeType 校验（与其余 5 处口径不一致） | service/cultural_style.go:492 | 待办 |
  | 21 | 低 | `buildComprehensiveInput` 裸 blankID 作 key，跨题复用取错序号/答案 | service/initial_insight.go:717-763 | 待办 |
  | 22 | 低 | `truncate` 按字节截断产生非法 UTF-8 | service/ai_chat.go:257-262 | 待办 |
  | 23 | 低 | repository 三处 Create 对 NOT NULL JSONB 无 `'{}'/'[]'` 兜底 | repository/class.go、creation.go、moment.go | 待办 |
  | 24 | 低 | model 层 `json.Unmarshal == nil` 静默吞错、无日志 | model/learning.go（多处） | 待办 |

- **验证**: `go build ./...`、`go vet ./...` 通过（exit 0）；gofmt 复核——本轮改动文件中的真实格式问题已修复（errors.go var 块 `=` 对齐、write_thoughts.go 尾部多余空行）；其余 `gofmt -l` 标记文件（含 server.go）为仓库 CRLF 行尾的历史遗留，非本轮引入，未做整文件换行改动以避免污染 diff

### 2026-08-21 修复节点完成判定与作答计时问题（初步感悟未完成 + 耗时恒 0 秒）
- **现象**: ① 初步感悟（initial-insight）节点全部空位作答完成后，课堂详情页/综合评价仍可能显示"未完成节点内容"；② 部分节点提交记录的作答耗时一直显示 0 秒
- **根因一（完成判定）**: [finalizeInitialInsightProgress](file:///g:/zhonghuawenhua_backend/internal/service/initial_insight.go#L414-L437) 只回写 `error_count`，未设置 `node_progress.completed`，导致课堂详情页按 `node_progress.completed` 判断的节点恒为"未完成"
- **修复一**: 新增 [checkAllBlanksCompleted](file:///g:/zhonghuawenhua_backend/internal/service/initial_insight.go#L441-L470)（按 `qid:blankId` 空位维度统计 `is_completed` 记录），全部空位已完成（答对或达上限揭示答案）时置 `prog.Completed = true`，与写感想等其他节点对齐
- **根因二（计时）**: ① [ComputeSegmentDurations](file:///g:/zhonghuawenhua_backend/internal/service/service.go#L230-L317) 在乱序作答（先完成本段、上一段未完成）时理论基准晚于本段最早提交，耗时被负数截断成 0 秒；② `node_submissions.duration` 字段从未写入，各提交 DTO（`ToZhaoZhouQiaoResp`/`ToCulturalStyleDragSortResp` 等 `resp.Duration = s.Duration`）恒为 nil→前端显示 0 秒
- **修复二**: ① `ComputeSegmentDurations` 记录每段最早提交 `firstBySeg`，理论基准晚于本段最早提交时回退到节点进入时间（不可用则用本段最早提交），保证该段耗时不为 0；② [SubmitAsync](file:///g:/zhonghuawenhua_backend/internal/service/async_submit.go#L80-L87) 落库前通过新增的 [NodeSubmissionRepository.LatestSubmittedAt](file:///g:/zhonghuawenhua_backend/internal/repository/learning.go#L287-L301) 查询距上一次提交的秒数写入 `sub.Duration`（首条为 0）
- **验证**: `go build ./...`、`go vet ./...` 通过；单测 `TestComputeSegmentDurations_InOrder/OutOfOrder/MultiAttempt` 通过（乱序用例回归防 0 秒）；集成测试 `TestBuildNodeStats_WriteThoughts_Completed`、`TestBuildNodeStats_WenMingZhongWai_Completed` 通过（完成/积分判定正确）
- **报告路径复核**: `buildNodeStats`→`buildReportSegments` 对初步感悟已用复合键 `qid:blankId` 覆盖 `keyOf`（[report.go](file:///g:/zhonghuawenhua_backend/internal/service/report.go#L1441-L1469)），文明中外/文化采风/讲解优秀文化均有独立 completion builder，节点完成判定口径一致

### 2026-08-21 修复文明中外第一题填"热闹"恒判错（DB 缺 correct-answer 单元格）
- **现象**: 文明中外节点（node 21）第一题（r2，标准答案"热闹"）填入"热闹"仍被判错，走错误流程直至达上限自动填空
- **根因**: 标准答案按设计须从 `node_contents.params_json` 矩阵 `type=correct-answer` 单元格提取（见 [wenmingzhongwai.go](file:///g:/zhonghuawenhua_backend/internal/service/wenmingzhongwai.go#L42-L51)），但实际 DB 中 node 21 的 `params_json` 是**旧版本**——r2/r3/r4 只有 `text`+`input` 单元格、无 `correct-answer`，`correctAnswer()` 返回空串，[gradeRow](file:///g:/zhonghuawenhua_backend/internal/service/wenmingzhongwai_submit.go#L43-L52) 恒 false。seed.sql 已更新（含 correct-answer），但 DB 未重新执行种子脚本
- **修复**: 定向 UPDATE `node_contents` id=28（node_id=21）的 `params_json`，写入与 [seed.sql](file:///g:/zhonghuawenhua_backend/scripts/seed.sql#L224-L270) 一致的、含 correct-answer 单元格（r2=热闹 / r3=画上的街市可热闹了 / r4=完整中心句展开）的版本；新增脚本 [fix_wenmingzhongwai_node21_params.sql](file:///g:/zhonghuawenhua_backend/scripts/fix_wenmingzhongwai_node21_params.sql) 便于追溯重放
- **验证**: `UPDATE 1`；psql 复核 r2/r3/r4 的 `correct-answer` 均已写入；"热闹"（≤2 字）经 `GradeZhaoZhouQiaoText` 精确匹配判对，无需改代码
- **经验**: 类似"配置在 seed.sql 但生效异常"的问题，先对比 DB 实际 `params_json` 与 seed.sql 是否一致，再决定改数据还是改代码；本类标准答案一律来自 DB，避免硬编码

### 2026-08-20 遵循 API-first 删除「写感想」ending 手动路由及综合评价代码
- **背景**: `GET /api/v4/stu/class/:classId/node/:nodeId/write-thoughts/ending` 未定义在 `默认模块.openapi.json` 中，原在 [main.go](file:///g:/zhonghuawenhua_backend/cmd/server/main.go) 手动注册；按 API-first 约定"接口文档没有即接口不存在"，删除该路由及相关实现（其他模块 initial-insight/cultural-style/heritage-cultural 的 ending 均在 OpenAPI 定义，不受影响）
- **改动**:
  - `cmd/server/main.go`: 删除手动注册的 `write-thoughts/ending` 路由
  - `internal/handler/server.go`: 删除自定义 handler `GetWriteThoughtsEnding` 及不再使用的 `strconv` import
  - `internal/service/write_thoughts.go`: 删除 `GetEnding` + `buildComprehensiveInput` + `writeComprehensivePrompt`
  - `internal/service/write_thoughts_types.go`: 删除 `WriteThoughtsEndingResp` / `WriteThoughtsComprehensive{Record,Input,Result}`
- **保留**: `prompts/write-comprehensive.md` 提示词——报告路径 `reportComprehensivePromptKey`（report.go）仍会使用；`isKeyCompletion` / `ComputeSegmentDurations` 亦被 report.go、initial_insight.go 共用
- **验证**: `go build ./...`、`go vet ./...` 通过（exit 0）

### 2026-08-20 朋友圈写操作接入按用户并发锁（防连点/竞态）
- **背景**: 全量核对用户提交类接口锁覆盖后，朋友圈发布/点赞/评论被列为"无锁且非幂等"（学习类提交均已接入 `SubmitAsync` 按用户锁）
- **改动**:
  - [service.go](file:///g:/zhonghuawenhua_backend/internal/service/service.go#L304-L306) `Repositories` 新增独立 `MomentLock *synclock.UserSubmitLock` 并在 `NewServices` 初始化——**与学习提交 `SubmitLock` 相互独立**，学生在等 AI 批改时仍可正常点赞/评论，互不阻塞
  - [moment.go](file:///g:/zhonghuawenhua_backend/internal/service/moment.go) 四个写操作入口加锁（`TryAcquire` 失败即拒，`defer Release` 成对释放）：`Create`（发布）/ `Like`（点赞）/ `Unlike`（取消点赞）/ `Comment`（评论）；空内容等输入校验在加锁前完成，无效请求不占用锁
  - [errors.go](file:///g:/zhonghuawenhua_backend/internal/service/errors.go#L56) 新增 `ErrMomentOperationInProgress`（409 `MOMENT_OPERATION_IN_PROGRESS`，"操作过于频繁，请稍候再试"），与学习提交的 `SUBMIT_IN_PROGRESS` 区分
- **锁粒度说明**: 按用户粒度（复用 `synclock.UserSubmitLock`，立即拒绝不阻塞、空闲槽自动回收）；`moment_likes` 已有 `UNIQUE(moment_id, user_id)` 兜底，锁消除"检查-插入"竞态下并发点赞落 500 的问题；评论无唯一约束，锁直接防重复评论
- **验证**: `go build ./...`、`go vet ./...`、`gofmt` 通过（exit 0）

### 2026-08-19 修复讲解优秀文化提交轮次 off-by-one（第 2 次提交即触发"辅导+范文"）
- **现象**: 用户只提交 2 次就返回 `referenceAnswer`（辅导+范文），AI 侧显示"第 3 次/共 3 次"，但还能继续提交第 3 次，直到第 4 次才被拒绝（`SUBMIT_LIMIT_REACHED`）
- **根因**: `SubmitAsync` 会**先落库当前提交**（`is_processing=true`）再启动后台 goroutine 执行 `task.Process`；`processSubmit` 里 `countSubmissions + 1` 把当前这条已落库记录也数进去，导致 `submissionIndex` 恒多 1（第 2 次提交被误判为第 3 次、`isLast=true` 提前触发"辅导+范文"并完成节点）
- **修复**: [heritage_cultural.go](file:///g:/zhonghuawenhua_backend/internal/service/heritage_cultural.go#L168-L173) 去掉 `+1`——`countSubmissions` 已含当前记录，返回的即为本次提交轮次序号
- **验证**: `go build ./...`、`go vet ./...` 通过（exit 0）

### 2026-08-19 讲解优秀文化（heritage-cultural）全链路接入 AI 综合评价
- **目标**: 全量修改该模块——删除硬编码、使用数据库内数据、学生每次提交先调判定提示词接入 AI、判定后再调辅导模板、合格或第 3 次提交时 AI 基于最后一次提交内容异步综合评价，结束接口返回总评
- **删除硬编码**: 移除 `defaultHeritageCulturalParams()` 占位兜底；`loadHeritageCulturalParams` 一律从 `node_contents.params_json` 读取（含 `maxSubmissions`，seed.sql 已补 `"maxSubmissions": 3`）
- **双 AI 提交流程**（`service/heritage_cultural.go` + `heritage_cultural_types.go`）:
  - 每次提交异步执行：① `heritage-judge` 合格判定（`is_qualified`）→ ② `heritage-coaching` 生成辅导（输入含学生信息/提交轮次/历史交互/response_mode）
  - 未合格且未达上限：`response_mode=仅辅导`（只给引导不给范文）；合格或第 N 次（N=maxSubmissions）提交：`response_mode=辅导+范文`（反馈+优化范文 revised_example），节点完成
  - 复用 `SubmitAsync` 通用异步框架（按用户锁防并发、60s 整体超时、回写重试）
- **综合评价**（`service/report.go`）:
  - 新增 `buildHeritageCulturalNodeStats` 独立 builder：组装对齐 `heritage-comprehensive.md` 的专用数据包（`student_profile` + `learning_records` + `max_submissions`），完成=合格或达上限，积分=合格即 1 分
  - `reportComprehensiveInput` 新增 `HeritageCultural` 专用字段，`MarshalJSON` 只输出该数据包；`reportComprehensivePromptKey` 修正 key 为 `heritage-comprehensive`
  - 完成判定后经 `maybeTriggerNodeEvaluation` 异步生成评价缓存到 `node_progress.ai_evaluation_json`（与报告路径共用单飞锁）
- **结束接口** `GET /api/v4/stu/class/{classId}/node/{nodeId}/heritage-cultural/ending` 双分支:
  - 节点已完成：返回综合评价（studentFeedback/teacherEvaluation/teacherSuggestion/scoreGrade/scoreValue/scoreReason/cultureKeywords/errorKeywords），无缓存时加锁同步生成，生成中返回"AI 生成中"
  - 节点未完成：不触发总评，返回最后一次提交的辅导反馈（comment）
- **新增提示词**: `prompts/heritage-judge.md`、`prompts/heritage-coaching.md`、`prompts/heritage-comprehensive.md`
- **OpenAPI**: ending 响应 schema 补充综合评价字段（仅文档，gin 服务端不生成响应类型，`api.gen.go` 重生成无 diff）
- **验证**: `go build ./...`、`go vet ./...` 通过（exit 0）

### 2026-08-19 修复 GET /evaluation 接口 404（运行进程为旧二进制）
- **现象**: `GET /api/v1/stu/class/127/node/17/evaluation` 返回 404 `page not found`（15ms），契约校验失败（期望 200）
- **根因**: 路由/Handler/Service 代码均已就位（`api.gen.go` L5697 注册、`server.go` `GetAPIV1StuClassClassIDNodeNodeIDEvaluation`、`report.go` `GetNodeEvaluation`），`go build` 通过；但 **8080 端口运行的是 10:31 启动的旧进程 `tmp/main.exe`**（PID 25492），早于 16:33 重建与 16:34 的拆分 commit `c65ec6a`，二进制不含该路由 → gin 直接 404。磁盘 `tmp/main.exe`（16:33 重建）已含路由（字符串 `ROUTE_PRESENT` 验证通过）
- **修复**: 直接重启服务——停旧进程（PID 25492）→ 启动现有 `tmp/main.exe`（工作目录 `G:\zhonghuawenhua_backend`）→ curl 验证
- **验证**: `HTTP/1.1 200 OK`，`Content-Type: application/json; charset=utf-8`，返回 `evaluation`（comment/items/rating E/ratingColor/title）+ `isProcessing:false` 完整 JSON（终端显示乱码为 PowerShell 按 GBK 显示 UTF-8 所致，实际字节正确）
- **经验**: 代码改动后若测试仍 404，先查运行进程启动时间是否早于二进制重建/提交时间；重启后务必用 curl 实测确认新路由已加载

### 2026-08-19 修复修改文件编译错误 + M13 超时落地
- **现象**: 部分修改过的文件编译报错，`go build ./...` 失败（`internal/handler` 包）
- **修复**:
  - `internal/handler/server.go`：`GetAPIV1StuClassClassIDNodeNodeIDCreationWorkshopTaskIDTextcontent` 的 `taskID` 参数类型由 `string` 改为 `int`，与重生成的 `api.gen.go` 接口签名一致
  - `internal/handler/server.go`：补齐缺失的独立接口 handler `GetAPIV1StuClassClassIDNodeNodeIDEvaluation`（对应 OpenAPI 新增 `GET /evaluation`，调用已存在的 `ReportService.GetNodeEvaluation`，返回 `api.NodeEvaluation`）
  - `cmd/server/main.go`：**M13 落地**——`http.Server` 补 `ReadHeaderTimeout`(10s)、`ReadTimeout`(60s)、`WriteTimeout`(120s)、`IdleTimeout`(120s)，消除慢速攻击面
- **验证**: `go build ./...`、`go vet ./...` 通过（exit 0）

### 2026-08-19 防御性编程专项审查修复
- **目标/范围**: 按防御性编程要点逐项审查——输入校验、身份来源、越权防护（IDOR）、空指针、第三方失败处理、并发安全、错误信息泄露，覆盖 handler / service / repository / pkg 各层
- **审查结论**: 基础防御良好（参数化 SQL 防注入、提交事务 + 按用户锁防并发、`BizError` 统一错误码不泄露内部细节、AI 会话按用户隔离）；主要缺口为 **越权访问（IDOR）**、**空指针**、**第三方 HTTP 失败被静默当作成功** 三类
- **userID 来源说明**: 所有业务接口的 userID 一律取自 JWT，**请求路径/请求体中的任何 userID 均不被信任**——认证中间件 `middleware/auth.go` 解析 token 后 `c.Set("user_id", claims.UserID)`，handler 经 `currentUserID(c)` 读取并作为参数传给 service。service 层拿到的 userID 即"当前登录身份"，用于：① 归属校验（`GetSubmission` 比对提交记录 owner）；② 班级成员校验（`EnsureClassMember` 比对 `class_members` 表）。API Key 鉴权路径不设 user_id，业务接口将拒绝（返回 401）
- **H1 IDOR 修复** (`internal/service/*` 6 处 `GetSubmission`): 按 submitId 查询提交时增加 `sub.UserID == userID` 归属校验，越权返回 `ErrQuestionNotFound`（不泄露记录是否存在），涉及 write_thoughts / zhaozhouqiao / initial_insight / heritage_cultural / wenmingzhongwai / cultural_style(drag-sort)
- **H2 班级成员校验** (`internal/service/service.go` 新增 `Repositories.EnsureClassMember`，`errors.go` 新增 `ErrClassMemberNotFound` 403): 覆盖全部 class-scoped 方法（moment 6 处、report 2 处、class_detail 1 处、initial_insight 4 处、write_thoughts 4 处、cultural_style 7 处、heritage_cultural 3 处、zhaozhouqiao/ wenmingzhongwai state+submit 各 1 处），防止非本班用户访问/写入；`HeritageCultural.GetEnding` 补充 userID 入参（handler 同步）
- **H3 空指针防御** (`internal/service/moment.go` `toUserBaseInfo`): `u == nil` 时返回空 `UserBaseInfo`，避免评论/点赞作者被删后 panic
- **H4 第三方客户端加固** (`pkg/ai/anthropic.go`): 补 HTTP 状态码校验（4xx/5xx 不再静默当作成功）、BaseURL 去尾斜杠防双斜杠（openai.go 已具备同款逻辑）
- **有意保留**: 纯内容端点 `HeritageCultural.GetParams` / `CulturalStyle.GetDragSortParams`（仅返回跨班级共享的节点内容、无 userID 无副作用）不强制班级校验
- **验证**: `go build ./...`、`go vet ./...` 通过（exit 0）
- **本轮（2026-08-19）已落地修复（下述问题清单标"已修复"）**: H1/H2/H3/H4（越权、班级校验、空指针、AI 客户端加固）+ M3/M5/M6/M7/M8/M9/M10/M11(整体超时+回写重试)/M14，均已实现并 `go build`/`go vet` 通过
- **仍待办（问题清单标"待办"）**: M12 朋友圈列表 N+1 且无分页；M11 子项——进程重启后 stale `is_processing=true` 清理
- **问题清单（防御性编程视角，覆盖 2026-08-18 最佳实践审查 + 2026-08-19 专项复核逐条确认）**——含问题、根因、当前修复状态；状态 = **已修复**（代码已落地并验证）/ **待办**（未处理，供后续修复）：

  | # | 级别 | 问题 | 位置 | 根因 / 影响 | 状态与修复落地 |
  | - | - | - | - | - | - |
  | M3 | 高 | 初步感悟综合评价正确率恒 0 | `service/initial_insight.go` `buildComprehensiveInput` | `lastCorrect` 写入用复合键 `qid:blankID`，统计时却用裸 `blankID` 读取，键不匹配致 `correctBlanks`/`accuracyRate` 恒 0 | **已修复**：读取改用 `q.ID+":"+c.Blank.ID` |
  | M5 | 高 | httpresp.toMap JSON 往返吞错 + int64 精度 + 200 假成功 | `pkg/httpresp/httpresp.go` `toMap` | data 先 `json.Marshal` 再 `Unmarshal`：① 序列化错误被吞成 `{"error":...}` 仍返回 200；② int64（ID/时间戳）经 JSON 往返变 float64 精度丢失 | **已修复**：`toMap` 返回 error、`dec.UseNumber()` 保精度、`OK` 失败返回 500 杜绝 200 假成功 |
  | M7 | 高 | 空 JWT 密钥无防护 + config 无校验 | `cmd/server/main.go` | HS256 允许空密钥，secret 为空时任何人可伪造 token 绕过鉴权 | **已修复**：启动校验 `cfg.JWT.Secret` 非空（空则 Fatal）+ `expire_hours>0` |
  | M8 | 高 | wenmingzhongwai rowStates nil map 写入 panic | `service/wenmingzhongwai_submit.go` `gradeSubmission` | `rowStates` 在 `ListByUserNode` 出错时返回 nil，`states[rowID]=st` 向 nil map 赋值 panic（被 recover 掩盖为"处理失败"） | **已修复**：出错返回非 nil 空 map |
  | M9 | 高 | 草稿读-改-写回写覆盖并发更新（丢失更新） | `service/draft.go` `saveDraftPartition` | 读整行→内存改 `draft_json`→整行回写，覆盖并发写入的 `attempt_count`/`ai_evaluation_json` 等字段 | **已修复**：新增 `NodeProgressRepository.SetDraftPartition`，SQL 层 `jsonb_set` 仅合并 `draft_json` 列 |
  | M11 | 中 | 异步提交无整体超时/重试，is_processing 卡死 | `service/async_submit.go` `SubmitAsync` | `task.Process`（内部 AI 调用）无整体超时；回写失败仅 log 返回，`is_processing` 永久 true | **已修复**：`Process` 加 60s 整体超时（`submitProcessTimeout`）；回写用 `updateSubmissionWithRetry` 自动重试一次；**待办**：进程重启后 stale `is_processing=true` 清理（启动任务重置超时提交） |
  | M13 | 中 | http.Server 无任何超时 | `cmd/server/main.go` | 未设置 `ReadTimeout/ReadHeaderTimeout/WriteTimeout/IdleTimeout`，存在慢速攻击面 | **已修复**（2026-08-19）：设置 `ReadHeaderTimeout`(10s)、`ReadTimeout`(60s)、`WriteTimeout`(120s)、`IdleTimeout`(120s) |
  | M14 | 中 | 上传无大小限制 + OCR/STT 全量读入内存 | `handler/upload.go` + `service/upload.go` | `Upload` 用 `io.Copy` 无大小上限（可写满磁盘）；OCR/STT 用 `os.ReadFile` 全量读入内存 | **已修复**：handler 用 `http.MaxBytesReader` 限请求体 50MB（`maxUploadSize`）；service 新增 `ErrFileTooLarge` + `maxUploadFileSize`(50MB)/`maxOCRFileSize`/`maxSTTFileSize`(各 10MB) 前置 `os.Stat` 大小检查 |
  | M6 | 低 | synclock.Release 阻塞式 drain | `pkg/synclock/synclock.go` | `Release` 在持有 `l.mu` 时 `<-ch` 阻塞等待；未成对调用将永久阻塞并持锁，所有用户提交全局死锁 | **已修复**：`<-ch` 改非阻塞 `select{case <-ch: default:}`，保持幂等，槽空则删除 |
  | M10 | 低 | AI 会话空 content 触发真实 LLM 调用 | `service/ai_chat.go` `Chat` | `req.Content` 为空仍保存 user 消息并调 LLM（真实调用 + 空消息垃圾数据） | **已修复**：入口校验 `strings.TrimSpace(req.Content)!=""`，否则返回 `ErrEmptyInput` |
  | M12 | 低 | 朋友圈列表 N+1 且无分页 | `service/moment.go` `List`+`buildMomentDTO` | 每条 moment 分别查作者/图片/点赞/评论 + 逐用户查询（O(moments×(5+likes+comments))）；`ListAllByClass` 全量无分页 | **待办**：`MomentRepository.ListByClass(limit, offset)` 已具备分页能力，service 层改用分页参数 + 按 `moment_id IN` 批量查用户/点赞/评论消除 N+1 |
  | M4 | 高 | 朋友圈作者被删 toUserBaseInfo(nil) panic | `service/moment.go` | 评论/点赞作者被删后 `u==nil` 解引用 panic | **已修复**：H3 加 `u==nil` 判空返回空 `UserBaseInfo` |
### 2026-08-19 节点报告综合评价未生成完访问时的兜底同步生成
- **现象**: 节点报告接口有时在 AI 综合评价还没生成完就被访问，`buildReportEvaluation` 只返回"基础评价"（无 Items/Rating），前端综合评价模块无法正确显示
- **根因**: 提交流程 `maybeTriggerNodeEvaluation` 的异步预热另起 goroutine 生成评价，与报告读取存在竞态；报告路径此前"无缓存即返回基础评价"，本次不等待
- **修复** (`service/report.go` + `service/errors.go` + `service/service.go`):
  - 新增单飞锁 `nodeEvalFlight`（挂 `Repositories.NodeEvalLock`，按 nodeID+userID 维度），报告路径同步生成与提交预热共用，避免并发重复调 AI
  - `buildReportEvaluation` 改返回 error 并分分支：有缓存秒回完整评价；未完成返回 `EVALUATION_NOT_READY`（提示继续作答）；已完成无缓存时加锁同步生成（30s 超时 `evalGenerationTimeout`），成功返回完整评价，超时/失败返回 `EVALUATION_GENERATING`（前端显示"AI 生成中，稍后刷新"）；无法生成（无 LLM/无提示词/无记录）降级基础评价
  - `triggerNodeEvaluationAsync` 预热路径先抢单飞锁，抢不到（报告路径正在生成）则不重复调用
- **验证**: `go build ./...`、`go vet ./...` 通过（exit 0）

### 2026-08-19 修复文件下载接口 id 解析失败（400 invalid id）
- **现象**: `GET /api/utils/download/rs1787105855074987200?url_expired=30` 返回 400 `{"code":400,"data":null,"msg":null,"tip":"invalid id"}`
- **根因**: 上传返回的 `resource_id` 为字符串（`rs<纳秒时间戳>`），但下载 handler 用 `parseSessionID` 将其按十进制 int64 解析，`rs` 前缀解析失败直接 400
- **修复**: 下载接口与 OCR/STT 一致，改为支持数字数据库 id 或唯一 `resource_id` 两种方式定位资源——handler 不再 ParseInt，直接透传字符串；`service/upload.go` 的 `DownloadURL` 改用已有的 `resolveResource` 兜底 `ErrResourceNotFound`（`handler/upload.go` + `service/upload.go`）
- **验证**: `go build ./...` 通过（exit 0）

### 2026-08-18 修复初步感悟综合评价 score_value 解析失败（500）
- **现象**: `GET /api/v4/stu/class/{classId}/node/{nodeId}/initial-insight/ending` 返回 500，报错 `call comprehensive ai: parse ai result: json: cannot unmarshal number into Go struct field initialComprehensiveResult.score_value of type string`，耗时 15s
- **根因**: AI 返回的 `score_value` 为数字，而 `initialComprehensiveResult.ScoreValue` 声明为 `string`，解析失败
- **修复**: 与 `reportComprehensiveResult` 保持一致，将 `initialComprehensiveResult.ScoreValue` 改为 `flexibleString`（兼容 string/number/bool/null），`service/initial_insight_types.go`
- **验证**: `go build ./internal/service/` 通过；该字段仅用于解析，下游只用 `TeacherEvaluation` 生成评论，无其它影响

### 2026-08-18 全代码库最佳实践审查
- **目标**: 全面审查当前所有代码是否符合最佳实践（入口/基础设施、Service 层、Repository/Model、Handler、pkg 外部集成），关键问题经独立验证代理二次核实，无 false positive
- **审查结论**: 共发现 2 项 Critical（必现）、12 项 Major、若干 Minor。整体架构规范（事务边界正确、异步提交按用户锁成对释放、参数化 SQL、`%w` 错误包装），主要问题集中在：AI 会话唯一键、上传/OCR 链路、朋友圈与报告 N+1、异步提交健壮性、跨模块重复逻辑
- **修复 Critical — C1 AI 会话 Code 未赋值**: `AISessionRepository.Create` 直接插入空 Code，而 `ai_sessions.code` 有 UNIQUE 索引，第 2 个会话起必 500。仿照 `NodeSubmissionRepository.Create` 在 Create 中为空 Code 自动生成 `sess_` 前缀随机值（`repository/ai.go`）
- **修复 Critical — C2 OCR 读取路径错误**: `RecognizeOCR` 用 `filepath.Join(uploadDir, fileURL)`，而 `file_url` 是 `/storage/rs...` 的 URL 路径，拼接成 `upload/storage/...` 必不存在，OCR 必现失败。改用 `resolveUploadFilePath`（与 `TranscribeAudio` 一致）取文件名（`service/upload.go`）
- **验证**: `go build ./...`、`go vet ./...` 通过（exit 0）
- **遗留 Major（待后续处理）**：详见上方"问题清单与修复状态表"（M3/M5/M6/M7/M8/M9/M10/M11/M14 已于 2026-08-19 修复，M4 已随 H3 修复；**剩余待办：M12、M13、M11 stale 清理**）
- **遗留 Minor（代表性）**: 5 模块约 9 处错误计数 / 5 处 finalize*Progress / 6 处 GetSubmission 重复（建议抽公共 helper）；OCR/STT/TTS 三客户端 accessToken/apiError 逐字重复（建议抽 pkg/baidu）；report.go 1240 行超 1000 行规则；`ErrAISessionNotFound/ErrAISessionAccessDenied` 定义未使用，AI 会话错误一律 500；model/learning.go 多处 `json.Unmarshal == nil` 静默吞错；config.yaml 明文真实密钥；DSN 字符串拼接；JWT 未校验 Issuer/Audience

### 2026-08-18 报告模块全面检查与补全
- **目标**: 编写节点报告的全部代码，覆盖初步感悟/文化采风等已实现接口的节点；整体审查 `service/report.go` 两接口（`GetStuNodeReport` / `GetStuClassReport`）逻辑正确性
- **修复 1 — 文化采风节点永远无法判定完成**: `buildReportSegments` 原先把文化采风 `questions`（纯展示型题目，无提交接口、永远无完成记录）计入 segments，导致节点永远不 Finished。改为**仅统计交互点**（quick-select / drag-sort），展示型题目不再计入完成判定
- **修复 2 — 文化采风学习过程数据为空**: 新增 `buildCulturalStyleProcessData`，按交互点组装
  - 拖拽排序：复用 `CulturalStyle.GetDragSortParams/GetDragSortState`（含该交互点全部提交记录），映射 `ReportProcessData2DataBpRecord1`
  - 快速选择：从提交记录还原最后一次选择的选项文本，映射 `ReportProcessData2DataBpRecord0`
- **完成判定（各节点）**: 节点内所有题/空/交互点均有完成记录（答对 / 达上限揭示 / get-answer）才算 Finished；完成判定遍历全部提交（含 get-answer），不依赖 `node_progress.completed`
- **文明中外**: 独立矩阵统计 builder（`buildWenMingZhongWaiNodeStats`），行级重判正误、积分=最终答对行数、总用时=末次−首次提交、AI 输入用对齐 `wenmingzhongwai-comprehensive.md` 的专用数据包
- **AI 综合评价**: 节点完成后异步生成并缓存 `node_progress.ai_evaluation_json`（提交后 `maybeTriggerNodeEvaluation` 触发 + 报告接口 `triggerNodeEvaluationAsync` 兜底存量节点）；无缓存时先返回基于数据的简要评价并在后台生成一次
- 验证：`go build ./...`、`go vet ./...` 通过

### 2026-08-18 修复 gin 路由通配符冲突导致的启动 panic
- **根因**: gin 路由树同一位置不允许不同名字的通配符（`:taskId` vs `:textId`、`:classId` vs `:classid`），注册时 panic
- ① `textcontent` 路由路径参数 `{textId}`(integer) 统一为 `{taskId}`(string)，与同 `creation-workshop` 树其余 14 条路由一致
- ② 13 条路由路径参数小写 `{classid}`/`{nodeid}` 统一为 camelCase `{classId}`/`{nodeId}`（write-thoughts/initial-insight/cultural-style/zhaozhouqiao/wenmingzhongwai/heritage-cultural 的 params/state/submissions 接口）
- 修改 `默认模块.openapi.json` → `go tool oapi-codegen -config oapi-codegen.yaml` 重新生成 `api.gen.go` → 同步 `internal/handler/server.go`（`Classid`→`ClassID`、`Nodeid`→`NodeID`，含两个 `JSONRequestBody` 类型引用）
- URL 结构不变，前端无感知
- 验证：`go build ./...`、`go vet ./...` 通过；服务启动时路由注册成功不再 panic（8080 被已有实例占用，属正常）

### 2026-08-18 适配 OpenAPI 重生成（初步感悟反馈 + 受影响模块）
- `默认模块.openapi.json` 移除全部 `operationId`、路径参数改小写（`classId`→`classid`），重新生成 `api.gen.go`
- 方法名全部改为按路径命名（如 `GetInitialInsightState`→`GetAPIV1StuClassClassIDNodeNodeIDInitialInsightState`），handler 层（`server.go`+`upload.go`）已同步重命名
- 初步感悟 `InitialImpressionsSubmitResult` 新增 `feedback`/`duration` 字段；`GetState` 的 `content` 改为 union 类型（`InitialInsightState_Questions_Content_Item`，text/blank 用 `From...0/From...1` 构造），blank 状态枚举改 `api.InitialInsightStateQuestionsContent1State`
- **feedback 链路确认完成**：答错时 `processInitialInsightAnswer` 调 `generateBlankFeedback`（未达上限 `feedback_mode=guidance` 引导重填 / 达上限 `explanation` 结合答案讲解并 `Revealed` 填答案），AI 文本写入 `node_submissions.feedback`，前端轮询 `GetSubmission` 时由 `ToInitialInsightResp` 透出
- 报告模块 `ReportEvaluation`→本地 `reportEvaluationDTO`、`ReportEvaluationRating`→`api.ReportDetailsDataEvaluationRating`
- 同步修复 `pkg/ai/openai.go` 推理模型 content 为空时回退 `ReasoningContent`；`pkg/synclock/synclock_test.go` 并发错误改用 channel 传递（`go vet` 通过）
- 验证：`go build ./...`、`go vet ./...` 全部通过

***

## 一、项目概况

**技术栈**: Go 1.24 + Gin + pgx/v5 + scany + koanf + zap + golang-jwt + golang-migrate + air
**架构**: handler → service → repository → model（三层架构）
**API 规范**: OpenAPI 3.0 → go tool oapi-codegen 生成 api.gen.go（71 个接口方法）

**全局约定**:

- 所有 submit 返回 submitId(string) 存 `code` 字段，`is_processing` 标识异步处理中
- `node_progress.draft_json` 以顶层模块 key 分区存草稿（如 `write-thoughts`、`initial-insight`）
- 所有需要用到ai的提交都需要异步，异步提交使用 `async_submit.go` 的 `SubmitAsync` 通用函数，只需注入 `Process` 和 `FinalizeProgress` 两个回调
- **异步提交按用户串行化**：`Repositories.SubmitLock`（`pkg/synclock/` 的 `UserSubmitLock`，基于 `map[userID]chan struct{}` 的独立提交槽）——同一用户同一时间只允许一个异步提交，被占用时 `SubmitAsync` 立即返回 `ErrSubmitInProgress`（业务码 `SUBMIT_IN_PROGRESS`，HTTP 409 经 respondError 统一映射为 400 并透出 Tip），不落库；不同用户各自独立槽位，互不影响。锁在后台 goroutine 结束时释放，落库失败提前返回时也会释放避免死锁
- 若 service 文件内结构体较多（>5 个），建议拆出 `*_types.go` 同包文件存放，如 `write_thoughts_types.go`、`initial_insight_types.go`
- 通用文本批改工具 → `pkg/grading/`，通用按用户并发锁 → `pkg/synclock/`，静态资源 → `assets/`

***

## 二、页面/模块完成情况

### ✅ 账号模块 — 完成

**文件**: `service/account.go`

| 接口             | 说明                                         |
| -------------- | ------------------------------------------ |
| AccountsLogin  | form-data 登录，支持用户名/学号 + bcrypt 密码校验，返回 JWT |
| AccountsStatus | 从 JWT 取 user\_id 查询登录状态                    |

### ✅ AI 对话 — 完成

**文件**: `service/ai_chat.go`

| 接口                  | 说明                               |
| ------------------- | -------------------------------- |
| ChatWithAi          | 非流式对话，通过 `resolvePrompt` 加载节点提示词 |
| StreamChatWithAi    | 降级为非流式，复用 ChatWithAi             |
| GetAiSessionContext | 返回会话历史消息                         |
| CreateAiSession     | 创建新会话                            |

### ✅ 朋友圈 — 完成

**文件**: `service/moment.go`

9 个接口全部实现，事务保证点赞/评论计数一致性，聚合作者/图片/点赞/评论/likedByMe。

| 接口                                  | 说明                   |
| ----------------------------------- | -------------------- |
| GetMoments                          | 列表聚合                 |
| PostMoments                         | 事务发布 + 关联图片          |
| GetMomentDetail                     | 单条详情                 |
| LikeMoment / UnLikeMoment           | 事务：+1/-1             |
| CommentMoment / DeleteMomentComment | 事务：+1/-1，校验班级归属      |
| GetMomentStats                      | 实时聚合：发布数/获赞/获评/点赞/评论 |
| GetMomentSources                    | 班级内 source 去重        |

### ✅ 写感想 — 完成

**文件**: `service/write_thoughts.go` + `service/write_thoughts_types.go`

完整链路：GetParams → GetState → Submit(异步) → GetSubmission(轮询) → GetEnding(综合评价)。

- Submit 异步设计：立即返回 submitId，后台 goroutine 执行 AI 评分，前端轮询
- 支持 submit/get-answer 双模式，AI 未配置降级为简单文本匹配
- **每次提交均回写 AI 反馈**（通过给鼓励、失败给讲解）；**参考答案优先由 AI 生成**，AI 未返回时兜底取数据库 `params_json.reference_answer`（见下一条；get-answer 分支同样兜底）
- 第 3 次失败（maxErrors=3）揭示参考答案并标记完成：**参考答案优先取 AI 生成结果；AI 未返回（返回空或未配置 LLM）时兜底使用 params_json 配置的 `reference_answer`**，保证达上限必定展示答案
- **后端兜底拦截**：该题历史失败次数达上限（>3 次）后的 submit 直接拒绝（`ErrSubmitLimitReached`），不调用 AI、不写入提交记录
- 综合评价 `GetEnding` 按 write-comprehensive.md 提示词生成
- 结构体已提取到 `write_thoughts_types.go`（10 个结构体）

> **写法提示**: 若 service 文件结构体较多，可建 `*_types.go` 同包存放，如 `write_thoughts_types.go`。

### ✅ 初步感悟 — 完成

**文件**: `service/initial_insight.go` + `service/initial_insight_types.go`

完整链路：GetParams → GetState(blank 实时状态) → Submit(按 blankId 提交，**异步** `SubmitAsync` + 按用户锁) → GetSubmission → GetEnding(AI 综合评价)。

- blank 状态实时计算：PENDING/CORRECT/WRONG/RETRYING/UNANSWERED
- 进入节点时间记入 draft\_json `initial-insight.entered_at`
- 结构体已提取到 `initial_insight_types.go`（11 个结构体）
- 错误辅导：答错时调用 AI（`prompts/initial_insight.md`）生成反馈文本写入 `feedback`；未达上限用 `feedback_mode=guidance`（引导重填），达上限用 `explanation`（结合答案讲解）并 `Revealed` 直接填入答案
- 错误上限：仅本页面（初步感悟节点 seed `maxErrors=2`）第 2 次错误后直接填入答案；全局 `getMaxErrors` 默认仍为 3，不影响其它节点；输入框变红由前端按 `WRONG` 状态渲染
- AI 降级：`llm` 未配置或调用失败时返回空反馈、不阻断提交，仍按 `GradeTextSimple` 判定对错
- 幂等保护：正常的作答（含答错，第 1、2 次）都会入库；仅当该空已解决（已答对 或 已达错误上限、答案已自动填入）之后的重复提交才不入库，也跳过 `node_progress` 更新。注意 `errCount` 是本次提交之前的错误次数，触发填入答案的那次答错仍正常入库

### ✅ 赵州桥 — 完成

**文件**: `service/zhaozhouqiao.go`（结构体 + getparams/getstate）、`service/zhaozhouqiao_submit.go`（ZhaoZhouQiaoService 提交批改，复用 `SubmitAsync` + 按用户锁）

| 接口                        | 状态 | 说明                   |
| ------------------------- | -- | -------------------- |
| GetZhaoZhouQiaoParams     | ✅  | 静态配置返回               |
| GetZhaoZhouQiaoState      | ✅  | 提交记录聚合               |
| SubmitZhaoZhouQiao        | ✅  | 异步批改，返回 submitId 后轮询 |
| GetZhaoZhouQiaoSubmission | ✅  | 查询单条提交结果             |

- **填空题**（voiceOnly=false）：本地宽松判定（`GradeZhaoZhouQiaoText` 允许程度修饰词，如“很美观”“非常美观”均判对，通过后仍只显示标准答案“美观”）；失败时按失败次数调用 AI 切换辅导策略（方向指引→方法引导→答案讲解，`prompts/zhaozhouqiao.md`），达上限（maxErrors=3）揭示卡片配置的标准答案并标记完成
- **朗读题**（voiceOnly=true）：只提供两次朗读录音机会（代码强制 `maxReadingErrors=2`）；语音转写可能存在识别错误，不做精准匹配，转写文本去除标点达到最小有效字数（`minReadingRunes=5`）即判完成；失败按次数调用 AI 切换策略（方向指引→最终讲解），两次机会用尽后任务结束
- **get-answer**：揭示卡片配置的标准答案（由系统展示，不经过 AI）并标记完成
- 批改判定以本地为准，AI 仅生成反馈；AI 未配置/调用失败时优雅降级为空反馈，不阻断提交回写
- 后端兜底拦截：该卡历史失败次数达上限后的 submit 直接拒绝（`ErrSubmitLimitReached`）

### ✅ 文明中外 — 完成

**文件**: `service/wenmingzhongwai.go`（结构体 + 辅助方法）、`service/wenmingzhongwai_types.go`（请求/回写结构体）、`service/wenmingzhongwai_submit.go`（WenMingZhongWaiService 提交批改 + AI 辅导 + 综合评价触发）、`service/report.go`（矩阵报告统计 builder）

| 接口                           | 状态 | 说明                   |
| ---------------------------- | -- | -------------------- |
| GetWenMingZhongWaiParams     | ✅  | 静态配置返回（含 Matrix 矩阵） |
| GetWenMingZhongWaiState      | ✅  | 提交记录聚合（submitLogs）    |
| SubmitWenMingZhongWai        | ✅  | 异步批改，返回 submitId 后轮询 |
| GetWenMingZhongWaiSubmission | ✅  | 查询单条提交结果             |

- **矩阵填空批改**：一次 submit 提交多行答案（`answers: map[rowId]answer`），逐行判定；短答案行（≤2 字，如“热闹”）允许程度修饰词（很/非常/十分/特别），复用 `GradeZhaoZhouQiaoText`；标准答案从矩阵 `type=correct-answer` 单元格（`params_json`，seed.sql 已配置）提取，无硬编码
- **错误三级递进辅导**（`prompts/wenmingzhongwai.md`）：第 1 次错误 = `direction_guide`（方向指引，数字人讲解，提示重答）；第 2 次错误 = `method_guide`（方法引导，弹出选项询问是否提供答案）；第 3 次错误（达上限 maxErrors=3） = `answer_explain`（直接帮学生填答案并标记该行完成）
- **节点完成判定**：所有 input 行均已解决（答对 / 达上限自动填入 / get-answer 揭示）；完成判定直接用含本次提交的本地行状态，避免当前提交未落库导致的漏判
- **get-answer**：一次性揭示全部行参考答案并标记完成（经系统展示，不经过 AI）
- 批改判定以本地为准，AI 仅生成反馈（`feedback_text`）；AI 未配置/调用失败时优雅降级为空反馈，不阻断提交回写；`result_json` 回写 `errors / referenceAnswer / ending / rowStates` 结构化内容（DTO 经 `ToWenMingZhongWaiResp` 还原）
- **综合评价**：节点全部完成后由 `maybeTriggerNodeEvaluation` 触发异步生成，使用 `prompts/wenmingzhongwai-comprehensive.md` 提示词，输入为对齐提示词的独立数据包（`task_context.slot_definitions` + `student_data.records`，行级重判正误、耗时、正确率），缓存到 `node_progress.ai_evaluation_json`
- 后端兜底：节点已全部完成后的 submit 直接拒绝（`ErrSubmitLimitReached`），空作答拒绝（`EMPTY_INPUT`）
- **提交 key 校验**（修复）：`gradeSubmission` 先遍历提交的 `answers`，凡 key 不是矩阵有效 input 行（rowId 不匹配）的，生成错误项并标记该行为 failed，避免未知 key 被静默跳过而误判为"全部正确"（如 `col1/col2/col3` 混传导致 "回答正确，你真棒！" 假阳性）

### ✅ 文化采风 — 部分完成（service 层完整，handler 已接入）

**文件**: `service/cultural_style.go`

**service 层 10 个接口全部实现**，handler 已全部接入，可直接使用。

| 接口                         | 说明                                                                                  |
| -------------------------- | ----------------------------------------------------------------------------------- |
| GetCulturalStyleParams     | 获取页面固定信息 + 记录进入时间                                                                   |
| GetCulturalStyleState      | 题目/交互点状态 + submitLogs                                                               |
| SyncCulturalStyleTime      | 同步学习进度时间                                                                            |
| GetCulturalStyleEnding     | 获取结束语                                                                               |
| GetStyleDragSortParams     | 获取拖拽排序交互点参数                                                                         |
| GetStyleDragSortState      | 拖拽排序状态                                                                              |
| SubmitStyleDragSort        | 提交拖拽排序答案（**异步**，复用 `SubmitAsync` + 按用户锁，返回 submitId 后轮询 GetStyleDragSortSubmission） |
| GetStyleDragSortSubmission | 查询拖拽排序提交结果                                                                          |
| GetStyleQuickSelectParams  | 获取快速选择交互点参数                                                                         |
| SubmitStyleQuickSelect     | 提交快速选择答案（**同步**，同样占用按用户提交槽，被占用返回 `SUBMIT_IN_PROGRESS`）                              |

> **注意**: 结构体定义在 `cultural_style.go` 中，暂未拆出，若后续新增可考虑提取到 `cultural_style_types.go`。

### ✅ 讲解优秀文化 — 完成（全链路 AI 接入，无硬编码）

**文件**: `service/heritage_cultural.go` + `service/heritage_cultural_types.go`（AI 输入输出结构 + ending DTO）+ `service/report.go`（综合评价独立 builder）

| 接口                            | 说明                                                                  |
| ----------------------------- | ------------------------------------------------------------------- |
| GetHeritageCulturalParams     | 获取页面内容（标题/视频/对话气泡，全部来自数据库 `node_contents.params_json`） |
| GetHeritageCulturalState      | 提交历史记录                                                              |
| SubmitHeritageCultural        | 提交讲解文本（**异步**，复用 `SubmitAsync` + 按用户锁；双 AI：判定→辅导） |
| GetHeritageCulturalSubmission | 轮询评测结果                                                              |
| GetHeritageCulturalEnding     | 返回总评信息（完成→综合评价；未完成→最后一次提交的辅导反馈）                              |

- **无硬编码**：内容（string/introVideo/introBubbles）与 `maxSubmissions` 均从数据库 `params_json` 读取（seed.sql 已配置 `"maxSubmissions": 3`），无内置占位兜底
- **双 AI 提交流程**：每次提交后台异步先调 `heritage-judge` 判定是否合格，再调 `heritage-coaching` 生成辅导；未合格且未达上限=`仅辅导`（核心词思维辅导框架引导，不给范文），合格或达上限=`辅导+范文`（反馈 + 优化范文 revised_example）并完成节点
- **综合评价**：节点完成（合格或达 maxSubmissions 次提交）后由 `maybeTriggerNodeEvaluation` 异步触发，使用 `heritage-comprehensive.md`，输入为对齐提示词的独立数据包（`student_profile` + `learning_records` + `max_submissions`），输出缓存到 `ai_evaluation_json`（学伴反馈/教师评价/建议/评分 A-E/文化核心词/错误类型关键词）
- **后端兜底**：节点已完成后的 submit 直接拒绝（`ErrSubmitLimitReached`），空作答拒绝（`EMPTY_INPUT`）；AI 未配置/提示词缺失时优雅降级（判定视为不合格、辅导返回引导话术），不阻断提交

### ⬜ 创作工坊 — 未实现（16 个接口）

**handler 全部返回 501**

| 子模块         | 接口数 | 说明               |
| ----------- | --- | ---------------- |
| generation  | 6   | 参数/状态/提交/发布/结果查询 |
| poemscripts | 5   | 参数/状态/提交/发布/结果查询 |
| share       | 2   | 提交/结果查询          |
| express     | 1   | 提交               |
| 通用          | 2   | 创作工坊任务提交、文本内容获取  |

### ✅ 报告 — 完成（2 个接口）

**文件**: `service/report.go`

| 接口                | 说明                                                                                          |
| ----------------- | ------------------------------------------------------------------------------------------- |
| GetStuNodeReport  | 节点级报告：AI 综合评价（各节点 comprehensive 提示词）+ 每题/总用时（含正确与错误）+ 错误回答 AI 讲解（Feedback）+ 积分（每题答对=1 分） |
| GetStuClassReport | 班级级报告：各学习节点状态 + 汇总指标（总积分/最近答对积分 PointsChange/完成节点学习用时/完成数/获赞/获评）                       |

- AI 综合评价统一按节点 key 读取 `prompts/*.comprehensive.md`（如 write-comprehensive / initial-comprehensive / wenmingzhongwai-comprehensive / heritage-comprehensive）；尚未提供提示词的节点（cultural-style-comprehensive / zhaozhouqiao-comprehensive / creation-comprehensive）自动降级为基于数据的简要评价，补齐提示词后即自动接入
- AI 综合评价在节点全部完成后由后台 goroutine 异步生成并缓存到 `node_progress.ai_evaluation_json`（迁移 `000002` 新增列；`SubmitAsync` 在 FinalizeProgress 后经 `maybeTriggerNodeEvaluation` 触发，报告接口经 `triggerNodeEvaluationAsync` 兜底触发存量已完成节点）；报告接口优先秒回缓存结果，无缓存时先返回基于数据的简要评价并在后台生成一次，下次请求即可读到完整 AI 评价，接口不等待 LLM
- 用时统计复用 `ComputeSegmentDurations`（分段基准累计耗时），覆盖每题/每空的全部 submit 记录（含答错），每条记录含学生答案、正确答案、正误、错误时的 AI 讲解（Feedback）与耗时，作为 AI 评价输入
- 总用时 = 末条 submit − 进入节点时间；进入时间未记录（赵州桥/文明中外/文化采风等）或与首次提交跨天（多日前进入、本次才作答）时，截断回退为最早提交时间，避免跨会话把总用时算成数天
- 节点完成判定（2026-08-18 修正）：节点内所有题/空/交互点均有完成记录（答对 / 达上限揭示 / get-answer）才算 Finished，实时计算不依赖 `node_progress.completed`；文化采风仅统计交互点（questions 为展示型题目不参与完成判定）
- 积分：每道题最终答对得 1 分（同一题多次提交只计一次）；`PointsChange` 取最后一次答对获得的积分（当前每题=1 分）
- 学习过程数据（Details.Process.Data）复用各子服务 GetState 组装，映射到 `ReportProcessData` 联合体；文化采风按交互点组装（拖拽排序 `DragSortParams+State` / 快速选择还原最后选项文本）；创作工坊暂返回空记录列表，后续按需补齐

### ✅ 课堂详情 — 完成

**文件**: `service/class_detail.go`（`NodeStateService.GetClassDetail`）

| 接口                                                          | 说明                                                                                              |
| ----------------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| GetAPIV1StuClassClassID (`GET /api/v1/stu/class/{classId}`) | 获取课堂详细信息：course=4（课程包固定值）+ sections（根节点的子节点）+ 各节点（key/title/isEnabled/isCompleted/allowReentry） |

> 结构约定：class\_nodes 为树形，根节点（parent\_id IS NULL）的子节点视为章节（section），章节的子节点视为学习节点（node）；isCompleted 取自 node\_progress.completed。

### ✅ 工具 — 部分实现（5 个接口）

**文件**: `service/upload.go` + `handler/upload.go` + `internal/config/config.go`

| 接口           | 状态 | 说明                                                                                                                                                                                                                                                   |
| ------------ | -- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| UploadFile   | ✅  | multipart 上传（file/media\_type/resource\_source 等字段），校验 media\_type ∈ image/audio/video，保存到 `server.upload_dir`（默认 `upload/`，环境变量 `SERVER__UPLOAD_DIR` 覆盖），文件名用唯一 `resource_id`（`rs<纳秒时间戳>`）+ 原后缀，`resources.file_url` 存相对 baseUrl 的访问路径 `/upload/<文件名>`，登记 `resources` 表，返回 `UtilsUploadResp`（id 为数据库自增 id） |
| DownloadFile | ✅  | 按 id 查询 `resources`（未软删除），支持数字数据库 id 或唯一 `resource_id`（如 `rs<时间戳>`），返回 `resources.file_url` 相对访问 url（如 `/static/videos/xxx.mp4` 或 `/upload/rs...mp4`，前端拼 baseUrl） |
| OcrRecognize | ✅  | 百度智能云 OCR（`pkg/ocr`，general\_basic 通用文字识别），resourceId 支持数字 id 或唯一 resource\_id，读取本地图片 base64 直传，token 带缓存+失效自动刷新；配置 `ocr.api_key`/`ocr.secret_key`（环境变量 `OCR__API_KEY`/`OCR__SECRET_KEY`）                                                            |
| GetTranscription | ✅  | 百度智能云语音识别（`pkg/stt`，短语音识别 server\_api），resourceId 支持数字 id 或唯一 resource\_id，读取本地音频 base64 直传，格式由资源后缀确定（百度仅支持 pcm/wav/amr/m4a，aac→m4a；mp3/webm/ogg 返回 `UNSUPPORTED_AUDIO_FORMAT`），同步返回转写文本，token 带缓存+失效自动刷新；配置 `stt.api_key`/`stt.secret_key`（环境变量 `STT__API_KEY`/`STT__SECRET_KEY`） |
| PostAPIUtilsTts | ✅  | 百度智能云语音合成（`pkg/tts`，text2audio），query 传 `text`（非空、≤1024 字节），合成 mp3（per=1、spd/pit/vol=5、aue=3）保存到 `upload_dir` 并登记 `resources`（`tts<纳秒时间戳>.mp3`），返回相对访问 url（`/upload/tts<...>.mp3`）；token 带缓存+失效自动刷新，失败时区分 JSON 业务错误；配置 `tts.api_key`/`tts.secret_key`（环境变量 `TTS__API_KEY`/`TTS__SECRET_KEY`） |

> 📌 2026-08-14 排错记录：OCR 接口曾持续报 500 `baidu token failed: invalid_client`。排查确认代码链路与密钥有效性均正常，根因是 `config.yaml` 中 `ocr.secret_key` 被误改为无效值 `Lt1gXV...`（直接调用百度 token 接口返回 401）。已将其修正为有效 Secret Key 并重启验证通过（200，识别正常）。

### ⬜ 其他 — 未实现（1 个接口）

| 接口             | 说明     |
| -------------- | ------ |
| EnterClassNode | 进入节点事件 |

***

## 三、实现进度总览

| 状态      | 数量     | 分布                                                                                    |
| ------- | ------ | ------------------------------------------------------------------------------------- |
| ✅ 全部完成  | 33     | 账号 2 + AI 4 + 朋友圈 9 + 写感想 5 + 初步感悟 5 + 赵州桥 4 + 文明中外 4 + 报告 2                           |
| ⚠️ 部分完成 | 10     | 文化采风 10（service 完整，handler 已接入）                                                       |
| ✅ 完成    | 5      | 讲解优秀文化 5                                                                              |
| ✅ 完成    | 5      | 工具 5（UploadFile / DownloadFile / OcrRecognize / GetTranscription / PostAPIUtilsTts）        |
| ⬜ 未实现   | 17     | 创作工坊 16 + 其他 1                        |
| **合计**  | **72** | <br />                                                                                |

***

## 四、开发提示（常见模式）

### 1. 项目结构

```
g:\zhonghuawenhua_backend\
├── cmd/server/         # 入口
├── internal/           # 业务代码（不可外部导入）
│   ├── api/            # oapi-codegen 生成
│   ├── config/         # 配置
│   ├── handler/        # HTTP 处理器
│   ├── middleware/      # 中间件
│   ├── model/          # 数据模型
│   ├── repository/     # 数据访问
│   └── service/        # 业务逻辑
├── pkg/                # 通用工具（可复用）
│   ├── ai/             # AI 客户端（OpenAI/Anthropic）
│   ├── db/             # 数据库连接与迁移
│   ├── grading/        # 文本批改工具（GradeTextSimple 等）
│   ├── httpresp/       # HTTP 统一响应
│   ├── jwt/            # JWT 认证
│   ├── logger/         # 日志
│   ├── ocr/            # 百度 OCR
│   ├── stt/            # 百度语音识别
│   ├── tts/            # 百度语音合成
│   └── synclock/       # 按用户并发锁（UserSubmitLock）
├── assets/             # 静态资源文件
├── migrations/         # 数据库迁移脚本
├── prompts/            # AI 提示词模板
├── scripts/            # 种子数据
└── config.yaml         # 配置文件
```

### 2. 异步提交通用模式

```go
// 在 service 中复用 async_submit.go 的 SubmitAsync
submitID, err := SubmitAsync(ctx, repos, AsyncSubmitTask{
    ClassID:     classID,
    NodeID:      nodeID,
    UserID:      userID,
    NodeType:    model.NodeTypeXXX,
    QuestionID:  questionID,
    SubmitType:  "submit",
    PayloadJSON: payload,
    Process: func(ctx context.Context) (*AsyncSubmitResult, error) {
        // 执行 AI 评分/批改逻辑
        return &AsyncSubmitResult{IsPassed: true, ...}, nil
    },
    FinalizeProgress: func(ctx context.Context, sub *model.NodeSubmission) error {
        // 更新进度聚合字段
        return nil
    },
})
```

### 3. 结构体分离

当 service 文件包含较多结构体定义（>5 个），建议拆出 `*_types.go` 同包文件：

```
service/
├── xxx.go              # 业务逻辑（结构体方法、服务方法）
├── xxx_types.go        # 结构体定义（params、DTO、AI 输入/输出等）
```

### 4. 综合评价（getending）模式

各节点的 `GetEnding` 接口可复用 `service.ComputeSegmentDurations` 计算作答耗时，然后调用 AI 生成综合评价。

**作答时间计算规则（分段基准累计耗时）**，核心函数 `ComputeSegmentDurations(subs, entryTime, segments, keyOf, isKey)`：

- 仅统计 `SubmitType == "submit"` 记录（get-answer 不计入），按 `submitted_at` 升序；
- `segments` 按作答顺序给出分段键：**写感想按题**（q1/q2/q3）、**初步感悟按空**（qid:blankId）；
- 每段"关键完成记录"= 该段内 `IsPassed=true`（答对那次，必然同时 `IsCompleted=true`）或 `IsCompleted=true`（get-answer / 达上限自动填入答案那次）中 `submitted_at` 最晚的一条；该段无关键完成记录时回退为该段最后一条 submit；
- 段 i 起始基准 = 进入节点时间（首段）或 上一段关键完成时间；
- 每条 submit 记录的 `time_cost` = 该次 `submitted_at` − 所属段起始基准（秒，负数取 0），因此每段"完成那次"的 `time_cost` 恰好 = 本段完成时间 − 上一段完成时间；
- 返回 `total` = 末条 submit − 进入节点时间（整节点总用时）。

具体到两个节点：

- **写感想**：q1 作答时间 = q1 答对那次 − 进入节点时间（GetParams 记录的 `entered_at`）；q2 作答时间 = q2 答对那次 − q1 答对那次；q3 作答时间 = q3 答对那次 − q2 答对那次。
- **初步感悟**：每空作答时间 = 本空完成那次 − 上一空完成那次；首空 = 首空完成那次 − 进入节点时间。

### 5. draft_json 分区存储

各节点在 `draft_json` 中以顶层 key 区分，避免冲突：

- `write-thoughts` → `draft_json["write-thoughts"]`
- `initial-insight` → `draft_json["initial-insight"]`
- 使用 `loadDraftPartition` / `saveDraftPartition` 读写

***

## 五、已知问题 & 技术债务

1. **~~Submit 无事务保护~~**：已修复——提交插入与进度计数放入同一 pgx 事务（`SubmitAsync` → `RunInTx`），`attempt_count` 改为 `ON CONFLICT` 原子自增（`BumpAttemptTx`），消除并发竞态
2. **进程内 goroutine**：异步任务重启丢失（未采用持久化任务队列）
3. **~~content\_json schema 无校验~~**：部分修复——submit 路径新增强类型校验（write-thoughts 的 `submit` 类型拒绝空 input）；各节点 JSONB 仍靠代码隐式约定，可进一步引入 JSON Schema 校验 AI 输出
4. **密码未加密**：users.password 存储方案待确定
5. **~~文件存储未对接~~**：upload 接口已对接本地存储——文件保存到 `server.upload_dir`（默认 `upload/`），`resources.file_url` 记录相对 baseUrl 的访问路径（上传/合成：`/upload/<文件名>`；预置资源：`/static/...`），下载接口返回 `resources.file_url` 真实相对路径（前端拼 baseUrl）。**待办**：仅本地存储未对接 OSS（`upload_oss` 字段保留待扩展）；未做文件大小上限与类型白名单限制、过期清理
6. **~~错误码体系~~**：已修复——引入 `service.BizError`（HTTP 状态 + 稳定业务码 Code + Msg），handler 层统一 `respondError` 映射，前端按 Tip 透出的业务码分支，消除了各模块重复 switch

> 日志说明：`pkg/logger.New` 已改为输出到 **stdout**（原来 `zap.NewProductionConfig` 默认写 stderr 导致日志不可见），console 编码显示 `INFO/DEBUG/WARN/ERROR` 大写级别 + 人类可读 ISO8601 时间；`encoding` 为 `json` 时仍输出 JSON。访问日志由 `middleware/accesslog.go` 的 `AccessLog(lg)` 中间件统一输出（需在 `api.RegisterHandlersWithOptions` 之前 `r.Use` 注册，否则仅对之后注册的路由生效）。已排查并修复"访问日志始终不显示"：根因是 `cmd/server/main.go` 中的 `r.Use(middleware.AccessLog(lg))` 曾未真正写入磁盘文件，导致编译产物中该中间件从未注册；现已确认该行存在并在本地启动后用 `curl` 实测，终端正确输出 `request {"method","path","status","latency"}` 访问日志。访问日志已增加调用方 IP 字段 `ip`，通过 `c.ClientIP()` 获取（未配置可信代理时取 `RemoteAddr`；若部署在 Nginx 等反向代理之后需调用 `SetTrustedProxies` 才会取 `X-Forwarded-For` 中的真实 IP）。

> 错误码约定：业务码为 UPPER\_SNAKE 稳定字符串（如 `NODE_NOT_FOUND`、`ACCOUNT_INVALID_CREDENTIALS`、`EMPTY_INPUT`），通过响应 `Tip` 字段透出；`Msg` 为可读描述；未知错误统一 500。

***

## 六、数据库表概览

数据库基于 PostgreSQL，使用 `bigserial` 主键 + `varchar(64) code` 对外业务编码。以下按功能分组说明。

### 1. 用户与班级体系

| 表名              | 数据说明                                            |
| --------------- | ----------------------------------------------- |
| `users`         | 用户账号（学生/教师/管理员），含用户名、学号、bcrypt 密码哈希、角色、年级班级、头像等 |
| `classes`       | 班级定义，包含名称、所属教师、场景（课堂/课后）、起止时间                   |
| `class_members` | 班级成员关联，多对多关系，记录学生加入班级的时间                        |

### 2. 课程节点体系

| 表名              | 数据说明                                                                                         |
| --------------- | -------------------------------------------------------------------------------------------- |
| `class_nodes`   | 课程节点树形结构，每个节点对应一个学习活动（写感想、赵州桥、文化采风等），可嵌套文件夹                                                  |
| `node_contents` | 节点静态配置（1:1），存储各节点的 `params_json` 如 `introVideo`、`title`、`description`、`questions`、`matrix` 等 |

### 3. 学习进度与提交记录

| 表名                 | 数据说明                                                       |
| ------------------ | ---------------------------------------------------------- |
| `node_progress`    | 学习进度缓存（1:1 per node+user），记录完成状态、尝试次数、错误次数、草稿 JSON、累计用时等   |
| `node_submissions` | 提交明细（保留全部历史），每次 submit 都 INSERT 新行，含 AI 判定结果、反馈文本、参考答案、得分等 |

### 4. 创作工坊

| 表名                     | 数据说明                                                       |
| ---------------------- | ---------------------------------------------------------- |
| `creation_tasks`       | 创作工坊任务定义（generation/poemscripts/share/express），含标题和配置 JSON |
| `creation_submissions` | 创作工坊提交记录，含用户输入文本/结构化内容、AI 生成图片、AI 反馈/修改/评价、发布状态等           |

### 5. 文件资源

| 表名                     | 数据说明                                           |
| ---------------------- | ---------------------------------------------- |
| `resources`            | 文件资源元数据，支持图片/音频/视频，含文件 URL、大小、后缀、所有者、OSS 上传标记等 |
| `audio_transcriptions` | 语音转写任务，记录转写状态（processing/finish/fail）和转写文本     |

### 6. AI 对话

| 表名                     | 数据说明                                  |
| ---------------------- | ------------------------------------- |
| `ai_sessions`          | AI 对话会话，按用户/班级/节点/业务类型区分，含提示词模板       |
| `ai_messages`          | 会话中的消息记录，按 seq 排序，含角色、内容、token 用量、模型名 |
| `ai_search_references` | AI 引用来源，记录搜索结果的标题、链接、摘要               |

### 7. 朋友圈

| 表名                | 数据说明                                    |
| ----------------- | --------------------------------------- |
| `moments`         | 朋友圈动态，含来源、内容、附加对象 JSON、点赞/评论冗余计数        |
| `moment_images`   | 动态关联图片，多对多关系，含排序                        |
| `moment_likes`    | 点赞记录，唯一约束 (moment\_id, user\_id) 防止重复点赞 |
| `moment_comments` | 评论记录，含评论内容，支持软删除                        |

### 8. 枚举类型速查

| 枚举名                   | 取值                                                                                                                                                                                                      |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `user_role`           | `admin`, `teacher`, `student`                                                                                                                                                                           |
| `scene_type`          | `inclass`, `afterclass`                                                                                                                                                                                 |
| `node_type`           | `write-thoughts`, `initial-insight`, `cultural-style`, `cultural-style-quick-select`, `cultural-style-drag-sort`, `zhaozhouqiao`, `wenmingzhongwai`, `heritage-cultural`, `creation-workshop`, `folder` |
| `creation_task_type`  | `generation`, `poemscripts`, `share`, `express`                                                                                                                                                         |
| `creation_status`     | `pending`, `processing`, `successful`, `published`                                                                                                                                                      |
| `resource_media_type` | `image`, `audio`, `video`                                                                                                                                                                               |
| `audio_trans_status`  | `processing`, `finish`, `fail`                                                                                                                                                                          |
| `ai_message_role`     | `user`, `assistant`, `system`                                                                                                                                                                           |
| `chat_finish_reason`  | `stop`, `length`, `tool_calls`                                                                                                                                                                          |
| `activity_status`     | `NotStarted`, `InProgress`, `Finished`                                                                                                                                                                  |

