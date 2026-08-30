# 本机实际业务应用测试部署

### 2026-08-30 TAR/7Z与媒体库覆盖报告、schema v7部署

- 从清洁提交`28e71a7a4c095b095b58d9c742ccc48941d30769`构建并部署`cgm_web_embed cgm_galleryepic` Linux amd64单文件，SHA-256为`07cc18ecbe5611a812170e753b8e48db2224adbbfc6d851a43556d8cce6f6729`；VCS元数据为`modified=false`，About精确对应源码。
- 停服后创建并实际解包核验0600完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260830T142000Z-28e71a7.tar.gz`，SHA-256为`2e2cb02cca5d2227bd1b30d74c5355418b739a4030733d7b42bd4ad29d68ff7b`。其schema v6 SQLite快照、旧二进制、配置、systemd单元与Coser托管资源均和正式来源一致，未包含媒体、缓存或日志。
- 自动迁移快照`product.sqlite.pre-schema-v6-1788100566661475656.bak`保持schema v6、62张表、`integrity_check=ok`和2/5/5/322计数，SHA-256为`bc3330368f500efe20118ba18bec3a21aca2f22e51491f73b6fcd0148a38e6f8`。正式库迁移到schema v7、64张表后完整性和业务计数不变，覆盖摘要/诊断表初始为空。
- 服务只启动一次并保持`active/running`、`NRestarts=0`；Health/Ready为204，Root/Legal/Session为200。配置SHA-256未变，本次启动日志只有正常停止、启动、工作器和探针记录，无迁移错误或异常级别日志。

### 2026-08-30 媒体库自动化与schema v6部署

- 从清洁提交`ed275690d5f13dd60e868557eba6ef3327953f06`完成自动化主体部署，并以验收修复提交`ce953f24b9033855647305f7805ca00a9b344588`更新最终二进制。最终SHA-256为`c049b359fb05b040c9448de463b4b34f52e5ab46291c148e713870c40547f585`，About报告精确提交且`exactSourceAvailable=true`。
- 停服后创建并实际解包验证0600完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260830T125712Z-ed27569.tar.gz`，SHA-256为`62c388e2c62e2d5fd662e82a220ca7095fc9de19eeb90f5c6fe8bba6380c60a5`；包含一致schema v5数据库、旧二进制、配置、systemd单元和Coser托管资源，不包含媒体、缓存或日志。
- 自动迁移快照`product.sqlite.pre-schema-v5-1788094705533130987.bak`保持schema v5、无v6表、`integrity_check=ok`，SHA-256为`937aafa399e01f1b1ea8aa578941ec85c55699d3efb1bf784b895f7fcdf2251d`。正式数据库迁移至schema v6后仍为`integrity_check=ok`，既有2个媒体库、5个Gallery、5个来源和322个Item不变；自动化三表为空，全部媒体库仍为隐式MANUAL。
- 正式二进制和正式数据库隔离副本完成10,000目录/PNG发现、扫描、SIGKILL、过期租约接管及运行中取消验收；测试媒体和测试业务记录只存在于`/tmp`隔离实例。正式服务保持`active/running`、`NRestarts=0`，Health/Ready均为204，启动日志无迁移错误。
- 正式媒体库不会因升级自动启用。所有者可在Manage → Libraries中为选定媒体库保存ASSISTED或TRUSTED策略；生产30分钟租约的10分钟长扫描心跳周期尚未实际等待，应在可接受长任务的后续窗口验证。

### 2026-08-29 Archive成员排除与标题/实体候选增量部署

- 从清洁提交 `d9420fce4cf9584229ec5bcd31ef87e991d14b74` 构建并部署带 `cgm_web_embed cgm_galleryepic` 的 Linux amd64 二进制，SHA-256 为 `835597af497f93042633e9742a42301432df3f8ba51d9e8b1d6a21e982639b8c`。
- 替换前正式二进制保存在 `/tmp/cgm-before-archive-rules-20260829`，SHA-256 为 `ab1156db14e21c6f50f7cbe91a92be6c5df65021f6e2f3a2a086854a59412825`；候选逐字节校验后原子替换，服务只重启一次。
- 产品数据库保持 `cosplay-gallery-manager / schema 5`，`PRAGMA integrity_check=ok`；配置 SHA-256 仍为 `ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`，未迁移数据库或修改媒体、Manifest与缓存。
- 用户服务保持 `active`、`NRestarts=0`，Health/Ready 均为 204；About 报告完整提交号与 `exactSourceAvailable=true`，启动 journal 无 WARN、ERROR、FAILED、panic 或 fatal。


