# CGM 1.5 视频处理第一、第二阶段功能规划

> 状态：第一、第二阶段代码已实现；本机合成样本真实工具门禁已通过，部署配置与目标浏览器人工业务验收待执行
> 记录日期：2026-08-15
> 当前分支：`agent/cgm-migration-handoff-20260726`
> 适用范围：1.5 本机业务迭代；Windows原生构建继续延期
> 上游约束：[开发备忘录](../COSPLAY_DEVELOPMENT_MEMO.md)、[第一版开发计划](../COSPLAY_V1_DEVELOPMENT_PLAN.md)、[媒体处理与Browse API](../architecture/MEDIA_PROCESSING_AND_BROWSE_API.md)、[Stash复用边界](../architecture/STASH_REUSE_BOUNDARY.md)

## 1. 目标与现状

本计划补齐既有P04-04“兼容则直接播放、其次Remux、必要时生成H.264/AAC代理”的实际业务闭环，不改变Gallery聚合、只读来源、认证资源、双层缓存和Gallery卡片Scrubber只显示静态Poster等已确认设计。

当前已实现：

- DIRECTORY视频发现、内容分类、BLAKE3指纹和独立GalleryItem身份；
- `STATIC_POSTER`基础派生任务及FFmpeg静态JPEG适配器；
- `DIRECT | REMUX | TRANSCODE`纯规划函数；
- `VIDEO_PLAYBACK`增强缓存variant声明；
- 媒体详情前端的`video/*`渲染能力和认证派生资源Range响应。

当前缺口：

- 未探测或持久化容器、轨道、时长、编码、旋转和HDR等视频技术信息；
- Poster固定取第0秒，且尺寸沿用默认4096上限；
- 没有`VIDEO_PLAYBACK`生成器、按需任务入口或原始视频直读路由；
- 当前本机正式部署没有配置FFmpeg，系统也未发现FFmpeg/FFprobe，新视频无法完成Poster真实业务处理；
- 现有播放规划函数未接入数据库探测结果、资源授权、任务队列或前端。

## 2. 不可改变的产品边界

1. Gallery仍是聚合根，视频只是GalleryItem的一种媒体类型，不恢复Stash Scene业务模型、数字ID或旧`/scene`路由。
2. 用户媒体永久只读；探测、Poster、Remux和转码不得移动、重命名、删除或改写原视频。
3. ZIP/CBZ中的视频继续属于激活阻断项；第一、第二阶段只为DIRECTORY视频提供播放。
4. Gallery卡片Scrubber继续只显示静态Poster，不播放视频、不请求播放代理、不记录观看状态。
5. 不持久化播放进度、观看次数或观看时长；不引入多音轨选择、字幕、画质档、时间轴Sprite、HLS/DASH、外部播放器、360°或硬件转码。
6. 所有播放资源必须认证；HTTP和GraphQL契约只接受不透明UUID/revision，不接受或返回用户媒体物理路径。
7. `STATIC_POSTER`属于不可回收BASE；Remux/转码结果`VIDEO_PLAYBACK`属于可重建ENHANCED并受现有LRU上限约束。
8. 不复制Stash的Scene Repository、旧生成目录或旧缓存身份，只通过CGM适配器复用FFprobe、FFmpeg命令构建和Range机制。

## 3. 目标处理链

```text
完整成功扫描
  -> VIDEO Item写入/更新content_revision
  -> 幂等排队技术探测
  -> FFprobe选择主视频/音频轨并写入产品技术元数据
  -> 幂等排队BASE Poster
  -> 约20%位置截帧并发布STATIC_POSTER
  -> Gallery获得可展示成员

用户在Lightbox或媒体详情实际打开视频
  -> 读取已持久化技术元数据并计算播放方案
  -> DIRECT：认证路由直接Range读取原视频
  -> REMUX/TRANSCODE：幂等排队VIDEO_PLAYBACK
  -> Poster保持可见并显示准备状态
  -> 原子发布增强缓存MP4后切换到播放器
```

扫描、列表加载、Gallery卡片曝光和Scrubber悬浮均不得触发Remux或转码。

## 4. 第一阶段：依赖、技术元数据与可靠Poster

### VP1-01 FFmpeg/FFprobe依赖解析与诊断

