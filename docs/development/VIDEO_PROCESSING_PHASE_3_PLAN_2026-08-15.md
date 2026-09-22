# CGM 视频处理第三阶段功能规划

> 状态：条件式后续规划，尚未实现
> 记录日期：2026-08-15
> 当前分支：`agent/cgm-migration-handoff-20260726`
> 阶段定位：第一、第二阶段稳定后的视频体验增强；不属于当前第一版/1.5完成门禁
> 前置规划：[视频处理第一、第二阶段功能规划](VIDEO_PROCESSING_PHASE_1_2_PLAN_2026-08-15.md)

## 1. 阶段定位

第三阶段不重做第一、第二阶段已经确定的探测、Poster、DIRECT、Remux和H.264/AAC MP4代理，而是在完整基础播放稳定后解决两个可选问题：

1. 用户拖动视频进度时缺少画面定位能力；
2. 大型且必须转码的视频需要等待完整MP4代理生成后才能开始播放。

第三阶段拆成两个可独立验收的子阶段：

- **第三阶段A：时间轴Storyboard**。按需生成缩略图Sprite和WebVTT，在媒体详情与Lightbox播放器的辅助时间轴中显示画面预览。
- **第三阶段B：大视频渐进播放**。只有第二阶段真实业务指标证明完整MP4代理冷启动不可接受时，才通过独立ADR启用单清晰度渐进HLS；没有证据时保持不实现。

视频pHash、自动去重和跨Gallery匹配不属于播放体验，应作为独立未来项目，不并入第三阶段。

## 2. 前置条件与启用门槛

第三阶段A开始前必须满足：

- VP1全部门禁通过，技术元数据、选定轨道、旋转/HDR和Poster结果可信；
- VP2的认证播放资源、按需状态、取消、LRU和Lightbox停止上一视频均通过；
- 本机真实FFmpeg/FFprobe可用，DIRECT、REMUX、TRANSCODE三类样本均已验收；
- 当前媒体processing profile、数据库schema和回退备份点已记录。

第三阶段B额外要求先完成只在本机保存的无观看画像基准：

- SSD及实际业务媒体挂载分别测试至少短、中、长三档视频；
- 分开记录DIRECT、已缓存MP4代理和必须转码的冷启动首帧/随机seek耗时；
- 不保存“用户看了什么”、播放进度或按Gallery的观看统计，只保存测试样本类别、技术参数和聚合耗时；
- 若第二阶段已缓存播放首帧和seek满足日常需要，或冷代理等待可接受，则不实施HLS；
- 只有必须转码的大视频冷启动反复超过经实际业务测试确认的阈值，才提交ADR冻结HLS协议、依赖、并发和缓存模型。

## 3. 不可改变的边界

1. Gallery卡片、Gallery网格和Gallery卡片Scrubber继续只显示静态Poster，绝不加载Storyboard、HLS或自动播放视频。
2. Storyboard和分段资源只在当前Lightbox视频或媒体详情中由用户实际交互触发。
3. 用户媒体继续只读；所有Sprite、VTT、playlist、segment和临时文件只能写入CGM生成缓存。
4. 所有资源继续使用Session认证和不透明UUID/revision；playlist、VTT和segment内不得出现物理路径或无需认证的永久URL。
5. 第三阶段仍不保存播放进度、观看次数、观看时长或行为画像。
6. 不恢复Stash Scene、旧流媒体路由、旧hash目录或旧Manager业务依赖；只包装底层FFmpeg和流生命周期机制。
7. 不引入字幕、多音轨选择、多清晰度自适应、硬件编码、DLNA、外部播放器、360°或Dolby Vision专用流程。
8. Windows原生构建继续延期；第三阶段规划不新增Windows任务。

## 4. 第三阶段A：时间轴Storyboard

### VP3A-01 派生资源身份