### 2026-08-28 可管理媒体自动排除规则部署

- 从提交 `18d6485b34cb52bc0db77dca835142c2d4714bc6` 构建并部署带 `cgm_web_embed cgm_galleryepic` 的 Linux amd64 二进制，SHA-256 为 `ab1156db14e21c6f50f7cbe91a92be6c5df65021f6e2f3a2a086854a59412825`。
- 停止服务后创建并校验额外完整回滚包 `/home/rainbowrunner/cos/bk/cgm-predeploy-20260828T123000Z-18d6485.tar.gz`，SHA-256 为 `4aa68c94c9361a86b3422726fadc57acf34fae5510e0baec4dd15e766b3da913`；包含旧二进制、启动配置、systemd 单元、Coser 托管元数据及 SQLite 快照，不包含媒体、缓存和日志。
- 新服务启动时完成产品数据库 schema v4→v5 迁移；`cgm_product_identity.schema_version=5`，`PRAGMA integrity_check=ok`，`media_exclusion_rules` 与 `media_exclusion_decisions` 初始均为 0 行，既有媒体状态未改变。
- 用户服务保持 `active`、`NRestarts=0`；`/healthz` 与 `/readyz` 均返回 204，`/about.json` 报告完整提交号、`exactSourceAvailable=true`。启动 journal 仅见正常停止/启动、2 个工作器及健康检查，无迁移错误、WARN、ERROR、FAILED、panic 或 fatal。


## 1.5 增量开发状态

### 2026-08-19 MARKER单子目录标题保底增量部署

- 2026-08-19 00:32 CST将正式服务升级到清洁提交`fbba7e2c9e2673ae652b67d2ebd748a0d6972dd4`。Go 1.25.12以`cgm_web_embed cgm_galleryepic`构建，`go version -m`确认`vcs.modified=false`；正式二进制SHA-256为`f35b84abdab4d5c5a8c42745a88a37917af5d2c3d38d8d90b997bea8e0aa498d`，旧二进制保留于`/tmp/cgm-before-marker-title-20260819`。
- 新发现的`.cosplay-root`根若只有一个直属真实子目录，Candidate、手动导入和自动建DRAFT的标题取该子目录名；零个或多个子目录仍取根目录名。来源根、已有Gallery、根级媒体默认Exclude及ZIP/CBZ流程不变。
- 服务保持`enabled/active/running`、`NRestarts=0`，Health/Ready为204，首页和Session为200；About报告完整提交和`exactSourceAvailable=true`。数据库保持schema v4、inode `19679716`且`integrity_check=ok`，配置SHA-256未变，本次启动日志无迁移或异常。

### 2026-08-17 媒体分类规则与schema v4增量部署