- 保留现有`ffmpeg_path`，新增可选`ffprobe_path`启动配置；旧配置省略该字段仍然合法。
- `ffprobe_path`为空时，优先从`ffmpeg_path`同目录解析同套`ffprobe`；不得因PATH中另一套版本而静默混用。
- 启动时分别验证可执行性、版本和所需参数，不因视频依赖缺失阻止图片/RAW业务启动。
- Manage → Settings以只读状态显示FFmpeg/FFprobe是否可用、解析来源和版本；不显示探测过的媒体路径。
- 缺失或不兼容使用稳定错误码：`FFMPEG_UNAVAILABLE`、`FFPROBE_UNAVAILABLE`、`FFMPEG_VERSION_UNSUPPORTED`、`FFPROBE_VERSION_UNSUPPORTED`。
- Docker仍使用同一镜像内固定版本；本阶段不安排Windows原生构建。

### VP1-02 数据库前向迁移与产品自有技术表

- 将产品数据库从schema v1前向迁移到下一版本；迁移前必须通过SQLite Online Backup生成并校验安全快照。
- 新增`video_technical_metadata`，与`gallery_items.item_uuid`严格1:1并级联删除，至少保存：
  - `content_revision`和`probe_profile_hash`；
  - `probe_state = PENDING | READY | ERROR`与稳定`last_error_code`；
  - 规范化container、duration/start time、总/视频bitrate；
  - 主视频轨index、codec/profile、像素格式、coded/display宽高、帧率；
  - rotation；color range/space/primaries/transfer和派生HDR布尔值；
  - 可空主音频轨index、codec、channel count和sample rate；
  - FFprobe版本与完成时间。
- 表中不保存标题、文件名、来源绝对路径、ffprobe原始JSON或命令stderr。
- 增加`ITEM_TECHNICAL_METADATA`任务种类；探测任务不伪装成文件派生资源，也不向`media_derivatives`写空记录。
- 数据库迁移只改变产品数据结构，不扫描来源、不批量读取视频。跨schema回退必须恢复迁移前安全快照，不能只替换旧二进制。

### VP1-03 产品级FFprobe适配器

- 在`internal/mediaprocessing`定义产品自有`VideoProbe`接口和结果类型；底层可包装`pkg/ffmpeg`，上层不得依赖Stash `models.VideoFile`或Scene。
- 轨道选择固定为：忽略attached picture；优先可解码且标为default的主视频/音频轨；否则选第一个可解码轨；字幕全部忽略。
- 容器、codec和色彩字段规范化后再入库，未知值保留为空或`unknown`，不得猜测。
- 探测设置有限执行时间、可取消context和受限输出；原始stderr只允许进入既有受限诊断日志。
- 探测失败不改变`metadata_revision`，只更新技术状态/任务状态；重新扫描、内容替换或人工重试可恢复。

### VP1-04 调度、幂等与内容替换

- 新发现或内容发生变化的VIDEO在扫描原子提交后只先排技术探测，任务键包含item UUID、content revision和probe profile。
- 探测成功后再排当前revision的`STATIC_POSTER`，避免Poster为获取时长重复执行独立ffprobe。
- 内容替换时旧技术元数据失效，旧内容的Poster/播放代理HARD_INVALID，旧任务取消；新revision重新探测。
- 既有视频采用有界低优先级回填，不在数据库迁移事务或单次启动中一次性读取全部来源；当前查看和人工重试优先。
- 旧revision Poster只有在内容未替换且处于渐进profile升级时才可继续服务，新Poster发布后原子切换。

### VP1-05 Poster策略

- 默认截帧时间为视频主轨有效时长的20%；短视频将时间限制在合法范围，时长未知时才回退第0秒。
- 先使用快速seek，失败后在同一时间点使用精确seek；仍失败时允许一次第0秒保底，之后返回明确错误。
- `STATIC_POSTER`长边上限固定为960px、JPEG、不放大，正确应用旋转；HDR视频生成Poster时执行确定性的SDR色调映射。
- Poster继续属于BASE并可作为纯视频Gallery的effective cover；不会另外预生成4096视频静态图。
- 新策略进入媒体processing profile hash；旧profile可渐进替换，不删除用户来源。

### VP1-06 API、Manage与媒体详情

- MediaDetail白名单DTO在探测成功后显示时长、显示宽高、容器、视频/音频codec和帧率；不返回轨道元数据原文或路径。
- Manage Gallery媒体行显示probe状态、稳定错误码、选定轨道摘要和“重试视频处理”入口。
- 缺少工具时明确显示依赖不可用；不能把任务长期PENDING伪装成正常处理中。
- Browse列表和Gallery卡片DTO不增加大块技术信息，避免扩大列表查询和泄漏边界。

