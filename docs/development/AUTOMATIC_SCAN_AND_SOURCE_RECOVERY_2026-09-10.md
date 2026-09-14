# 自动扫描与来源异常恢复开发记录

> 日期：2026-09-10
> 分支：`agent/cgm-migration-handoff-20260726`
> 状态：自动扫描及异常处理第二个闭环完成，尚未部署

## 1. 实施基线

- 不引入实时文件监听；继续使用启动触发或周期调度的完整扫描。
- 自动扫描复用现有候选发现、媒体库自动化策略和Source原子扫描，不建立第二套扫描器。
- 默认关闭；维护模式或恢复后的`automatic_schedules_suspended`继续阻止任何自动计划。
- 扫描、重新绑定和忘记记录都不能删除、移动或改写用户媒体文件。
- 来源整体不可读时只改变Source健康并暂停Browse，不批量把既有Item标为MISSING。

## 2. 自动扫描第一阶段

产品数据库升级为schema v11，只给`runtime_settings`增加：

- `automatic_scan_on_startup`：默认关闭；所有者可选择服务启动后扫描一次。
- `automatic_scan_interval_minutes`：默认1440分钟，允许15～10080分钟。

既有`automatic_scan_enabled`仍是总开关。调度循环每分钟检查持久化到期状态，以`scheduled_operations`租约防止重启或多进程重复执行。启动扫描使用同一租约并重置下一周期；每日数据库快照继续独立运行。

后台Settings提供中英文开关、启动扫描和周期输入。已启用媒体库按现有逻辑处理：MANUAL库运行发现并扫描已绑定Source；ASSISTED/TRUSTED库排队既有自动化流程，并避免同时重复扫描其待处理DRAFT。

## 3. 异常处理首个闭环

### 3.1 Gallery异常入口

Manage Gallery新增服务端全库筛选：ALL、DRAFT、UNAVAILABLE、MISSING、PROCESSING_ERROR、BLOCKING和OVER_LIMIT。异常行高亮，MISSING Gallery在普通列表排序中提前；筛选状态进入URL并保持分页。

### 3.2 媒体缺失处理

Gallery Media显示总成员与MISSING数量，可只显示MISSING。MISSING或已排除Item提供显式“Forget record”；确认后只删除CGM成员记录、永久Tombstone UUID并递增Gallery metadata revision，不删除源文件。再次Manifest Push后该条目才从`.cosplay.json`消失。

### 3.3 来源诊断与重新绑定

扫描读取错误现稳定区分`SOURCE_NOT_FOUND`、`SOURCE_PERMISSION_DENIED`和`SOURCE_READ_FAILED`。Gallery列表及Source页显示最近扫描错误与完成时间，既有Item继续保留上次成功状态。

Libraries发现相同Manifest `set_id` 时，`SOURCE_REBIND_CANDIDATE`提供显式确认入口；新旧来源都可访问时再次显示高风险提示。确认只迁移既有GallerySource身份并置为`NEEDS_RESCAN`，随后仍须完整扫描。

### 3.4 Manifest成员变更

扫描新增Item或唯一指纹路径重绑定时，将已跟踪Gallery Manifest置为`DB_DIRTY`。Manifest Check保留该状态，避免纯扫描只增加`scan_revision`而错误显示`CLEAN`。MISSING本身继续留在Manifest成员索引，只有显式Forget才删除。

### 3.5 文件内容与名称同时更换

系统不会根据名称相似或目录位置自动继承身份。Media页可为一个MISSING Item人工选择同一媒体分组内的AVAILABLE Item，先预览“保留身份/采用文件”，再输入`REPLACE`确认。事务保留旧UUID、Caption、排序、Exclude、收藏、评分及封面意图，只采用新路径、类型和指纹，递增内容版本并重建派生缓存；新临时UUID永久Tombstone。新Item已有Caption、Exclude、个人状态或封面引用时拒绝操作，避免静默覆盖。

### 3.6 批量缺失清理与封面门禁