- 2026-08-17 01:24 CST将正式服务升级到提交`6e61b9a6b0864a9619c74cfbe14c9f87210b33f4`。清洁提交以Go 1.25.12、`cgm_web_embed cgm_galleryepic`构建，`go version -m`确认`vcs.modified=false`；正式二进制SHA-256为`5aa52944757232585f36effcb2f62b0f932e15be8f0b18c6f785c77bee83c7e0`，旧二进制保留于`/tmp/cgm-before-media-classification-20260817`。
- 停机且MainPID为0后创建额外完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260816T172208Z-6e61b9a.tar.gz`，权限`0600`、SHA-256为`63cb688b8520c2159dcd76dbb95f7b76e285ef0b6fc66922d43271ae3ead6d3f`。归档已实际解包，schema v3数据库`integrity_check=ok`、55张表/1930行；旧二进制、启动配置、systemd定义和Coser元数据均与正式来源逐项一致。
- 服务只启动一次即完成schema v3→v4迁移。自动迁移快照`product.sqlite.pre-schema-v3-1786901055992698276.bak`权限`0600`、SHA-256为`a7f2842136e05904490c042dd2ed7b5d994cf84d377ce39a2b9bc6e34eaa22a0`，独立验证仍为schema v3、`integrity_check=ok`、55张表/1930行且不含v4表。
- 迁移后正式库保持inode `19679716`，`integrity_check=ok`、schema v4、57张表/1932行；新增两条可编辑的全局默认规则，其中目录Exact规则启用、文件名Glob规则关闭。正式库迁移前没有旧自拍建议，因此新建议表为空；没有产生或丢失业务建议。
- 服务保持`enabled/active/running`、`NRestarts=0`，Health/Ready均为204，首页/Setup/Legal/Session为200；About精确报告完整提交和`exactSourceAvailable=true`，入口使用`index-CvDrpo1q.js`与`index-8_Nt1C6e.css`。启动确认两个工作器及FFmpeg/FFprobe/LibRaw可用，本轮journal无WARN、ERROR、FAILED、panic或fatal。
- 配置SHA-256继续为`ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`。本轮未替换配置、媒体、Gallery/Coser Manifest或缓存；数据库变更仅为经备份验证的前向schema迁移和两条默认规则。

### 2026-08-15 Gallery视频标识透明化增量部署

- 2026-08-15 21:46 CST将Gallery详情媒体卡片右上角VIDEO/GIF类型标识改为无背景、无内边距的轻量文字；82%白字与轻微文字阴影只保证不同Poster上的基本可读性，不形成新的遮罩或色块。媒体类型识别、文字内容、右上角操作菜单和Lightbox均未改变。
- 源码提交为`4b3c1ec982544da46dd48791d5ad17057b09d930`，正式二进制SHA-256为`2561c13c628f3fef8a244ac74e866fa62b059b05989e511fb3f4660314b813e2`；构建报告`vcs.modified=false`和`exactSourceAvailable=true`。替换前二进制保存为`/tmp/cgm-before-video-label-20260815-2145`，SHA-256为`e9dc5dc4fe3f1143cdecad8a6a2c86934015a4b63310423bd28d43b96959200c`。
- Gallery详情专项Vitest 7项、TypeScript检查、675模块生产构建及Chromium离线完整业务Playwright 1项均通过；视觉基线无需更新。实际部署CSS资源`index-Btj_g1dP.css`已核对包含`padding:0;background:transparent`。
- 用户服务保持`enabled/active`且`NRestarts=0`，Health/Ready为204、首页为200；本次启动后未发现ERROR、WARN、FAILED、panic或fatal。没有修改数据库、配置、媒体、Manifest或缓存，也没有触发schema迁移。

### 2026-08-15 视频第一、第二阶段正式部署

- 2026-08-15 21:32 CST已将本机用户服务升级到提交`135174fc3a17d76c8887ebfba99120b55dfb976a`。部署产物版本为`1.5.0-dev`，使用`cgm_web_embed cgm_galleryepic`标签从清洁工作树构建，二进制SHA-256为`e9dc5dc4fe3f1143cdecad8a6a2c86934015a4b63310423bd28d43b96959200c`；发行校验确认`vcs.modified=false`且源码归档与完整提交对应。
- 替换前先停止服务，在实际备份根创建额外回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260815T132857Z-135174f.tar.gz`，SHA-256为`e94a134919ec0b1e334096eb798c00e59e472accaf6386f21ee65be6707dce4c`，文件与校验记录权限均为`0600`。回滚包包含一致SQLite快照、启动配置、Coser托管元数据、替换前二进制和systemd用户服务定义，不包含媒体来源、可重建缓存或日志。
- 回滚包已执行SHA-256校验、实际解包、配置/旧二进制/systemd定义/Coser文件树比对；源库与解包库`integrity_check`均为`ok`，规范化SQL dump SHA-256同为`15cbb03765bb65d352f4bcc890b5b294693d3b5a7691ae8bcbee4547f400aa3`，54张表、1648行及产品身份/Setup根一致。
- 新版本首次打开正式数据库时已完成schema v1→v2前向迁移；产品数据库inode仍为`19679716`。迁移前自动快照为`product.sqlite.pre-schema-v1-1786800729629296221.bak`，权限`0600`、SHA-256为`5572e531562c6d77333af522fdf2267b2185a11d8b54d1919495367c48f924f6`，单独验证`integrity_check=ok`且仍报告schema v1；正式库验证`integrity_check=ok`并报告schema v2。
- 宿主机`/usr/bin/ffmpeg`、`/usr/bin/ffprobe`和`/usr/bin/dcraw`均可用。启动配置保持不变，空`ffmpeg_path`和缺省`ffprobe_path`由新适配器按PATH解析；启动日志确认`ffmpeg_enabled=true`、`ffprobe_enabled=true`、`libraw_enabled=true`。
- 启动后11个既有DIRECTORY视频均完成技术探测并写入READY状态，随后11个新版960px BASE Poster任务全部完成；本轮任务ID 382～403均为COMPLETED且无错误码。该回填会只读访问原视频并新增可重建缓存，不修改媒体来源或Manifest。
- 用户服务保持`enabled/active`且`NRestarts=0`；Health/Ready均为204，首页为200。`/about.json`报告完整提交、`buildTime=2026-08-15 20:15:00`和`exactSourceAvailable=true`；主JS/CSS、Gallery详情块和按需视频播放块均返回200，启动后journal未发现ERROR、WARN、FAILED、panic或fatal。
- 自动化与正式部署验证已经完成；仍需由所有者在已登录目标浏览器中以真实DIRECT、Remux和Transcode视频人工验证拖动、暂停、全屏、长时播放及代理首次等待体验。尚未实际执行回滚恢复演练，不应把“备份可校验”等同于“恢复演练已通过”。

