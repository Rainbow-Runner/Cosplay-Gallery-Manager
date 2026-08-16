# Cosplay Gallery Manager 实施状态

> 当前里程碑：1.5 本机业务迭代与可诊断性增强
> 状态：进行中
> 更新日期：2026-08-17

## 已规划、尚未实现

- 后续（视频处理第三阶段）：已持久化[第三阶段条件式功能规划](VIDEO_PROCESSING_PHASE_3_PLAN_2026-08-15.md)。第三阶段A规划按需Storyboard Sprite/WebVTT和可访问辅助时间轴；第三阶段B仅在第二阶段真实大视频冷启动指标证明必要并完成ADR后，才规划单清晰度渐进HLS、会话治理和完整bundle缓存。该阶段不属于当前第一版/1.5门禁，尚未实现或部署。

## 已完成

- 1.5（可管理媒体分类规则，已部署）：新增独立于Gallery根发现的全局/媒体库规则，支持父目录、文件名、文件stem、完整相对路径与Exact/Glob/Go RE2；order相同时媒体库规则优先，首个命中停止且PHOTO可显式排除。只评估STATIC_IMAGE并只生成待审核建议，扫描或保存规则不自动覆盖人工/Manifest分类。
- 1.5（分类校验与审核，已部署）：产品schema v4持久化规则revision及PENDING/ACCEPTED/REJECTED/SUPERSEDED建议，迁移保留旧目录语义建议并播种可编辑/删除/恢复的默认规则。RE2由后端保存层强制编译，前端必须验证当前表达式后才能保存；后台提供单路径测试、最多200项数据库预览、显式存量评估及逐项/批量接受拒绝。2026-08-17已从清洁提交`6e61b9a`完成额外完整回滚包、自动schema v3快照与正式schema v4完整性验证后增量部署。
- 1.5（Coser详情跨作品类型）：仅Coser详情将原LIST/MAGIC/ALL控件替换为“全部作品/COSPLAY/ALBUM”；默认混合查询该人物全部类型，COSPLAY固定`scope=ALL`并合并LIST与MAGIC，ALBUM沿用全部分级口径。筛选与分页写入`type/page` URL；Model详情、Coser/Model索引及其他Browse分级逻辑不变。
- 1.5（Gallery动画播放窗口）：Gallery详情完整时长动画预览由固定视口前4项改为可配置连续窗口；安全上限默认12、范围1～16，超过上限时悬浮150ms锁定以目标为中心的N项，切换冷却默认800ms并可在Manage Settings设为700～1000ms，移开不重置。实际解码仍与视口求交，Lightbox/reduced-motion优先停播；触控设备按可见动画移动窗口。两项设置由产品schema v3持久化，v1/v2升级前均创建来源版本准确的在线快照。
- 1.5（Gallery视频标识弱化）：Gallery详情媒体卡片右上角VIDEO/GIF类型文字取消75%黑色背景和内边距，改为透明背景、82%白字及轻量文字阴影；保留媒体类型语义和右上角操作菜单层级，不新增遮罩色块。
- 1.5（视频第一阶段）：产品schema v1→v2迁移在写入前创建并校验SQLite Online Backup；新增FFmpeg/FFprobe成对诊断、产品自有技术元数据、确定性主轨探测、每分钟25项有界存量回填、人工重试，以及20%时间点/960px/不放大/旋转/HDR到SDR的BASE Poster链路。Manage Settings、Gallery媒体行与媒体详情只显示白名单技术状态和稳定错误码。
- 1.5（视频第二阶段）：实现MP4/H.264/AAC保守DIRECT矩阵、认证且路径无关的原视频GET/HEAD/Range路由，以及按Lightbox/媒体详情实际需求才排队的Remux/H.264-AAC Fast Start MP4代理；代理固定为ENHANCED并受容量、磁盘余量、原子发布和LRU约束。前端准备态保持Poster，完成后切换原生播放器，成员切换会卸载上一视频；Gallery卡片和Scrubber不触发播放。
- 1.5（视频安全、真实工具与正式部署门禁）：原视频授权复核ACTIVE/Scope/Hidden/可用性/Exclude/Blocking Issue/current revision与DIRECT方案，逐级拒绝符号链接并保持同一文件描述符响应；日志只写稳定`CGM_VIDEO_*`技术事件。本机FFmpeg/FFprobe/dcraw合成媒体门禁已实际覆盖MP4、MOV、MKV、WebM、无音频、双音轨、旋转、HDR、Poster、Remux和Transcode。2026-08-15已先创建并完整校验额外回滚包，再将正式库由schema v1迁至v2；自动v1快照和迁移后主库`integrity_check`均通过，11个既有视频探测与11个新版Poster全部READY。目标浏览器中的真实DIRECT/Remux/Transcode交互及实际回滚恢复演练仍待人工执行。
- 1.5（Manage设置输入框浅色收敛）：清除Settings页数字输入框残留的旧`#100e10`黑底/白字规则，数字输入和Home source下拉统一使用浅色设计Token，并补齐主色边框与键盘焦点环；浏览器回归锁定全部相关控件的白底深色计算样式和axe A/AA结果。
- 1.5（Coser详情直达管理）：Coser详情头像改为指向`/manage/cosers?uuid=<coser UUID>`的认证后台深链接，点击后直接载入对应Coser资料编辑器，不依赖目标是否出现在后台当前分页；入口保留原头像视觉，新增中英文可访问名称、悬浮边框与键盘焦点反馈。Model详情不展示人物资料头像，因此不扩展该入口。
- 1.5（Browse人物头像与社交视觉）：修复Coser/Model索引转换遗漏`avatarURL`以及Gallery卡片人物摘要没有头像资源身份的问题；两处均使用认证、不透明、带revision的`/resource/coser/.../avatar-480`，不暴露托管文件路径。Gallery卡片人物行对齐为32px头像、6px图文间距和14px/500名称；Coser详情按参考站固定显示X、Facebook、Instagram、微博、Patreon、Linktree六项精确实心SVG，缺失账号以灰色不可点击图标占位，自定义平台继续追加并保留通用回退。
- 1.5（可拔除Coser网络资料Provider）：新增站点无关`cosermetadata`契约与短期预览、编译期可选GalleryEpic适配器、安全出站HTTP、按名称候选搜索以及头像/Banner/社交账号人工逐项导入。默认配置关闭，扫描/Browse/启动/计划任务不外联；无Provider核心构建不依赖GalleryEpic，完整功能构建只通过`cgm_galleryepic`标签组合站点模块。全部选中资产先校验暂存，再与SocialAccount在一个数据库事务中发布，Coser revision只递增一次并标记Manifest dirty。
- 1.5（Gallery卡片参考站密度）：索引网格改为与GalleryEpic公开页一致的2/3/4/5列断点和16px统一间距，取消宽屏6列；媒体计数改为无底色的12px/16px、400字重白字；Gallery收藏按钮在精确鼠标设备仅悬浮/键盘聚焦时显示，触屏/粗指针保持常驻。3:4比例、零值过滤、P/S/G/V语义、分页和Gallery详情媒体网格均不变。
- 1.5（Gallery详情标题响应式）：移除主标题`34ch`人为宽度上限；桌面标题使用标题列全部可用宽度，窄屏将操作区移到标题/统计区下方并右对齐，标题字号采用响应式范围，极长无空格标题可安全换行而不产生横向溢出。
- 1.5（Browse滚动与Gallery定位）：桌面64px Logo/品牌栏随页面滚动离开视口，左侧分区导航保持固定；1024px以下继续保留粘性顶部栏和Drawer入口。Gallery“更多详情”支持点击外部/`Escape`关闭，显示非MISSING实际媒体的去重绝对父目录并直达对应Manage媒体页；这是经确认的认证单所有者有限路径例外，不返回文件名、相对路径、指纹或缓存路径。
- 1.5（实体候选列表关闭）：通用`ManageEntitySelector`将弹层开关与Apollo查询缓存解耦；Character选择Primary Work以及Gallery关系、Tag父级、合并目标等场景在选择成功后立即关闭候选列表，再次聚焦仍可重新搜索。
- 1.5（Lightbox操作提示）：Gallery列表三点按钮不显示悬浮提示；进入单媒体Lightbox后，右上角收藏、评分、封面、媒体详情和关闭五项操作显示与状态、语言及`aria-label`一致的提示。
- 1.5（媒体菜单消隐）：Gallery单媒体三点菜单在打开期间支持点击外部或按`Escape`关闭；监听器按需注册/清理，菜单内部操作、媒体详情跳转和封面revision语义保持不变。
- 1.5（Gallery详情高保真）：详情页移除独立封面Hero并采用紧凑标题/操作区、2/3/4/6列媒体网格和256px相关推荐栏；混合媒体按Photo/Selfie、GIF、Video依次连续展示，隐藏分类标题，同时保留各组每次24项的增量加载边界。
- 1.5（单媒体操作）：Gallery媒体卡片与Lightbox已接入单媒体收藏、半星评分、静态图片设封面和媒体详情入口；Lightbox以`?item`作为深链状态，支持浏览器返回、筛选内导航、关闭后定位和`last_item_id`记录。
- 1.5（Cosplay/Album双分区）：Browse主侧栏按GalleryEpic参考结构重组为Cosplay（Lists/Cosers/Parodies/Magic）与Album（Lists/Models）；新增`/albums`、`/models`、`/model/:slug`，Characters退出主导航但旧路由继续保留。
- 1.5（派生人物视图）：不新增Model实体，继续由GalleryCast是否存在推导COSPLAY/ALBUM并统一使用Coser UUID；Cosers只列出有可见COSPLAY的人物，Models只列出有可见ALBUM的人物，同一人物可同时出现于两边，卡片按类型进入对应详情路由。
- 1.5（分区查询口径）：Cosplay Lists固定`COSPLAY + NON_ADULT`，Magic固定`COSPLAY + ADULT`；Album Lists与Models按确认方案展示全部分级ALBUM。Browse GraphQL新增向后兼容的可选`collectionType`，旧调用不传时保持原行为。
- 1.5（GalleryEpic实体索引层级）：Coser/Model索引采用居中搜索、四列圆形头像文字行和数字分页；作品来源使用四列文字索引，Work详情先列Character，Character详情再列Gallery；Model详情采用Breadcrumb后直接显示Album网格。
- 1.5（人物索引垂直对齐）：Cosplay/Coser与Album/Model列表固定为36px头像加主名称的单行Grid，不显示Alias；头像与名称中心线由浏览器几何回归约束在1px内，人物Alias搜索和其他实体索引的Alias展示保持不变。
- 1.5（人物索引字型对齐）：根据GalleryEpic公开页实际类名和CSS，将Coser/Model名称精确收敛为14px字号、14px行高、500字重及8px头像间距；Playwright读取浏览器计算样式执行严格回归，通用实体索引不受影响。
- 1.5（项目README产品化）：根README已从原Stash宣传与安装内容改写为CGM项目入口，准确记录Gallery业务闭环、功能/安全边界、Linux与Docker支持矩阵、构建/Setup步骤、数据保护、测试和文档导航，并保留Stash衍生归属与AGPL源码对应要求。
- 1.5（按需Lightbox缓存）：`CARD_480`与静态Poster继续作为不可回收BASE，扫描不再预生成4096大图；首次打开静态图Lightbox/媒体详情时幂等排队`LIGHTBOX_4096`，页面先显示480图并轮询单Item状态，生成结果属于ENHANCED。
- 1.5（缓存容量管理）：Manage → Settings显示缓存绝对路径、文件数/总量及BASE/可回收分层占用，并以GiB编辑可回收上限；后台每分钟按上限和磁盘余量执行真实访问时间LRU，只删除ENHANCED，回收后在下次查看时重建。
- 1.5（缓存兼容迁移）：启动工作器前幂等地把已有`LIGHTBOX_4096`记录和任务Payload迁为ENHANCED，不立即删除文件；默认50 GiB上限不会使当前145.46 MiB大图缓存因部署自动消失。
- 1.5（来源访问审计）：显式Gallery扫描完整读取来源并计算BLAKE3，普通静态图只预生成480；首次查看尚未缓存的大图会由后台任务读取来源，HTTP仍只服务CGM派生缓存且不直接暴露原图。
- 1.5（Gallery卡片计数）：P/S/G/V媒体数量改为后置类型缩写并过滤零值，例如`100P`或`96P 4S 2V`；四类全为0时不渲染计数，既有后端计数口径和顺序不变。
- 1.5（GalleryEpic视觉改造起步）：建立2026-07-30参考版本的浅色语义Token与208/256px侧栏、64px顶栏等冻结几何变量；新增原创内嵌SVG图标、原创CGM字标及首批Button/Input/Badge/Alert基础组件，Browse顶栏不再使用Unicode搜索、收藏、信息和设置符号。
- 1.5（GalleryEpic BrowseShell）：Browse已迁移为浅色固定桌面侧栏、64px粘性品牌顶栏和1024px以下移动Drawer；主路由按Cosplay/Album分组，扩展功能保留在Explore与底部工具区，Drawer支持遮罩/Escape关闭、焦点循环与菜单按钮焦点恢复。
- 1.5（Browse无障碍与视觉回归）：新增BrowseShell/Drawer/分页/Tab/DataTable组件测试；修复浅色迁移暴露的侧栏标题与Gallery facts低对比度，Chromium离线主流程axe A/AA、键盘、1440桌面与390移动截图回归通过。
- 1.5（GalleryEpic UI-03～UI-05）：Gallery卡片完成Character/Work/Coser三层信息、头像组和封面右下零值过滤计数；Gallery详情完成Breadcrumb、小封面紧凑标题、Related右栏、4/3/2媒体网格及首尾不循环Lightbox；实体、搜索、时间线、随机、个人和媒体详情统一浅色高密度视觉。
- 1.5（GalleryEpic UI-06～UI-07）：Manage及Setup、Login、Legal、Maintenance统一浅色Token与无衬线层级；恢复、Gallery删除、实体生命周期和Coser托管资源清理共用可聚焦Dialog，原确认词、密码、revision、Mutation和危险语义不变。
- 1.5（GalleryEpic UI-08）：视觉回归阈值由1%收紧为0.2%，桌面Browse和390px Gallery详情基线已更新；离线完整业务流程、axe A/AA、键盘和焦点恢复回归通过。
- 1.5（Gallery卡片Coser入口）：卡片中每个Coser头像与名称均可通过鼠标或键盘进入对应详情页；复用UUID入口到规范slug的既有重定向，不扩展DTO或增加查询；同名不同UUID不会被错误合并。
- 1.5（Coser详情高保真复核）：按指定GalleryEpic Coser页冻结4:1 Banner、128/80px方形头像叠层、紧凑名称/社交行、36px控件和移动两列几何；增加Breadcrumb与无Banner中性占位，保留Country、Profile、Biography、社交链接、Scope和Timeline原语义。
- 1.5（Coser Gallery数字分页）：多页作品集使用居中的Chevron和最多5个连续数字页码，当前页浅边框圆角高亮；点击更新现有`scope/page`并读取本机GraphQL，不改变24项分页、DTO或数据库，Gallery网格现统一采用后续确认的2/3/4/5列约束。
- 1.5（开发日志）：启动配置新增`DEBUG/INFO/WARN/ERROR`日志级别；systemd journal可按稳定事件码、请求ID、端点类别、状态和耗时关联排查，默认不记录查询字符串、GraphQL变量、媒体路径或业务元数据。
- 1.5（增量开发）：React开发服务器固定在loopback `3100`，通过同源代理复用`127.0.0.1:9999`后端、真实数据库和Session，可使用Vite HMR检查前端；后端继续采用单进程增量构建与用户服务重启，避免两个进程同时访问SQLite。
- 1.5（扫描规则管理）：媒体库确定性扫描规则已补齐编辑和二次确认删除；可修改名称、类型、顺序、启用、自动建DRAFT、深度或RE2模板，类型切换会清空不适用字段。
- 1.5（媒体目录排序与根目录扫描策略）：Manage媒体页按完整父目录分组，根目录在各媒体类型组固定置顶；文件夹只在PHOTO/SELFIE/动画/视频各自组内移动并保留内部顺序，可对具体文件夹显式执行文件名自然排序。人工扫描默认自动排除本次新发现的根目录媒体并提供本次开关，既有Exclude/Restore选择不被覆盖，Restore会立即补排基础派生任务。
- 1.5（规则安全与审计）：规则更新/删除复用既有确定性校验并写入无路径管理审计；删除规则只解除最近发现候选的规则引用，不删除Candidate、DRAFT、GallerySource或媒体文件。
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
- 1.5增量：MARKER导入在缺少Gallery Manifest时以来源根目录文件夹名作为标题保底；对已有Coser、Work、Character名称/Alias的目录名匹配仅作为Manage Cast页人工应用提示，不自动创建实体或关系，PATH_TEMPLATE待审核语义不变。
- P02-05：实现source-scoped扫描暂存、完整成功后的原子提交，以及失败/取消不产生半扫描MISSING。
- P02-06：接入完整BLAKE3和采样BLAKE3；同路径保留身份、同来源完整指纹唯一时重绑定、歧义时保留旧MISSING并创建独立Item。
- P02-06：Manifest `set_id`只生成SOURCE_REBIND_CANDIDATE；来源改绑必须显式确认，两个可访问副本还需要重复来源二次确认。
- P02-07：IgnoredGallerySource优先于规则并支持set_id；Item排除跨重扫保留；显式忘记只删数据库记录并永久Tombstone UUID。
- P02-08：新增ZIP/CBZ只读安全校验，覆盖路径穿越、NFC/大小写冲突、链接、特殊Entry、加密Entry、嵌套归档、Entry/体积/压缩比/像素上限和归档媒体限制。
- P02-09：首次成员按媒体组和自然相对路径分配全局Position；Manage完整父目录/文件名自然排序以完整同组UUID序列原子提交并复用既有Position槽位；旧单项移动契约仍限制同组；1,000上限只统计非排除成员。
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
- P05（GraphQL）：新 schema 已按 Browse 与 Manage 用途分层；除认证单所有者`GalleryDetail.mediaParentDirectories`父目录摘要有限例外外，Browse DTO 不返回物理路径，Manage Mutation 使用 Gallery 或实体 `metadata_revision` 乐观锁。
- P06（BrowseShell）：React 19 已实现 Home、List、Magic、Gallery、Coser、Work、Character、Tag、时间线、搜索、随机、收藏、历史和单媒体详情路由；Gallery详情使用完整轻量成员索引与每组手动展开24项。
- P06（Gallery卡片）：实现横向位置映射 ordinal、离开恢复封面、精确指针设备启用及全局后台开关；PHOTO/SELFIE/RAW/GIF/VIDEO统一使用静态派生资源或Poster。
- P07（ManageShell）：实现高密度Gallery问题索引、两阶段媒体库发现/导入、来源扫描、任务队列、运行时设置，以及Gallery五页签和Coser四页签编辑结构。
- P07（Gallery编辑）：基本元数据显式保存；成员分类、Caption、排除、组内排序和封面即时生效；Coser→Character与Tag关系按Gallery范围原子批量保存，Album/Cosplay由Cast自动推导。
- P07（核心实体）：Coser/Work/Character/Tag可独立创建与编辑；Coser社交账号保持人工顺序，未关联Gallery的Coser仍可在Manage中维护。
- P07（管理表单校验）：Character要求Name和Primary Work；媒体库、扫描规则、核心实体、社交账号、Gallery关系/外链、Tag父级和运行时设置在必填项或数值范围无效时禁用提交，Mutation进行中防止重复提交。
- P07（社交平台键）：Coser社交账号提供常用Platform key选择，同时保留符合`[a-z0-9][a-z0-9_-]{0,63}`的自定义键；外部账号URL仍只保存不抓取。
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
- P07（关系回显）：Gallery和核心实体详情绕过Apollo陈旧内存缓存从本机GraphQL后端读取；关系保存后显式refetch，已保存的目录名匹配显示`Already saved`，不会误导用户重复应用。
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
- Gallery关系GraphQL集成回归额外验证Credit/Cast实际持久化及无路径技术审计；前端回归验证重新读取后正式关系显示、目录匹配已保存状态和未完成关系行提交门禁。
- Character缺少Primary Work时，前端创建按钮保持禁用；后端防御性回归验证直接GraphQL调用返回明确校验错误而非`internal server error`，并记录失败审计。
- GraphQL集成测试验证Gallery与Coser Manifest只能通过显式Mutation写入各自确定路径，并返回CLEAN状态。
- 运维集成测试验证每日快照到期租约/保留、自动扫描opt-in、完整恢复、Session撤销、任务取消、安全备份注册、异机路径映射门槛、人工恢复，以及数据库交换后故障的自动回滚。
- 完整备份损坏矩阵验证POSIX/Windows路径穿越、重复/大小写/NFC碰撞、保留名、符号链接、未知Entry、畸形JSON、缺项、无效SQLite产品身份、压缩炸弹和截断ZIP均在替换前拒绝。
- Operations GraphQL测试验证备份/恢复由Server服务执行且响应不泄漏数据库或存储根。
- React 19 TypeScript `--noEmit`通过；Vitest当前23个文件、58项测试全部通过。
- Vite生产构建通过，共转换671个模块；当前主JS minify后、gzip前约474.33KiB，其余页面按路由生成懒加载块，原大于500KiB分包警告已消失。
- `cgm_web_embed`标签下的产品UI嵌入和Server回归测试通过；`make build-cgm`生成约24MiB单文件验证产物，深层SPA路由、哈希资源immutable缓存和旧UI依赖隔离均已验证。
- 实体生命周期回归验证了按关系类型返回删除阻断、合并冲突时禁用提交、明确确认词、GraphQL预览/提交、永久Alias/Tombstone和管理审计。
- Gallery删除回归验证了归档前阻断、过期revision拒绝、密码与确认词双重门禁、Item/Link/Set UUID Tombstone、任务取消保留、IgnoredGallerySource建立，以及来源媒体和Manifest字节不变。
- Coser托管资源回归验证了未认证上传拒绝、同源Session上传、实际JPEG派生、过期revision冲突、GIF拒绝、认证资源读取、URL不泄漏根路径，以及替换后旧原图/派生图仍保留。
- Coser未引用资源回归验证了现用组排除、已替换/已删除分组、未知文件与符号链接跳过、引用变化后过期审阅拒绝、密码/确认词/同源门禁、选择性清理、Manifest保留和无路径审计。
- BLAKE3依赖固定为`github.com/zeebo/blake3 v0.2.4`并记录模块校验和。
- 用户来源能力审计未发现删除调用；应用删除仅限失败备份、缓存、临时文件等明确生成数据。
- Chromium离线Playwright主流程通过，Setup/Browse/Gallery/Coser/Operations axe扫描无WCAG A/AA违规，桌面与390px移动截图在0.2%像素差异门禁下回归通过；Gallery详情新增1440px桌面基线，并实际验证单媒体收藏、封面切换、Lightbox深链/返回与关闭后定位。
- 真实媒体门禁通过：合成标准DNG由dcraw实际生成代理，动画GIF与FFmpeg生成MP4实际生成Poster，内容分类与扩展名无关。
- 2026-08-16新增静态原图混合直读与完整时长动画预览：合格JPEG/PNG/静态WebP经认证原图端点直读，其余静态图回落4096 ENHANCED代理；Gallery详情按后台可配置播放窗口与视口交集按需播放480px/15FPS完整时长动画WebP，Lightbox/媒体详情播放精确GIF/动态WebP原字节。DIRECTORY及ZIP/CBZ、安全scope、reduced-motion和真实2秒FFmpeg时长均有回归。
- 危险归档单元矩阵通过；完整备份损坏矩阵继续在替换前拒绝危险输入。
- 固定`GOMAXPROCS=4`的百万Item性能门禁通过：最慢Gallery列表p95约493ms、时间线约477ms，均低于500ms；Tag未缓存约141ms、缓存约143ms，其他目标均通过。
- 固定工具链CGO构建通过Linux amd64与Linux arm64；Linux amd64 Docker镜像实际构建、非root启动和`/healthz` 204通过。
- About/Legal后端、前端、产品版本/源码URL测试通过；SPDX文件可解析且包含1个产品包与52项应用依赖。
- `verify-cgm-release`已在干净本地提交上通过：嵌入式Linux amd64二进制报告完整Git revision，Go VCS元数据为`modified=false`，同提交AGPL源码归档包含`LICENSE`并生成独立SHA-256。

