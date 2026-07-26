# Cosplay Gallery Manager 实施状态

> 当前里程碑：0.6 运维、安全与恢复
> 状态：进行中
> 更新日期：2026-07-26

## 已完成

- P00-01：建立[Stash复用与隔离边界](../architecture/STASH_REUSE_BOUNDARY.md)。
- P00-02（部分）：新增`internal/product`，定义稳定product ID、工作名、默认路径名称及产品/数据库/Manifest/媒体处理四套独立版本。
- P00-02（部分）：本地未注入构建版本时使用`0.1.0-dev`，不再显示不明确的`unknown`产品版本。
- P00-03（骨架）：创建`ui/web` React 19应用，包含Apollo、GraphQL、i18n以及Browse/Manage/Setup三个Shell。
- P00-03（骨架）：新增React路由测试和独立Makefile构建、测试、开发、验证入口。
- P01-01（核心守卫）：新增`internal/persistence/productdb`，以只读预检和写连接二次检查区分空数据库、产品数据库、原Stash数据库与未知数据库。
- P01-01（核心守卫）：新数据库写入稳定`product_id`和独立Schema版本；原Stash与未知非空数据库拒绝接管且不写入身份表。
- P01-01（连接基线）：产品SQLite连接启用Foreign Keys、5秒Busy Timeout、单写连接、WAL和`synchronous=NORMAL`。
- P01-01（备份基线）：新增SQLite原生Online Backup一致性快照，目标独占创建、禁止覆盖，完成后重新执行产品身份与完整性验证。
- P01-01（设计约束）：记录[数据库身份守卫](../architecture/DATABASE_IDENTITY.md)，新连接暂不注入旧Manager/Repository，避免触发原Stash迁移。
- P01-02（UUID基础）：新增规范UUIDv4/v7和实体Kind校验；实现全局Registry、永久Alias链解析与永久Tombstone。
- P01-02（数据库约束）：Registry、Alias、Tombstone记录只增不改，跨类型复用与删除后重建由数据库主键、外键和Trigger共同阻止。
- P01-02（设计约束）：记录[Portable UUID全局注册表](../architecture/PORTABLE_UUID_REGISTRY.md)，并明确后续实体合并/删除必须升级为聚合事务。
- P01-03：实现Gallery、唯一GallerySource、GalleryItem、Credit/Cast基础表和GalleryStore；首次激活时间、ACTIVE校验、状态机及metadata_revision乐观锁已落地。
- P01-04：实现Source availability/reconcile_state/Issue三层模型、Item availability/processing双状态及后端browsable派生。
- P01-05：实现无user_id的Gallery/Item个人状态、半星评分、收藏、Hidden和Gallery历史；彻底不含view_count与播放进度。
- G1纵向约束：一个Source、跨Source Item隔离、ACTIVE失效回DRAFT、来源故障不篡改Item、多人Cast、1,000成员硬上限均有SQLite集成测试。
- 阶段1设计记录：[Gallery聚合根与阶段1数据模型](../architecture/GALLERY_AGGREGATE.md)。
- P02-01：实现媒体库根、最具体子根所有权、禁用子根边界、改根/删除影响预览，以及GallerySource显式转移或未分配。
- P02-02～P02-03：实现默认关闭的MARKER、PATH_TEMPLATE和FIXED_DEPTH确定性规则；PATH_TEMPLATE按完整NFC相对路径使用Go RE2命名捕获，只生成待审核建议。
- P02-04：实现Candidate到DRAFT的两阶段导入；自动规则只能逐规则启用AUTO_CREATE_DRAFT，且不会把建议直接写入正式元数据。
- P02-05：实现source-scoped扫描暂存、完整成功后的原子提交，以及失败/取消不产生半扫描MISSING。
- P02-06：接入完整BLAKE3和采样BLAKE3；同路径保留身份、同来源完整指纹唯一时重绑定、歧义时保留旧MISSING并创建独立Item。
- P02-06：Manifest `set_id`只生成SOURCE_REBIND_CANDIDATE；来源改绑必须显式确认，两个可访问副本还需要重复来源二次确认。
- P02-07：IgnoredGallerySource优先于规则并支持set_id；Item排除跨重扫保留；显式忘记只删数据库记录并永久Tombstone UUID。
- P02-08：新增ZIP/CBZ只读安全校验，覆盖路径穿越、NFC/大小写冲突、链接、特殊Entry、加密Entry、嵌套归档、Entry/体积/压缩比/像素上限和归档媒体限制。
- P02-09：首次成员按媒体组和自然相对路径分配全局Position；拖拽优先单项更新，间隔耗尽只迁移当前组；1,000上限只统计非排除成员。
- 真实来源闭环：DIRECTORY和ZIP/CBZ扫描只读取用户媒体；实际内容决定静态/动画/视频分类，扩展名错配形成阻断Issue；归档成员大小使用压缩后大小。
- 阶段2设计记录：[来源发现、扫描与安全边界](../architecture/SOURCE_DISCOVERY_AND_SCAN.md)。
- P03-01～P03-03：实现 Gallery 完整业务元数据、Album/Cosplay 推导、Credit/Cast 约束，以及 Coser/Work/Character/Tag 最小模型和 SocialAccount。
- P03-04：所有核心实体具备独立乐观锁更新；普通改名保持 Slug，显式改 Slug 记录历史；无引用删除永久 Tombstone；无冲突合并原子迁移关系并保留永久 UUID Alias。
- P03-04：合并预览覆盖 Coser 资料、重复 Credit/Cast、Work 同名 Character、Tag 重复关系和 DAG 环；存在冲突时禁止提交，不静默丢弃关系。
- P03-05～P03-06：实现 Gallery Manifest 严格部分导入、完整 Push、BLAKE3 哈希状态检查、字段/集合成员级三方比较，以及逐冲突 DATABASE/FILE 显式解决。
- P03-07：实现独立 Coser Manifest、托管头像/Banner 相对路径与裁切/焦点同步、独立 Coser 导入，以及已提交合并的 `redirect_to_uuid` 可重试写入。
- P03-08～P03-09：实现 Item 分类/Caption/Position/评分/排除同步、受限 Markdown、HTTP(S) 链接和 Manifest/字段/集合硬限制。
- 阶段3设计记录：[核心元数据、实体生命周期与 Manifest](../architecture/CORE_METADATA_AND_MANIFEST.md)。
- P04-01：扫描按实际内容识别普通图片、动画、视频和常见RAW；格式错配形成阻断Issue，自拍语义、EXIF/XMP日期和RAW/JPEG伴生只形成待确认建议。
- P04-02：实现SQLite持久化优先队列、幂等任务键、租约/心跳、重试/死信、取消/重排和异常租约恢复；扫描原子提交后才排派生任务。
- P04-03～P04-04：实现普通图片、LibRaw兼容RAW和Stash FFmpeg Poster生成适配器，以及Video直接播放/Remux/H.264-AAC代理规划契约。
- P04-05：实现BASE/ENHANCED双层缓存、profile渐进替换、原子文件发布和增强资源LRU；BASE variant由后端固定，不能被任务Payload降级。
- P04-06：实现preferred/effective双层封面、首次随机、缺失替代、Video Poster回退、人工设置/撤销及Manifest同步。
- P04-07～P04-08：实现统一认证资源服务、Range/ETag、Browse/Manage可见性、无路径DTO，以及Gallery UUID+revision+ordinal的混合媒体静态Poster Scrubber。
- P05-01～P05-03（数据服务）：实现Gallery稳定Slug/历史重定向、Home/List/Magic/All、24项Gallery分页、30项Coser和60项Work/Character/Tag索引。
- P05-04：实现仅名称/sort name/Alias的分级搜索，关系间接命中Gallery，所有实体结果必须关联当前scope可见Gallery。
- P05-05：实现全局/Coser时间线、强关系推荐、Tag相似推荐和带软配额/重复衰减的随机媒体单页。
- P05-06：实现完整轻量成员索引，AVAILABLE的PENDING/ERROR保留，MISSING/排除跳过；资源只返回不透明身份。
- 运行时设置：新增统一`settings_revision`，覆盖首页scope、Scrubber、媒体筛选、三组属性控件、推荐/随机参数、缓存阈值和自动扫描开关。
- 阶段4～5设计记录：[媒体处理与Browse API](../architecture/MEDIA_PROCESSING_AND_BROWSE_API.md)。
- P00/P05（运行入口）：实现本机五步 Setup 所需的后端状态、Argon2id 单所有者密码、可撤销 Session、可信模式基础契约、公开 `healthz/readyz` 与统一认证 GraphQL/媒体资源入口。
- P04（本机工作器）：产品服务启动数据库租约队列工作器，按配置复用图片、LibRaw 与 FFmpeg Poster 生成器；异常任务保持可恢复、可取消和可重试。
- P05（GraphQL）：新 schema 已按 Browse 与 Manage 用途分层；Browse DTO 不返回物理路径，Manage Mutation 使用 Gallery 或实体 `metadata_revision` 乐观锁。
- P06（BrowseShell）：React 19 已实现 Home、List、Magic、Gallery、Coser、Work、Character、Tag、时间线、搜索、随机、收藏、历史和单媒体详情路由；Gallery详情使用完整轻量成员索引与每组手动展开24项。
- P06（Gallery卡片）：实现横向位置映射 ordinal、离开恢复封面、精确指针设备启用及全局后台开关；PHOTO/SELFIE/RAW/GIF/VIDEO统一使用静态派生资源或Poster。
- P07（ManageShell）：实现高密度Gallery问题索引、两阶段媒体库发现/导入、来源扫描、任务队列、运行时设置，以及Gallery五页签和Coser四页签编辑结构。
- P07（Gallery编辑）：基本元数据显式保存；成员分类、Caption、排除、组内排序和封面即时生效；Coser→Character与Tag关系按Gallery范围原子批量保存，Album/Cosplay由Cast自动推导。
- P07（核心实体）：Coser/Work/Character/Tag可独立创建与编辑；Coser社交账号保持人工顺序，未关联Gallery的Coser仍可在Manage中维护。
- P07（实体生命周期）：Manage GraphQL/UI已接入Coser/Work/Character/Tag合并与删除预览；合并明确显示双方revision、受影响Gallery和全部阻断冲突，只有无冲突预览可提交。
- P07（高影响确认）：实体合并与删除使用明确确认词，提交时重新执行乐观锁和关系约束；成功合并保留永久UUID Alias/Slug重定向，成功删除只允许无引用实体并永久Tombstone UUID。
- P07（Coser合并收尾）：Coser数据库合并提交后尝试写入`redirect_to_uuid`；文件系统收尾失败时返回不泄漏路径的待处理警告，不把已提交数据库事务误报为回滚。
- P09（实体生命周期审计）：核心实体合并/删除的成功与失败均进入管理审计；摘要只包含实体UUID、目标UUID、受影响Gallery数量和Coser重定向状态，不包含名称或路径。
- P07（Gallery永久删除）：仅`ARCHIVED` Gallery可进入删除事务，GraphQL要求重新验证所有者密码和精确确认词；预览返回受影响Item、ExternalLink、可执行任务及忽略源状态，提交时再次检查`metadata_revision`。
- P07（Gallery删除聚合事务）：删除业务/个人/Item记录前取消并解除相关任务引用，建立`IgnoredGallerySource`，永久Tombstone Gallery、全部Item与ExternalLink UUID；来源媒体、Gallery Manifest和托管来源资源不发生文件系统写入。
- P09（Gallery删除审计）：Gallery删除的确认、密码和事务失败均记录无路径管理审计；成功摘要只记录受影响记录数量和是否建立忽略源。
- P07（关系选择）：Manage提供不继承Browse scope的全库名称/Alias搜索；Gallery关系编辑器可搜索并选择任意现有Coser、Character和Tag，同时保留UUID人工输入能力。
- P07（Tag DAG）：Tag编辑页可搜索并批量替换多个直接父级；事务要求所有新增、保留和移除父Tag的revision，数据库触发器继续承担严格无环校验。
- P07（Manifest UI）：Gallery与Coser均可检查同步状态、显式Push/Pull，并对三方冲突逐字段选择DATABASE/FILE；Coser资料使用Setup完成后保存在产品数据库中的单一元数据根。
- P07（外部链接）：Gallery仅保存人工HTTP(S)链接，Manage可显式新增，Browse详情末尾弱化展示且服务端不抓取外部内容。
- P09（备份）：每日SQLite Online Backup默认开启并保留7份，保留数量可配置；持久化调度租约保证启动、定时和异常重启时不会并发重复执行。
- P09（完整包）：手工完整备份包含一致性数据库、Coser托管元数据、必要启动配置和四套版本清单；不包含媒体、缓存或日志，持久化SHA-256并在恢复解包前校验，归档解包禁止路径穿越和未知Entry。
- P09（恢复）：Web与CLI共用维护恢复服务；替换前创建安全完整包，数据库与Coser元数据分阶段交换，任一交换/重开失败自动回滚原状态。
- P09（恢复后状态）：成功恢复撤销全部Session、取消旧可执行任务、清除调度租约并暂停自动计划；重新登录后必须显式完成路径、写权限和媒体工具校验才能恢复。
- P09（Operations UI/API）：新增备份/恢复/维护状态/审计GraphQL契约、Operations管理页、二次输入确认和专用维护页；DTO不返回备份根或媒体物理路径。
- P09（CLI）：`cgm -create-backup`、`-restore-backup <uuid>`与`-resume-maintenance`要求交互式所有者重新认证，恢复另需输入`RESTORE`。
- P09（审计）：新增无用户画像管理审计表和50项分页；记录状态切换、导入/扫描、规则/设置、任务操作、Manifest、备份/恢复和计划摘要，不记录浏览、搜索、收藏或评分。
- P09（扫描计划）：默认关闭的自动扫描在启用后于启动和每24小时执行；发现与来源对账复用人工操作的确定性发现、原子扫描和归档安全限制，并使用持久化租约。

