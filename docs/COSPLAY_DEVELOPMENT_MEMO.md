# Cosplay Gallery Manager 开发备忘录

> 状态：第一版产品与架构基线；0.1～0.7 主干功能已进入实现与联调
> 最后更新：2026-08-15
> 原始代码基线：Stash `develop` / `c7d2fe4f97b99c6a2aac968ac8a2aad3adf5b800`  
> 分支策略：独立产品，不考虑与 Stash 上游合并  
> 工作名：Cosplay Gallery Manager；最终品牌名延期决定  
> 许可证：GNU AGPLv3，保留 Stash 归属和对应源码提供义务

## 1. 文档地位

本文档是当前第一版开发的有效规范，覆盖此前讨论中与其冲突的旧结论。实现、Schema、GraphQL、前端和测试均以本文档为准。

“第一版”指达到 `1.0.0` 的功能范围。明确列入“延期”的内容不得在第一版中提前形成不稳定的半成品模型。

## 2. 目标、范围与非目标

### 2.1 产品目标

- 将 Stash 从单图片/单视频中心的媒体管理器改造成以 Gallery（作品集）为唯一最小业务单元的个人 Cosplay/原创 Album 收藏管理器。
- 一个 Gallery 对应一个物理来源，包含图片、动态图和视频，并统一管理 Coser、Character、Work、Tag、日期、分级和成员顺序。
- 保留并重构 Stash 的底层文件扫描、图片处理、FFmpeg、GraphQL、SQLite和任务能力。
- 新建 GalleryEpic 风格的浏览前台，同时重建适用于作品集聚合编辑的管理后台。
- 元数据仍以人工数据库编辑和本地 Manifest 为核心；1.5允许默认关闭、所有者显式触发、人工逐项确认且可完全拆除的Coser网络资料Provider，只导入托管头像、Banner和SocialAccount，不参与核心业务正确性。

### 2.2 明确非目标

- 不支持多用户、权限组、评论、社交互动或行为画像。
- 不支持原 Stash 数据库、旧 Tag、NFO、插件或 GraphQL 兼容迁移。
- 不保留独立 Image/Scene 业务页面、单媒体网络刮削或单文件物理删除。
- 不支持跨 Gallery 共享媒体身份。
- 第一版不实现自动网络刮削、社交账号联网检测、定时Provider任务、AI视觉推荐或向量数据库；1.5的Coser资料导入是用户确认后的可选外联例外。
- 除1.5已确认的所有者显式Coser资料导入外，不实现服务端主动外联、遥测、更新检查、CDN资源或运行时插件市场。
- 不实现应用内静态加密、内置HTTPS、外部数据库或实时文件监听。

## 3. 原 Stash Code View 结论

### 3.1 可复用基础

- 后端：Go；API：gqlgen GraphQL；数据库：SQLite；前端：React/TypeScript/Apollo/Vite。
- `pkg/image` 已有 FFmpeg/libvips 缩略图、按需生成和动画预览基础。
- `pkg/ffmpeg`、File流式响应、HTTP Range、路径和任务框架可复用。
- SQLite Repository、GraphQL Codegen、React Intl和Apollo网络层可复用。

### 3.2 必须替换的旧业务

- 旧 Gallery 是目录/归档派生的弱组合，图片为主要成员，Scene关系和人员关系不构成强聚合。
- 旧扫描可按目录自动创建 Gallery，但不是候选审核、单来源、严格状态机和Manifest驱动模型。
- 旧 Image 可通过指纹关联多个文件，Gallery与Image是软关系，不符合严格1:1成员身份。
- 旧清理、物理删除、单媒体页面、O计数、Studio/Performer旧语义、插件和Scraper入口全部移除。
- 原项目元数据主要存数据库；没有本项目所需的 Gallery级 `.cosplay.json` 三方同步机制，也不原生以传统NFO作为双向业务源。

### 3.3 前端重建边界

- 新建 `ui/web`：React 19、TypeScript、Vite、Apollo Client、GraphQL Codegen、React Intl和稳定React Router SPA/Data模式。
- BrowseShell与ManageShell在同一个应用中共享登录、路由、i18n和基础设计Token，但使用不同DTO和页面组件。
- 不在旧React 17/Bootstrap 4页面上继续叠加；功能对齐后移除旧 `ui/v2.5` 构建入口。
- 不使用SSR、React Server Components、Redux、远程字体或CDN。

## 4. 术语与核心不变量

- 代码、数据库和GraphQL内部继续使用 `Gallery`；中文前端显示“作品集/图集”。
- `Gallery` 是聚合根和唯一业务事务单元。
- `GallerySource` 是 Gallery 的唯一物理来源；Gallery为 `0..1` Source，只有无来源DRAFT可为0。
- `GalleryItem` 是成员业务身份；底层严格 `File ↔ Image/Scene ↔ GalleryItem` 1:1。
- 同内容但不同物理文件始终是独立Item；不同Gallery永不共享Item、收藏、评分、Caption、缓存身份或处理状态。
- 一个Gallery只有一个来源；不嵌套；来源内成员不能越过根目录或媒体库边界。
- Gallery内部名义类型不存字段：有GalleryCast即COSPLAY，无GalleryCast即ALBUM。
- GalleryCredit不区分Coser/Model角色；全部参与者都引用Coser。
- 应用不能删除、移动、改写任何用户媒体来源文件。

## 5. Portable UUID与名称身份

### 5.1 UUID Registry

- Gallery `set_id`、Item、Coser、Work、Character、Tag、ExternalLink、SocialAccount及UUID Alias全部进入全局Portable UUID Registry。
- UUID跨类型全局唯一；系统生成规范小写UUIDv4，导入合法v4/v7后保持。
- 合并后的旧UUID永久为Alias；删除后的UUID永久为Tombstone，均不得回收。
- Slug、数据库自增ID和BLAKE3不进入Registry。
- Manifest跨类型复用UUID时产生 `PORTABLE_UUID_KIND_CONFLICT`，绝不自动改写。

### 5.2 名称规则

- UUID决定身份，`name_hint`只用于提示和缺失实体首次命名。
- Coser、Work允许重名，Slug使用稳定短后缀。新建Coser在提交前以主名和Alias执行规范化精确匹配；若命中，显示头像、别名、UUID末段和Gallery数，可打开已有实体，仅在所有者明确确认为不同人物后才继续创建；该检查是防重复复核，不是唯一性约束。
- Character在同一Work内规范化主名称唯一；不同Work可同名。
- Tag主名称和Alias在Tag命名空间全局无歧义。
- 名称查重使用NFC、Unicode大小写折叠、首尾清理和连续空白折叠；不做简繁、标点或罗马音转换。
- 同UUID多Manifest名称提示一致时确定性采用；真正冲突集中人工选择主名称和Alias，不按扫描顺序决定。

### 5.3 Slug

- Slug是UTF-8、可读、稳定的前端地址，不是永久身份。
- 重命名不默认改变Slug；冲突使用稳定后缀。
- 保留轻量SlugHistory重定向；实体删除可清理，不承诺永久410。

## 6. 领域模型

### 6.1 Gallery

主要字段：

- `set_id`：创建时立即生成、永久、不可编辑；即使从不创建Manifest也必有。
- `state`：`DRAFT | ACTIVE | ARCHIVED`。
- `title`、受限Markdown `description`。
- 无时区日历 `shoot_date`及 `MONTH | DAY` 精度。
- `created_at`：技术记录创建时间；`added_at`：首次成功激活时间。
- `content_rating`：`NON_ADULT | ADULT`，DRAFT可空。
- `photographer_name`、`studio_name`：单值自由文本，仅展示和Manifest同步。
- `metadata_revision`、`scan_revision`。
- preferred/effective cover。

`added_at`规则：

- DRAFT为null，首次转ACTIVE写入UTC当前时间。
- 归档恢复、重扫、元数据修改、Manifest同步和来源故障均不刷新。
- 普通编辑不可改；管理端可审计地修正。

### 6.2 GalleryCredit与GalleryCast

- GalleryCredit：`gallery_id, coser_id, position`；同Gallery同Coser唯一。
- GalleryCast绑定具体Credit和Character；唯一键为 `(gallery_id, gallery_credit_id, character_id)`。
- 一个Credit可配多个Character；多个Credit可配同一Character。
- Credit按Gallery全局排序；Cast按所属Credit内部排序。
- ACTIVE COSPLAY要求每个Credit至少有一条Cast；ACTIVE ALBUM要求至少一个Credit且Cast为空。
- Manifest Cast必须引用已列出的Credit，并显式提供Coser、Character、Work UUID。
- Character现有Work与Manifest Work不一致时冲突，绝不静默移动。