### VP1-07 第一阶段测试与退出门禁

- Go单元：轨道选择、容器/codec规范化、旋转、HDR识别、20%时间点、短视频和未知时长回退。
- SQLite集成：schema升级/安全快照、探测幂等、内容revision失效、有界回填、错误后重试、技术状态不修改metadata revision。
- 媒体契约：用本地生成的MP4、MOV、MKV、WebM、旋转、无音频、双音轨和HDR元数据样本实际运行FFprobe/FFmpeg。
- API/UI：路径不泄漏、技术字段白名单、依赖缺失提示、重试、纯视频Gallery Poster激活与Gallery Scrubber仍只取Poster。
- 第一阶段只有在本机安装并配置真实FFmpeg/FFprobe后才可标记业务验收通过；mock测试不能替代真实工具门禁。

## 5. 第二阶段：认证直放与按需播放代理

### VP2-01 保守的播放方案矩阵

- 播放方案只根据当前content revision的READY技术元数据计算，不根据扩展名猜测。
- DIRECT初始采用目标浏览器共同通过的保守白名单：MP4容器、H.264视频、AAC/MP3或无音频、无需旋转滤镜且非HDR。
- WebM/VP8/VP9/Opus等组合只有通过Chrome、Firefox、Safari目标矩阵后才进入共同直放白名单；否则生成代理，不能直接照搬Stash的H.265或浏览器假设。
- H.264主视频无需滤镜但容器或音频不兼容时优先REMUX：复制视频轨，音频兼容则复制，否则转AAC。
- 需要缩放、旋转、HDR转SDR或视频codec不兼容时TRANSCODE为H.264/AAC Fast Start MP4。
- 代理以显示方向为准限制在1920×1080或1080×1920边界内且不放大；无音频视频保持无音频，不制造静音轨。

### VP2-02 原视频认证直读路由

- 新增仅接受`item_uuid + content_revision`的不透明DIRECT视频路由，例如`/resource/video/<uuid>/<revision>/direct`；不得接受path查询参数。
- 数据库授权同时验证Session、Browse/Manage模式、Gallery scope、ACTIVE/Hidden、Source/Item AVAILABLE、未排除、无阻断Issue和技术方案确为DIRECT。
- 只允许DIRECTORY来源；路由内部解析并逐级验证真实目录、普通文件和无符号链接，再保持同一已打开文件描述符完成响应，降低TOCTOU风险。
- 支持GET、HEAD、单/多Range浏览器行为、206/416、`Accept-Ranges`、稳定ETag、正确MIME、`nosniff`和inline；未知与无权限统一404。
- 原视频不复制进缓存。播放、暂停后继续读取和拖动进度条都会访问原始媒体，取消HTTP请求必须停止继续读取。
- 原视频响应不使用长期immutable缓存；每次通过content revision和ETag重新验证。

### VP2-03 按需播放状态与任务契约

- 新增路径无关的`videoPlaybackStatus(itemUUID)`和`requestItemVideoPlayback(itemUUID)`契约，或等价的现有OnDemandResource扩展。
- 返回`mode = DIRECT | REMUX | TRANSCODE`、`status = PENDING | PROCESSING | READY | ERROR`、可选不透明资源身份和稳定错误码。
- DIRECT立即返回可播放身份，不创建派生任务；REMUX/TRANSCODE仅在Lightbox当前选中视频或媒体详情实际打开时幂等排队。
- Gallery卡片、Gallery列表、预览索引、Scrubber、推荐和随机查询不得调用请求Mutation。
- 同一item/revision/profile并发请求汇聚为一个任务；COMPLETED资源被LRU回收后，下次实际查看可重新打开任务。

### VP2-04 Remux/转码生成器

- 新建CGM `VideoPlaybackGenerator`并纳入现有Generator/Worker边界；只接收已持久化播放方案和选定轨道，不自行选择业务身份。
- FFmpeg显式`-map`选定的一条视频轨和可选一条音频轨，忽略字幕、附件和其他轨道。
- REMUX复制H.264视频；按方案复制兼容音频或转AAC；输出MP4并启用`+faststart`。
- TRANSCODE使用CPU libx264、yuv420p、AAC、固定质量/profile及不放大缩放；正确应用旋转并清理已消费的rotation metadata。
- HDR输入使用固定、可测试的BT.2020/PQ或HLG到SDR流程；所需FFmpeg filter不可用时返回`VIDEO_TONEMAP_UNAVAILABLE`，不得输出明显错误色彩的代理。
- 继续使用同目录临时文件、fsync、原子rename和发布事务；取消、失败或过期revision只清理应用临时/未发布缓存。

