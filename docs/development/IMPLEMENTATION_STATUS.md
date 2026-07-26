# Cosplay Gallery Manager 实施状态

> 当前里程碑：0.7 性能、跨平台与发行加固
> 状态：进行中
> 更新日期：2026-07-27

## 已完成

- P00-01：建立[Stash复用与隔离边界](../architecture/STASH_REUSE_BOUNDARY.md)。
- P00-02（部分）：新增`internal/product`，定义稳定product ID、工作名、默认路径名称及产品/数据库/Manifest/媒体处理四套独立版本。
- P00-02（部分）：本地未注入构建版本时使用`0.1.0-dev`，不再显示不明确的`unknown`产品版本。
- P00-03（骨架）：创建`ui/web` React 19应用，包含Apollo、GraphQL、i18n以及Browse/Manage/Setup三个Shell。
- P00-03（骨架）：新增React路由测试和独立Makefile构建、测试、开发、验证入口。
- P00-03（发行入口）：Browse、Manage、Setup、登录和维护页按路由使用`React.lazy`拆分；`make cgm`固定先构建React 19并以`cgm_web_embed`标签嵌入单文件产品二进制。
- P00-03（新旧隔离）：CGM只依赖独立`ui/web` Go嵌入包，不导入会携带`ui/v2.5/build`的旧`ui`包；Makefile加入精确依赖边界检查，显式`web_root`仅作为开发/定制覆盖。
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
- P07（Coser托管图片）：新增同源、Session认证的头像/Banner multipart上传；按实际内容只接受JPEG、PNG和静态WebP，执行20MiB/50MP上限，原图与头像480、Banner 960/1600派生图只写入`<coser_root>/<uuid>/assets/`。
- P07（Coser裁切与焦点）：上传时保存归一化头像裁切和Banner焦点，替换原图不继承旧参数；数据库更新使用独立`metadata_revision`乐观锁并把既有Coser Manifest标记为`DB_DIRTY`。
- P07（Coser资源安全）：资源服务使用UUID、revision和variant不透明URL，逐级拒绝符号链接且不返回物理路径；替换不删除旧托管文件，失败提交最多留下未引用托管资源，不会影响用户媒体来源。
- P06/P07（Coser图片界面）：Manage资料页可上传并预览头像/Banner，Browse Coser索引显示1:1头像、详情按有无资源显示头像/Banner，无头像继续使用名称首字符占位且无Banner隐藏区域。
- P07/P09（Coser资源审阅）：Operations集中列出已替换、合并或删除Coser留下的未引用头像/Banner生成组；DTO仅返回不透明组ID、Coser UUID、原因、类型、文件数量/容量和时间，不返回文件名或路径。
- P07/P09（Coser资源清理）：只允许人工选择严格匹配CGM UUID命名的原图/派生图组；提交要求Session、同源、所有者密码和精确确认词`CLEAN`，并在SQLite immediate事务中重新检查全部引用后逐文件隔离复核再删除。
- P07/P09（清理安全与审计）：未知文件、临时文件、符号链接、Coser Manifest、Coser资料目录和全部用户媒体永不进入候选；成功/失败审计只记录组数、文件数和字节数，不记录名称或路径，失败不自动重试。
- P07（关系选择）：Manage提供不继承Browse scope的全库名称/Alias搜索；Gallery关系编辑器可搜索并选择任意现有Coser、Character和Tag，同时保留UUID人工输入能力。
- P07（Tag DAG）：Tag编辑页可搜索并批量替换多个直接父级；事务要求所有新增、保留和移除父Tag的revision，数据库触发器继续承担严格无环校验。
- P07（Manifest UI）：Gallery与Coser均可检查同步状态、显式Push/Pull，并对三方冲突逐字段选择DATABASE/FILE；Coser资料使用Setup完成后保存在产品数据库中的单一元数据根。
- P07（外部链接）：Gallery仅保存人工HTTP(S)链接，Manage可显式新增，Browse详情末尾弱化展示且服务端不抓取外部内容。
- P09（备份）：每日SQLite Online Backup默认开启并保留7份，保留数量可配置；持久化调度租约保证启动、定时和异常重启时不会并发重复执行。
- P09（完整包）：手工完整备份包含一致性数据库、Coser托管元数据、必要启动配置和四套版本清单；不包含媒体、缓存或日志，持久化SHA-256并在恢复解包前校验。写入与解包共用平台中立ZIP路径约束，拒绝路径穿越、反斜杠/盘符绝对路径、NFC/大小写碰撞、Windows保留名、符号链接、未知Entry、缺项、异常压缩比及版本清单不一致。
- P09（恢复）：Web与CLI共用维护恢复服务；替换前创建安全完整包，数据库与Coser元数据分阶段交换，任一交换/重开失败自动回滚原状态。
- P09（恢复后状态）：成功恢复撤销全部Session、取消旧可执行任务、清除调度租约并暂停自动计划；每个恢复媒体库必须以旧根并发校验后显式映射到本机真实绝对目录或在本机禁用，来源与Manifest只标记待重新对账且不自动扫描；完成路径、写权限和媒体工具校验后才能恢复。
- P09（异机路径）：路径映射使用平台中立词法处理Windows盘符、UNC与POSIX旧路径；同步改写所属GallerySource、IgnoredGallerySource与Gallery Manifest路径，清空可再生成的发现快照；备份内Coser Manifest自动映射到本机保留的Coser元数据根。
- P09（Operations UI/API）：新增备份/恢复/维护状态/审计GraphQL契约、Operations管理页、二次输入确认和专用维护页；DTO不返回备份根或媒体物理路径。
- P09（CLI）：`cgm -create-backup`、`-restore-backup <uuid>`、`-map-restored-paths`与`-resume-maintenance`要求交互式所有者重新认证；恢复输入`RESTORE`，逐库映射或禁用后再输入`MAP`，与Web复用同一事务、校验和审计。
- P09（审计）：新增无用户画像管理审计表和50项分页；记录状态切换、导入/扫描、规则/设置、任务操作、Manifest、备份/恢复和计划摘要，不记录浏览、搜索、收藏或评分。
- P09（扫描计划）：默认关闭的自动扫描在启用后于启动和每24小时执行；发现与来源对账复用人工操作的确定性发现、原子扫描和归档安全限制，并使用持久化租约。
- P10（离线E2E）：新增运行时生成真实JPEG/GIF的Playwright夹具，覆盖Setup、登录、Coser、媒体库/规则、发现、DRAFT、扫描处理、Cast、激活、Manifest Push、Browse、完整备份恢复、异机路径映射、显式恢复与重扫；E2E不访问外网。
- P10（无障碍与截图）：Chromium中对Setup、Browse、Gallery移动布局与Operations执行WCAG 2.0/2.1 A/AA axe检查，验证键盘焦点并提交桌面1440与移动390基准截图。
- P10（真实媒体）：新增可重复合成DNG、动画GIF、MP4与JPEG的opt-in媒体门禁，实际经过内容分类、dcraw TIFF代理和FFmpeg Poster生成；修正dcraw TIFF输出及原子临时文件保留目标扩展名。
- P10（归档安全）：危险归档矩阵补齐绝对/Windows路径、非NFC、特殊设备、加密Entry、条目数、总大小和压缩比边界。
- P10（性能）：新增10,000 Gallery/1,000,000 GalleryItem、约503MiB SQLite的可重复性能门禁，固定4核并覆盖主页、列表、详情、实体、搜索、时间线、推荐、Tag、随机与Manifest diff。
- P10（平台与容器）：新增固定Go 1.25.12的CGO交叉构建环境，Linux amd64与Linux arm64产品二进制均可构建；新增非root、只读根文件系统、内置FFmpeg/dcraw与健康检查的Dockerfile/Compose。Windows原生支持已由产品决策延期，不属于第一版门禁。
- P11（许可证与源码）：新增公开About/Legal页面和`/about.json`，显示构建版本、提交、AGPL、无担保、Stash归属与精确对应源码链接；发行构建可注入完整Git提交。
- P11（发行资料）：新增52项应用生产依赖清单、SPDX 2.3 SBOM及确定性生成器；新增原生/Docker安装、媒体库、反向代理、升级和备份恢复手册，以及干净提交上的二进制/源码归档对应校验脚本。
- P11（CI/夜间定义）：新增独立CGM工作流；提交门禁覆盖React、CGM产品包、真实媒体、归档、许可证确定性、源码对应、Chromium离线E2E、Linux双架构CGO和Docker双架构构建，夜间门禁覆盖百万Item性能及Firefox/WebKit离线E2E。工作流不包含已延期的Windows目标。