## 本机实际业务应用测试部署（2026-07-27）

- 2026-08-16 22:49 CST已从精确提交`4c0dadacee45b47850f3d3a4074f804838b16925`增量部署静态原图混合直读与完整时长动画预览构建，SHA-256为`c195f0e16b4c56628e241fa54ef4e113b2db1bbce2d1873830dc4abc373f68bf`；旧二进制备份于`/tmp/cgm-before-image-animation-20260816`。服务保持`active/running`、`NRestarts=0`，Health/Ready为204，About报告精确源码且`exactSourceAvailable=true`；配置和产品数据库inode不变，启动日志无迁移或处理错误。
- 2026-08-17 00:03 CST已从精确提交`f22ca41d237def7d70e489522422dd4f7a3819a5`增量部署可配置动画播放窗口构建，SHA-256为`a66ab60a43cc5a9a572afca8b9767cf529f6e32b3bcfa0a6a36598bd8c252cfc`。部署前额外完整回滚包和自动schema v2快照均已解包/独立完整性校验；正式库无记录损失迁至schema v3并采用12项/800ms默认值。服务保持`active/running`、`NRestarts=0`，Health/Ready为204，About精确指向部署提交，启动日志无异常。
- 2026-08-17 00:36 CST已从精确提交`edf4bf30c8090e70cbe2624487d65d4648853a10`增量部署Coser详情跨作品类型筛选构建，SHA-256为`f34233ad63c524cb60564c7fe19ba03f628819d70343a58aaa90671d4edc7c8f`；旧二进制保留于`/tmp/cgm-before-coser-types-20260817`。服务保持`active/running`、`NRestarts=0`，Health/Ready为204、About精确指向部署提交；配置/数据库inode不变且无schema迁移，启动日志无异常。
- 2026-08-12 00:29 CST已增量部署Gallery详情标题响应式与Gallery卡片参考站密度构建，SHA-256为`9e9477e36d1572150a7fe0502d7981e3fcd0dcff533168254e34baf06a43bf35`；旧二进制备份于`/tmp/cgm-before-gallery-cards-20260812`，服务保持`enabled/active`且Health/Ready为204。
- 该阶段入口引用`index-BN0Udqs0.js`与`index-qdWPUXcG.css`；`about.json`报告`gitHash=local`、`buildTime=20260812`和`exactSourceAvailable=false`。配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，产品数据库inode仍为`19679716`；没有替换配置、数据库、媒体、Manifest或缓存。
- 2026-08-09 20:28 CST已增量部署Browse桌面滚动与Gallery目录定位构建，SHA-256为`e3e595503e80082d15d0c6c49e225b47ce159aa5c34b47b239567f7360685041`；旧二进制备份于`/tmp/cgm-before-gallery-detail-locations-20260809`，服务保持`enabled/active`且Health/Ready为204。
- 当前入口引用`index-9hJr0i02.js`与`index-hDZ83M8Y.css`；`about.json`报告`gitHash=local`、`buildTime=20260809`和`exactSourceAvailable=false`。配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，产品数据库inode仍为`19679716`；没有替换配置、数据库、媒体、Manifest或缓存。
- 2026-08-09 17:30 CST已增量部署Gallery完整父目录分组/排序与根目录新媒体默认排除构建，SHA-256为`007347be19cd1058d94bb554a0dcfd55df5fbcbd90493f613eb44f9ba65cc90b`；旧二进制备份于`/tmp/cgm-before-gallery-media-folders-20260809`，服务保持`enabled/active`且Health/Ready为204。
- 当前入口引用`index-Br2kDKX5.js`与`index-dwLJ7y5E.css`；配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，产品数据库inode仍为`19679716`。启动日志确认两个工作器正常启动，没有迁移、启动或任务错误。
- 2026-08-08 16:27 CST已增量部署通用实体选择器选择后自动关闭修复，SHA-256为`6766a0c08c714af46face98239953d614cdaa12bdc5c97cdd78aa2848cc56817`；旧二进制备份于`/tmp/cgm-before-entity-selector-dismiss-20260808`，服务保持`enabled/active`且Health/Ready为204。
- 当前入口引用`index-D45_orVP.js`与`index-B5AaBjOr.css`，实体选择器Chunk为`ManageEntitySelector-Be-gqGJ9.js`；配置SHA-256和产品数据库inode部署前后不变。
- 2026-08-01 04:20 CST已增量部署纠正后的Lightbox右上角五项操作提示，SHA-256为`d482e86092d1831b00266b431ad52e3430902ad07c11c4af781e2f94a3cdfb1c`；被替换的误定位版本备份于`/tmp/cgm-before-lightbox-toolbar-tooltips-20260801`，服务保持`enabled/active`且Health/Ready为204。
- 当前入口引用`index-BPUu1Roj.js`与`index-B5AaBjOr.css`；配置SHA-256和产品数据库inode部署前后不变。
- 2026-08-01 04:13 CST曾短暂部署把提示误加到列表三点按钮的构建，SHA-256为`543c40f5995bf2d812d37ce23020514cad94bbe7e8796d8c3d8b65eeb1ccbdd5`；该行为已由04:20构建撤销，记录仅用于回退链追踪。
- 2026-08-01 04:06 CST已增量部署单媒体菜单点击外部/Escape关闭修复，SHA-256为`30c9a63ec9c176043d8b92956e8a7283abcdf4cc77dd4344583ea9d61064ede7`；旧二进制备份于`/tmp/cgm-before-media-menu-dismiss-20260801`，服务保持`enabled/active`且Health/Ready为204。
- 当前入口引用`index-BzI7BpIi.js`与`index-B5AaBjOr.css`；配置SHA-256和产品数据库inode部署前后不变。
- 2026-08-01 03:57 CST已增量部署Gallery详情高保真与单媒体操作构建，SHA-256为`66b235a1f93babc839fd3245c9c8305ff578c1f6b9ba02b1b75128504e49901f`；旧二进制备份于`/tmp/cgm-before-gallery-detail-20260801`，服务保持`enabled/active`且Health/Ready为204。
- 当前入口引用`index-BB00ESCj.js`与`index-B5AaBjOr.css`；配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，产品数据库inode仍为`19679716`。
- 2026-08-01 02:39 CST已增量部署Coser/Model人物名称14px/14px/500字型对齐修复，SHA-256为`5bbf3557bd3e56ed0eb3839797561f70336db2ebde4258e58ff457020d80ec55`；旧二进制备份于`/tmp/cgm-before-person-name-typography-20260801`，服务保持`enabled/active`且Health/Ready为204。
- 当前入口引用`index-Dagulhjn.js`与`index-DUKMwGwd.css`；配置校验和与产品数据库inode部署前后不变。
- 2026-08-01 02:26 CST已增量部署Coser/Model索引单行垂直对齐修复，SHA-256为`53bfdbc288f8e9aecd7f89167fab877234b3edc53e88f5c878e58ad4dfe3f041`；旧二进制备份于`/tmp/cgm-before-person-index-alignment-20260801`，服务保持`enabled/active`且Health/Ready为204。
- 该阶段入口引用`index-B-wD7SgW.js`与`index-pRt2Qbb1.css`，人物索引块为`EntityIndexPage-ikuTtgbU.js`；配置校验和与产品数据库inode部署前后不变。
- 2026-07-31 00:58 CST已增量部署Manage缓存位置/占用只读状态，SHA-256为`c1503fe9653617789e0e522a2dd122aaf4c5b146cf03392160275076e67d1007`；旧二进制备份于`/tmp/cgm-before-cache-status-20260731`，服务保持`enabled/active`且Health/Ready为204。
- 2026-07-31 00:29 CST已增量部署Coser详情高保真与数字分页最终构建，SHA-256为`1aac8277d217cac3a5c5d7d0ab41970937a7802b26ee28f2555c069d17b5e2b0`；旧二进制备份于`/tmp/cgm-before-coser-detail-20260731`，服务保持`enabled/active`且Health/Ready为204。
- 2026-08-01 01:54 CST已增量部署Cosplay/Album双分区与Model视图构建，SHA-256为`c1ffa2edc8f9086b8e6e160c1bd17c325dbe4db9b10dfb5b4be68ac482ef6a0b`；旧二进制备份于`/tmp/cgm-before-cosplay-album-20260801`，服务为`active`且Health/Ready为204。
- 该阶段入口引用`index-DXmm-gQE.js`与`index-WDjoyUJX.css`；配置校验和与产品数据库inode部署前后不变。
- 2026-07-30 23:56 CST已增量部署Gallery卡片Coser详情入口构建，SHA-256为`87e87d2128f1fd2d258d6ef2768b991c152c0c41adeb74643a536bb6912c99ce`；旧二进制备份于`/tmp/cgm-before-coser-card-links-20260730`，服务保持`enabled/active`且Health/Ready为204。
- 2026-07-30 02:40 CST已增量部署UI-03～UI-08完成后的`1.5.0-dev (local)`单文件构建，SHA-256为`38af89d5afe479a6f38d88d3a08735b68a03044c5648cf5a00f28a79e089dded`；旧二进制备份于`/tmp/cgm-before-galleryepic-ui-20260730`。
- 用户服务保持`enabled/active`，Health/Ready为204，Root/Legal为200；入口HTML已确认引用本轮GalleryCard、GalleryDetail和Patterns哈希资源。
- 配置校验和和产品数据库inode部署前后不变；没有替换数据库、配置、媒体库、缓存或Manifest。运行中的SQLite文件大小变化属于服务重启后的正常写入/检查点。
- 在干净提交`3dc86fb6be38216349fb039a6b3253fb392ae422`上重新运行TypeScript、8文件/15项Vitest、661模块生产构建和主要Go/SQLite/API包回归，结果通过。
- 构建并安装Linux amd64原生单文件到`~/.local/bin/cgm`；二进制报告完整提交，SHA-256为`ef2dc87446daaee84ddc8c187aebfb877e6419545d4782ee987a5d6c688e918a`，且依赖边界未包含旧Stash UI。
- 建立私有配置、数据库、缓存、Coser元数据和备份目录；用户级`cosplay-gallery-manager.service`已启用并运行，仅监听`127.0.0.1:9999`。
- `/healthz`、`/readyz`为204，Setup/Legal为200，About显示精确源码提交；真实systemd重启后数据库inode和未完成Setup状态保持不变。
- 按产品流程没有在部署时预置媒体库根、所有者密码或扫描任务；当前`setupComplete=false`，下一步由所有者在Setup完成后从Manage → Libraries配置真实绝对路径。
- 本机当前已可发现`/usr/bin/dcraw`、`/usr/bin/ffmpeg`和`/usr/bin/ffprobe`，源码合成视频门禁已通过；运行中的已部署服务仍是旧构建，本轮没有修改其配置、迁移正式数据库或重启服务。正式部署必须先核对/补齐`ffmpeg_path`与可选`ffprobe_path`、保存数据库维护快照，再执行真实业务媒体与目标浏览器验收。
- 详细路径、首次初始化、服务操作和第一轮业务验收清单见[本机实际业务应用测试部署](LOCAL_BUSINESS_TEST_DEPLOYMENT_2026-07-27.md)。