### 2026-08-13 上一增量部署

- 2026-08-13 01:44 CST已将本机用户服务增量升级为Manage Settings输入控件浅色主题修复构建；Coser详情头像直达管理、精确社交图标、Browse人物头像链路、GalleryEpic可拔除Provider与此前1.5功能一并保留。
- 当前二进制SHA-256：`a42fb73f919daf1aa8abef6e8f1bf24936f17684ab09f7b08de4cc7ae9c52e0a`。产物以`cgm_web_embed cgm_galleryepic`标签构建，Provider仍与核心业务保持构建期解耦。
- 替换前二进制保存在`/tmp/cgm-before-settings-light-controls-20260813`，SHA-256为`0674c59a06dcb305ab2c6f46c2e5758e5855009a2a7e7534ee02f6486ef11d8d`；更早的Coser管理直达部署备份仍为`/tmp/cgm-before-coser-manage-link-20260813`。
- 部署后用户服务保持`enabled/active`；`/healthz`和`/readyz`为204，`/about.json`报告`buildTime=20260813`、`gitHash=local`、`exactSourceAvailable=false`。
- 首页已核对实际引用本轮`index-fsxEioKQ.js`和`index-OhZ8A7Bj.css`，两项资源均返回200。浏览器若保留旧页面，只需正常刷新以取得新的入口HTML。
- 本机配置已按本次明确要求加入`"metadata_scraping_enabled": true`，SHA-256为`ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`且权限仍为`0600`。产品数据库inode仍为`19679716`；本次部署没有替换数据库、媒体库、缓存或Manifest。
- 本构建来自包含尚未提交1.5修改的本地工作树，因此`/about.json`正确报告`exactSourceAvailable=false`，不能作为正式发行源码对应证明。
- 启动配置已加入`"log_level": "DEBUG"`；journal可查看结构化请求、管理操作和工作器事件。日志不记录GraphQL变量、搜索词、媒体路径或业务元数据。
- 1.5新增扫描规则编辑和二次确认删除。修改规则后必须重新执行`Scan selected library`，最近一次发现快照不会被静默重写；删除规则不影响现有Gallery或媒体。
- 没有`.cosplay.json`时，空`.cosplay-root`识别出的新Gallery以来源文件夹名称作为确定性标题保底；Cast页可显示对现有Coser、Work、Character名称/Alias的非强制匹配提示，关系仍必须人工应用并保存。
- Coser社交账号Platform key可选择常用平台，也可继续输入符合既有格式的自定义key。
- Manage → Core entities → Cosers → Profile现可使用“导入网络Coser资料”。它只在所有者主动搜索、选择候选、预览并勾选字段后访问GalleryEpic和写入头像、Banner或关联账号；启动、扫描、Browse及后台计划任务不会自动外联。部署后受控在线测试已确认已知Coser `298`的搜索、头像、Banner和至少2个账号可取得；首次静态资源TLS超时后重试成功，第三方网络波动不会阻塞本地业务。
- Gallery与核心实体详情始终从本机CGM GraphQL后端读取最新值；保存关系后再次主动读取，避免Apollo内存缓存让已保存Credit/Cast看似丢失。该行为不访问互联网。
- Character必须选择Primary Work后才能创建；Gallery空关系行、非法外部URL、无效媒体库/扫描规则/运行时设置等同类输入均在前端禁用提交。
- Browse、Gallery详情、实体/搜索/个人页面、Manage和系统页面已迁移到GalleryEpic参考几何的浅色设计系统；Gallery媒体计数只显示非零类型，详情媒体为2/3/4/6列连续网格，Photo/Selfie、GIF、Video按既定顺序展示但隐藏分类标题，Lightbox首尾不循环。
- Gallery索引统一使用参考站2/3/4/5列断点和16px间距；右下媒体计数为无底色12px常规白字。精确鼠标设备仅在封面悬浮或卡片内键盘聚焦时显示Gallery收藏按钮，触屏/粗指针设备保持常驻。
- Gallery详情标题不再受`34ch`人为上限约束；700px以下标题/操作区改为单列，极长标题可安全换行且不会产生横向溢出。
- Gallery媒体卡片与Lightbox均可收藏单项；静态图片可设为封面，Lightbox可设置半星评分并进入媒体详情。`?item=<uuid>`支持深链和浏览器返回，关闭后自动定位到原媒体；这些操作继续使用本机GraphQL及revision校验。
- 单媒体三点菜单打开后，点击菜单外任意区域或按`Escape`会自动关闭；点击菜单内部不会误关，进入媒体详情或设置封面后也会收起菜单。
- Gallery列表卡片的三点菜单按钮不显示悬浮提示；点击媒体进入Lightbox后，右上角收藏、评分、封面、媒体详情和关闭操作分别显示本地化功能提示，且与屏幕阅读器读取的名称一致。
- Core Entities → Character选择Primary Work后，候选列表立即关闭且Character表单保留所选Work；相同修复同时覆盖Gallery关系、Tag父级和实体合并目标等通用实体选择器。
- Gallery媒体页按PHOTO、SELFIE、动画和视频固定类型组，再按完整父目录分区；每个类型中的Gallery根目录固定置顶，文件夹只能在本类型内移动且保留内部顺序，具体文件夹可显式按文件名自然排序。同名文件以父目录区块和完整路径提示区分。
- 人工扫描默认开启“自动排除本次新发现的Gallery根目录媒体”，可在本次扫描前关闭；子目录媒体继续默认纳入，既有Item的Exclude/Restore选择不会被重扫覆盖，Restore会立即补排基础派生任务。
- Gallery卡片中的每个Coser头像和名称均可点击或通过键盘进入对应Coser详情；当前GalleryCard Chunk为`GalleryCard-D4XuBiZt.js`。
- 已保存托管头像的Coser现在会在Gallery卡片和Coser/Model人物索引中显示实际`avatar-480`；头像URL为带revision的同源认证资源，不暴露文件路径。Gallery卡片人物行使用32px头像、6px间距和14px/500名称；无头像仍回退名称首字符。
- Coser详情固定显示X、Facebook、Instagram、微博、Patreon和Linktree六个本地精确SVG；缺失账号显示灰色不可点击占位，已有账号显示深色链接。ACTIVE/INACTIVE、键盘焦点、自定义Platform key追加和通用图标回退均保留，不从参考站加载图标资源。
- Coser详情头像现在可点击或通过键盘进入`/manage/cosers?uuid=<UUID>`，后台直接读取该人物的资料编辑器，不受Coser管理列表当前分页限制；入口仍受现有所有者Session认证保护。
- Coser详情已按指定GalleryEpic页面复核为4:1 Banner、方形头像叠层、紧凑社交行和36px控件；多页作品集显示居中数字分页并保留`scope/page`URL。当前详情块为`EntityDetailPages-CME-83GY.js`。
- Browse侧栏现在按Cosplay（作品集/Coser/作品来源/Magic）与Album（作品集/Model）分区；Cosplay Lists和Magic分别只显示非成人/成人COSPLAY，Album与Model展示全部分级ALBUM。同一人物仍共享一个Coser UUID，可依据实际作品类型同时出现在Coser与Model视图。
- Coser/Model人物索引只显示主名称，不显示Alias；36px头像与名称固定在同一Grid行并垂直居中。名称按参考站使用14px字号、14px行高、500字重和8px头像间距；Alias仍可用于搜索，Work、Character等其他实体索引的Alias展示不变。
- 新增`/albums`、`/models`和`/model/:slug`；Album卡片人物入口进入Model详情。作品来源页先显示Work文字索引，Work详情显示Character，Character详情再显示Gallery。
- Manage → Settings现在只读显示实际缓存绝对路径、逻辑占用和文件数；不提供修改、迁移、清理或重建操作。当前Settings块为`ManageSettingsPage-BorC3vfI.js`。
- Manage → Settings的数字输入框和Home source下拉现在统一为浅色设计系统的白底深色控件，并具有一致的键盘焦点轮廓；旧主题`#100e10`黑底规则已从该页面移除。
- Chromium离线完整业务流程、Album/Cosplay隔离、Models归属、单媒体收藏/封面、Lightbox深链/返回、axe A/AA、键盘焦点以及桌面/390px截图在0.2%视觉差异门禁下通过；Firefox/WebKit、真实移动设备、读屏和200%缩放仍需目标环境人工验收。
- `make web-ui-start`可在`127.0.0.1:3100`启动Vite HMR并同源代理当前9999后端。已验证首页与`/session/status`代理均返回200；开发服务器验证后已停止。
- 详细实现、测试和回退记录见[V1_5_DEVELOPMENT_LOG.md](./V1_5_DEVELOPMENT_LOG.md)。

