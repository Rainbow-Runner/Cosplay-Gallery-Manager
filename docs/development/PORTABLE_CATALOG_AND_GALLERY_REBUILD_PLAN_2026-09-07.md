# 可移植核心目录与 Gallery 重建长期开发方案

> 日期：2026-09-07  
> 状态：方案基线；核心CLI迁移闭环源码完成，尚未提交或部署  
> 适用范围：1.5 之后的长期迁移、异机重建与灾难恢复能力  
> 当前源码产品数据库：schema v10；已部署正式数据库仍为schema v8

## 1. 背景与结论

CGM 当前的完整备份会同时保存产品数据库、Coser 托管元数据和必要启动配置。它适合整机灾难恢复，但数据库中同时存在核心实体、Gallery、媒体库绝对路径、运行任务和本机设置，因此不适合作为长期唯一的跨机器迁移格式。

长期方案采用分层迁移，而不是把所有信息继续塞进数据库备份或 Gallery Manifest：

1. **可移植元数据包**保存全局身份账本和 Coser、Work、Character、Tag 等核心目录数据，不保存媒体根绝对路径。
2. **Gallery Manifest**继续随媒体目录或存档文件保存 Gallery 业务元数据，以相对路径描述成员。
3. **机器本地绑定**在新机器上重新配置或显式映射，包括媒体库根、缓存、日志、FFmpeg/FFprobe 和调度状态。
4. **完整备份**继续作为同机回滚和整套系统灾难恢复手段，不由可移植元数据包取代。

因此，核心实体迁移与媒体根、Gallery 来源路径应当解耦；但 Portable UUID 身份历史不能被拆散或重新生成。

## 2. 不改变的既有架构约束

- Gallery `set_id`、GalleryItem、Coser、Work、Character、Tag、ExternalLink 和 SocialAccount 继续共享全局 Portable UUID 命名空间。
- UUID 是身份依据；名称、Alias、Slug、路径和文件指纹都不能代替 UUID。
- 合并后的 UUID Alias 和删除后的 Tombstone 永久保留，不得回收。
- Gallery Manifest v1 仍是媒体侧 Gallery 元数据载体；DIRECTORY 使用 `.cosplay.json`，ARCHIVE_FILE 使用相邻的 `<完整存档文件名>.cosplay.json`。
- Coser Manifest 与托管头像/Banner继续使用独立元数据根；Work、Character、Tag 不新增零散文件 Manifest。
- Gallery Manifest 不写入 `state`、`collection_type`、`slug`、`added_at`、`favorite`、`hidden`、浏览历史、播放进度或 `view_count`。
- Manifest 写回继续由所有者显式 Push；迁移、实体编辑或扫描不得静默改写用户媒体目录。
- 应用不删除、移动或重新打包用户媒体；存档来源继续只读。
- 第一版已延期 Windows 原生支持。本方案的首轮实现与验收只覆盖 Linux 和 Docker，格式本身保持平台中立。

## 3. 数据分层与责任边界

| 层 | 权威内容 | 是否可跨机器 | 是否包含绝对路径 |
| --- | --- | --- | --- |
| 全局身份账本 | 所有 Portable UUID 的 kind、创建状态、Alias 链、Tombstone | 是 | 否 |
| 核心目录 | Coser、Work、Character、Tag、SocialAccount、Slug 历史、关系和当前 Coser 资源 | 是 | 否 |
| Gallery Manifest | Gallery 业务元数据、实体 UUID 引用、成员相对路径、Item UUID、封面与排除项 | 随媒体迁移 | 否 |
| 可选连续性状态 | Gallery生命周期/首次收录时间、Gallery收藏/隐藏、Item收藏 | 可选、独立导入 | 否 |
| 机器本地绑定 | 媒体库根、来源绝对路径、缓存/日志/备份根、工具路径、Session、任务和调度 | 否 | 是 |
| 完整备份 | 某一时点的完整数据库、Coser 元数据和必要启动配置 | 是，需路径映射 | 是 |

