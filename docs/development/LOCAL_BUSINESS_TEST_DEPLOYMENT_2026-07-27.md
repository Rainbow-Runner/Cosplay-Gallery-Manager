# 本机实际业务应用测试部署

## 1.5 增量开发状态

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
- 源码提交：`3dc86fb6be38216349fb039a6b3253fb392ae422`。
- 初始产品版本：`0.1.0-dev`；当前运行版本见上方1.5增量开发状态。
- 二进制：`/home/rainbowrunner/.local/bin/cgm`。
- 二进制 SHA-256：`ef2dc87446daaee84ddc8c187aebfb877e6419545d4782ee987a5d6c688e918a`。
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
- 当前 `/setup/status` 为 `{"complete":false}`；尚未代替所有者设置密码、配置媒体库或启动扫描。

本机已安装 `/usr/bin/dcraw`。系统尚未安装 FFmpeg/FFprobe，因为系统包安装需要所有者在交互式终端输入 sudo 密码；当前配置将 `ffmpeg_path` 留空。应用可正常启动，图片与 RAW 流程可测试，但 Video Poster/代理处理必须等 FFmpeg 安装并写入配置后再验收。

## 首次真实业务初始化

1. 浏览器打开 `http://127.0.0.1:9999/setup`。
2. 环境选择“本机直接运行 / NATIVE”；loopback 原生 Setup 不需要一次性票据。
3. 由所有者设置不少于 8 位的独立密码，选择 `zh-CN` 和实际拍摄时区。
4. Coser 元数据根填写：
   `/home/rainbowrunner/.local/share/cosplay-gallery-manager/cosers`
5. 备份根填写：
   `/home/rainbowrunner/.local/share/cosplay-gallery-manager/backups`
6. 完成 Setup 并重新登录。Setup 不会自动扫描媒体。
7. 进入 Manage → Libraries，新建实际媒体库并填写真实绝对路径；先用小型代表性目录验证权限、规则和发现结果，再扩大范围。
8. 显式执行发现，审阅 Candidate 后导入为 DRAFT；再显式扫描、解决阻断问题、补充 Cast/关系并激活。

媒体库根应优先以只读权限开始验收。只有需要显式 Gallery Manifest Push 时才授予对应来源写权限；CGM 不删除、移动或改写源媒体，但 Manifest Push 会按产品确认流程写入 sidecar。

## FFmpeg 补齐

所有者在自己的交互式终端执行：

```bash
sudo apt-get install -y ffmpeg
command -v ffmpeg
command -v ffprobe
```

确认路径后，将启动配置中的 `ffmpeg_path` 从空字符串改为 `/usr/bin/ffmpeg`，然后执行：

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