## 部署基线

- 运行形态：Linux amd64 原生单所有者服务。
- 当前源码提交：`28e71a7a4c095b095b58d9c742ccc48941d30769`。
- 当前产品版本：`1.5.0-dev`。
- 二进制：`/home/rainbowrunner/.local/bin/cgm`。
- 当前二进制SHA-256：`07cc18ecbe5611a812170e753b8e48db2224adbbfc6d851a43556d8cce6f6729`。
- 启动配置：`/home/rainbowrunner/.config/cosplay-gallery-manager/cgm.json`，权限 `0600`。
- 产品数据库：`/home/rainbowrunner/.local/share/cosplay-gallery-manager/product.sqlite`，权限 `0600`。
- 生成缓存：`/home/rainbowrunner/.cache/cosplay-gallery-manager/`。
- 用户服务：`cosplay-gallery-manager.service`，已启用并绑定 `127.0.0.1:9999`。

这是原生部署而不是 Docker 部署。媒体库根不属于启动配置，也没有在部署时预先写入数据库；它必须在首次 Setup 完成后从 Manage → Libraries 按真实业务流程新增。原生进程直接使用当前用户的文件系统权限，因此以后可新增、禁用或迁移任意真实绝对路径，而不需要先修改容器挂载。

## 已完成的部署验证