“可移植”不等于“完整恢复”。只导入核心目录与 Gallery Manifest 时，目标系统重建的是内容目录；Session、队列、缓存、发现快照和本机路径不会继承。

## 4. 新增可移植元数据包

建议使用与现有 `.cgm-backup.zip` 明确区分的新格式，例如：

```text
CGM-portable-<timestamp>-<export-id>.cgm-portable.zip
├── package.json
├── checksums.json
├── identity-ledger.json
├── core-catalog.json
├── gallery-index.json
└── coser-assets/
    └── <coser-uuid>/...
```

### 4.1 `package.json`

包含稳定 product ID、portable package format version、导出 ID、UTC 时间、数据库/Manifest/媒体处理版本、内容计数、规范化规则和所含可选分区。格式版本与数据库 schema 版本独立演进。

### 4.2 `identity-ledger.json`

保存**全部实体种类**的 UUID Registry、Alias 和 Tombstone，而不只保存当前活跃核心实体。这样即使 Gallery 内容仍位于媒体侧，也不会在新数据库中丢失已删除 Item、ExternalLink 或 Gallery 的永久占号历史。

它只描述身份和生命周期，不包含 Gallery 业务字段、媒体路径或原始文件信息。

导入器不得把尚未重建业务行的 Gallery、Item 或 Link 直接写成普通 ACTIVE Registry 记录。此类身份先进入受源包摘要绑定的“待接管身份声明”，并参与全局 UUID 分配冲突检查；只有对应 Manifest 和实际来源通过验证后，才在同一事务中注册正式 UUID 与业务行。无法完成重建的声明保持可见、可重试，不会形成看似 ACTIVE 的孤儿身份。Alias/Tombstone 也按依赖顺序验证和发布，不能留下断链。

### 4.3 `core-catalog.json`

保存：

- Coser：UUID、名称、Sort name、Aliases、Slug 与历史、国家、资料、简介、元数据 revision、当前资源引用及裁切/焦点。
- Work：UUID、名称、Sort name、Aliases、Slug 与历史、revision。
- Character：UUID、名称、Sort name、Aliases、Slug 与历史、所属 Work UUID、revision。
- Tag：UUID、名称、Sort name、Aliases、Slug 与历史、父级 UUID 关系、revision。
- SocialAccount：UUID、所属 Coser UUID、platform key、URL、position。
- 核心实体合并重定向所需的引用信息。

数据采用确定性顺序、NFC 文本和严格单一 JSON 对象。任何冗余显示名都只作人工提示，不能改变 UUID 解析结果。

### 4.4 `coser-assets/`

首轮只导出当前被引用的头像/Banner原始托管资源及其必要描述；480/960/1600 等可再生派生图默认不导出，在目标机重新生成。缺失、符号链接、类型不符或超限资源在导出预检中阻断或形成明确报告，不静默跳过。

已替换但未引用的托管资源不属于可移植目录；它们仍由现有未引用资源审阅和完整备份负责。

### 4.5 `gallery-index.json`

该文件不是 Gallery 数据的第二权威副本，只是迁移校验清单。建议记录媒体库逻辑标签、来源类型、库内相对来源标识、`set_id`、Manifest revision、BLAKE3/SHA-256 摘要及同步状态，用于发现：

- 未 Push 的 `DB_DIRTY` Gallery；
- 缺失、损坏或过期的 `.cosplay.json`；
- 媒体复制后丢失的相邻 Archive Manifest；
- 重复 `set_id` 或同一来源的多份 Manifest。

它不得保存旧机器媒体库绝对路径，也不得作为缺失 Manifest 时静默恢复 Gallery 的依据。

## 5. Gallery Manifest 中 UUID 的实际语义

当前 Gallery Manifest v1 已保存或允许保存：

- Gallery 的 `set_id`；
- Credit/Cast/Tag 中 Coser、Work、Character、Tag 的 UUID；
- Gallery Item 的 `item_uuid`；
- ExternalLink 的 `link_uuid`；
- Cover 引用的 `item_uuid`。