### 6.3 Coser

第一版字段：

- UUID、name、sort_name、aliases、稳定Slug、metadata_revision。
- `profile_summary`：最多20,000字符纯文本，详情头部默认六行可展开。
- `biography`：最多20,000字符受限Markdown，作为扩展资料。
- `country_or_region`、avatar、banner、social accounts。
- 不含Favorite、通用Tag、Custom Field、认证状态或结构化身体资料。

SocialAccount：

- account_uuid、可扩展字符串platform_key、label、handle、HTTP(S) URL、ACTIVE/INACTIVE、visible、position。
- platform_key格式 `[a-z0-9][a-z0-9_-]{0,63}`；未知平台使用通用本地图标。
- INACTIVE不改变顺序，只原位弱化；visible=false时Browse不返回。
- 第一版仅定义未来检测Provider接口，不实现账号状态检测、定时任务或结果表。1.5新增的可拔除资料导入Provider只提供候选、头像、Banner和账号建议，与检测Provider分离。

### 6.4 Work与Character

- Work仅保留UUID、name、sort_name、aliases、Slug、metadata_revision和时间戳。
- Character在上述字段外只增加必填唯一主要Work关系。
- 不使用图片、描述、Tag、外链、结构化资料或Manifest。
- Work详情只显示相关Character，不显示Gallery；Character详情显示相关Gallery。

### 6.5 Tag

- UUID、name、sort_name、aliases、Slug、`use_in_recommendation`、metadata_revision和时间戳。
- 允许多父级但严格无环的DAG；单Tag直接父级最多50。
- 不使用图片、描述、Favorite、Custom Field、自动标记或Manifest。
- Gallery只绑定直接Tag；Tag详情包含自身及全部后代；多Tag筛选使用AND。

### 6.6 GalleryItem

- 永久 `item_uuid`、唯一source_id、底层Image或Scene 1:1、caption、category、position、可用性和处理状态。
- `media_kind = STATIC_IMAGE | ANIMATED_IMAGE | VIDEO`由实际内容派生，不写Manifest。
- 静态图 `image_category = PHOTO | SELFIE`；动态图和视频为null。
- Caption是最多1000字符纯文本，不搜索、不推荐、不扩展成单媒体完整元数据。

## 7. Gallery状态与可浏览性

### 7.1 业务状态

- DRAFT：允许缺来源、缺媒体和不完整元数据，不进入Browse。
- ACTIVE：显式激活；必须满足全部校验。
- ARCHIVED：保留全部数据，退出Browse；恢复时重新校验，不满足则回DRAFT。
- 编辑ACTIVE的Credit/Cast等关系可保存，但不满足时整次事务后转DRAFT，绝不静默删除关系。

### 7.2 ACTIVE校验

- 唯一Source存在；Source可用；无阻断Issue；未超限。
- 至少一个可直接显示或已READY的非排除成员。
- content_rating存在；标题可人工或按已确认关系规则生成。
- COSPLAY/ALBUM关系满足第6节规则。
- Manifest可选，NONE不阻止激活；未处理的Coser/Work/Character路径建议阻止激活，标题、日期和SELFIE建议不阻止。

### 7.3 来源三层健康模型

- `availability_state = AVAILABLE | MISSING | UNREADABLE`
- `reconcile_state = NEVER_SCANNED | SCANNING | IN_SYNC | NEEDS_RESCAN | ERROR`
- GallerySourceIssue可并存，严重度 `INFO | WARNING | BLOCKING`。
- `browsable`由ACTIVE、Source AVAILABLE、无阻断Issue、至少一个可展示Item共同派生。
- 来源不可访问时只修改Source并暂停浏览，不批量把成员改MISSING；恢复必须完成一次完整成功扫描。

### 7.4 Item状态

- 可用性：AVAILABLE、MISSING、UNREADABLE；处理：PENDING、PROCESSING、READY、ERROR。
- ERROR/PENDING保留并计入P/S/G/V，但不进入随机或effective cover；网格显示占位。
- MISSING/UNREADABLE不进入前台可用计数和网格，但保留全部业务状态。

## 8. 媒体分组、分类与排序

### 8.1 固定展示顺序

- PHOTO → SELFIE → GIF/其他ANIMATED_IMAGE → VIDEO。
- 前端详情将PHOTO和SELFIE放在同一“图片”视觉分组，内部仍先PHOTO后SELFIE。
- 默认展示全部；All/Photo/Selfie/GIF/Video筛选由全局设置开启，默认关闭。

### 8.2 SELFIE建议

- 新静态图默认PHOTO；人工或Manifest可正式设为SELFIE。
- 自拍识别使用独立的“媒体分类规则”，不复用Gallery根发现规则。后台可按全局或单一媒体库管理规则，匹配对象支持父目录段、文件名、去扩展名文件名和完整相对路径，操作符支持Exact、Glob和Go RE2。
- Exact/Glob每行一个模式并按OR处理；RE2只允许一个表达式。路径统一NFC和`/`，默认不区分大小写；RE2保存前必须由后端Go RE2引擎编译，前端当前表达式未通过后端校验时禁止保存，Glob同样由后端校验语法。
- 优先级按较小order在前；相同order时媒体库专属规则先于全局规则，再按规则ID。首个命中即停止，PHOTO结果可作为显式排除规则阻断后续SELFIE规则。
- 内置目录关键词以可编辑、可禁用、可删除并可显式恢复的数据库默认规则一次性播种，不在运行时硬编码；默认文件名规则保持关闭，避免误判。
- 命中只生成非阻断建议；接受后正式分类，拒绝后普通重扫不重复提示。
- 规则修改递增revision；同一Item对同一revision的接受/拒绝结果保持稳定，旧待处理建议会被更新规则或更高优先级命中标记为SUPERSEDED。只有STATIC_IMAGE参与，动画和视频永不生成PHOTO/SELFIE建议。
- 保存规则不批量改写现有Item；人工“评估现有媒体”只产生预览/建议，必须逐项或批量明确接受。人工与Manifest现值不会被扫描自动覆盖；不使用人脸、相机信息或AI自动定类。

### 8.3 自动排除规则

- GalleryItem自动排除使用独立于根发现及PHOTO/SELFIE分类的数据库规则；后台支持全局/媒体库作用域、父目录段、父目录路径、文件名、文件stem与完整相对路径，以及Exact、Glob和Go RE2。
- 规则结果为EXCLUDE或INCLUDE例外，按order、媒体库优先和ID确定首个命中；可限制STATIC_IMAGE、ANIMATED_IMAGE、VIDEO或全部媒体。路径只在NFC规范化Gallery相对字符串上匹配，不访问文件系统或执行Shell。
- 第一阶段仅应用于DIRECTORY新Item；Archive保持现有行为。根目录新媒体本次扫描开关继续优先，既有路径/唯一指纹Item的人工Exclude/Restore及Manifest结果不被重扫覆盖。
- 保存、编辑或删除规则不批量改写存量Item；显式存量评估只生成EXCLUDE待审核建议，接受后才改变状态。规则命中来源和revision持久化，但`gallery_items.excluded`仍是当前状态事实来源。
- 详细schema v5、扫描、API、UI和部署门禁见[可管理媒体自动排除规则实施计划](development/MEDIA_EXCLUSION_RULES_PLAN_2026-08-28.md)。

### 8.4 Position

- GalleryItem共用一套Gallery内全局唯一int64 position，但只在分类组内比较。
- 初始成员按固定组顺序及规范化完整相对路径自然排序；数字按数值比较，DIRECTORY和归档一致。
- 初始间隔1024；后续新增或重新分类追加到目标组末尾。
- 同组拖拽优先写中点；间隔耗尽时只把当前组按原顺序迁移到Gallery全局高水位后的新区间。
- Manage媒体页按完整父目录分组；Gallery根目录作为独立分组并固定排在该媒体类型的所有文件夹之前。
- 文件夹排序只在所属PHOTO、SELFIE、ANIMATED_IMAGE或VIDEO组内生效；移动文件夹时保持文件夹内部既有顺序。
- 按文件名自然排序必须由用户对具体文件夹显式触发，比较文件名而非完整父目录；整个媒体组顺序在单事务和单次metadata_revision递增中提交。

### 8.5 Credit/Cast Position

- 两者同样使用int64间隔值。
- Credit为Gallery全局顺序；Cast为所属Credit内部顺序。
- 卡片Character/Work摘要按Credit顺序、Cast内部顺序首次出现去重。