- 新增`VIDEO_STORYBOARD_SPRITE`和`VIDEO_STORYBOARD_VTT`两个ENHANCED variant，均以item UUID、content revision和storyboard profile为身份。
- Sprite与VTT是一个逻辑生成结果：只有两个文件均完成、校验通过并写入同一数据库事务后才标为current；任何单边失败都不能被前端使用。
- 两个资源统一计入现有ENHANCED容量和LRU；任一个被回收时同组整体失效，避免VTT引用已删除Sprite。
- 不把Storyboard计入Gallery可展示条件、effective cover或Item基础READY判定；Poster仍是唯一视频BASE资源。

### VP3A-02 采样与生成规则

- 仅使用第一阶段已经选定的主视频轨，不重新选择轨道，不读取字幕或附件。
- 根据有效时长自适应采样25～81帧，均匀覆盖约1%～99%的有效区间；相邻采样至少间隔0.5秒，极短视频减少帧数而不重复制造相同画面。
- 默认每格最大160px且不放大，按应用旋转后的显示方向保持比例；横向视频使用横格，竖向视频使用竖格，不强行拉伸为16:9。
- Sprite固定最多9列，使用JPEG并采用一致背景；VTT cue覆盖整个有效时长并只使用规范`xywh`片段。
- 截帧先快速seek，单帧失败后精确seek；允许跳过少量失败帧并重建连续cue，低于最低有效帧数时整个任务失败。
- HDR输入沿用第一阶段Poster的SDR色调映射，避免时间轴预览与播放器画面色彩方向明显不一致。
- Storyboard profile至少包含采样算法版本、帧数、格尺寸、布局、JPEG质量、旋转/HDR策略和FFmpeg主版本。

### VP3A-03 按需任务与状态

- 新增路径无关的Storyboard状态/请求契约，返回`PENDING | PROCESSING | READY | ERROR`、Sprite/VTT不透明资源身份和稳定错误码。
- 打开视频不自动排队；只有用户第一次悬浮、聚焦、触摸或键盘操作辅助时间轴时才请求。
- 同一item/revision/profile的并发请求汇聚为一个任务；内容替换取消旧任务并HARD_INVALID旧资源。
- 任务优先级低于当前视频播放代理但高于后台回填；用户离开播放器不强制取消已经接近完成的小型Storyboard任务。
- LRU回收后，下次实际操作时间轴可重新生成；列表、卡片和Scrubber不能触发重建。

### VP3A-04 认证资源与VTT安全

- Sprite继续复用认证派生资源服务；VTT使用明确`text/vtt; charset=utf-8`、`nosniff`和私有缓存响应。
- VTT只引用同一item/revision/profile的应用内相对不透明Sprite URL，服务端生成时禁止绝对URL、用户路径、反斜杠、`..`和未知scheme。
- Browse与Manage继续使用各自可见性规则；未知、过期和无权限资源统一404。
- VTT和Sprite使用不可变ETag；成功读取更新同组`last_accessed_at_utc`，避免只读VTT时Sprite被单独回收。

### VP3A-05 前端辅助时间轴

- 保留浏览器原生视频控件，同时在播放器下方增加可选CGM辅助时间轴，不在第一步替换为完整自定义播放器。
- 悬浮或拖动辅助时间轴时显示对应缩略图和本地化时间；点击/触摸位置只调用当前video元素seek，不写数据库。
- 键盘支持聚焦、左右方向键小步移动、Home/End定位，并以`aria-valuetext`读出时间；缩略图本身为装饰，不让读屏重复朗读图片。
- Storyboard准备中继续允许使用原生播放器；失败只隐藏缩略图预览并显示可重试提示，不能阻断视频播放。
- 切换Lightbox成员或关闭播放器时清理事件、对象引用和状态轮询；不预取相邻视频Storyboard。
- reduced-motion不禁用静态缩略图，但时间提示的出现/消失不使用强动画。

### VP3A-06 测试与退出门禁