`name_hint`仅用于缺少实体时的首次命名或人工识别，不能覆盖同 UUID 的现有实体名称。

可能发生的风险包括：

- 旧 Manifest 未经过一次完整 Push，部分 Item 或 Link 没有 UUID；
- Manifest 引用的核心实体没有进入目标数据库；
- 同一 UUID 在目标库已被其他 kind 占用；
- 同一规范化名称在源、目标两边对应不同 UUID；
- Manifest 的 Character 与目标数据库中的 Work 归属冲突；
- 同一 `set_id` 出现在两份可访问媒体副本；
- 现有首次导入流程能按相对路径找 Item，但不能在所有场景安全接管 Manifest 中尚未注册的 Item UUID。

长期实现必须解决这些冲突，不能通过重生成 UUID、按名字自动合并或按扫描顺序覆盖来规避。

## 6. 导出流程

1. 创建数据库一致性只读快照，所有清单都从同一逻辑时点生成。
2. 执行预检：UUID Registry 完整性、Alias 无环、Tombstone 状态、Character→Work、Tag DAG、Slug 历史、Coser 资源和 Manifest 同步状态。
3. 默认要求所有可访问 Gallery 为 `CLEAN`；允许“带警告导出”时必须在报告中逐项列出 `DB_DIRTY/FILE_DIRTY/CONFLICT/MISSING`，不得声称可以完整重建。
4. 按 UUID 和稳定关系顺序生成规范 JSON；对每个文件写 SHA-256，并对整包再次计算摘要。
5. 复用现有完整备份的安全 ZIP 规则：拒绝路径穿越、反斜杠/盘符条目、非 NFC 名称、大小写碰撞、Windows 保留名、符号链接、特殊文件、异常条目数/大小/压缩比和未知 Entry。
6. 导出完成后返回包含项、排除项、警告、实体计数、Gallery 清单覆盖率和摘要；日志及审计不记录姓名、URL、物理路径或媒体标题。

可移植包不得包含所有者密码哈希、Session、审计内容、网络抓取临时数据、媒体库绝对路径、缓存、日志、处理任务或原始 Gallery 媒体。

## 7. 导入与重建流程

### 7.1 阶段 A：只读检查

导入包先进入临时隔离目录，只做格式、版本、Entry、摘要、JSON、资源类型和上限检查。检查完成前不写产品数据库，也不替换 Coser 元数据根。

系统生成冲突矩阵：

| 情况 | 默认处理 |
| --- | --- |
| UUID 不存在，kind 合法 | 可导入并保留原 UUID |
| UUID 存在、kind 相同、内容等价 | 复用，记为无操作 |
| UUID 存在、kind 相同、内容不同 | 进入字段级冲突审核 |
| UUID 已被其他 kind 占用 | 硬阻断 |
| 源 Alias/Tombstone 与目标 ACTIVE 冲突 | 硬阻断 |
| 同规范化名称、不同 UUID | 仅提示身份复核，不自动合并 |
| Character 的 Work 归属不同 | 硬阻断，先显式迁移或选择身份 |
| Tag 关系形成环 | 硬阻断 |

### 7.2 阶段 B：核心目录导入

首个正式版本只支持“新建/空业务数据库”导入。核心实体 UUID 与对应业务行按 Work→Coser/Tag→Character→SocialAccount/关系的依赖顺序原子写入；尚未重建的 Gallery/Item/Link 身份进入待接管声明而不是伪 ACTIVE Registry 行。数据库和 Coser 当前资源在维护模式下分阶段暂存，任何失败都回滚，不产生部分目录。

导入后逐项验证 UUID、Alias/Tombstone、关系、revision、资源摘要和数据库完整性。不得把导入的 revision 当作乐观锁之外的全局新旧排序依据。

已有业务数据的 Merge 模式延后实现，且必须经过冲突审核；不能把“同名”作为自动合并依据。

### 7.3 阶段 C：新机器绑定媒体库

所有者为每个媒体库逻辑标签选择新的真实绝对根或明确跳过。路径映射只建立本机绑定，不回写旧绝对路径，也不自动开始扫描。