## 9. 来源、媒体库与发现

### 9.1 GallerySource

- 类型仅 `DIRECTORY | ARCHIVE`；一个Gallery恰好最多一个物理来源。
- DIRECTORY允许纯视频Gallery；所有成员必须在根内，递归发现但永不跟随符号链接。
- ARCHIVE支持ZIP/CBZ、TAR、TAR.GZ/TGZ和7Z；不能附加外部媒体；内部未排除Video、RAW或AVIF均为阻断错误。
- Gallery不嵌套；确认根后内部Marker/Manifest不得拆出新Gallery。
- DRAFT无来源时不能包含媒体。

### 9.2 媒体库边界

- 父子媒体库可并存；最具体根拥有来源，父扫描跳过所有配置子根。
- 删除或改根前必须预览；已有Source显式转移到父库或进入未分配状态，绝不静默重导。
- 禁用的子库仍是边界，直到明确移除。
- 媒体库可为只读或操作系统已挂载的SMB/NFS；应用不内置网络挂载客户端。
- SQLite数据库必须在本地磁盘/可靠持久卷，不放SMB/NFS。

### 9.3 根识别优先级

1. IgnoredGallerySource；
2. 已绑定GallerySource；
3. 有效Gallery Manifest；
4. 安全且包含受支持媒体的Archive文件本身；
5. 可选空文件 `.cosplay-root`；
6. PATH_TEMPLATE；
7. FIXED_DEPTH（DIRECT_CHILD是深度1预设）；
8. 人工绑定。

- 所有自动规则默认关闭；显式来源直接建DRAFT。
- 每条自动规则默认只生成Candidate，可逐规则开启AUTO_CREATE_DRAFT；仍只建DRAFT。
- Archive文件是内置确定性来源根，不依赖PATH_TEMPLATE或FIXED_DEPTH；手工发现默认只生成`ARCHIVE_FILE` Candidate，ASSISTED/TRUSTED可通过默认关闭的媒体库级开关授权自动创建DRAFT。有效相邻Manifest继续优先；加密、损坏、不安全或无受支持媒体的Archive进入覆盖诊断，超限Archive保留为受阻Candidate，二者均不得自动导入。
- 已确认的DIRECTORY来源或候选拥有完整子树，其内部Archive不得再形成嵌套Gallery；普通组织目录不阻止其中每个安全Archive分别成为候选。
- `.cosplay-root`所在父目录始终是DIRECTORY来源根；新候选若根内恰好一个直属真实子目录，标题保底取该子目录名，否则取来源根目录名。该规则不回写已有Gallery，不改变根级媒体默认Exclude，也不适用于ARCHIVE来源。
- 完全移除启发式候选、评分和证据Provider。
- 未归属媒体只指未命中DIRECTORY根的散装媒体，并按实际父目录聚合诊断；安全受支持Archive不得误列为未分配媒体文件夹。用户可从诊断项预填精确目录规则或使用其他人工根确认流程。

### 9.4 PATH_TEMPLATE

- 使用Go RE2匹配媒体库根下完整规范化相对路径，要求命名捕获和规则顺序预览。
- 可捕获title/coser/work/character/year/month，只生成结构化建议，不写正式元数据。
- Coser/Work/Character/日期/标题均需用户接受；拒绝Character建议后才可按Album激活。

### 9.5 三层忽略

- IgnoredGallerySource高于一切身份规则，优先用set_id，阻止已删Gallery重建。
- LibraryPathIgnoreRule仅影响未绑定路径发现，支持EXACT/PREFIX/RE2。
- GalleryItemExclusion只作用已绑定Source内部。
- 系统保留路径、Manifest、`.cosplay-assets`、临时和备份文件始终忽略。

### 9.6 候选审核

- `/manage/import/candidates`先确认物理根，再创建DRAFT；元数据建议进入Gallery编辑页处理。
- 批量仅允许无冲突、未超限、根明确的Candidate建DRAFT；不批量接受人物/角色/日期。
- Candidate和未归属报告只使用最近一次完整扫描快照。

### 9.7 Archive结构与资源安全

- 默认限制：总Entry 20,000、非排除Gallery成员1,000、单成员解压后2GiB、总解压估算100GiB、单图200MP、压缩比1000。
- 上述资源阈值可在后台调整；Gallery 1,000成员仍是产品硬上限。
- 路径穿越、绝对路径、Unicode/大小写重复、符号链接、硬链接、设备/特殊文件、加密Entry和嵌套归档等结构性校验不得关闭。
- CRC或成员读取失败产生明确Issue；不把归档完整解压到用户媒体库，也不递归处理内嵌压缩包。
- Archive中的Video、RAW和AVIF沿用阻断规则；必须明确排除，或解压为DIRECTORY后再激活。

## 10. 扫描、指纹与对账

### 10.1 扫描方式

- 只支持手工、可选启动和定时扫描；默认自动扫描关闭，最短15分钟，不做实时Watcher。
- 扫描只做发现与对账；缩略图、RAW、动画、Video进入独立持久化处理队列。
- 每次扫描有scan_run_id，观察结果先暂存；以单GallerySource完整成功为最小原子提交。
- 取消、网络中断或失败时丢弃未完成暂存，保留上一版成员状态，不产生半扫描MISSING。
- 用户发起扫描时默认把本次新发现的Gallery根目录媒体记录为excluded，并可在扫描前关闭该选项；子目录媒体仍默认纳入。该策略只作用新Item，按路径或唯一指纹识别出的既有Item继续保留人工Exclude/Restore决定。
- 移除原Stash Clean：`Reconcile Sources`只标记来源/成员状态，`Cache Maintenance`只清理应用生成的缓存、临时文件、轮换备份和明确托管的元数据资源。
- 应用任何路径都不能删除用户媒体来源文件；文件物理删除和移动完全由外部文件系统完成。

### 10.2 指纹

- 使用BLAKE3快速采样和完整内容指纹；完整格式 `blake3-v1:<hex>`。
- 同Source同路径优先保留item_uuid；内容变化记录CONTENT_REPLACED并重建派生资源。
- 原路径缺失且新路径完整BLAKE3唯一匹配时自动重绑定同一item_uuid。
- 多候选时不自动判断；旧Item保持MISSING，新文件建立独立Item，用户审核。
- 指纹永不跨Source/Gallery合并身份；pHash只可用于未来诊断/推荐。
- ZIP CRC只作快速筛选，完整匹配使用解压后内容BLAKE3。

### 10.3 来源移动与重复set_id

- 其他位置发现相同set_id只生成SOURCE_REBIND_CANDIDATE；实际改绑始终人工确认。
- 两个来源同时可访问且set_id相同产生DUPLICATE_SET_ID，不自动选主。
- 用户可把复制品显式“分叉为新Gallery”：重新生成set_id、item_uuid和link_uuid，保留业务内容与共享实体UUID，进入DRAFT。
- 分叉必须能原子写回副本Manifest；只读来源需用户外部修正后再扫。

## 11. 媒体格式与处理

### 11.1 发现与实际内容

- 默认图片候选扩展名：`png, jpg, jpeg, gif, webp, avif, jxl`，另加第11.2节RAW扩展名；默认视频：`m4v, mp4, mov, wmv, avi, mpg, mpeg, rmvb, rm, flv, asf, mkv, webm, f4v`。
- 扩展名集合可在后台增减；HEIC、SVG、音频和RAR不在第一版默认/支持范围；7Z只作为受安全校验的ARCHIVE容器支持。
- 最终使用签名、容器探测和实际解码分类；错配格式保留并警告，不自动重命名。
- 图片后缀实际为Video需用户确认后Gallery才可激活。
- 不支持SVG、音频和RAR；无后缀/未知扩展即使内容可解码也不发现。

### 11.2 RAW

- 使用固定版本LibRaw；DIRECTORY支持常见CR2/CR3/CRW、NEF/NRW、ARW/SR2/SRF、RAF、ORF/ORI、RW2/RWL、PEF、DNG、SRW、3FR/FFF、X3F等。
- 不默认启用含义模糊的 `.raw`；扩展名只发现，LibRaw确认内容。
- RAW为STATIC_IMAGE，默认PHOTO；原始文件永不修改，优先嵌入预览，必要时解码为sRGB代理。
- Archive内RAW阻断；LibRaw不可用产生RAW_DECODER_UNAVAILABLE。
- 同目录同主名RAW+JPEG保持独立Item，只生成伴生提示，由用户决定排除。

### 11.3 EXIF/XMP

