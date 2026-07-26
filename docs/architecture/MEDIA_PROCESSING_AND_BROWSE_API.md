# 媒体处理与 Browse API 实施记录

## 1. 媒体处理边界

- 扫描事务只发现和对账，派生资源进入SQLite持久化租约队列。
- 任务身份包含`item_uuid`、内容revision、variant和媒体处理profile；重复投递幂等，内容替换硬失效旧资源并取消旧任务。
- `CARD_480`、`LIGHTBOX_4096`和`STATIC_POSTER`固定属于不可被LRU淘汰的BASE层；更高卡片尺寸、动画预览和播放代理属于ENHANCED层。
- 新profile成功发布前继续服务旧profile；发布在数据库事务中切换current，缓存文件使用同目录临时文件、fsync和原子rename。
- 缓存读取和删除拒绝绝对路径、路径穿越、符号链接及特殊文件。应用只能删除自身生成的缓存。

## 2. 媒体格式与来源

- 扩展名只参与发现，文件头决定STATIC_IMAGE、ANIMATED_IMAGE、VIDEO及IMAGE/RAW/VIDEO内容族。
- DIRECTORY不跟随符号链接；ZIP/CBZ成员按既定资源和结构限制读取到应用临时目录，使用后清理。
- RAW由LibRaw兼容适配器只读生成JPEG代理；普通图片复用Stash已有图片栈；Video/动画Poster复用Stash FFmpeg命令构建能力。
- EXIF/XMP日期、自拍目录语义和RAW/JPEG伴生只形成建议，明确接受后才改变业务元数据。

## 3. 封面与Scrubber

- `preferred_cover`保存人工或Manifest意图，`effective_cover`保存实际可显示结果。
- 首次可用静态成员只随机一次；人工封面缺失时保留意图并随机替代，纯视频可临时使用Poster。
- Gallery卡片Scrubber索引固定按PHOTO、SELFIE、GIF、VIDEO排序；GIF/Video只返回静态Poster。
- 专用资源地址使用`set_id + scrubber_revision + ordinal`。旧revision返回冲突，绝不让同一URL静默指向另一成员。

## 4. 认证资源服务

- HTTP资源服务只接受不透明UUID和资源版本，不接受用户媒体路径或缓存路径。
- AccessResolver负责把Session转换为Browse或Manage能力；未认证返回401，不可见、跨scope和未知资源统一返回404。
- Browse校验ACTIVE、LIST/MAGIC/ALL、Hidden、来源可访问、OVER_LIMIT、阻断Issue、成员AVAILABLE且未排除。
- 资源支持GET/HEAD、Range、ETag、私有不可变缓存和`nosniff`，响应中不包含物理路径。

## 5. Browse白名单DTO

- `BrowseGalleryCard`只包含稳定UUID/Slug、标题、类型、分级、封面资源身份、关系摘要、日期、P/S/G/V、个人状态和Scrubber轻量状态。
- P/S/G/V统计AVAILABLE且未排除的原始成员；PENDING/ERROR仍计数，MISSING/UNREADABLE不计。
- Gallery详情成员一次返回最多1,000项的完整轻量索引；前端按视觉组每次展开24项，Lightbox使用完整索引导航。
- 随机媒体卡不返回Gallery标题，只返回Character行、Coser行和Gallery路由身份；Album的Character列表为空，由前端保留空白行。

## 6. 已实现的查询契约

- 首页按数据库设置选择LIST/MAGIC/ALL，只执行最近收录查询。
- Gallery列表24项分页；Coser 30项；Work/Character/Tag 60项。
- 时间线排除未知日期，MONTH按当月1日排序，同日按`added_at DESC,id DESC`，支持Coser过滤。
- 搜索仅使用Gallery和核心实体名称、sort name、Alias；关系可间接命中Gallery，实体必须关联当前scope可见Gallery。
- 强关系推荐权重为Character 100、Work 50、Coser 40、同类型5，同类型不能单独成为候选。
- Tag推荐采用最多3层、`p^depth`、多路径最大值、IDF加权Jaccard和可配置最低分。
- 随机页默认24项、静态/GIF/Video软配额70/10/20、Gallery重复衰减0.25，均由带revision的运行时设置管理。