随后运行只读发现，对照 `gallery-index.json` 和实际 sidecar 生成报告；所有者确认后才创建/重建 Gallery 来源。

### 7.4 阶段 D：Gallery 身份重建

对每份有效 Manifest：

1. 先验证 `set_id`、核心实体引用和全局 kind。
2. 建立 DRAFT Gallery 或形成明确的 Source Rebind Candidate；重复可访问副本不得自动选择。
3. 扫描实际成员，按来源内规范相对路径建立临时成员集合。
4. 新增“首次身份接管”事务：只有目标 UUID 未被目标业务占用、与导入声明一致、相对路径唯一、来源归属唯一且内容校验一致时，才把 Manifest 的 `item_uuid` 和对应声明原子转为正式 Registry/成员关系。
5. 不能安全接管的成员保持 DRAFT 并生成逐项问题；不得自动生成新 UUID 后假装已完整恢复。
6. ExternalLink UUID、Cover Item UUID 和排除项在 Item 身份完成后解析。
7. 完成三方状态基线，确认后才能 Push 或激活。

只靠核心目录和 Gallery Manifest 重建时，Gallery 默认保持 DRAFT。只有通过现有激活校验后才能人工激活；可选连续性状态也不能绕过 `DISPLAYABLE_ITEM_REQUIRED`、`CONTENT_RATING_REQUIRED` 等门禁。

## 8. 可选连续性状态

为了不推翻 Gallery Manifest v1 的既有边界，Gallery 生命周期、收藏、隐藏和 Item 收藏不加入 `.cosplay.json`。Gallery 与 Item 评分继续由 Manifest 负责，不在 owner continuity 重复保存；Gallery Slug/历史以及最后浏览时间、最后浏览项目等浏览状态明确不参与可移植迁移。

长期可在可移植包中增加独立、可选的 `owner-continuity.json`，全部以 `set_id`/`item_uuid` 为键。它应满足：

- 默认不导出，用户明确选择后才包含；
- 只允许分别选择 Gallery 生命周期（`state`、`added_at`）和轻量个人标记（Gallery 收藏/隐藏、Item 收藏）；
- 不含认证凭据、Session、审计、任务或物理路径；
- 不含 Gallery Slug/地址历史、Gallery/Item 评分、最后浏览时间、最后浏览项目或其他浏览历史；
- 只能在对应 Gallery/Item 身份已安全重建后应用；
- ACTIVE 状态仅作为“请求恢复状态”，必须重新通过当前环境激活门禁；
- 导入失败不影响已经完成的核心目录导入，且可单独重试。

完整无损迁移仍优先使用现有完整备份；可移植重建用于路径和运行环境变化较大的场景。

## 9. 管理界面与 CLI

Manage → Operations 新增与“完整备份/恢复”并列但明确区分的“可移植元数据”区域：

- 导出前预检、包含/排除范围、Manifest 覆盖率和警告；
- 导入包选择/注册、只读检查、空库或 Merge 模式识别；
- UUID/名称/关系冲突分组审核；
- Coser 资源校验；
- 媒体库逻辑标签到新根的映射；
- Gallery 发现、Item UUID 接管和未完成项目报告；
- 可重入进度、取消与失败重试。

高影响提交继续要求所有者重新认证和精确确认词。前端保持中英文，错误使用稳定代码，页面和日志均不泄漏不必要的绝对路径；只有既有认证所有者的管理界面可在映射步骤显示必要路径。

CLI 提供与 Web 共用服务的检查、导出、导入、路径映射和重建入口，便于无图形界面的 Docker/服务器部署。具体参数名在接口阶段冻结，避免在方案期形成不必要的兼容承诺。

## 10. 数据库与版本策略