- 只读辅助；不回写RAW/JPEG/XMP。
- 方向、尺寸、预览、ICC/色彩空间和拍摄时间用于处理；相机/镜头/GPS不进入第一版业务字段，GPS默认不持久化。
- Gallery无shoot_date时：至少80%同日建议日精度；否则至少80%同月建议月精度；否则只显示范围。
- 建议必须人工接受，绝不覆盖已有Gallery日期。

### 11.4 静态图片

- 卡片/网格派生480/960/1600响应尺寸；Lightbox和媒体详情默认4096长边代理。
- 浏览器兼容的JPEG、PNG和静态WebP在来源文件不超过20MiB、宽高均不超过4096时，经认证不透明资源接口直接查看原图；超过任一阈值、RAW或其他格式回落到按需4096代理。
- 原图直读支持DIRECTORY和受支持Archive Entry；必须继续校验ACTIVE/Browse可见性、content revision、实际文件签名和非符号链接边界，不向GraphQL或URL暴露物理路径。
- 正确应用方向、转换sRGB、保留Alpha、不放大。
- 默认最大解码像素200MP；第一版无裁剪写回、滤镜或图片编辑。

### 11.5 动画

- GIF/APNG/动态WebP/动态AVIF/动态JXL归ANIMATED_IMAGE，前端统一放GIF组但保留真实格式。
- 每项生成长期静态Poster；Gallery详情网格的可见动画项按需生成ENHANCED动画WebP：保持原动画完整时长、最长边480px、最高15FPS并循环，不得以固定秒数截断。
- Gallery详情动画播放安全上限默认12、允许在Manage Settings设为1～16。动画总数不超过上限时，所有进入视口的动画均可播放且不安装悬浮切换监听；超过上限时初始窗口取排序前N项，精确鼠标在动画项停留150ms后把窗口锁定到以该项为中心的连续N项，偶数N向右多取一项并在首尾夹紧。鼠标移开保持窗口；再次锁定须遵守默认800ms、可配置700～1000ms的切换间隔。
- 播放窗口只授予资格，实际播放仍须项目进入视口；Lightbox打开或`prefers-reduced-motion: reduce`时全部恢复Poster。无悬浮能力的设备按当前可见动画中位项移动窗口，避免后段动画永久无法播放。
- Gallery卡片混合媒体Scrubber始终只显示静态Poster，不使用上述动画预览。
- 仅视口内播放，离开即停；同页默认最多4个，可配置1～8；reduced-motion只显示Poster。
- Lightbox和单媒体详情对当前已识别的GIF/动态WebP经认证接口播放未经重编码的完整原动画；列表、Related、Scrubber等其他入口保持静态Poster。不提供进度、逐帧、速度或编辑。

### 11.6 Video

- 使用固定FFmpeg/FFprobe；原始只读。
- 浏览器兼容则直接Range播放；容器不兼容优先Remux；必要时生成H.264/AAC Fast Start MP4代理，默认最高1080p且不放大。
- 第一版只做基础播放：确定性选择default/首个可解码主视频轨和音轨，不提供音轨选择、字幕、画质档、预览精灵、360°或外部播放器。
- 正确应用旋转；代理将HDR Tone Map为SDR。
- 不持久化播放进度、观看状态、次数或时长。
- 1.5按两个实现阶段补齐该闭环：第一阶段完成FFmpeg/FFprobe诊断、产品自有技术元数据与约20%位置可靠Poster；第二阶段完成认证DIRECT Range路由、按需Remux/H.264-AAC代理及Lightbox/媒体详情播放。完整任务、数据、缓存、安全和测试规格见[视频处理第一、第二阶段功能规划](development/VIDEO_PROCESSING_PHASE_1_2_PLAN_2026-08-15.md)。
- Gallery列表、卡片曝光和Gallery卡片Scrubber不得触发原视频读取、Remux或转码；只有当前Lightbox视频或媒体详情实际打开才请求播放资源。
- Storyboard辅助时间轴和条件式单清晰度渐进HLS已形成[第三阶段后续规划](development/VIDEO_PROCESSING_PHASE_3_PLAN_2026-08-15.md)，但不加入当前第一版/1.5完成门禁；HLS必须先由第二阶段真实大视频指标证明必要并另行ADR确认。

### 11.7 复用Stash底层

- 复用FFmpeg/libvips封装、任务并发、流式响应、Range、按需生成和路径管理思路。
- 重建GalleryItem级 `MediaDerivativeGenerator`和资源路由；不复用旧Image业务API。
- 缓存键为item_uuid、内容revision、variant和processing profile；不能只按checksum共享。
- 失败时返回占位和明确状态，不回退加载不受控超大原图。

## 12. 派生资源、缓存与任务

### 12.1 双层缓存

- 基础展示资源长期保留：RAW基础代理、Video Poster、动画Poster；普通LRU不淘汰。
- 增强缓存：响应图、高分辨率代理、动画预览、Video Remux/代理等，按LRU清理。
- 默认增强缓存50GiB；最低磁盘余量 `max(10GiB, 5%)`，均可配置。
- 低空间先清增强缓存，仍不足则暂停新处理并报告DISK_SPACE_LOW，不删媒体。
- 备份不包含派生资源。

### 12.2 处理版本

- 每种资源由生成器代码、依赖兼容版本、配置和内容revision计算processing_profile_hash。
- 配置变化后旧资源STALE但继续服务，新版本后台渐进生成并原子切换。
- 内容替换后旧资源不得继续展示；安全问题可HARD_INVALID。
- 普通处理只更新技术状态/scan_revision，不更新metadata_revision。

### 12.3 持久化任务队列

- Job：Library Scan、Gallery Processing、Manifest、Cache、Backup；Item Task按variant执行。
- 状态PENDING/RUNNING/RETRY_WAIT/PAUSED/COMPLETED/FAILED/CANCELLED；租约+心跳，重启后恢复。
- 唯一任务键包含Item、Variant、内容revision和Profile；重复只提优先级。
- 默认失败重试3次指数退避；结构性不支持不重试。
- 优先级：当前查看、封面/卡片、ACTIVE首批、人工重试、ACTIVE其余、DRAFT/ARCHIVED。
- 完成明细默认保留30天；日志摘要受限。

### 12.4 Gallery卡片混合媒体Scrubber资源

- 继承Stash“横向位置映射成员序号、离开恢复封面”的交互思想，但不复用旧 `/gallery/{id}/preview/{imageIndex}`、旧Image查询或数字ID接口。
- 可预览序列只包含非排除、文件AVAILABLE且已有可安全展示基础资源的GalleryItem；MISSING、UNREADABLE及无可用Poster/代理的PENDING/ERROR成员跳过。
- 序列顺序与Gallery详情一致：PHOTO→SELFIE→ANIMATED_IMAGE→VIDEO，各组内按Position；横向坐标映射该完整可预览序列的ordinal。
- PHOTO/SELFIE使用卡片响应式派生图，RAW使用LibRaw基础代理，ANIMATED_IMAGE和VIDEO都只使用长期静态Poster；Scrubber中绝不播放动画或Video，也不触发Remux/转码。
- RAW仍是STATIC_IMAGE并按其PHOTO/SELFIE分类进入对应顺序和P/S计数，不建立独立RAW展示分组。
- 预览资源使用item_uuid、内容revision、variant和processing profile缓存；不得回退加载不受控原图。
- BrowseGalleryCard不得携带全部成员URL；通过新的GalleryItem级认证预览资源契约按ordinal获取，并可缓存Gallery的可预览item_uuid序列，避免沿用逐次SQL `OFFSET` 查询。
- 预览请求必须支持过期请求取消、客户端节流和已访问ordinal缓存；Gallery无可预览成员时Scrubber保持无效，卡片继续显示effective cover。

## 13. 封面

- 来源：静态GalleryItem、Video帧、独立自定义封面资源。
- DIRECTORY托管路径 `.cosplay-assets/cover.*`；ARCHIVE使用相邻 `<完整归档名>.cosplay-assets/cover.*`。
- 自定义封面只允许JPEG/PNG/静态WebP；Manifest引用相对路径。
- 详情静态图菜单可立即设封面：更新数据库、metadata_revision、Manifest待Push；保留旧独立封面并支持撤销。
- 首次成员发现后无显式封面时随机一次可用静态图片并持久化AUTO_RANDOM。
- preferred_cover保存人工/Manifest意图；缺失、排除或不可用时effective_cover使用持久随机静态图替代，preferred恢复后自动恢复。
- AUTO_RANDOM缺失时重新初始化；纯Video/无静态图不随机，可用Poster作为临时effective但不写随机意图。
- 用户清空/重置为自动时重新触发初始化。