## 当前验证结果

- Go 1.25.12临时工具链下，`internal/persistence/productdb`、`internal/productapi`与`internal/productserver`当前单元/SQLite集成测试通过。
- DIRECTORY重复扫描、来源失联、危险归档、父子媒体库、两阶段导入、set_id重绑定候选、成员排除/忘记/排序均有回归测试。
- 媒体内容替换、任务恢复、原子缓存、双层封面、认证Range资源、Scrubber ordinal、分页/scope、时间线、推荐、随机、搜索和成员索引均有回归测试。
- Gallery关系批量保存验证了多人、多角色、Tag、单次revision递增、过期revision拒绝且不产生部分写入。
- GraphQL集成测试验证Gallery与Coser Manifest只能通过显式Mutation写入各自确定路径，并返回CLEAN状态。
- 运维集成测试验证每日快照到期租约/保留、自动扫描opt-in、完整恢复、Session撤销、任务取消、安全备份注册、人工恢复，以及数据库交换后故障的自动回滚。
- Operations GraphQL测试验证备份/恢复由Server服务执行且响应不泄漏数据库或存储根。
- React 19 TypeScript `--noEmit`通过；Vitest当前5个文件、11项测试全部通过。
- Vite生产构建通过，共转换657个模块；当前主JS minify后、gzip前约620KiB，存在大于500KiB的非阻断分包警告。
- 实体生命周期回归验证了按关系类型返回删除阻断、合并冲突时禁用提交、明确确认词、GraphQL预览/提交、永久Alias/Tombstone和管理审计。
- Gallery删除回归验证了归档前阻断、过期revision拒绝、密码与确认词双重门禁、Item/Link/Set UUID Tombstone、任务取消保留、IgnoredGallerySource建立，以及来源媒体和Manifest字节不变。
- BLAKE3依赖固定为`github.com/zeebo/blake3 v0.2.4`并记录模块校验和。
- 用户来源能力审计未发现删除调用；应用删除仅限失败备份、缓存、临时文件等明确生成数据。