- 当前分支为 `agent/cgm-migration-handoff-20260726`，部署构建前工作树干净。
- React TypeScript、8 个 Vitest 文件/15 项测试、661 模块生产构建通过。
- 主要 Go/SQLite/API 包回归通过。
- 单文件二进制不依赖旧 Stash UI，报告完整提交号。
- systemd 用户服务启用、启动和重启通过；重启前后数据库 inode 保持一致。
- `/healthz`、`/readyz` 返回 204，`/setup` 与 `/legal` 返回 200。
- `/about.json` 显示精确提交源码 URL，`exactSourceAvailable=true`。
- 只监听 IPv4 loopback `127.0.0.1:9999`，不对局域网或公网开放。
- 当前`/session/status`确认`setupComplete=true`；本次无浏览器Cookie的部署探针显示`authenticated=false`属于预期，不改变所有者浏览器中的既有Session和业务配置。

本机已安装`/usr/bin/ffmpeg`、`/usr/bin/ffprobe`和`/usr/bin/dcraw`。当前配置继续将`ffmpeg_path`留空且未显式写入`ffprobe_path`，1.5适配器已按PATH正确解析成对工具；正式启动与既有视频探测/Poster回填已验证。显式路径仍可用于固定部署依赖，但不是当前本机运行的必要条件。