### VP2-05 缓存、profile与生命周期

- `VIDEO_PLAYBACK`固定属于ENHANCED，计入现有可回收缓存上限、最低磁盘余量和真实访问时间LRU。
- profile hash至少包含播放矩阵版本、mode、选定轨道、FFmpeg主版本、目标容器/codec、分辨率、旋转/HDR策略和编码参数。
- 同一item只保留一个current播放代理；新profile成功前旧profile可继续服务，内容revision变化后旧代理立即HARD_INVALID。
- 代理生成前执行空间预算；空间不足返回`DISK_SPACE_LOW`并暂停，不删除BASE Poster或用户媒体。
- LRU回收正在响应的文件时必须避免中断已打开响应；删除只作用数据库确认的ENHANCED普通文件。

### VP2-06 前端播放闭环

- Gallery媒体网格仍显示Poster；只有进入Lightbox当前视频或`/media/:item_uuid`才请求播放状态。
- DIRECT就绪后直接显示HTML5播放器；代理未就绪时继续显示Poster、准备状态和取消后可重试的错误提示，完成后无整页刷新切换。
- 切换Lightbox成员或关闭Lightbox时暂停上一视频、清理事件和轮询；只预取相邻视频Poster，不预取原视频或代理。
- 播放器提供浏览器原生基础播放、音量、进度和全屏控件；不保存播放进度，也不新增画质/音轨/字幕UI。
- reduced-motion不改变用户主动视频播放，但继续禁止卡片自动播放。

### VP2-07 日志、审计与可诊断性

- 增加稳定事件码：`CGM_VIDEO_PROBE_*`、`CGM_VIDEO_POSTER_*`、`CGM_VIDEO_PLAYBACK_REQUESTED`、`CGM_VIDEO_DIRECT_SERVED`、`CGM_VIDEO_PROXY_*`。
- 默认INFO只记录job ID、item UUID短前缀、mode、状态、耗时和错误码；不记录标题、文件名、绝对路径、请求Range内容或FFmpeg完整命令。
- 用户主动重试属于技术操作摘要；普通播放、暂停和Range请求不写管理AuditLog，不形成观看画像。
- Manage任务诊断可区分probe、poster、remux和transcode失败，显示清理后的错误摘要。

### VP2-08 第二阶段测试与退出门禁

- 规划单元：DIRECT/REMUX/TRANSCODE矩阵、无音频、旋转、HDR、未知codec和过期技术元数据。
- HTTP集成：未认证、跨scope、Hidden、DRAFT、排除、MISSING、旧revision、GET/HEAD、Range、416、ETag和取消读取。
- Worker集成：并发请求幂等、Remux、音频转AAC、完整转码、原子发布、取消、重试、内容替换和LRU回收后重建。
- 真实媒体：目标浏览器分别播放DIRECT、Remux代理和Transcode代理，并验证拖动、暂停、全屏、无音频、旋转及HDR SDR色彩。
- React/Playwright：Poster准备态、无刷新切换、错误重试、Lightbox切换停止上一视频，且卡片/Scrubber从未请求播放资源。
- 安全回归：所有响应和GraphQL DTO无物理路径；来源文件hash、mtime和size在探测、播放、Remux及转码前后完全不变。

## 6. 实施顺序与阶段提交

1. 先以契约测试冻结技术表、探测结果和播放矩阵，再引入数据库前向迁移。
2. 完成VP1-01～VP1-04，验证无FFmpeg时图片业务不退化、视频错误可诊断。
3. 完成VP1-05～VP1-07，在真实FFmpeg/FFprobe环境验证Poster后更新实施状态。
4. 完成VP2-01～VP2-03，先打通授权与按需状态，不立即加入复杂转码。
5. 完成VP2-04～VP2-05，分别用Remux和Transcode样本验收缓存生命周期。
6. 完成VP2-06～VP2-08，最后接入Lightbox/媒体详情并执行完整回归。