- 导出格式和 Gallery Manifest v1 可在第一阶段保持不变；不得为了实现导出而提前新增 schema。
- 当实现可恢复的导入会话、待接管身份声明、冲突决策和阶段进度时，持久化传输会话、声明、冲突摘要和阶段游标是合理的 schema 变更理由；所有 UUID 分配入口必须同时检查有效声明，避免重建前被本机新对象抢占。
- 实际编号取实现时的下一可用版本，不在本文固定为 schema v9，以免与其他并行迁移冲突。
- 新表只保存技术状态、稳定错误码、计数和不透明身份；包的完整业务 JSON 保留在受控临时文件中，不复制成第二套长期业务表。
- schema 写入前继续创建来源版本准确的 Online Backup，并验证产品身份与 `integrity_check`。
- 可移植包 format version、数据库 schema version、Gallery Manifest version 和媒体处理 version 继续独立演进。

## 11. 分阶段开发顺序

### 阶段 0：ADR、数据清单与金样本

- 固化本方案为 ADR，逐表标注“核心目录 / Gallery / 所有者状态 / 机器本地 / 可再生”。
- 建立包含合并、删除、Slug 历史、Coser 资源、跨 Work Character 和 Tag DAG 的最小金样本数据库。
- 建立 DIRECTORY、ZIP/CBZ、TAR/TGZ、7Z Gallery Manifest 金样本。
- 明确迁移包大小、实体数和资源数上限。

门禁：任何产品表都必须有唯一归属；没有“顺便全部导出”的未分类数据。

### 阶段 1：确定性导出与离线检查器

当前状态：只读导出、导出前预检、身份账本流式写入及流式离线检查源码完成，尚未提交或部署；Web管理界面仍属于后续工作。

- 实现包模型、规范 JSON、摘要、ZIP 安全规则和只读导出服务。
- 实现独立 inspector，可在不打开产品数据库的情况下验证包。
- 输出 Gallery Manifest 覆盖报告，但不修改任何 Manifest。
- 暂不实现导入，不新增产品 schema。

门禁：相同快照重复导出得到一致业务内容；包内无绝对路径、秘密、Session、任务、缓存和日志。

当前首切片提供CLI入口：

```bash
# 需要交互式所有者重新认证；默认遇到不完整Gallery索引时阻断
cgm -config /absolute/path/cgm.json -preflight-portable

# 需要交互式所有者重新认证；默认遇到不完整Gallery索引时阻断
cgm -config /absolute/path/cgm.json \
  -export-portable /absolute/path/export.cgm-portable.zip

# 只有所有者明确接受Gallery索引警告时使用
cgm -config /absolute/path/cgm.json \
  -export-portable /absolute/path/export.cgm-portable.zip \
  -allow-incomplete-portable

# 完全离线检查，不读取配置或打开产品数据库
cgm -inspect-portable /absolute/path/export.cgm-portable.zip
```

当前实现导出全局UUID Registry/Alias/Tombstone、核心实体及关系、核心Slug重定向、Coser帐号和当前头像/Banner原始文件；派生头像/Banner不导出。`-preflight-portable`只读取数据库和当前托管原件，输出按kind/state分类的身份计数、核心/Gallery/资源计数及稳定问题码，不创建包、不输出姓名、URL或路径；预检和导出均写入无路径审计。导出成功后由同一离线检查器复验。

身份账本Writer保持v1 JSON结构并从同一数据库只读事务逐条编码，不再构造全量身份切片。离线Inspector逐条解码，在0600临时SQLite索引中验证顺序、唯一性、kind/state、Alias目标及环，只把受128MiB核心/Gallery文档约束的对象身份保留在Go内存；临时索引在成功、失败或取消后关闭并删除。身份账本独立允许最多20,000,000条且仍受10GiB整包解压上限、摘要和压缩比门禁约束，核心/Gallery JSON继续保持128MiB限制。`-allow-incomplete-portable`只能接受Gallery覆盖警告，不能绕过完整性阻断。

### 阶段 2：空库核心目录导入

- 实现全局身份账本导入、核心实体依赖排序及 Gallery/Item/Link 待接管身份声明。
- 实现 Coser 当前资源的暂存、原子发布与派生重建。
- 只支持空业务数据库；非空库明确拒绝。
- 导入前自动安全备份，失败后无部分写入。