## 尚未通过的门禁

- 主机仍没有正式安装的Go 1.25工具链；当前依赖`/tmp/cgm-go1.25.12`，需纳入开发镜像与CI。
- 新前端尚未接入正式发行包，旧`ui/v2.5`仍在仓库；需要完成新二进制静态资源打包和旧业务入口隔离验收。
- G4的真实FFmpeg/LibRaw跨平台样本矩阵尚未执行；当前仅完成适配器、规划器、图片实际样本和伪生成器处理闭环。
- Playwright端到端、截图、键盘/读屏、目标浏览器与响应宽度矩阵尚未执行。
- Coser头像/Banner托管上传尚未完成。
- 前端需要路由级分包，消除当前单入口620KiB构建警告并验证低性能移动设备首屏。

## 下一批工作

1. 完成Coser头像/Banner托管上传与剩余高影响管理操作。
2. 完善恢复中的异机路径映射提示与完整备份跨平台损坏样本演练。
3. 将React 19构建产物接入正式发行二进制，加入路由分包并移除旧业务UI入口。
4. 增加Setup→导入→审核→激活→浏览→Manifest→备份恢复的Playwright离线E2E及无障碍矩阵。
5. 准备真实RAW/GIF/Video与危险归档样本，执行FFmpeg/LibRaw、平台、性能和安全发布门禁。

任何未执行的测试不得在状态记录或发布说明中标记为通过。