## 14. 排除、MISSING与硬上限

- 来源内支持媒体默认自动纳入；人工排除写数据库和Manifest，重扫不得恢复。
- DIRECTORY中新发现媒体可由已启用的数据库自动排除规则设置初始状态；排除决策必须在1000上限和处理任务排队前统一计算。规则不覆盖既有Item，Archive第一阶段不应用。
- 排除已有Item保留GalleryItem、UUID、分类、Position、Caption、评分和收藏，状态EXCLUDED，不进入Browse、计数、封面、随机或1000上限。
- Manifest Push中被排除已有Item同时存在于 `items[]` 和 `excluded_items[]`；后者可含item_uuid。
- 恢复沿用同一Item；Position冲突时追加目标组。
- “忘记成员记录”仅允许EXCLUDED或MISSING，明确丢失元数据并Tombstone UUID；绝不删源文件。
- 单Gallery非排除成员1000为硬上限，包含MISSING/ERROR/PENDING；超限Source进入OVER_LIMIT并退出Browse/禁止激活。
- 全库10,000 Gallery、1,000,000 Item是性能保障范围，不是硬上限。

## 15. Manifest

### 15.1 路径与可选性

- DIRECTORY：根内 `.cosplay.json`。
- Archive：相邻 `<完整归档文件名>.cosplay.json`；永不因写Manifest重打包归档。
- Gallery和Coser Manifest均可选；`NONE`与曾同步后丢失的`MISSING`明确区分。
- 首次发现自动读入DRAFT；后续扫描只检测状态。后续Pull/Push必须用户显式触发。

### 15.2 同步状态与冲突

- `NONE | CLEAN | DB_DIRTY | FILE_DIRTY | CONFLICT | MISSING | ERROR`。
- 保存基线快照、文件哈希和revision，执行逐字段三方比较；双方人工修改同字段且结果不同不自动覆盖。
- 集合稳定键：credits=Coser UUID；cast=Coser+Character；tags=Tag UUID；links=link_uuid；items=item_uuid；exclusions=item_uuid/指纹/路径。
- 不同成员或不同子字段可自动合并；删除-修改冲突必须人工处理。
- name_hint不覆盖正式实体名称；media_kind不参与同步。
- 任一冲突未解决时不能最终Pull/Push。

### 15.3 文件规则

- `schema_version`管格式；`revision`只在系统Push时递增；哈希检测外部修改。
- 仅`extensions`允许未知扩展字段；普通未知字段报错；只保留一份`.bak`。
- 原子临时写、fsync和rename；写失败不得标为CLEAN。
- 路径NFC、正斜杠、相对路径，禁止绝对路径、`..`、控制字符、路径穿越；DIRECTORY不跟随符号链接。
- Gallery Manifest最大16MiB；Coser最大2MiB；超限不部分解析。

### 15.4 Gallery Manifest v1

必需顶层身份：schema_version、revision、set_id、updated_at。业务字段：

- title、description、shoot_date、content_rating、rating；
- photographer_name、studio_name；
- credits、cast、tags、external_links；
- cover、items、excluded_items、extensions。

不保存：collection_type、state、slug、added_at、favorite、hidden、history、view_count、media_kind。

- 手写Manifest可部分提供；缺字段=不修改，显式null=清除；系统Push输出完整快照。
- 初次导入items可凭精确相对path省略item_uuid，系统创建并首次Push补全；建立基线后既有Item更新必须用UUID。
- ExternalLink/SocialAccount首次导入或明确新增可省略子UUID，由系统生成；既有记录更新必须用UUID。
- Credits/Cast/Tags实体UUID、Gallery set_id和Coser UUID不可省略。

### 15.5 实体创建

- 有效Gallery Manifest可按严格UUID和名称提示创建缺失Coser、Work、Character、Tag；关系冲突不静默补全。
- Cast引用Coser必须已在Credits；Character现有Work必须匹配。
- 同UUID不同名称提示集中审核；相关Gallery保持DRAFT直至解决。

### 15.6 Coser Manifest v1

- 单一可配置Coser元数据根：`<uuid>/coser.json`及相对资料资源。
- 字段：schema_version、revision、coser_uuid、updated_at、name、sort_name、aliases、profile_summary、biography、country_or_region、avatar、banner、avatar_crop、banner_focal_point、social_accounts、extensions。
- avatar_crop为归一化合法1:1矩形；banner_focal_point为归一化x/y；更换原图后清空旧裁切。
- Coser Manifest可创建数据库缺失Coser；无Gallery关联时仅Manage可见。
- 合并后的旧Coser目录使用独立 `redirect_to_uuid` 文件。
- Work/Character/Tag完整资料只在数据库；Gallery Manifest只存UUID和名称提示。

## 16. Coser托管资源

- 头像/Banner仅本地JPEG/PNG/静态WebP；实际内容校验，不接受SVG、动画、AVIF/JXL、RAW或远程URL。
- 默认单文件20MiB、50MP；原图存 `<coser_root>/<uuid>/assets/...`。
- 头像生成1:1裁切和卡片派生；Banner按焦点生成响应图。
- 无头像使用名称首字符稳定占位；无Banner隐藏区域；不借用Gallery封面。
- 缺失资源保留Manifest意图并警告，不阻止Gallery浏览。
- 替换旧资源进入未引用托管资源列表，不立即删除；完整备份包含必要Coser资料资源。

## 17. 个人状态与内容分区

### 17.1 GalleryPersonalState

- 无user_id的Gallery 1:1：favorite、favorited_at、rating_half_steps、hidden、last_viewed_at、last_item_id。
- 评分五星制、半星，数据库1～10整数，null未评分；评分同步Manifest。
- Favorite/Hidden/History只存数据库。
- 完全移除Gallery浏览次数和view_count，MVP也不记录。

### 17.2 GalleryItemPersonalState

- 无user_id的Item 1:1：favorite、favorited_at、rating_half_steps。
- Item评分同步Manifest；Item收藏只存数据库；与Gallery收藏/评分独立。
- MISSING/排除/同UUID重绑定保留状态；显式忘记Item时删除。

### 17.3 浏览历史

- 打开Gallery详情、`/media/:item_uuid`或首次打开Lightbox更新所属Gallery last_viewed_at。
- 卡片曝光、资源加载、GIF自动播放和菜单不记录。
- Lightbox切换只更新last_item_id，不反复更新last_viewed_at；五分钟内写入去抖。
- MISSING媒体页不更新；清历史同时清last_item_id，不影响收藏/评分。

### 17.4 LIST/MAGIC

- NON_ADULT进入LIST，ADULT进入MAGIC；成人卡片使用小型三角R-18徽标。
- 内容范围贯穿搜索、推荐、时间线、随机、收藏和历史。
- Work/Character/Tag统一详情默认ALL，可显式切All/List/Magic，不继承入口。Coser详情改用作品类型筛选，不再使用内容分级筛选。
- LIST/MAGIC是浏览组织，不是权限边界；所有页面仍需唯一所有者认证。

## 18. BrowseShell 信息架构

### 18.1 路由

- `/` 独立近期收录首页；默认来源LIST，可后台切LIST/MAGIC/ALL。
- `/list`、`/magic`及各自 `/cosplay`、`/album`、`/cosers`、`/parodies`、`/characters`、`/tags`、`/timeline`、`/random`。
- 统一详情：`/gallery/:slug`、`/media/:item_uuid`、`/coser/:slug`、`/parody/:slug`、`/character/:slug`、`/tag/:slug`。
- 收藏与历史显式支持LIST/MAGIC/ALL，默认LIST。
- 不保留旧Stash业务路由。

### 18.2 首页

- 只显示按 `added_at DESC, id DESC` 的近期收录Gallery，默认24项。
- 无任何推荐、热门、最新Cosplay/Album或近期Coser模块。
- 无结果时隐藏内容区；查看更多进入相应Gallery索引。

### 18.3 BrowseGalleryCard

- 统一3:4封面；超宽/桌面/小桌面/平板/手机按6/5/4/3/2列响应。字段为id/slug/title、派生类型、content rating、cover、Credit/Cast/Work摘要及总数、shoot date precision、added_at、P/S/G/V、favorite、rating。
- 成人三角徽标；不展示管理状态、路径、Tag全文、总大小或热门数据。
- P/S/G/V只统计AVAILABLE原始成员；处理ERROR/PENDING若文件可访问仍计数，MISSING/排除不计；独立封面不计。