门禁：源/目标全部核心 UUID、Alias、Tombstone、名称、关系和有效资源摘要相同；未重建 Gallery 身份只表现为有效声明，不存在孤儿 ACTIVE Registry 记录。

当前阶段2源码已实现：`-import-portable`要求所有者重新认证、绝对ZIP路径、空业务数据库及确认词`IMPORT`；检查通过后保留隔离包并自动创建完整安全备份。Schema v9持久化导入会话与Gallery/Item/Link声明，`PENDING`声明阻止普通UUID分配抢占，后续只能按`CLAIMING→CLAIMED`接管。核心身份、实体、关系、Alias/Tombstone和revision在一个数据库事务中写入；Coser原件先在目标托管根内暂存、逐项复验并重建派生图，再以所有权标记原子发布。普通失败会撤销事务和本轮目录；进程强制中断后可用`-recover-portable-import`仅清理由对应导入标记的未提交目录，或复验已提交资源后清除标记并退出维护状态。媒体映射、Gallery业务行重建和Web入口仍属于后续阶段。

### 阶段 3：媒体映射与 Gallery 重建

- 实现媒体库逻辑映射、sidecar 覆盖核对和缺失报告。
- 实现 Manifest `set_id` 重建、Source Rebind Candidate 和首次 Item UUID 安全接管。
- 覆盖目录和全部已支持存档来源；存档本体不修改、不重新打包。
- 重建结果默认 DRAFT，并接入现有问题与激活门禁。

门禁：移动媒体根后仍保留可证明的 set/item/link UUID；歧义、冲突和缺失均可见且不静默修复。

当前阶段3源码已实现CLI闭环：schema v9导入会话在核心导入前保存逻辑媒体库标签、Gallery相对来源定位、导出时sidecar状态/摘要和逐Gallery重建游标，不保存旧机器绝对路径。`-map-portable-libraries <import-id>`要求所有者为每个逻辑库选择本机已启用媒体库或明确跳过，且不会自动扫描；`-preflight-portable-rebuild <import-id>`只读复验映射边界、逐级非符号链接路径、目录/存档类型、sidecar精确BLAKE3摘要、set_id/revision、安全来源扫描、Manifest Item路径一一对应及待接管身份状态。`-rebuild-portable-galleries <import-id>`再次要求确认词`REBUILD`，按Gallery创建DRAFT及本机Source、复用统一目录/存档扫描器，以Manifest相对路径在扫描事务中接管Item UUID，并在Manifest应用事务中接管Link UUID及恢复元数据关系。任何阻断只记录稳定问题码，不生成替代UUID或静默改写Manifest/媒体/存档；中途失败保留`REBUILDING`游标，重试会核对已接管身份确属同一Gallery。只有所有ACTIVE声明完成接管后才按依赖发布Gallery类Alias/Tombstone并标记会话完成，明确跳过或不完整来源仍保留声明与可重试状态。目录及ZIP来源在完全不同媒体根上的端到端UUID保持测试已通过；其余已支持存档格式复用已有安全扫描实现，真实跨机与Web产品化入口仍属于后续工作。

### 阶段 4：非空库 Merge 与冲突工作台

- 在空库闭环稳定后再开放 Merge。
- 实现字段级复用/冲突、同名异 UUID 复核、关系冲突和显式实体合并跳转。
- 决策持久化并受源包摘要绑定；包变化后旧决策失效。

门禁：重试幂等；扫描顺序不影响结果；任何硬冲突不能被“全部接受”绕过。