- 单元：自适应帧数、采样时间、横/竖格布局、VTT cue/xywh、极短视频、旋转和HDR profile。
- Worker：双资源原子发布、部分失败清理、取消、内容替换、重复请求、LRU同组回收和回收后重建。
- 安全：恶意VTT内容、路径穿越、跨item/revision引用、未认证、跨scope和过期资源均被拒绝。
- React/Playwright：首次交互才请求、准备/失败不阻断播放、鼠标/触摸/键盘seek、Lightbox切换清理且Gallery卡片零Storyboard请求。
- 真实媒体：横屏、竖屏、短视频、长视频、旋转和HDR样本的缩略图时间对应正确，来源hash/mtime/size保持不变。

## 5. 第三阶段B：条件式大视频渐进播放

### VP3B-01 协议选择门禁

- 默认不同时实现HLS和DASH；若ADR批准，只选择HLS，避免两套manifest、segment、播放器和缓存清理路径。
- Safari优先使用原生HLS；Chrome/Firefox只有在本地打包、离线可用且通过许可证审计的HLS客户端支持下启用，禁止CDN脚本。
- DIRECT和已就绪的第二阶段MP4代理仍是首选；HLS只解决“必须转码且完整代理尚未就绪”的大视频冷启动，不替换所有播放。
- 初版只生成单一最高1080p且不放大的rendition，不做自适应码率、多清晰度或用户画质选择。

### VP3B-02 渐进会话模型

- 用户实际播放且方案命中HLS条件时，创建短期、不透明、仅当前所有者Session可用的播放会话；会话ID不是媒体身份。
- 会话只存在于应用运行期，记录item UUID、content revision、profile、状态、最后访问时间和已发布segment序号，不记录观看位置或历史。
- 默认同一实例只允许一个CPU实时转码会话，额外请求排队并显示明确准备状态；并发上限以后只能通过有界设置调整。
- 浏览器断开或停止请求后进入短暂空闲期；超过空闲阈值取消FFmpeg并清理未完成会话文件，不能留下孤立进程。
- 相同item/revision/profile的并发页面复用同一只读会话输出，但每个HTTP请求仍独立认证。

### VP3B-03 HLS生成与完成后提升

- 使用固定约4秒片段、关键帧对齐和fMP4 HLS；playlist只列出已经完整fsync并发布的segment，完成后写入`ENDLIST`。
- FFmpeg显式选择第一阶段固定的视频/音频轨，编码、旋转、HDR和1080p策略与第二阶段TRANSCODE一致。
- 活跃会话文件位于CGM cache临时命名空间，不进入`media_derivatives` current集合；HTTP只允许数据库/会话注册过的manifest和segment名称。
- 会话成功完成后，校验playlist与全部segment，原子提升为一个ENHANCED HLS bundle；以后同一profile直接复用完整bundle。
- 未完成会话、取消或失败只删除应用临时segment；不能触碰用户来源、BASE Poster或已经发布的MP4代理。
- HLS bundle按整体容量和最后访问时间参与LRU，不能只删除其中一个segment形成残缺current资源。

### VP3B-04 数据与缓存扩展

- 在ADR批准后才设计并迁移`media_derivative_bundles`及bundle成员表；第三阶段A不提前创建HLS专用空表。
- bundle身份继续使用item UUID、content revision、variant和profile；成员只保存规范相对缓存名、MIME、byte size、序号和校验值。
- 增强缓存统计增加完整HLS bundle、活跃会话临时占用和未完成清理量；临时占用也必须受磁盘余量门禁。
- 数据库迁移前继续使用Online Backup安全快照；旧二进制回退需要恢复对应schema备份。

### VP3B-05 认证manifest与segment服务