Media页可在显示缺失数量后输入`FORGET N`批量忘记当前Gallery全部MISSING记录。所有UUID在同一事务内Tombstone，只推进一次Gallery metadata/scan/scrubber revision，不删除源文件。单项或批量忘记前会解除该Item的首选、回退及生效封面引用，避免外键清空与封面约束冲突。

### 3.7 扫描历史

Source页显示最近20次扫描的状态、开始/完成时间和稳定错误码。现有schema不长期保留已提交观察明细，因此本阶段不伪造新增/缺失数量；更细计数需要未来显式扩展持久化字段。

### 3.8 Manifest Push差异预览

Manifest页在写回前按Portable Item UUID比较当前数据库与上次同步基线，显示新增、删除、保留及业务字段更新数量；点击Push后还会展示同一摘要并二次确认。预览本身不读取或写入媒体，实际Push仍重新验证Manifest文件哈希，因此不能绕过外部修改冲突门禁。

## 4. 后续阶段

1. 若未来确有审计需要，为扫描运行增加触发来源和新增/重绑定/MISSING技术计数；不得记录业务名称或默认路径。
2. 媒体库改根/删除工作台已完成影响预览、逐Source转移、执行前并发复核和实际执行；全局/库级ignored Source撤销入口也已补齐。后续可补非Gallery Source的发现会话门禁和更细的目标文件系统诊断，继续禁止父媒体库静默重新导入。
3. 将来源错误进一步细分为存档加密、结构不安全、I/O和挂载类诊断，并提供对应恢复建议。

“疑似高清替换”只能形成建议。文件名和内容指纹都变化时不存在确定身份依据，任何属性迁移必须由所有者逐项确认。

### 4.1 媒体库变更第一段（2026-09-14，本地源码）

Libraries现提供只读影响预览：现有根、拟议新根或删除、直接绑定的Gallery Source及其Gallery标题/路径/建议归属、ignored Source数量和子媒体库根。建议归属仅供参考，不会自动转移。单项Source转移须选择目标并二次确认；后端校验原库归属仍与预览一致、目标库实际包含该路径、两端没有排队或运行中的自动化任务，并在同一事务里更改绑定和使涉及库的旧发现快照失效。可显式转为未分配，不移动或删除媒体文件，也不更改Gallery身份或元数据。审计记录动作与稳定结果。

此段**没有**开放实际媒体库改根或删除动作：两者还需要对ignored Source、识别规则、自动化策略/运行、子库边界及目标路径冲突做完整预览和执行前重新校验。界面明确标为“仅预览”，不能把该预览解释为已完成变更。新Source转移操作不需要schema迁移；与上一阶段的schema v11源码仍一起保持未提交、未部署，正式库仍为v10。

验证：正式三标签产品数据库/API/Server/CMD测试通过；Web 31文件106项测试、TypeScript与682模块生产构建通过；`git diff --check`通过。未执行真实媒体库变更或正式业务数据库迁移。

### 4.2 媒体库改根/删除执行闭环（2026-09-14，本地源码）

影响预览现在逐项列出ignored Source精确路径及原因、该库专属识别/分类/排除规则，并显示自动化模式/策略revision、历史任务及活动任务数量、扫描中的Source、迁移映射、原有子库和拟议根下的其他库冲突。全局规则不在删除范围。预览返回由上述状态及Source绑定生成的校验令牌；用户必须输入所有者密码和精确确认词，执行事务重新加载相同影响清单，令牌不一致即拒绝，不允许绕过旧预览。活动自动化任务、扫描中的Source和可移植迁移映射均阻断执行。

删除要求所有直接绑定的Gallery Source先显式转移或转为未分配。该库专属规则、自动化策略及历史任务按外键清除；子媒体库原样保留。ignored Source改为`library_id=NULL`的全局精确路径忽略；库根下已未分配的Source也在预览列出，并在删除事务为其精确路径建立全局忽略，防止父库在边界消失后静默重新导入。媒体文件、Gallery及其身份/属性不删除。未来需要单独提供全局忽略撤销入口，不能通过删除库隐式撤销。