## 当前验证结果

- Go 1.25.12临时工具链下，CGM全部产品包单元/SQLite/API集成回归当前通过。
- DIRECTORY重复扫描、来源失联、危险归档、父子媒体库、两阶段导入、set_id重绑定候选、成员排除/忘记/排序均有回归测试。
- 媒体内容替换、任务恢复、原子缓存、双层封面、认证Range资源、Scrubber ordinal、分页/scope、时间线、推荐、随机、搜索和成员索引均有回归测试。
- Gallery关系批量保存验证了多人、多角色、Tag、单次revision递增、过期revision拒绝且不产生部分写入。
- GraphQL集成测试验证Gallery与Coser Manifest只能通过显式Mutation写入各自确定路径，并返回CLEAN状态。
- 运维集成测试验证每日快照到期租约/保留、自动扫描opt-in、完整恢复、Session撤销、任务取消、安全备份注册、异机路径映射门槛、人工恢复，以及数据库交换后故障的自动回滚。
- 完整备份损坏矩阵验证POSIX/Windows路径穿越、重复/大小写/NFC碰撞、保留名、符号链接、未知Entry、畸形JSON、缺项、无效SQLite产品身份、压缩炸弹和截断ZIP均在替换前拒绝。
- Operations GraphQL测试验证备份/恢复由Server服务执行且响应不泄漏数据库或存储根。
- React 19 TypeScript `--noEmit`通过；Vitest当前8个文件、15项测试全部通过。
- Vite生产构建通过，共转换661个模块；当前主JS minify后、gzip前约471KiB，其余页面按路由生成懒加载块，原大于500KiB分包警告已消失。
- `cgm_web_embed`标签下的产品UI嵌入和Server回归测试通过；`make build-cgm`生成约24MiB单文件验证产物，深层SPA路由、哈希资源immutable缓存和旧UI依赖隔离均已验证。
- 实体生命周期回归验证了按关系类型返回删除阻断、合并冲突时禁用提交、明确确认词、GraphQL预览/提交、永久Alias/Tombstone和管理审计。
- Gallery删除回归验证了归档前阻断、过期revision拒绝、密码与确认词双重门禁、Item/Link/Set UUID Tombstone、任务取消保留、IgnoredGallerySource建立，以及来源媒体和Manifest字节不变。
- Coser托管资源回归验证了未认证上传拒绝、同源Session上传、实际JPEG派生、过期revision冲突、GIF拒绝、认证资源读取、URL不泄漏根路径，以及替换后旧原图/派生图仍保留。
- Coser未引用资源回归验证了现用组排除、已替换/已删除分组、未知文件与符号链接跳过、引用变化后过期审阅拒绝、密码/确认词/同源门禁、选择性清理、Manifest保留和无路径审计。
- BLAKE3依赖固定为`github.com/zeebo/blake3 v0.2.4`并记录模块校验和。
- 用户来源能力审计未发现删除调用；应用删除仅限失败备份、缓存、临时文件等明确生成数据。
- Chromium离线Playwright主流程通过，Setup/Browse/Gallery/Operations axe扫描无WCAG A/AA违规，桌面与390px移动截图回归通过。
- 真实媒体门禁通过：合成标准DNG由dcraw实际生成代理，动画GIF与FFmpeg生成MP4实际生成Poster，内容分类与扩展名无关。
- 危险归档单元矩阵通过；完整备份损坏矩阵继续在替换前拒绝危险输入。
- 固定`GOMAXPROCS=4`的百万Item性能门禁通过：最慢Gallery列表p95约493ms、时间线约477ms，均低于500ms；Tag未缓存约141ms、缓存约143ms，其他目标均通过。
- 固定工具链CGO构建通过Linux amd64与Linux arm64；Linux amd64 Docker镜像实际构建、非root启动和`/healthz` 204通过。
- About/Legal后端、前端、产品版本/源码URL测试通过；SPDX文件可解析且包含1个产品包与52项应用依赖。