当前阶段4首个只读切片已实现：需所有者重新认证的`-preflight-portable-merge <absolute.zip>`先用独立Inspector完整验证包，再在同一数据库只读事务中把本机Registry与包内身份账本按UUID顺序流式归并，不将百万Item/Link身份载入内存。报告区分可新增身份、完全一致复用、UUID kind/state/target/history硬冲突；核心目录进一步区分可新增、字节语义一致复用、同UUID内容差异、不同UUID规范化主名/Alias候选、同kind Slug占用、SocialAccount URL候选、Tag边位置及Slug重定向冲突。报告只输出稳定问题码、kind和双方UUID，不输出名称、URL、路径或字段正文；同名候选永远只是`REVIEW`，不会自动视为同一实体。预检前后复核来源文件inode、普通文件属性和整包SHA-256，不复制包、不创建Merge/Import会话、不注册声明、不修改核心实体、Gallery、Manifest、媒体或资源；仅保留无业务正文的既有审计记录。持久化决策模型、字段级选择、实际写入Merge和Web工作台仍未开放，必须在下一切片先冻结决策语义并新增受包摘要约束的schema后实施。

阶段4第二切片现已新增schema v10技术状态：`portable_merge_sessions`绑定merge/export UUID、受控包相对路径、包SHA-256、目标Registry+核心目录指纹、预检计数和状态；`portable_merge_conflicts`只保存稳定issue key/code、severity、kind、双方UUID、可选field key及决策，不复制名称、URL或字段正文。`-prepare-portable-merge`要求确认词`PREPARE`，先预检调用者文件，再复制到Backup根专属0600目录并对保留副本重新完整预检，最后才原子建立会话；硬冲突会话固定`BLOCKED`。`-decide-portable-merge`要求逐项输入和最终`DECIDE`：内容/Tag边差异只接受`KEEP_LOCAL/USE_INCOMING`，同名/帐号候选只接受`KEEP_SEPARATE/MAP_TO_LOCAL`，硬冲突不接受任何决策；必须一次覆盖全部REVIEW。提交决策前重新预检保留包和当前目标，包摘要或目标指纹变化会把会话标为`STALE`并拒绝旧决策。当前`READY`只表示决策集合完整，不代表数据已合并；实际核心写入、字段级拆分、Gallery Merge和Web工作台仍未开放。

阶段4基础写入闭环现已完成：READY会话可应用完整人工决定；`MAP_TO_LOCAL`把传入UUID登记为指向已确认本地实体的永久Alias并重映射包内关系，`KEEP_LOCAL/USE_INCOMING`控制同UUID对象、SocialAccount位置和Tag边。应用前创建完整安全备份并进入`PORTABLE_MERGING`；新增Coser资源安全发布，`USE_INCOMING`替换既有资源时保留旧assets回滚目录，事务失败自动还原，进程中断可按所有权标记恢复。Merge不再建立一套孤立重建表，而是原子创建兼容阶段3的技术重建会话及Gallery/Item/Link声明，因此同一Merge UUID可继续执行逻辑媒体库映射、只读sidecar核验和DRAFT Gallery身份接管。应用前可显式ABORT，应用后重复执行拒绝。CLI基础迁移闭环已具备；双语Web工作台现已在源码中接入同一服务，支持会话选择、全部REVIEW决定、应用/终止、媒体库映射、重建预检与执行。

### 阶段 5：可选所有者连续性状态

- 增加独立 owner continuity 分区。
- 在 Gallery 重建完成后，仅按导出时选择恢复生命周期和/或轻量收藏隐藏标记；不实现 Gallery 地址连续性。
- Gallery/Item评分由Manifest恢复；最后浏览时间、最后浏览项目和其他浏览历史保持目标系统本地轻量状态，不迁移。
- ACTIVE 请求重新执行环境校验，不恢复 Session、任务和调度租约。

门禁：不选择该分区时导入结果与阶段 3 完全一致；失败可独立重试。

当前阶段5源码已实现portable package format v2的可选`owner-continuity.json`：默认导出不包含该Entry，用户可分别选择Gallery生命周期和Gallery/Item收藏隐藏标记。Entry只以`set_id`/`item_uuid`引用ACTIVE身份并纳入checksums；v1包继续可读。它只能在会话达到`GALLERIES_REBUILT`后整批原子应用，ACTIVE请求重新执行当前激活门禁，失败不回滚已完成的核心导入与Gallery重建并可重试。写入只触及所选字段，明确保留Manifest负责的Gallery/Item评分和目标机既有浏览历史。CLI与双语Web工作台均调用同一服务并要求所有者重新认证及`CONTINUITY`确认词；本阶段不新增数据库schema。