改根拒绝现有子库和拟议根覆盖其他媒体库的情况；事务按相对路径重映射**绑定于该库**的Source、ignored Source及Gallery Manifest路径，保留规则与自动化配置。库根下已未分配的Source只在影响预览中提示，不会被该库改根隐式接管或重映射。已绑定Source标记`MISSING/NEEDS_RESCAN`、Manifest标记待复核并清除旧发现快照；不移动媒体文件、不自动重扫、也不声称新路径已可访问。路径唯一性或映射错误会整体回滚。无新增数据库schema；本机正式库仍是旧部署，本段未提交或部署。

验证：正式三标签产品数据库/API/Server/CMD测试及同范围Go Vet通过；Web 31文件107项测试、TypeScript和682模块生产构建通过，保留既有主共享chunk超过500KiB提示；`git diff --check`通过。未在正式媒体库执行变更。

### 4.3 忽略路径撤销工作台（2026-09-14，本地源码）

Libraries新增按全局或当前媒体库作用范围查看ignored Source，服务端按路径子串搜索并每页50项分页。列表明确显示路径、原因、所在库以及可选Set ID。单项撤销前预览受影响的媒体库、这些库的活动自动化任务数量、同路径已存在的Gallery Source和绑定状态；默认不扫描。纯路径忽略只影响所属库，或全局忽略路径下的全部根；带Set ID的忽略在既有发现逻辑中跨库生效，因此撤销时全部媒体库都纳入预览、活动任务门禁和快照失效。用户输入所有者密码及`REVEAL`确认词后，事务重新计算预览令牌。旧预览或活动自动化任务会被拒绝；成功只删除该条CGM忽略记录，并失效受影响媒体库的旧发现快照，不访问或改写源媒体。下一次发现可能重新见到该路径，但已有Set ID Tombstone等其他门禁仍可能阻止导入。失败只返回稳定的过期预览/活动任务码，不在审计中写路径或Set ID。

该入口适用于删除媒体库时保留下来的全局精确路径忽略，也适用于普通库级忽略；不会批量清除。没有新增schema、自动扫描或文件清理。完整业务数据库仍未部署本轮源码。

验证：正式三标签产品数据库/API/Server/CMD测试及同范围Go Vet通过；Web 31文件108项测试、TypeScript和682模块生产构建通过，保留既有主共享chunk超过500KiB提示；`git diff --check`通过。仍需在真实媒体库上由所有者验收撤销后发现行为，本轮没有访问或迁移正式业务数据库。

## 5. 本阶段验证与部署状态

### 发现快照并发复核（2026-09-14，本地源码）

发现流程原先在事务外分别读取媒体库、识别规则、绑定Source、忽略路径/Set ID和Gallery身份，再开启事务写入快照。若期间撤销忽略、转移Source或修改规则，旧判断可能被保存为新快照。现将这些读取、候选计算及快照写入放进同一`BEGIN IMMEDIATE`事务；文件系统遍历仍在事务外，但写入前复核遍历所用媒体库根与子库边界，变化时拒绝本次快照并提示重试。数据库规则/忽略状态在事务开始时取当前值，事务内不允许并发写入穿插。未对媒体执行写入，未增加schema。回归覆盖遍历期间改根、增加子库和新增忽略路径。

- Go：产品数据库（含正式v10→v11迁移快照及默认值）、Product API、Server、Manage、Manifest及SourceScan测试通过；正式`cgm_web_embed cgm_galleryepic cgm_moegirl`标签组合测试与Go Vet通过。
- Web：TypeScript检查、31个测试文件108项测试及682模块生产构建通过；仅保留既有主共享chunk超过500KiB提示。
- `git diff --check`通过。
- 尚未提交或部署；正式业务数据库仍应保持schema v10，部署schema v11前必须执行完整备份和迁移前校验。