- manifest入口只接受不透明会话或bundle身份；segment URL由服务端生成，不能接受任意文件名、路径或远端URL。
- manifest和每个segment请求均验证Session、Gallery/Item可见性、scope、content revision和会话/bundle归属；不能仅依靠难猜URL。
- 响应使用正确HLS/fMP4 MIME、`nosniff`和private缓存；活跃manifest短缓存，完成bundle使用带revision/profile的不可变私有缓存。
- 会话取消、内容替换、Item排除、Gallery退出Browse或Session撤销后立即停止继续发布新segment，后续请求统一不可见。

### VP3B-06 前端选择与回退

- 播放状态明确返回`DIRECT | MP4_PROXY | HLS_SESSION | HLS_BUNDLE`，前端不根据文件后缀猜测。
- HLS播放器只在服务端方案明确选择后加载；Safari走原生能力，其余目标浏览器按本地HLS客户端能力初始化。
- HLS启动失败时停止会话并回退到第二阶段完整MP4代理请求；失败不能形成自动无限重试或同时运行两份转码。
- 切换媒体、关闭Lightbox、注销或进入维护模式时释放播放器并通知会话空闲；服务端超时清理仍是最终保障。
- UI只显示“正在准备视频/回退到兼容播放”等业务状态，不显示FFmpeg命令、物理路径或内部segment名称。

### VP3B-07 进程、日志与恢复

- 复用Stash流管理中的context取消、idle清理和只读锁思想，但以CGM会话、UUID授权和缓存边界重建，不导入Scene Manager。
- 启动时清理确认属于CGM临时命名空间的过期会话目录；不扫描或删除用户媒体，也不把临时HLS纳入备份。
- 稳定事件码覆盖会话排队、启动、首segment、完成、空闲取消、回退和清理；默认日志不记录路径、标题、Range或观看位置。
- Manage只显示当前/排队会话数量、技术item UUID、mode、耗时、临时字节和错误码，不提供观看历史。

### VP3B-08 测试与退出门禁

- 协议：活跃playlist只引用已完成segment、完成后ENDLIST、关键帧seek、Safari原生与Chrome/Firefox本地客户端播放。
- 并发：单转码上限、同item会话复用、排队、公平取消、断线idle清理、服务重启孤儿清理和磁盘不足暂停。
- 安全：伪造会话/segment、跨scope、旧revision、排除/MISSING、注销、维护模式和路径遍历全部拒绝。
- 回退：HLS失败只能启动一个MP4代理任务；不能出现HLS与MP4无限互相重试或重复转码。
- 真实业务：大视频冷启动首帧和随机seek相对第二阶段完整代理有可证明改善，否则撤销HLS实现，不因已经投入开发而保留无收益复杂度。
- 来源不变性：所有样本在会话、取消、完成、LRU和重启清理前后hash、mtime和size一致。

## 6. 实施顺序

1. 第一、第二阶段全部通过后，先实现VP3A-01～VP3A-04及双资源原子发布。
2. 接入VP3A-05辅助时间轴，完成VP3A-06门禁；Storyboard可作为独立提交完成。
3. 使用第二阶段真实播放器执行第三阶段B基准，不采集日常观看历史。
4. 如果没有达到HLS启用条件，在实施状态中记录“不实施”及证据，第三阶段到此结束。
5. 如果达到条件，先提交ADR，再依次实现VP3B-01～VP3B-05、VP3B-06～VP3B-08。
6. HLS schema迁移和部署单独进入维护窗口，先备份、再迁移、最后真实大视频验收；不得和Storyboard普通增量部署混为一次不可回退变更。

## 7. 明确不纳入第三阶段

- Gallery卡片视频Hover播放或Gallery卡片内时间轴；
- 多清晰度自适应码率、DASH和云端/CDN转码；
- 字幕、音轨/视频轨切换、倍速记忆和跨设备播放状态；
- 硬件编码、GPU自动探测和厂商专用滤镜；
- 视频pHash、重复媒体建议、自动合并或AI内容分析；
- 观看统计、进度同步、热门度或推荐权重；
- ZIP/CBZ视频、外部播放器、投屏/DLNA和来源文件操作；
- Windows原生构建与验收。