### 阶段 6：产品化、恢复演练与文档

- 完成双语 Web/CLI、维护模式、取消/恢复、审计和操作手册。
- 完成“完整备份恢复”和“可移植重建”的选择指南。
- 在新 Linux 主机与 Docker 上执行真实迁移演练，再考虑其他平台。

门禁：从源机导出到全新目标机的整套演练可由文档独立复现，并能证明没有访问或修改源媒体。

当前状态：双语Web工作台已补齐导出就绪报告、导入包可选owner-continuity范围摘要、最近200条会话窗口以及`PORTABLE_IMPORTING`/`PORTABLE_MERGING`中断恢复入口；所有动作要求重新认证和精确确认，服务失败只返回不含路径/正文的动作级稳定码。CLI与Web共用同一服务、保留包和持久化会话。源码和自动化门禁已闭合；真实Linux→Docker迁移与恢复模拟由所有者在独立目标环境执行后，阶段6环境门禁才能最终标记通过。

## 12. 测试矩阵

- 确定性：同一快照多次导出、JSON NFC、稳定排序和摘要一致。
- 身份：全 kind 冲突、Alias 多级链、Tombstone、同名异 UUID、UUIDv4/v7、跨 Work Character。
- 关系：Coser/SocialAccount、Character/Work、Tag DAG、Gallery Credit/Cast/Tag。
- 资源：头像/Banner缺失、替换遗留、符号链接、格式伪装、像素/大小上限和原子发布失败。
- Gallery：旧 Manifest 缺 Item UUID、重复 set_id、重复相对路径、内容替换、缺失成员、DB_DIRTY、FILE_DIRTY 和 CONFLICT。
- 来源：DIRECTORY、ZIP/CBZ、TAR/TAR.GZ/TGZ、7Z、加密/危险/损坏存档和相邻 Manifest 丢失。
- 安全包：路径穿越、大小写/NFC碰撞、保留名、未知 Entry、压缩炸弹、截断 ZIP、摘要不符和版本不兼容。
- 原子性：预检失败、数据库提交失败、Coser 资源发布失败、进程强杀、取消、重试和过期会话。
- 隐私：包、GraphQL、日志和审计均不泄漏被排除的绝对路径、凭据或业务正文。
- 性能：至少覆盖 100,000 个核心实体、10,000 Gallery 和 1,000,000 Item 的清单/冲突检查，不把完整媒体内容载入内存。
- 端到端：源 Linux → 新 Linux、源 Linux → Docker，以及媒体根完全变化后的重建与再次 Push。

## 13. 首个实施切片

建议从阶段 0 和阶段 1 开始，首个纵向切片仅完成：

1. 从一致性快照导出身份账本、少量 Coser/Work/Character/Tag/SocialAccount 和当前 Coser 资源；
2. 生成不含绝对路径的 Gallery 覆盖索引；
3. 使用独立 inspector 校验包、摘要和安全边界；
4. 用金样本证明导出确定性和敏感信息排除。

该切片不导入正式数据库、不迁移 schema、不部署，也不改变 Gallery Manifest。通过后再进入空库导入，可把身份格式错误与数据库写入风险分开验证。

## 14. 完成定义

长期方案完成必须同时满足：

- 核心目录可在全新数据库中保留原 UUID、Alias、Tombstone、Slug 历史、关系和当前 Coser 资源；
- 媒体根变化后，Gallery 可依靠随媒体 Manifest 重建，并在无歧义时保留 set/item/link UUID；
- 缺失或冲突不会静默生成替代身份；
- 机器本地路径、任务、缓存和凭据不会混入可移植包；
- 完整备份恢复与可移植重建都有清晰、互不混淆的入口和文档；
- 所有导入写入均可预检、可审计、可取消/重试，并在失败时不留下部分业务状态。