## 尚未通过的门禁

- 主机仍没有系统级Go 1.25；本轮使用校验过的`/tmp/cgm-go1.25.12`，交叉构建镜像和新增CI均固定1.25.12。
- Firefox、WebKit、Edge及真实iOS Safari/Android Chrome最近两版尚未执行；当前Playwright证据仅为Linux Chromium、1440/390视口与axe自动检查，不能代替真实读屏/浏览器/设备验收。
- Linux arm64已通过CGO构建，尚未在真实arm64系统完成安装、路径、SQLite、FFmpeg/LibRaw、升级和备份恢复运行验收。
- Docker linux/amd64已实际健康启动；linux/arm64镜像运行需要宿主机全局binfmt或原生arm64 runner。本轮特权binfmt注册被安全策略拒绝，故双架构Docker门禁尚未全部通过。
- 只读媒体源由E2E和容器挂载覆盖基础流程；真实操作系统网络挂载、Manifest Push无写权限和低性能移动设备首屏仍需目标环境验收。
- 新增CI/夜间工作流尚未在远端runner执行，不能根据本地语法和子门禁结果推定通过。
- RC版本/API/schema冻结、正式多平台产物签名/发布校验和与容器平台SBOM尚未执行；本地源码对应脚本已通过，但不等同于正式RC产物验收。

## 下一批工作

1. 为视频schema v2安排本机维护窗口：先核验自动`pre-schema-v1`快照与独立完整备份，再部署新二进制、配置同套FFmpeg/FFprobe，并用真实业务视频完成DIRECT/Remux/Transcode、拖动、暂停、全屏、来源hash/mtime不变和恢复演练。
2. 在Linux arm64及arm64 Docker runner执行安装、媒体、升级和完整恢复验收，并发布各平台容器SBOM。
3. 在Firefox、WebKit、Edge及真实移动浏览器执行目标宽度、键盘、读屏、200%缩放与低性能首屏矩阵。
4. 在远端执行新增CI/夜间工作流并修复runner差异；随后冻结0.9接口与schema，从干净提交执行`verify-cgm-release`并生成签名发行物。

任何未执行的测试不得在状态记录或发布说明中标记为通过。