每个阶段均独立记录代码、测试和开发结果。首次schema升级部署属于维护窗口变更：必须先完整备份并记录数据库版本；普通前端HMR不能替代迁移验收。本轮实现不执行部署，自动化合成样本也不能替代部署后的真实媒体与目标浏览器人工验收。

## 7. 2026-08-15实施结果

### 第一阶段

- 产品schema由v1前向迁移至v2；打开旧库时先用SQLite Online Backup创建并校验`pre-schema-v1`安全快照，再迁移任务约束和`video_technical_metadata`。新库直接初始化为v2，旧二进制回退仍需恢复快照。
- 增加成对FFmpeg/FFprobe解析、最低主版本检查、Manage只读诊断、稳定错误码以及受限输出/30秒探测适配器；未配置工具不阻断图片和RAW启动。
- VIDEO扫描只先排`ITEM_TECHNICAL_METADATA`；探测成功后再排960px BASE Poster。主轨按default后first选择，忽略attached picture、字幕和额外轨道；存量DIRECTORY视频每分钟最多低优先级回填25项，已记录ERROR不自动无限重试。
- Poster使用有效时长20%、快速seek→精确seek→第0秒保底，显式应用已持久化旋转、HDR到SDR以及不放大缩放；技术状态和人工重试已接入Manage Gallery与媒体详情白名单DTO。

### 第二阶段

- 播放矩阵收敛为保守DIRECT白名单：MP4/H.264/AAC、MP3或无音频且无旋转/HDR；H.264不兼容容器/音频优先Remux，其余转H.264/AAC Fast Start MP4。WebM仍走代理。
- 新增认证`/resource/video/<uuid>/<revision>/direct`，数据库与文件打开时复核ACTIVE、Scope、Hidden、Source/Item可用、Exclude、Blocking Issue、DIRECT方案、普通文件和无符号链接；GET/HEAD、单/多Range、416、ETag、`nosniff`和长时播放写超时均覆盖。
- 新增路径无关播放状态/请求GraphQL；DIRECT不创建任务，Remux/Transcode按item/revision/profile幂等汇聚，profile包含矩阵、轨道、FFmpeg版本、编码、旋转和HDR策略。代理固定为ENHANCED，生成前检查容量/磁盘余量，继续使用原子缓存发布和现有LRU。
- Lightbox当前视频与媒体详情成为唯一前端需求信号；准备期间保持Poster，完成后无刷新切换原生播放器，切换成员通过组件卸载终止上一视频；列表、卡片、Scrubber和相邻成员不请求播放资源。
- 默认日志使用`CGM_VIDEO_*`稳定事件，只记录技术任务、短item前缀、模式、耗时和错误码，不记录标题、文件名、绝对路径、Range内容或FFmpeg完整命令。

### 已执行门禁与保留项

- Go目标包、SQLite迁移/安全快照、回填、并发按需任务、Worker、GraphQL和DIRECT HTTP测试通过；React TypeScript与全部Vitest通过。
- 使用本机`/usr/bin/ffmpeg`、`/usr/bin/ffprobe`和`/usr/bin/dcraw`实际通过合成MP4、MOV、MKV、WebM、无音频、双音轨、旋转、HDR标记、Poster、Remux和Transcode门禁，并修复FFprobe返回`matroska,webm`时容器错误归类为MKV的问题。
- 本轮按要求未部署、未迁移正式数据库、未改正式配置或媒体。真实业务媒体来源只读校验、DIRECT/代理在Chrome/Firefox/Safari中的拖动/暂停/全屏以及部署后回退恢复仍待维护窗口人工验收，不能据合成样本标记为全部业务通过。

## 8. 明确延期

- HLS/DASH、边转边播、GPU/硬件编码和多清晰度自适应；其中单清晰度渐进HLS仅进入[第三阶段条件式规划](VIDEO_PROCESSING_PHASE_3_PLAN_2026-08-15.md)，不属于第一、第二阶段交付；
- 视频时间轴Sprite/WebVTT及短视频Hover预览；其中仅媒体详情/Lightbox辅助时间轴进入[第三阶段规划](VIDEO_PROCESSING_PHASE_3_PLAN_2026-08-15.md)，Gallery卡片Hover视频继续禁止；
- 字幕、多音轨/多视频轨选择和Dolby Vision专用处理；
- 播放进度、观看历史、次数或时长；
- ZIP/CBZ视频播放、外部播放器、投屏/DLNA和物理文件操作；
- 视频pHash、自动去重或跨Gallery媒体合并；
- Windows原生构建与验收。