## 尚未通过的门禁

- 主机仍没有系统级Go 1.25；本轮使用校验过的`/tmp/cgm-go1.25.12`，交叉构建镜像和新增CI均固定1.25.12。
- Firefox、WebKit、Edge及真实iOS Safari/Android Chrome最近两版尚未执行；当前Playwright证据仅为Linux Chromium、1440/390视口与axe自动检查，不能代替真实读屏/浏览器/设备验收。
- Linux arm64已通过CGO构建，尚未在真实arm64系统完成安装、路径、SQLite、FFmpeg/LibRaw、升级和备份恢复运行验收。
- Docker linux/amd64已实际健康启动；linux/arm64镜像运行需要宿主机全局binfmt或原生arm64 runner。本轮特权binfmt注册被安全策略拒绝，故双架构Docker门禁尚未全部通过。
- 只读媒体源由E2E和容器挂载覆盖基础流程；真实操作系统网络挂载、Manifest Push无写权限和低性能移动设备首屏仍需目标环境验收。
- 新增CI/夜间工作流尚未在远端runner执行，不能根据本地语法和子门禁结果推定通过。
- RC版本/API/schema冻结、正式多平台产物签名/校验和、容器平台SBOM以及干净发行提交的源码归档对应脚本尚未执行。

## 下一批工作

1. 在Linux arm64及arm64 Docker runner执行安装、媒体、升级和完整恢复验收，并发布各平台容器SBOM。
2. 在Firefox、WebKit、Edge及真实移动浏览器执行目标宽度、键盘、读屏、200%缩放与低性能首屏矩阵。
3. 在远端执行新增CI/夜间工作流并修复runner差异；随后冻结0.9接口与schema，从干净提交执行`verify-cgm-release`并生成签名发行物。

任何未执行的测试不得在状态记录或发布说明中标记为通过。