## 首次真实业务初始化（历史流程，已完成）

本机最初按以下真实业务流程完成Setup；这些路径以正式数据库中的当前记录为准：

1. 浏览器打开 `http://127.0.0.1:9999/setup`。
2. 环境选择“本机直接运行 / NATIVE”；loopback 原生 Setup 不需要一次性票据。
3. 由所有者设置不少于 8 位的独立密码，选择 `zh-CN` 和实际拍摄时区。
4. Coser 元数据根填写：
   `/home/rainbowrunner/cos/coser`
5. 备份根填写：
   `/home/rainbowrunner/cos/bk`
6. 完成 Setup 并重新登录。Setup 不会自动扫描媒体。
7. 进入 Manage → Libraries，新建实际媒体库并填写真实绝对路径；先用小型代表性目录验证权限、规则和发现结果，再扩大范围。
8. 显式执行发现，审阅 Candidate 后导入为 DRAFT；再显式扫描、解决阻断问题、补充 Cast/关系并激活。

媒体库根应优先以只读权限开始验收。只有需要显式 Gallery Manifest Push 时才授予对应来源写权限；CGM 不删除、移动或改写源媒体，但 Manifest Push 会按产品确认流程写入 sidecar。

## FFmpeg/FFprobe诊断

当前无需再次安装。可使用以下命令确认宿主机工具，并在Manage → Settings查看CGM解析后的版本、来源和稳定错误码：

```bash
command -v ffmpeg
command -v ffprobe
ffmpeg -version
ffprobe -version
```

如以后需要把PATH解析改为固定路径，可在启动配置同时设置`ffmpeg_path`和`ffprobe_path`后重启：

```bash
systemctl --user restart cosplay-gallery-manager.service
curl -fsS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9999/readyz
```

预期状态码为 `204`。补齐后再用真实 MP4 样本完成 Poster、Range 播放和必要代理验收。

## 日常操作

Manage → Settings中的`Generated cache`显示实际缓存位置、总量、永久BASE和可回收ENHANCED占用。
`Reclaimable cache limit (GiB)`可设置按需Lightbox等可重建缓存的上限；保存后后台最多约一分钟内
执行LRU。`CARD_480`与静态Poster不会被该上限删除，缓存位置仍只能通过部署配置设置。

首次打开尚未生成大图的静态媒体时，页面会先显示480代理并提示正在准备全尺寸视图；后台只读
访问该来源媒体生成4096代理，完成后自动切换。回收后的大图在下次打开时按相同流程重建。

```bash
systemctl --user status cosplay-gallery-manager.service
systemctl --user restart cosplay-gallery-manager.service
journalctl --user -u cosplay-gallery-manager.service --since today
```

停止应用：

```bash
systemctl --user stop cosplay-gallery-manager.service
```

重新启动：

```bash
systemctl --user start cosplay-gallery-manager.service
```

不要同时启动第二个进程访问同一 SQLite 文件。升级前先在 Manage → Operations 创建并核验完整备份，同时单独备份媒体文件；CGM 完整包不包含媒体、Gallery sidecar、缓存或日志。

## 第一轮业务验收记录项

- Setup、登录、注销和服务重启后的重新登录。
- 实际媒体库路径权限、最具体子根和禁用子根边界。
- 小批量 DIRECTORY、ZIP/CBZ、JPEG/PNG/GIF/RAW；FFmpeg 补齐后加入 Video。
- Candidate 审阅、DRAFT 导入、原子扫描、问题修复、Cast 与 ACTIVE 门禁。
- Browse 列表、Gallery 详情、封面、Scrubber、搜索、时间线、收藏、评分与历史。
- 只读来源的 Manifest Push 失败提示，以及单独可写测试来源的显式 Push/Pull/冲突处理。
- 每日快照、手工完整备份、独立副本和一次隔离恢复演练。
- 服务重启后的任务恢复、来源暂时失联和重新挂载。

本机业务测试不替代尚未完成的真实 Linux arm64、Docker arm64、Firefox/WebKit/Edge、真实移动浏览器、读屏、200% 缩放、远端 CI/夜间任务和正式 RC 签名验收。