### 18.3.1 Gallery卡片快速预览

- 所有使用BrowseGalleryCard的Gallery网格统一支持混合媒体静态Poster Scrubber；随机页和媒体收藏页的单媒体卡片不使用该功能。
- 在支持Hover且为精确指针的设备上，光标进入封面预览区后，横向位置映射第12.4节的成员ordinal并替换封面内容；离开后立即恢复effective cover，卡片比例、文字区和布局不得变化。
- PHOTO、SELFIE和RAW显示响应式图片/代理；GIF等ANIMATED_IMAGE与VIDEO只显示静态Poster，不播放、不计浏览历史，也不记录媒体观看状态。
- 预览轴和进度提示必须弱化并融入GalleryEpic风格，不使用旧Stash醒目的红色样式；R-18徽标、收藏/评分控件和卡片主点击区域不能被遮挡。
- `gallery_card_scrubber_enabled`为全局数据库设置，默认true；后台可开关。关闭后不挂载Scrubber交互层、不请求预览索引或资源，并只显示effective cover。
- 触摸设备不模拟鼠标Scrubber，维持正常卡片点击/菜单行为；该增强功能不是访问Gallery内容的唯一入口。
- 卡片曝光、Scrubber激活、ordinal切换和Poster加载均不更新last_viewed_at；只有既定的Gallery详情、媒体详情或Lightbox有效打开才记录历史。

### 18.4 Gallery索引

- `/list`和`/magic`默认最近收录；可按shoot_date新旧、名称、个人评分排序；搜索时默认相关度。
- 不提供热门、浏览数、趋势或随机排序。
- 筛选Coser、Work、Character、多Tag AND、媒体类型、shoot/added范围，写入URL Query。
- Gallery及个人列表默认24项/页，页码分页。

### 18.5 Gallery详情与Lightbox

- 详情一次返回最多1000项完整轻量成员索引，不做服务端成员分页。
- 视觉分为图片、GIF、Video；每个非空组首次渲染24项，各组手工“加载更多”24项，无无限滚动。
- 图片懒加载；完整索引不等于加载全部媒体资源。
- Lightbox深链接 `/gallery/:slug?item=<item_uuid>`；基于完整可展示索引导航，与网格展开无关。
- 顺序Photo→Selfie→GIF→Video；当前媒体筛选启用时只遍历筛选结果；首尾不循环。
- 关闭Lightbox自动展开并定位尚未渲染的Item；返回键先关Lightbox。
- Lightbox停止上一动画/视频，只预取相邻图片代理，视频只预取Poster。
- 详情“拍摄时间”为主、“收录于”为次，不显示文字标签，以图标、字体、字号、颜色区分，并保留Tooltip/aria-label。
- 总大小只统计当前AVAILABLE Item实际存储大小；归档用压缩后成员大小。
- 已认证单所有者可在详情“更多详情”中查看该Gallery实际存在媒体的去重绝对父目录，并直达其Manage媒体页；这是Browse物理路径约束的有限例外。DIRECTORY包含被排除但非MISSING的Item，根目录优先、其余自然排序；ARCHIVE只显示归档文件所在目录。不得返回文件名、Item相对路径、指纹、缓存路径，也不得提供`file://`、文件管理器或外部命令入口。
- “更多详情”点击弹层外或按`Escape`关闭，弹层内部操作不得误关闭。

### 18.6 媒体详情与媒体卡片

- 随机页/媒体收藏页左键进入 `/media/:item_uuid`；右键或三点进入所属Gallery。
- 媒体详情桌面为媒体主体+Gallery侧栏，移动端上下布局；无前后切换、无单媒体完整业务元数据。
- 展示收藏、评分、种类、Caption、尺寸/时长/codec及所属Gallery摘要；无路径、指纹、删除。
- 随机/收藏媒体卡片3:4，不显示Gallery标题，只显示Character行+Coser行；Album保留空Character行。
- GIF只在视口预览；Video只显示Poster。

### 18.7 个人控件

- 完整Item收藏和半星评分集中在Gallery Lightbox与媒体详情。
- 卡片只显示轻量收藏/评分摘要；随机卡可快速收藏，评分进入详情。
- 三组全局设置：media_card_personal_controls(HIDDEN/SUMMARY/INTERACTIVE，默认INTERACTIVE)、media_lightbox_personal_controls、media_detail_personal_controls。
- 隐藏只影响Browse，不删除数据、不改变API权限；收藏页始终保留移出收藏操作。

### 18.8 Coser、Work、Character、Tag

- Coser详情默认显示该人物全部COSPLAY与ALBUM，保留个人介绍、Biography、社交账号和专属时间线按钮；作品筛选固定为“全部作品 / COSPLAY / ALBUM”。COSPLAY使用内容范围ALL，因此等于LIST与MAGIC合集；ALBUM继续展示全部内容分级。Model详情保持既有ALBUM专用列表，不增加该筛选。
- Coser卡片使用1:1头像、6/5/4/3/2列，30项/页；Work/Character/Tag为无图片高密度文字索引，5/4/3/1列，60项/页。
- Work详情只显示当前选择范围内至少关联可见Gallery的Character；不显示Gallery。
- Character详情显示相关Gallery网格；Tag详情包含自身及全部后代Gallery。
- 默认名称排序；可用最近收录和搜索相关度；彻底无热门排序。

### 18.9 时间线

- 只包含有月/日精度shoot_date的Gallery；未知和年精度跳过。
- 排序：shoot_month DESC、timeline_sort_date DESC、added_at DESC、id DESC；月精度排序日视为1号但UI只显示YYYY-MM。
- 默认24项，可配置12～120且为12倍数。
- Coser详情可进入该Coser专属时间线，仍默认ALL并可切分级。

### 18.10 随机

- 单页默认24项；STATIC/GIF/VIDEO软配额70/10/20，可配置；PHOTO/SELFIE只作筛选。
- 先按类型槽位，再等权抽Gallery，再抽Item；同Gallery可重复，不同Item不得重复。
- Gallery每入选一次后权重乘默认0.25衰减，可配置0～1。
- seed+配置版本保证同页稳定；不区分Cosplay/Album；仅READY/可直显Item。

### 18.11 收藏与历史

- `/favorites?tab=galleries|items&scope=...`；Gallery按favorited_at，Item卡复用随机卡，支持评分排序和分页。
- 取消Item收藏支持短暂撤销并保留原favorited_at。
- 历史按last_viewed_at；可清当前页/当前范围，不记录媒体级浏览历史。

## 19. 搜索与推荐

### 19.1 搜索

- 搜索Gallery标题；Coser/Work/Character/Tag主名、sort_name和Alias；Gallery可通过关联实体间接命中。
- 不搜Item、Caption、路径、文件名、Description、Biography、URL或技术字段。
- 相关度：主名完全、Alias完全、主名前缀、Alias前缀、主名包含、Alias包含、Gallery关系间接。
- 同等级实体按名称/UUID，Gallery按added_at/id；不使用热度、浏览、收藏或评分加权。
- 搜索框显式显示Home/List/Magic/All范围；实体必须至少关联当前范围可见Gallery才出现，点击统一详情后默认ALL。
- 下拉每实体最多5项；完整搜索按Gallery24、Coser30、Work/Character/Tag60独立页码分页。

### 19.2 强关系相关推荐

- 分数：Character×100、Work×50、同Coser×40、同派生类型×5；Studio权重完全移除。
- 候选至少共享Character、Work或Coser；同类型本身不足；跨Cosplay/Album允许通过共享Coser。
- 多角色/多人不在MVP重复累加；LIST/MAGIC硬边界；返回可解释Reasons，默认6项。
- 无候选不回退。

### 19.3 Tag猜你喜欢

- 只用Gallery直接Tag及最多3层祖先；父层因子p默认0.25，p^depth，可配置。
- 同祖先多路径取最大贡献；`use_in_recommendation=false`排除。
- 使用IDF加权Jaccard（sum min/sum max）；最低分0.05可配置。
- 不要求直接共同Tag；允许Cosplay/Album互荐；LIST/MAGIC硬边界。
- 无结果时详情区明确显示“近期收录”并排除已展示最新内容。
- Provider契约预留未来AI视觉相似度，但第一版不实现Embedding或外部模型。

## 20. ManageShell

### 20.1 Gallery索引

- 顶部队列摘要：Candidate、DRAFT、激活阻断、Source问题、媒体错误、Manifest冲突、SELFIE建议、RAW伴生、OVER_LIMIT、ARCHIVED。
- 下方高密度表格，覆盖标题、类型、分级、状态、Coser/Character、P/S/G/V、Source、Manifest、问题、日期和扫描时间。
- 批量仅允许分级、Tag、归档、重扫、重试、Manifest Push及完全通过校验的激活。
- 无强制激活、批量静默接受实体建议、冲突覆盖、跨Gallery拖媒体或磁盘删除。

### 20.2 Gallery五页签

1. 基本信息；
2. 人物与角色；
3. 媒体成员；
4. 来源与扫描；
5. Manifest。

- 顶部固定状态栏显示状态、派生类型、LIST/MAGIC、revision、阻断数、前台预览、保存和状态操作。
- 复杂业务字段使用草稿+显式Gallery范围批量保存；收藏、评分、封面、同组排序、排除/恢复、处理重试即时生效。
- 即时操作后客户端刷新自身expected revision，不丢未保存表单；外部revision变化仍冲突。

### 20.3 Coser四页签

1. 个人资料；
2. 社交账号；
3. 关联作品集；
4. Manifest。

- 资料和账号显式批量保存；Gallery关系只读跳转编辑；合并为独立高风险流程。
- Coser索引显示头像、名称、Gallery统计、账号数、Manifest、最近收录和问题。

### 20.4 Work/Character/Tag管理

- 使用最小字段表单、高密度索引、关系预览和合并/删除检查。
- Work编辑关联Character；Character编辑主要Work及只读Gallery；Tag编辑父子DAG及影响预览。

### 20.5 路径与外部命令

- Manage可显示并复制相对/绝对路径；Docker可保存仅展示用host_path_hint。
- 不调用Explorer/Finder/xdg-open，不提供External Player、Shell Hook、file://或任意外部命令。

## 21. 编辑、版本与实体生命周期

### 21.1 乐观锁

- Gallery、Coser、Work、Character、Tag均有独立metadata_revision。
- Coser额外维护Manifest revision/hash/baseline；Work/Character/Tag无完整Manifest。
- Mutation必须带expected revision；不匹配拒绝并显示差异。
- Gallery扫描技术修正只改scan_revision；个人Favorite/Hidden/History不改metadata，Gallery/Item评分因同步Manifest会改。
- 产品设置使用统一settings_revision。

### 21.2 合并

- 所有实体采用源→目标完整预览；全部冲突解决后原子提交，不支持拆分。
- 目标UUID保留，源UUID为永久Alias；源Slug重定向；受影响Gallery标记Manifest待Push。
- Coser合并人工处理Profile/资产/同Gallery Credit冲突；源Manifest变redirect。
- Work合并处理同名Character；Character跨Work合并以目标Work为准并预览Gallery变化；Tag合并模拟DAG并禁止环。
- 合并不修改Gallery added_at和个人状态，不移动媒体。

### 21.3 删除

- Gallery必须先ARCHIVED才能删数据库记录；重新输入密码和确认短语。
- 删除Gallery业务/个人/Item记录，Tombstone set/item/link UUID，建立IgnoredGallerySource，取消任务；源文件、Manifest和托管来源资源原样保留。
- 无应用内回收站；Archive承担可逆需求，删除恢复依赖备份。
- Coser/Work/Character/Tag仅完全无引用时可删；UUID永久Tombstone，旧Manifest不得静默重建。
- Coser资料目录默认保留，进入已删除资料待处理队列。

## 22. ExternalLink与文本安全

- GalleryExternalLink类型 `SOURCE | PROFILE | REFERENCE`，字段link_uuid/type/label/url/position。
- 只允许绝对HTTP(S)，同Gallery规范化URL唯一；不抓取、检查、下载、代理或favicon。
- 详情元数据末尾弱化显示，默认第一条SOURCE，其余折叠；新标签+noopener noreferrer。
- Photographer/Studio为最多200字符单值自由文本，仅详情和Manifest；不搜索、筛选、推荐或建实体。
- Markdown只用于Gallery description和Coser biography：段落、H2起标题、强调、列表、引用、代码、表格、HTTP(S)链接；禁止原HTML、图片、iframe、embed、style和危险协议。

## 23. 长度与结构安全限制

- 名称、sort_name、Gallery标题、Alias：300字符；每实体Alias最多100。
- Label 100；Handle 200；URL 2048；Country/Region 100。
- Gallery description、Coser profile_summary、Coser biography各20,000；Caption 1000。
- Gallery Credits 100、Cast 500、直接Tags 200、Links 50；Coser SocialAccounts 50；Tag直接父级50。
- Gallery非排除Item 1000硬上限；Manifest大小见第15节。
- 按NFC后Unicode码点计数；超限拒绝且返回具体路径，不静默截断。

## 24. 认证与资源安全

### 24.1 单所有者认证

- 无User表、用户名、邮箱、角色或API Key；首次只设置单一Argon2id密码。
- 登录页仅密码；服务端随机Session Token，数据库仅哈希，HttpOnly、SameSite=Strict，HTTPS有效时Secure。
- Session默认30天，可配置1～90；改密码/恢复/注销全部设备撤销全部Session。
- 登录限速、CSRF、Origin和Host校验。
- 本地可信模式显式开启后绕过认证但保留密码/Session；关闭即恢复，界面持续警告。
- 忘记密码使用本机一次性Recovery Token，默认10分钟、单次使用。

### 24.2 首次Setup

- 五步：环境、所有者认证、界面/时间、存储位置、确认创建；完成后手工启动首次扫描。
- 原生loopback可直接Setup；Docker/非loopback必须从服务器CLI生成15分钟单次Setup Token。
- Token换短期HttpOnly Setup Cookie后从URL移除；Setup完成后入口永久失效。

### 24.3 媒体资源

- item_uuid仅定位，不是凭证；全部Browse/Manage媒体资源统一认证。
- 路由只接受UUID和variant白名单，校验Source归属、路径边界、状态和可见性，不接受任意路径。
- Browse和Manage使用不同可见性规则；除第18.5节已确认的认证单所有者Gallery详情父目录摘要外，Browse DTO永不返回路径、指纹或技术详情。该摘要只聚合文件夹路径，不扩散到卡片、索引、成员轻量DTO或资源URL。
- MIME正确、nosniff、inline、private cache；带不可变revision派生资源可长期缓存；原图/视频支持Range。
- 不生成永久公开、无认证签名或CDN URL。

### 24.4 HTTP与健康检查

- 应用只提供HTTP，默认监听127.0.0.1；HTTPS由受信任反向代理/VPN处理。
- 仅信任配置CIDR的Forwarded Header；非loopback明文HTTP持续警告。
- `/healthz`、`/readyz`无需认证但只返回最小状态；Docker只用healthz判断存活。

## 25. 配置、数据库与部署

### 25.1 两层配置

- 启动配置文件：数据库/数据/缓存路径、监听、日志、依赖路径覆盖、代理基础配置和紧急启动项。
- 数据库产品设置：媒体库、Coser根、识别/忽略规则、扫描、格式、媒体限制、缓存、首页、控件、Gallery卡片Scrubber、推荐、随机、语言、时区、Session和审计。
- 不允许同一设置双写；settings_revision乐观锁；恢复异机时绝对路径进入待映射。

### 25.2 SQLite

- 第一版仅本地SQLite WAL，不支持PostgreSQL/MySQL。
- Foreign Keys、Busy Timeout、受控短写事务和有限读池；默认synchronous=NORMAL，可改FULL。
- 在线备份API；定期被动Checkpoint，VACUUM仅手工维护。
- integrity_check失败进入只读恢复模式，不迁移/扫描写入。
- 数据库写唯一product_id；原Stash或未知非空数据库拒绝启动，绝不原地转换。

### 25.3 平台

- 第一版正式支持Linux amd64/arm64与Docker双架构；CPU处理为验收基线。
- Docker包含固定FFmpeg/FFprobe和LibRaw；libvips可选加速。
- Windows原生支持整体延期；第一版不承诺macOS原生、32位、GPU硬件转码或移动原生App。
- Manifest和备份在正式支持平台间可迁移。

### 25.4 离线与插件

- 服务端核心流程零主动外联；前端资产全部本地；核心功能断网可用。1.5 Coser资料导入默认关闭，只在所有者显式操作时外联，移除全部Provider后产品仍可完整编译和使用。
- 第一版彻底移除原Stash插件市场、执行入口、旧Hook、Scraper和主题兼容。
- 只保留内部代码级Recommendation、Account Check和Derivative Generator接口。
- Manifest `extensions`只保存JSON，不执行代码。

## 26. 时间、语言与路由

- UI支持zh-CN和en-GB；元数据只有主名称、sort_name和aliases，不维护全文翻译。
- shoot_date是无时区日历值；事件时间全部UTC/RFC3339，前端按全局IANA显示时区。
- EXIF无Offset按媒体库capture_timezone，未设则全局时区解释，只用于日期建议。
- 路由不本地化；前端文案中文“作品来源”，URL使用`parody`。

## 27. 备份与恢复

- 每日SQLite在线快照，默认保留7份；Schema升级前强制快照。
- 手工完整包包含数据库、Coser元数据/资料和必要配置，不含媒体、Gallery sidecar、缓存和日志。
- 第一版备份不加密，明确提示；依赖OS权限和BitLocker/LUKS/FileVault等磁盘加密。
- Web与CLI共用维护模式恢复：重新认证、包Hash/产品/Schema校验、临时安全解包、替换前安全快照、失败自动回滚。
- 恢复后撤销Session，取消旧可执行任务，自动计划SUSPENDED_AFTER_RESTORE；完成路径/依赖检查后用户显式恢复。
- 搜索索引、Tag闭包、推荐缓存和计数可重建；不自动扫描或Manifest Pull/Push。

## 28. 日志与审计

- 默认INFO只记录技术ID短前缀、Library ID、错误码、数量和耗时，不记录路径、标题、Coser、Caption、URL、Token或Manifest正文。
- 原始媒体工具stderr只进受限轮换诊断日志；UI显示清理摘要。
- DEBUG需显式开启，默认24小时恢复INFO，并警告可能含文件名；仍不记录凭据。
- 日志默认20MiB×5轮换；备份不含日志；故障包先预览并匿名化。
- AuditLog无用户画像，只记录高影响操作和任务摘要；不记录浏览、搜索、收藏和评分。

## 29. API架构

- 保留GraphQL，但管理API与Browse API分层；Browse只使用Gallery级专用Query和白名单DTO。
- 主要Browse Query：browseGalleries、galleryBySlug、galleryMemberIndex、entityBySlug、timeline、recommendations、randomItems、search。
- BrowseGalleryCard只返回Scrubber可用数量/版本等轻量状态，不内嵌全部预览URL；专用认证资源接口以Gallery UUID、ordinal和不可变revision定位对应GalleryItem派生图/Poster。
- Scrubber资源沿用Browse可见性校验，不接受物理路径，不复用旧Gallery Preview API；设置关闭时前端不得请求，后端仍不得因此绕过正常资源授权。
- 所有索引服务端分页；默认Gallery/个人列表24、Coser30、Work/Character/Tag60；随机单页；Gallery详情成员不分页。
- Manage Coser主从编辑列表默认30项并可切换60/100项；名称、Sort name、Alias搜索和头像/Banner完善度筛选必须在服务端分页前作用于全库，搜索、筛选、页量、页码和当前实体保留在URL。Manage不使用仅过滤当前页的前端假搜索或无限滚动。
- Gallery业务Mutation均以Gallery为范围并带expected_metadata_revision；扫描技术Mutation独立。
- Coser/Work/Character/Tag和Settings Mutation同样带各自revision。
- 前后端同包发布，第一版不支持跨版本前端/后端混用。

## 30. 性能基线

- 参考硬件：4核CPU、8GiB内存、SSD、SQLite WAL。
- 性能范围：10,000 Gallery、1,000,000 Item；单Gallery1000硬上限。
- p95目标：主页/列表/详情/实体≤500ms（建议300ms）、搜索≤500ms、时间线≤500ms、强关系推荐≤500ms、Tag缓存≤300ms/未缓存≤800ms、随机≤800ms、Manifest diff≤1s。
- 详情完整轻量Item索引最多1000，不能连带加载原图/代理正文。
- Gallery索引初次响应不得为Scrubber加载全部成员或全部Poster；只有实际Hover的卡片按需请求，连续移动必须节流、取消过期请求并复用私有缓存。
- 随机不得全库ORDER BY RANDOM；Tag后代/推荐需闭包或可索引缓存；所有稳定排序包含兜底键。

## 31. 浏览器与无障碍

- 桌面Chrome/Edge/Firefox/Safari最近两个稳定主版本；移动iOS Safari/Android Chrome最近两个主要/稳定版本。
- Browse验收宽度360、390、768、1024、1440、1920；Manage主要1280+，768+可用，手机保留关键功能。
- 不支持IE、EdgeHTML、旧WebView、无JS或低于360px专门布局。
- 触摸不依赖Hover；右键均有三点替代；拖拽有键盘/移动到替代。
- Gallery卡片Scrubber仅是精确指针设备的快速预览增强；键盘和触摸用户仍通过卡片进入Gallery详情，不以Scrubber承载独占信息或操作。
- 支持焦点、Escape、Lightbox方向键、ARIA、reduced-motion和200%缩放。

## 32. 测试与发布阻断

### 32.1 五层测试

1. Go单元：日期、名称、UUID、Position、DAG、推荐、路径、Manifest三方、BLAKE3。
2. SQLite集成：全新DB、状态机、锁、扫描原子性、排除、合并、Tombstone、任务租约。
3. 媒体契约：本地可再分发格式样本、RAW/Video/动画、Scrubber静态Poster映射、归档安全和依赖诊断。
4. GraphQL/API：DTO泄漏、认证、CSRF、revision、范围、Scrubber资源、普通资源和无磁盘删除API。
5. React/Playwright：全部Browse/Manage关键流程、Scrubber横向映射/离开恢复/后台开关、响应宽度、键盘、ARIA和本地截图回归。

### 32.2 发布阻断

- 应用删除或改写用户媒体；跨Gallery共享身份/个人状态；未认证媒体可读；Browse泄漏路径。
- Manifest冲突静默覆盖；半扫描批量MISSING；Tag成环；OVER_LIMIT进入Browse。
- LIST/MAGIC范围错误；原Stash/未知DB被转换；核心p95不达标。
- Scrubber播放动画/Video、回退读取大原图、泄漏不可见Gallery资源、设置关闭后仍请求资源，或其Hover行为错误记录浏览历史。
- Linux amd64跑全套；Linux arm64跑核心数据库/路径/媒体契约；Docker双架构启动健康检查；CI禁外网。

## 33. 产品、许可证与版本

- 建立独立产品身份，不继续使用Stash产品名/Logo/配置目录/数据库标识。
- 工作名Cosplay Gallery Manager；机器product_id使用稳定独立键，不随最终品牌变化。
- 衍生项目继续AGPLv3；保留Stash归属与版权，About/Legal提供无担保、许可证及当前二进制精确对应源码入口。
- 正式发行包含LICENSE、版权、Third-Party Notices、构建信息和SBOM；不复制GalleryEpic品牌、代码或媒体资产。
- 产品SemVer、数据库整数Schema、Gallery/Coser Manifest主版本、Media Processing Profile Hash四套版本独立。
- 阶段：0.1数据库/扫描/状态机；0.2 Manifest/实体；0.3媒体/任务；0.4 Browse；0.5 Manage；0.6+性能安全收敛；全部验收后1.0。

## 34. 明确延期项

- 最终产品名称、Logo和完整品牌系统。
- AI/视觉向量相似推荐、Embedding模型和向量存储。
- Coser社交账号联网检测Provider和结果建议表。
- 自动网络Scraper、在线更新、遥测或后台远程素材获取；1.5已确认的人工Coser资料导入例外不得扩展到扫描、Browse或定时任务。
- Coser真实姓名、出生日期、身高、体重、三围等结构化字段；第一版写Biography。
- 字幕/多音轨UI、视频进度、硬件转码、360°/VR、Dolby Vision专用处理。
- 应用内加密备份、SQLCipher、内置TLS、外部数据库和多用户。
- Gallery级运行时插件API；未来设计也不兼容原Stash插件。
- Windows原生发行（含amd64/ARM）、macOS原生发行和移动原生应用。

## 35. 当前结论

- 第一版关键产品和架构决策已闭合，没有阻塞Schema设计的待确认项。
- 旧数据迁移、上游兼容和插件兼容均不再是实现约束。
- Gallery卡片正式继承横向Scrubber交互，但以GalleryItem混合媒体静态派生图/Poster重建；默认开启并可在后台全局关闭。
- 下一步应从0.1阶段开始：冻结领域Schema草案、建立全新数据库product_id/UUID Registry、实现Gallery状态机与Source扫描原子提交，然后按版本阶段推进。
- 实现过程中如需改变本文档已确认约束，必须记录变更原因、影响和替代方案，不能以局部代码便利静默偏离。
