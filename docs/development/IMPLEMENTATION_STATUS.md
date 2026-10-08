# Cosplay Gallery Manager 实施状态

> 当前里程碑：1.5 本机业务迭代与可诊断性增强
> 状态：进行中
> 更新日期：2026-10-09

## 2026-10-09 后台保存按钮对比度与完整E2E恢复（源码完成，尚未部署）

- 修复`.settings-metadata-visibility__save`被后台通用按钮颜色规则覆盖：使用带`.manage-shell`作用域的主按钮规则保留#fafafa文字／#171717背景，并提高悬停规则优先级，避免浅色文字落在#f5f5f5背景上。新增独立生产CSS及真实设置页的普通／悬停颜色断言，原Axe无障碍断言不豁免。
- 恢复旧生命周期测试与当前UI一致性：初始化等待允许按钮卸载和导航中的短暂上下文切换；同步元数据旁置文件写回、发现规则、仅扫描检查的现有控件名称，收紧库选择定位以排除规则评估按钮。Manifest先等待成员预览，再核对并确认原生对话框，不绕过写回确认。
- 人工查看差异后更新Coser桌面／手机和Gallery桌面／手机4张陈旧截图基线。长标题单行检查改用确有足够空间的1920px视口，截图仍为1440px；动图先确认实际ANIMATED_PREVIEW播放，再通过已有减少动态效果功能确认STATIC_POSTER回退进行稳定截图，随后恢复正常动画偏好，不放宽0.002像素差异阈值。
- 正常固定基线模式`corepack pnpm exec playwright test --trace off`全部6项Chrome E2E通过（34.7s），完整生命周期25.6s，覆盖初始化、登录、设置无障碍、扫描／草稿／关联／激活、Manifest、宽屏布局／名称颜色、浏览／媒体操作、完整备份、恢复、映射及恢复后扫描。另有45文件199项Vitest、TypeScript和2560模块生产构建通过。恢复维护窗口的短暂503属于既有暂停服务行为，流程正常恢复；Firefox／WebKit未在本轮验证。
- 上一节及历史宽屏记录中的完整E2E阻断描述为修复前状态，现已解除。仅使用`ui/web/e2e/.runtime`隔离业务数据，未修改正式服务、业务库或媒体；未提交、部署或推送镜像，未触及`Check_VAAPI.md`或实机截图。

## 2026-10-08 Coser紧凑月度垂直时间轴（源码完成，尚未部署）

- 仅Coser时间线改为按月倒序分组、月内作品倒序的网格，取消可见分页；普通全站时间线保持现有分页。桌面月份文字区68px，连同轴线间距仅88px；手机轴区24px，月份文字位于该月网格上方，不保留宽大空白列。月份间距桌面48px／手机32px。
- 新增`coserTimeline`有界游标接口，首批按视口估算、每批最多24项；后续临近页底250px预取，首屏不足自动补齐。复用四种日期来源、可见性和卡片关系逻辑；同月跨批合并、重复卡片去重、失败保留已有内容并允许重试，卸载／切换范围或日期取消旧请求。游标绑定Coser／范围／日期模式，以日期及稳定次序键续读，不返回全部历史、不使用不断增长的OFFSET。
- Go `productdb`／`productapi`回归及`go vet`通过；新增游标排序、月精度、跨模式／范围／Coser拒绝与批量上限测试。Web TypeScript、45文件199项测试及2560模块生产构建通过（保留既有大chunk提示）；独立生产CSS Chrome测试3项通过，覆盖390～3840px紧凑轨道、宽屏卡片与后台阅读宽度。
- 开发备忘录与日志同步。未提交、部署、推送或修改正式数据库／媒体；未触及`Check_VAAPI.md`和截图。完整离线生命周期的既有后台按钮对比度阻断仍未处理，不能据此宣称全量业务E2E通过。

## 2026-10-08 浏览区宽屏流式布局（源码完成，尚未部署）

- 补充Coser文字视觉对齐：`.gallery-card__coser-link`显式使用主文字色#0a0a0a，不再默认继承人员行#737373；作品来源与评分仍为辅助灰，固定字号和字重不变。生产CSS布局测试增加390～3840px的名称／辅助文字颜色断言，真实页面生命周期增加默认名称颜色断言；既有完整E2E阻断说明仍适用。

- 根据`Actual Device Screenshot`的2560px实机截图及GalleryEpic公开页面，确认差异来自`.browse-main`的107.75rem居中宽度上限。改为`width:100%`，浏览区填满侧边栏右侧工作区，保留既有内边距、移动端规则、网格断点／列数／间距与3:4封面、4:1展示Banner比例。
- 不全局取消max-width：后台设置／实体表单／说明面板、帮助文字及作品集描述等继续保留各自阅读宽度；卡片角色标题、作品来源和Coser字号及单行省略规则不变。
- 新增使用生产CSS的真实浏览器布局回归，覆盖390／768／1280／1536／1920／2560／3840px、卡片持续扩展、Banner尺寸、无横向溢出、固定文字字号及后台宽度上限；两项Chrome测试通过。已有离线生命周期补充1920／2560／3840px真实页面宽度断言。Web 43文件196项单元测试、TypeScript及2559模块生产构建通过，保留既有大chunk提示。
- 本轮未提交、部署或更新镜像；截图文件夹与`Check_VAAPI.md`不修改、不纳入功能提交。
- 验收限制：完整离线生命周期在进入图库前被既有后台`.settings-metadata-visibility__save`按钮对比度失败阻断（前景#0a0a0a、背景#171717，1.1:1）；隔离浏览器加载改动前HEAD样式已复现相同颜色，本次未修改该样式或豁免检查。因此新增真实页面宽屏断言尚未随完整生命周期跑到，不能宣称全量E2E通过；独立生产CSS布局两项测试通过。

## 2026-10-08 Docker Intel 硬件加速部署与诊断改进

> 功能提交`495057363530c52d9df965a619fb85d38a1db3a2`已推送GitHub当前产品分支并完成本机部署；Docker Hub发布记录见本节后续验收记录。下方“未提交／部署／发布”描述保留为源码阶段历史记录。

- Docker Hub已发布`rainbowrunner2015/cosplay-gallery-manager:sha-495057363530c52d9df965a619fb85d38a1db3a2`，OCI索引摘要`sha256:708284c1bc5563b1ea74128217748e73c591ff3a3de239a303b5d61a2dd93ac3`，linux/amd64应用清单`sha256:ae61a4003c8e1851dc4525aa6b2831db504b6cbb70c1227fc997d6ec0b726bfc`。README和安装手册同步正式标签，GitHub同步当前产品分支`agent/cgm-migration-handoff-20260726`；不覆盖旧main代码或更改默认分支，不更新GHCR或latest标签。上传经本机Docker代理耗时约15分钟，最终成功。

- 本机正式程序从干净克隆构建，VCS `modified=false`，SHA-256为`8fc89c73774e7d812a9d875cedb713900841db88df14733bcbb0722d19e9d093`；停服一致回滚目录为`/home/rainbowrunner/cos/bk/cgm-pre-intel-4950573-iSb9H6tY`，保留数据库、旧程序、配置、服务单元及两处Coser资源。部署后active、NRestarts=0、Health／Ready均204、About精确对应功能提交，warning以上日志为空；正式库完整性ok、schema v22及七类计数`136/108/697/23/10/10/780`不变。
- 正式linux/amd64镜像从同一干净提交构建，非root／只读／no-new-privileges容器完成四挂载空库初始化、登录、未登录401、跨Origin403、重新检测202／冷却429，检测后保持SOFTWARE且VAAPI AVAILABLE；vainfo及真实H.264编码成功，Health healthy。正式三标签后端／CLI回归通过。临时容器均已清理，不涉及正式业务卷；本轮未更新GHCR镜像。

- 最终复验：定向Race与两个包Vet通过；真实驱动日志分类加入负返回值门禁，正常驱动加载后媒体错误不会误报。最终验证镜像应用探测正常为AVAILABLE、模拟缺驱动为DRIVER_UNAVAILABLE；Compose双文件组合校验通过。
- Docker amd64运行镜像加入完整Intel iHD用户态驱动、vainfo及驱动依赖／VAAPI编码器构建检查；保留非root与只读根文件系统。新增可选`docker/cgm/compose.intel.yml`，仅映射指定render节点并补充宿主机数字设备组，不默认要求GPU、不启用privileged、不改变现有卷。
- 设置页按设备不可见、权限不足、驱动加载失败、FFmpeg未编译、实际转码测试失败提供操作建议，显示保存模式。所有者可重新检测，复用启动探测执行锁、25秒超时及30秒冷却；端点要求登录、同源POST，检测完成刷新能力，保留未保存设置。
- Web全量43文件196项测试、TypeScript检查、生产构建通过；追加重新检测／保留未保存设置的定向页面回归通过。Go后端与媒体处理包测试通过。验证镜像构建成功；真实非root只读容器映射Intel节点并添加设备组后，应用VAAPI／QSV探测AVAILABLE、H.264实际编码成功；缺少设备组为PERMISSION_DENIED，未映射设备为DEVICE_MISSING。
- 当前仅源码与本地验证镜像，未提交、部署或推送Docker Hub；README现有已发布SHA标签不含本轮改动。未修改`Check_VAAPI.md`，未访问正式业务数据库。本机通过不替代目标NAS（i3-8100T）的实机验收；Intel仍使用VAAPI执行，QSV仅诊断，arm64未做镜像验收。

## 2026-10-01 已部署：视频耗时诊断与渐进播放

- 真实4K HEVC冷代理耗时84.082／125.362秒，已满足第三阶段B的启用条件；[本轮ADR](../architecture/PROGRESSIVE_VIDEO_PLAYBACK_2026-09-30.md)确认单清晰度HLS临时会话、成功后流复制为现有可回收MP4缓存，无schema、Manifest或迁移包变更。
- 本地源码已加入队列／容量／来源证明／编码／首段／缓存同步及登记的阶段耗时和FFmpeg进度；认证HLS会话、4秒临时片段、可定位Seek、单CPU转码槽、45秒空闲取消、容量检查及启动残留清理；浏览器原生HLS或本地hls.js按需播放，失败可回退现有MP4代理。DIRECT、已就绪MP4优先；Remux继续用既有完整MP4路径。
- 合成视频已验证首段先于完整转码、从中途片段继续、时间戳对齐、完整HLS流复制为MP4及缓存复用；会话认证、旧revision拒绝、临时目录清理和前端播放器销毁有定向回归。功能提交`03965091a3480fc59003eccde7fab593951907e7`已部署本机，无schema迁移；仍须对真实赛博修女4K存档、目标浏览器拖动、首帧／CPU／磁盘峰值进行业务验收，未推送远端或更新镜像。

## 进行中或尚未完成验收

- VAAPI EOF兼容收敛：2026-10-03已把烟测从短帧同尺寸提升为带B帧HEVC全量排空，实机正确识别FFmpeg 6.1.1固定VPP surface pool问题并选择`VAAPI_CPU_SCALE`兼容链；规划器v4、HLS／完整MP4独立Profile、surface pool／VPP稳定错误码及完整VAAPI→兼容VAAPI→软件有界降级均随`34c5625`部署。后端回归、Race、Vet、正式三标签构建及目录／TAR／Copy 7z实机门禁通过；Web检查／构建通过，既有Libraries并行测试稳定性缺口已单独记录。正式服务、数据库和回滚副本验收通过，真实4K浏览器连续播放与Seek仍由所有者继续验收。
- 视频硬件加速：已冻结[分阶段实施方案](HARDWARE_ACCELERATION_PLAN_2026-10-01.md)。HA-01诊断随`92fddf3`部署；HA-02 schema v22设置与规划器随`7beb411`部署；HA-03 NVDEC＋CUDA＋NVENC渐进HLS随`3805cc8`部署。2026-10-02正式设置改为`NVENC`后的真实浏览器请求明确新建硬件HLS而非命中缓存：首片段2.09秒，后续6.79倍／约201fps，片段实际返回浏览器，关闭后正常取消清理，无错误或软件回退，HA-03硬件执行链路验收通过。HA-04的HLS单次安全软件回退、两分钟运行期熔断、完整MP4共用计划／NVENC执行及软硬件Profile隔离随`a7b725c`部署；仅白名单硬件技术错误可回退，其他安全／来源／容量／磁盘／损坏输入错误禁止。HA-05的2秒首片段／后续4秒Profile v3时间表、12～16秒前向缓冲边界及客户端取消不误记504已随`9a5581b`提交并独立部署，真实NVENC目录／TAR／7z门禁、正式服务版本及健康状态通过；本阶段无schema迁移，动态playlist暂缓。HA-06第一闭环的VAAPI H.264／HEVC 8-bit实际解码、缩放、HLS与完整MP4编码已随`9246dcd`提交并部署，以本机iHD实际支持的CQP Profile隔离缓存，并把双解码实际烟测、一次回退和熔断接入原边界；合成实机完整MP4、目录／TAR／7z渐进链路及部署后增强诊断通过，真实4K业务播放仍待所有者验收。设备分槽调度、QSV、10-bit/HDR、多设备和Docker映射仍属后续。
- 存档视频真实业务验收：编码文件头有界解码和可直读成员按需转码已从`0b6afb7`部署本机，仍需对赛博修女的4K HEVC/AAC实际播放、首帧延迟、CPU／磁盘峰值、增强缓存回收和浏览器拖动进行业务验收；见[浏览器播放方案](ARCHIVE_VIDEO_BROWSER_PLAYBACK_PLAN_2026-09-30.md)。
- 封面相似选择的真实图库阈值与吞吐验收：功能已随 schema v21 部署；[实施规划](COVER_SIMILARITY_PLAN_2026-09-30.md)中的 64 位 pHash 距离门槛 6 仍只是保守初值。尚需用获许可的真实替换正例、相似不同图难负例校准误选／漏选，并测量 CARD 签名生成／历史回填耗时。上线不等于真实图库效果验收通过；匹配失败会保留自动重选托底。
- 后续（有界轮转深度校验）：当前已提供单Gallery显式深扫，常规扫描仍以大小/mtime证据快速判断；同大小同mtime替换只会由显式深扫或后续轮转校验发现。可选低I/O预算的按库轮转深扫及进度指标尚未实现，不应把常规快扫宣称为完整内容审计。
- 后续（并发调度与宿主机资源自适应）：已在[开发备忘录第12.5节](../COSPLAY_DEVELOPMENT_MEMO.md)固化实施边界。保留持久化租约队列，借鉴Stash顶层串行/作业内有界并行思路，后续按扫描、普通图片、FFmpeg/RAW重任务和Browse按需任务分池限流，并综合CPU配额、可用内存、Swap及I/O压力动态调整；当前`worker_count=2`及现有处理语义不变，本项暂不实施、不迁移、不部署。
- 长期迁移环境验收：可移植迁移的CLI/Web业务闭环、可选owner continuity、导出预检、范围摘要和中断恢复均已完成并部署；现在只剩由所有者执行真实Linux→Docker迁移与恢复模拟。现有完整备份继续承担无损灾难恢复。操作顺序见[可移植元数据迁移操作手册](PORTABLE_MIGRATION_RUNBOOK.md)。
- 后续（视频处理第三阶段A）：按需Storyboard Sprite/WebVTT和可访问辅助时间轴仍未实施。[第三阶段B原条件草案](VIDEO_PROCESSING_PHASE_3_PLAN_2026-08-15.md)已由真实冷启动证据及本轮ADR更新；当前渐进播放已经部署，但尚未完成真实4K浏览器验收。

## 已完成

- 2026-10-07 NAS首次认领已提交并部署：功能提交`64c99f1fec8ca7c612e358488d6f39845015a079`从干净副本构建，本机程序VCS `modified=false`，SHA-256为`f0ab40701b66dce0143e9a62a1e8f49dd304bd2a0c6a6005d2febde6a40a9b4b`。回滚目录`/home/rainbowrunner/cos/bk/cgm-pre-nas-64c99f1-RW16a2BG`保存停服一致数据库、旧程序、配置、服务单元及两处Coser资源；服务active、NRestarts=0、Health／Ready均204，About对应完整提交，启动journal无warning以上信息。正式库保持schema v22、完整性ok，七类业务计数`136/108/697/23/10/10/780`不变。Docker Hub已发布`rainbowrunner2015/cosplay-gallery-manager:sha-64c99f1fec8ca7c612e358488d6f39845015a079`（linux/amd64），远端索引摘要`sha256:a69be122e0d2c4a25300dcf07da2852a885d87c189681c2c3067f5cdd89649d4`；空库只读容器使用NAS IP Host直接初始化为204，重启保持complete、重复提交400、健康healthy。README镜像标签同步；未更新GHCR镜像或推送Git远端。下方源码阶段的未提交／部署记录为历史状态。

- 2026-10-07 NAS首次初始化源码改为Jellyfin式首次认领：删除一次性Setup门票的CLI、HTTP交换端点、Cookie／Session校验、前端输入和`CGM_LOCAL_DOCKER_SETUP`，不再根据可伪造的HTTP `Host`或容器内来源地址决定初始化权限。数据库尚未初始化时，任一可达浏览器都可直接完成五步向导；Owner密码与`complete=1`仍在同一事务写入，因此只有首个提交成功，完成后后续提交被拒绝。Docker Compose改为发布`9999`供可信局域网设备使用，README与安装手册同步首个用户认领及禁止初始化前公网暴露的边界；旧`setup_tokens`表仅为schema兼容保留且不再读写，无schema迁移。Go `productauth`／`productserver`／CLI及嵌入式UI测试与定向Vet通过；Web TypeScript、43文件195项测试及2559模块生产构建通过，仅保留既有大chunk提示。当前源码未提交、未部署，Docker Hub仍为上一镜像。

- 2026-10-07 最新分支Docker Hub镜像发布：从分支`agent/cgm-migration-handoff-20260726`的清洁提交`2c64fe0a0a1f6aad957b43fee9f56eddd2524398`构建`linux/amd64`镜像，前端生产构建、Go三标签编译及只读根文件系统／临时空数据库容器启动均通过；容器Health为`healthy`，`/healthz`为204，About报告`1.5.0-dev`及完整提交，日志无启动错误。镜像已推送为`rainbowrunner2015/cosplay-gallery-manager:sha-2c64fe0a0a1f6aad957b43fee9f56eddd2524398`，远端OCI索引摘要`sha256:3daa57fb13b6cedb69ed7afb7e6bc52e2f5798318c2f713faa3bfb4af570dc44`，包含`linux/amd64`清单`sha256:781abac94a070f33af8c7af0e59bb1ca3be35f82c6027165a1f422d327bda09f`及构建证明；证明中的VCS来源为项目GitHub URL和目标完整提交，Docker Hub仍无`latest`标签。构建上下文未包含数据库、媒体、配置覆盖或未跟踪的`Check_VAAPI.md`；尚未执行另一台机器的真实首次设置、卷权限及迁移恢复验收，本轮也未推送Git远端。

- 2026-10-03 Gallery外部命名推测规则及真实数据审计部署：实体匹配会先剔除作品序号与媒体数量／容量噪声；Character英文词界、中文强弱优先级和纯数字完整词元门禁生效，扫描层支持`&`／`x`／`×`／`+`多Coser署名。逻辑提交`10fde7bd5749cb5c4940a325aef2287ac53ccc79`、审计报告提交`5b43ca51b07fe39ee182422b02391ed31744b605`已部署；1170条业务名称更新为确认471、存疑688、冲突11，Character `02`的旧误候选201条和`Rem`单词内部误候选均降为0。正式程序SHA-256为`24f4a27b84ca14e3f4cefc7dfe8e9cbc64f416972040c6d3762d1aa40eedbb1f`，回滚目录为`/home/rainbowrunner/cos/bk/cgm-pre-inference-5b43ca5-CzwlCqD8`；服务active、`NRestarts=0`、Health／Ready均204，正式库保持schema v22、完整性`ok`及七类计数`136/108/697/23/10/10/780`。报告中的“确认”仍只表示两层规则一致，剩余存疑／冲突需人工复核。

- 2026-10-03 Coser头像／Banner裁剪器交互升级部署：Manage Coser已移除页内参数控件，选图后弹出1:1头像或3:1 Banner裁剪器；支持1×～3×夹紧缩放、50%框外遮罩、桌面拖拽／滚轮、移动单指／双指、双击复位、Esc取消及Enter确认。浏览器确认后生成固定比例托管图片并复用既有multipart端点，无schema或路径暴露变更。功能提交`1db3a95e4fa0c9c3addfa412e06628c0325c5209`已从清洁独立克隆构建并部署，正式程序SHA-256为`b7f959079836641d5d3cf3b402d1c2fc1349c534eb4a79ba887369caec9a79d0`，最终回滚目录为`/home/rainbowrunner/cos/bk/cgm-pre-cropper-final-1db3a95-5BHjfM`。服务active、`NRestarts=0`、Health／Ready均204，About精确对应提交；正式库保持schema v22、完整性`ok`和七类计数`136/108/697/23/10/10/780`。真实鼠标／触控裁剪与图片上传仍待所有者业务验收。

- 2026-10-03 Coser头像／Banner浏览器上传与可视化取景部署：Manage Coser现可直接从浏览器选择JPEG、PNG或静态WebP，不依赖浏览器访问宿主机或容器内真实路径；选择后按服务端同算法预览1:1头像和3:1 Banner派生结果，头像可缩放并调整水平／垂直保留区域，Banner可移动焦点，确认后才上传。功能提交`aa9b23feeddefb6c0e13649a21bfa0c5c29afea5`已从清洁受跟踪源码构建并部署，正式程序SHA-256为`cf433fd1b20ed0d1a7a7d3a74a003a78c2a8cdc13513f2ee6726fb02c183b6f5`，About精确对应提交；服务active、`NRestarts=0`、Health／Ready均204。无schema迁移或业务数据写入，正式库保持schema v22、完整性`ok`及七类计数`136/108/697/23/10/10/780`。真实认证浏览器上传与Docker卷环境仍待所有者业务验收。

- 2026-10-03 VAAPI EOF兼容修复部署：功能提交`34c5625a43e4cabebadac6b124a1000597c93c9a`已从清洁源码构建，正式程序SHA-256为`0edbbbb24774f3d72539cfd9067ad5db15ac2f3954fe0e42bfe287c0624a863b`且`vcs.modified=false`。最终回滚目录`/home/rainbowrunner/cos/bk/cgm-pre-vaapi-eof-final-34c5625-TuoSfyrB`保存一致schema v22数据库、旧程序、配置、用户服务单元和两处Coser资源；数据库完整性`ok`、七类业务计数保持`135/108/697/23/10/10/780`。服务`active/running`、`NRestarts=0`，Health／Ready均204，About精确对应功能提交，最终启动后无warning以上日志。本阶段无schema迁移、媒体、Manifest、配置或缓存替换；未推送远端或更新Docker镜像。

- 2026-10-02 HA-06第一闭环部署：VAAPI功能提交`9246dcde0e3eebfb2f8147c404e8627cd0208f6d`已从清洁源码构建并部署，正式程序SHA-256为`4000f045717a4563d1a918bc4c9300bacfe403a879c0b9db6e9245220de47819`。回滚目录`/home/rainbowrunner/cos/bk/cgm-pre-ha06-9246dcd-6q1dvaZC`保存正式数据库、旧程序、配置、用户服务单元和Coser资源；服务`active/running`、`NRestarts=0`，Health／Ready均204，About精确对应完整提交。启动后增强诊断约1.35秒完成，仍为NVENC／VAAPI可用、QSV烟测失败；无schema迁移、媒体或Manifest写入，运行设置保持所有者此前选择的VAAPI。

- 2026-10-02 HA-05部署：功能提交`9a5581bab64cf8941acf0c9f1642f7c15f432cec`已从清洁源码构建并部署，正式程序SHA-256为`3acc9b1929325f3465ce3067fd73b2e944dad4595deceedefb6987cf795d9b44`。独立回滚目录为`/home/rainbowrunner/cos/bk/cgm-pre-ha05-9a5581b-LtKRERpt`；服务`active/running`、`NRestarts=0`，Health／Ready均204，About精确对应完整提交且`exactSourceAvailable=true`。无schema迁移、运行设置变更、媒体或Manifest写入；正式设置仍暂留VAAPI并按既有逻辑软件降级，需待HA-06后重新验收硬件执行。

- 2026-10-02 HA-03部署：功能提交`3805cc8d105578382a861ab381860297e48cf61f`已在完整停服回滚副本保护下部署，正式程序SHA-256为`49e36111c09bdcf6b20356c7550ad1c62312791006eab3b7f8104dfc845bad1a`。本阶段无schema迁移；正式库保持v22、完整性`ok`，七类业务计数保持`135/108/697/23/10/10/780`，硬件设置仍为`SOFTWARE`／允许回退／空设备。服务`active/running`、`NRestarts=0`，Health／Ready均204，About精确对应功能提交；启动诊断为NVENC／VAAPI可用、QSV烟测失败，日志无ERROR／panic／fatal。回滚目录为`/home/rainbowrunner/cos/bk/cgm-pre-ha03-3805cc8-TRNCGkvE`；未修改媒体、Manifest、缓存、配置或systemd单元，未远端推送或更新Docker镜像。

- 2026-10-01 HA-02部署：提交`7beb4112b001f7fe537c7e8143d2bd7d1070251e`已在独立停服回滚副本保护下部署，正式程序SHA-256为`534e77e62f0f61f7f28ea1dac8cc1a86ab04d2a65103f9da0685d4ea40ae1e9d`。正式库从schema v21迁移到v22，完整性`ok`、七类业务计数保持`135/108/697/23/10/10/780`，新增设置确认为`SOFTWARE`／允许回退／空设备；自动v21迁移快照及回滚库均验证正常。服务active、NRestarts=0、Health／Ready均204，硬件诊断仍为NVENC／VAAPI可用、QSV烟测失败。回滚目录`/home/rainbowrunner/cos/bk/cgm-pre-ha02-7beb411-UaQEOCsd`；未触碰媒体、Manifest或缓存，实际播放仍为软件管线。

- 2026-10-01最新部署：视频分阶段诊断与渐进播放提交`03965091a3480fc59003eccde7fab593951907e7`已部署本机，正式二进制SHA-256为`984742304e8ec2131ad5343db8c789320f69f24899bb3933bbfc958bb3d596c3`。服务`active/running`、`NRestarts=0`，Health／Ready均204，About精确对应提交；播放端点的未认证请求返回401。无schema迁移，正式库和停服回滚副本均为schema v21、`integrity_check=ok`，七类业务计数保持`135/108/697/23/10/10/780`。回滚目录为`/home/rainbowrunner/cos/bk/cgm-pre-progressive-0396509-TGEhyG`；真实4K存档首帧、拖动、CPU与磁盘峰值仍待所有者业务验收。

- 2026-09-30最新部署：存档视频按需转码功能提交`0b6afb7d6f60f78fb6b7be90f4cf934c7c5aac12`已部署，服务active且NRestarts=0，Health／Ready均204；无数据库迁移，schema v21完整性和七类业务计数保持不变。回滚目录`/home/rainbowrunner/cos/bk/cgm-pre-archive-playback-kYr5sKS5`，详细构建与验收证据见[开发日志](V1_5_DEVELOPMENT_LOG.md)。以下“未提交／部署”均为阶段历史记录；未推送远端或更新Docker镜像。

- 2026-09-30（存档视频安全直读与按需代理，本地源码完成、未提交／部署）：7z单独LZMA编码文件头通过压缩／解压大小、字典、CRC和布局有界验证后，继续走既有Copy成员物理区间证明。仅已证明可直读的存档视频可在用户打开时排队Remux／转码，使用既有ENHANCED可回收缓存；Worker严格直读，绝不复用后台Probe／Poster可临时提取的路径。非存档视频原有按需代理不变。业务7z的MP4成员只读定位与合成编码头Copy视频的FFmpeg完整代理回归通过；未变更schema、Manifest、迁移包或正式业务库。详情见[开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 2026-09-30最新部署：累计媒体替换逐项信息重置、扫描摘要与相似封面重选提交为`31430a9f99befda6eb386189dd9b6347bfb2d918`并部署本机。正式库 v20→v21，`integrity_check=ok`，七类 Coser／Work／Character／Tag／Gallery／Source／Item 计数保持`135/108/697/23/10/10/780`；Health／Ready 均204，服务 active 且 NRestarts=0。升级前完整回滚备份位于`/home/rainbowrunner/cos/bk/cgm-pre-v21-31430a9-Lve8IcwU`。下方同日“未提交／部署”是阶段历史记录，以本条为准。未手动触发全库媒体扫描或 Manifest Push，未推送远端或更新 Docker 镜像；真实图库阈值及吞吐验收仍待进行。详见[开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 2026-09-30（相似封面自动重选，本地源码完成、未提交／部署）：schema v21 加入版本化封面签名、每扫描轮次持久意图和结果；静态 `CARD_480` 生成后从安全缓存计算 dHash／pHash，空闲 Worker 回填旧 CARD。扫描事务保存旧封面签名、给出即时随机托底，随后有界 Worker 等新派生图就绪，dHash 前 5、pHash 绝对距离判断、多张重复图确定性选择；无签名／无匹配／超时保留托底，人工封面变更和后续扫描使旧意图失效。管理页展示每轮匹配结果和合格候选数。感知哈希不用于 Item UUID 或旧逐项信息继承。受影响五个 Go 包全量测试、定向 race、Go Vet、前端 42 文件 182 项测试及生产构建通过；真实图库阈值和吞吐校准未完成，正式库仍 v20。

- 2026-09-30（媒体替换逐项信息重置与扫描变化摘要，源码完成、未提交／部署）：同路径完整指纹变化时清理该Item旧收藏、评分、说明及建议，保留路径级排除决定；已登记存档来源的扫描出现同媒体类型且数量相等的新增／缺失成员时，视为无身份继承的替换，清理新缺失旧项的逐项信息，不强行按序配对。若原封面成员内容变化或缺失，按现有随机静态图规则重新选取封面；既有已缺失的指定封面也会在下一次成功来源扫描时重选。schema v21在扫描记录中保存新增、缺失、内容变化、移动、清理项数及封面重选状态，后台Gallery列表显式提示最近扫描变化项数并直达来源页详细摘要。未实现感知哈希匹配；普通BLAKE3指纹不衡量视觉相似度，若后续加入可基于已生成缩略图异步计算轻量dHash，不在扫描事务读取原图。正式业务库仍v20，旧扫描记录迁移后摘要默认0，不伪造历史变化；本轮没有对正式库、媒体或Manifest写入。

- 2026-09-30最新部署：同路径存档变更自动发现／有界重扫及旧内容版本派生缓存及时回收已提交为`f5ad79ff5a15ff340a6da64e8af2b528a5195006`并完成本机增量部署。下方两条“本地源码完成、未提交／部署”为阶段历史记录，本条覆盖当前状态。正式schema仍v20、完整性ok，Coser／Work／Character／Tag／Gallery／Source／Item计数`135/108/697/23/10/10/690`不变；服务active、NRestarts=0、Health／Ready均204。停服回滚备份为`/home/rainbowrunner/cos/bk/cgm-pre-archive-reconcile-f5ad79f-gF5NvwQ0`，详情见[开发日志](V1_5_DEVELOPMENT_LOG.md)。未自动扫描正式媒体；被替换的存档仍需在新版执行一次媒体库扫描后才会进入自动重扫流程。未远端推送或更新Docker镜像。

- 2026-09-30（内容替换后旧派生资源及时回收，本地源码完成、未提交／部署）：来源扫描使Item内容版本变化时，旧派生资源已`HARD_INVALID`且不可再提供，但原回收条件仍等新基础图READY；若新图生成失败则旧文件会长期占空间。现将“旧内容版本且HARD_INVALID”独立列为可回收条件，继续经过既有引用／任务复核、持久删除待办和失败重试；普通STALE与同版本暂时缺失来源仍等待新基础图，不改变保护策略。新增扫描→旧派生图失效→磁盘回收贯通回归及失败／保护场景测试；无schema、Manifest或原媒体变更。正式服务和业务库尚未改动，见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 2026-09-30（同路径存档替换自动复核，本地源码完成、未提交／部署）：媒体库手动发现和自动化发现现对已登记ARCHIVE与最近成功扫描的容器大小／mtime、扫描器版本及归档限制证据作轻量比较；变化时标记`NEEDS_RESCAN`，每分钟有界后台任务自动来源扫描，无须逐Gallery查找。定时扫描跳过证据未变化且`IN_SYNC`的存档；扫描失败保留`ERROR`与原有Item，不无限重试。相同文件名不同内容在完整来源扫描后按既有Item指纹规则增加`content_revision`并失效旧派生资源；同大小同mtime的隐藏替换仍须显式深扫。无schema迁移、Manifest或媒体写入；正式业务库和运行服务尚未改动。验证见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 2026-09-30最新部署：自动化阻断后人工接续闭环已提交为`03d2f9300c340bb1147b9d4c93ad47f80d792299`并完成本机增量部署。下方“源码完成、未部署”为阶段记录，当前以本条为准。正式schema仍v20、完整性ok，Coser／Work／Character／Tag／Gallery／Source／Item计数`135/108/697/23/10/10/690`不变；Health／Ready均204，About及前端资源对应提交，回滚备份位于`/home/rainbowrunner/cos/bk/cgm-pre-identity-review-03d2f93-LJPfwRC4`。旧 Gallery 10的建议仍PENDING、状态仍DRAFT，用户须在新版编辑页显式确认后再激活；未远端推送，详见[开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（自动化阻断后人工接续闭环，源码完成、未提交／部署）：Gallery编辑页新增当前激活阻断、来源Issue、待审实体建议及历史自动任务问题的分层展示。人工保存关系同事务确认唯一精确对应的旧建议，歧义／不匹配建议保持待审，可显式选择已有精确关联接受或拒绝；拒绝不移除已保存关系，来源安全问题须修复重扫。建议人工决议不提升Gallery元数据revision、不污染Manifest；留存审计与历史。GraphQL及Web接口回归、数据库原子回滚／并发revision测试见[开发日志](V1_5_DEVELOPMENT_LOG.md)。无需schema升级或数据迁移；正式业务库／服务尚未更改。

- 2026-09-30最新部署状态：本轮独立目录／存档候选、Character所属Coser快捷切换、搜索列表自动关闭累计15文件已提交为`44c612c5fb732296959bd2d88d0b50471e923fa8`并部署。下方“未提交／部署”为阶段历史记录，本条覆盖当前状态。Web42文件178项、TypeScript、生产构建及正式三标签相关Go回归／Vet通过；Health／Ready均204，About与实际管理页资源对应提交。schema v20、完整性ok、七类业务计数不变；回滚备份为`/home/rainbowrunner/cos/bk/cgm-pre-cast-44c612c-AZ48Pdbp`，详见开发日志。未推送远端或更新Docker镜像，实际业务UI验收待所有者操作。

- 1.5（Cast角色归属快捷切换及候选列表关闭，源码完成、未提交／部署）：多Coser时Character仍默认挂首位，可在角色行下拉切换到其他已选人物；仅移动该Cast并保留Work上下文、其他关系与标签，目标重复角色不可选，保存全部关系后才写入。共享Coser／Character／Work／Tag搜索列表新增搜索区域外点击、焦点离开和Esc关闭，内部选择不误关闭、异步返回不重开；中英文帮助同步。无schema、Manifest或迁移变更，测试记录见[开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（编辑页目录／存档独立实体候选，源码完成、未提交／部署）：Manage Gallery人物与角色页不再限MARKER目录；任意DIRECTORY按来源根目录名匹配，ARCHIVE按去扩展名的文件名及媒体库内外部父目录匹配，不读取媒体或存档正文。Coser／Character独立显示，Work仍只作角色作品上下文。应用仅修改页面草稿，保存全部关系后才写入；Character须有首位Coser，多人物时挂首位并保留其余Credit／Cast／Tag。候选文案及帮助支持中英文，已应用未保存项不再误称“已保存”；原识别建议状态和自动化精确匹配门禁不变。无需schema／GraphQL／Manifest／迁移格式变化，测试证据见[开发日志](V1_5_DEVELOPMENT_LOG.md)。本机正式服务仍运行`3fe7728`，需另行授权提交与部署。

- 2026-09-29最新部署状态：下方多实体与存档视频各阶段“未提交／部署”保留当时记录，现已统一提交为`3fe7728575a112f8c3934015f21bd897ad917f6a`并完成本机增量部署。正式schema仍v20，完整性`ok`，Coser／Work／Character／Tag／Gallery／Source／Item计数`135/108/697/23/10/10/690`不变；Health／Ready均204，About精确对应源码，嵌入资源逐字节复核通过。旧存档需所有者再次来源扫描，真实浏览器视频与业务大存档验收仍待执行；未推送或发布Docker镜像。完整回滚备份及异常处理见[部署日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（多实体／存档视频四阶段源码闭环完成，未提交／部署）：前两阶段基础现已接入来源扫描→技术探测／容器日期→Poster→认证成员Range及双语拒绝提示。后台直接优先，必要时仅临时提取单成员；任务10分钟有界，正常路径清理及启动前识别异常退出遗留。播放绝不提取／Remux／转码，压缩、未知布局、来源变更、超限及不兼容编码明确拒绝；请求与worker均禁止存档播放代理。探测／Poster终态失败、取消和显式重试同步Item状态，已有当前Poster保留；取消／缺失探测任务不无限轮询。Archive容器证据v2、目录／成员证据v1、schema v20及Manifest／迁移格式不变。真实六格式视频处理矩阵、资源授权／Range、无代理、Manifest不泄漏本机数据及视频存档换路径重建保持UUID并重新排队回归通过；Web 41文件165项、TypeScript及2556模块构建通过，只有既有大chunk提示。最终完整相关Go、race、Vet和候选编译证据见[本阶段日志](V1_5_DEVELOPMENT_LOG.md)。正式服务仍为旧版本，真实浏览器和业务大存档验收需后续部署后进行。

- 1.5（存档成员安全访问第二阶段，源码完成、未提交／部署）：ZIP／CBZ Store、普通TAR、可证明明文Copy布局的7z新增有界随机读取；压缩、无法证明的布局、加密、危险链接、重复路径、越界、来源变更和元数据超限均拒绝。固定原存档描述符，不提取、不复制视频；HTTP Range／HEAD／416及真实7z Store样本通过。Go定向测试、race、Vet、两轮30秒模糊测试（994,143／547,938次执行）、完整产品关联回归及正式候选编译通过，详见本阶段开发日志。无schema／Manifest格式变化；本阶段仅基础接口，后续第三／四阶段已完成对外端点及视频扫描接入。

- 1.5（多实体自动关联第一阶段，源码完成、未提交／部署）：显式实体自动接受开关现允许多个精确且身份无歧义的Coser／Character／Work建议；Coser按稳定建议顺序去重，Archive建议按外部路径首次出现顺序生成。角色归属不明时全部Cast挂首位Coser，Work仍通过Character体现，其他Credit无Cast不再阻断激活；没有Character时形成Album。真正身份歧义保留待审，既有Credit和已审核建议不被覆盖或重建。关系、Tag保留、建议状态及单次revision在同一事务提交，失败全部回滚；双语策略文案与帮助说明同步。正式三标签相关Go完整回归、定向race、Go Vet、Web全量40文件151项与帮助／媒体库2文件13项回归、TypeScript及2555模块构建通过；只有既有主chunk体积提示。未增加schema／Manifest字段，未触碰正式库、媒体或部署。

- 1.5（详情卡片封面标记仅图标，已提交并部署）：移除媒体卡片左下角“当前封面”常驻文字，保留原图标与紧凑底色，并减少内边距；双语Tooltip与ARIA说明保留，媒体菜单及Lightbox“当前封面”操作文字不变。TypeScript、40文件151项测试、2555模块生产构建与`git diff --check`通过；2026-09-28从干净提交`78f7baa`备份并增量部署，实际详情JS／CSS资源与本地构建一致，Health／Ready均204，schema仍为v20、业务计数不变。无数据库／Manifest／封面业务逻辑变化，部署证据见[1.5日志](V1_5_DEVELOPMENT_LOG.md)末尾。

- 1.5（详情布局切换控件收紧，已提交并部署）：根据首版业务反馈，将两种布局切换按钮移至详情页右上角、收藏按钮左侧，改为同高36px紧凑图标分段控件，双语Tooltip／ARIA名称和选中状态保留。移除媒体区域单独工具栏，按配置启用的媒体筛选仍独立保留。TypeScript、40文件151项前端测试、2555模块生产构建及独立Chrome桌面／390px手机布局、切换、节点保留、滚动锚点、偏好刷新E2E通过；2026-09-28从干净提交`3ea8ab9`完成完整备份与本机增量部署，正式资源匹配本地构建、Health／Ready均204。不改变数据库／Manifest／资源生成，schema保持v20、业务计数不变；详见[部署记录](V1_5_DEVELOPMENT_LOG.md)末尾。

- 1.5（Gallery详情双布局首版，已提交并部署）：默认卡片网格与等高自适应行可自由切换，浏览器本地记忆；沿用媒体节点、排序／分组、展开、大图和动画控制，切换尽量保留视口媒体锚点。新增GalleryMember已有派生宽高只读字段，未知比例稳定占位，无新增schema／源读取／生成任务。Web TypeScript、40文件149项测试、2555模块生产构建、独立Chrome桌面／手机／滚动锚点／节点保留／偏好刷新E2E通过；相关Go全量回归、Vet、正式三标签候选编译和`git diff --check`通过。2026-09-28从干净提交`4c54ba5`部署并核验实际服务的布局／宽高查询资源。未运行原离线完整生命周期E2E或覆盖旧像素基线，业务图片最终效果待用户验收。见[详情双布局计划](GALLERY_MEDIA_LAYOUT_PLAN_2026-09-28.md)。

- 1.5（Gallery派生缓存生命周期，已提交并部署）：schema v20新增本机删除outbox／轮转游标、FK删除级联清理记录、失效版本资格检查、安全文件删除和后台双语预览／确认入口。新版基础图READY前保留旧文件；缺失／不可读／Excluded媒体保留当前BASE。引用退役与待办先提交再删除文件，失败退避15分钟且重启可恢复，同路径重新发布取消待办。每分钟有界旧版本／待办清理与24小时宽限的孤儿分片巡检接入既有调度；Operations手动选择最多100项，密码＋CLEAN确认并执行前复核。替换、Profile、Forget、Gallery删除、失败重启、孤儿宽限／任务保护、只读预览、链接／inode、游标、提交先于删除、重新发布保护及认证API回归通过；正式三标签相关Go全量测试、Vet、定向race和Web完整回归通过。清理状态不进入Manifest／可移植包；2026-09-28完成停服完整备份、副本迁移预演及正式库v19→v20升级，完整性和七类业务计数不变、缓存巡检游标正常推进。真实大库性能、手动清理和实际媒体替换业务验收待所有者操作。见[缓存生命周期计划](CACHE_LIFECYCLE_PLAN_2026-09-28.md)及[部署日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（Gallery详情关联与时间信息行，已提交推送并部署）：对照2026-09-27指定GalleryEpic实例页，将现有信息行改为灰色14px/20px、16px Lucide图标与6px图文间距，按Work、Character、媒体数量、图片／视频拍摄时间、加入时间、发布时间排序并保留窄屏换行。Work/Character直接链接到已有实体详情页；发布时间使用参考页同款`CalendarDays`，加入时间仍仅展示`COMPLETE`静态图片文件修改时间区间。新增`lucide-react`本地依赖并更新许可清单与SPDX；前端38文件141项测试、TypeScript检查和正式构建通过（只有既有主chunk体积提示）。不更改GraphQL、数据库或时间字段语义。部署验收见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)末尾。

- 1.5（Coser综合时间线，已提交推送并部署）：新增第四种时间线依据`COMBINED`，逐Gallery按发布时间→拍摄时间→完整静态图片加入时间区间起点选取日期；月精度规范化为当月1日，UTC加入时间规范为日历日后统一排序，保留既有稳定次级排序、分页与未知日期略去规则。详情页时间区间、三种原模式、Coser默认拍摄模式与全局时间线均不变；不新增数据库字段、扫描任务或Manifest字段。Go产品库/API/Server测试及Web完整回归通过；部署验收见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)末尾。

- 1.5（Gallery来源增量内容扫描，已提交推送并部署）：schema v19新增仅本机使用的Item扫描证据版本和存档容器扫描证据表。常规扫描仍完整枚举DIRECTORY文件，但对同相对路径、大小与mtime且有当前版本BLAKE3证据的Item沿用内容结果；存档在容器属性、完整安全扫描、资源限制和扫描器版本一致时沿用成员快照。未解决SCAN Issue强制重验；后台单Gallery增加显式深度校验，强制读取全部媒体；来源改绑后的确认扫描可沿用未变化的Item身份与缓存。同一进程同Source拒绝重叠物理扫描，提交检查`scan_revision`防止旧快照覆盖新状态；未变化Item保留既有处理状态，不再重复设为PENDING。Go三标签相关包测试及Vet、Web 38文件141项测试和正式构建通过；正式库v18→v19迁移、完整备份与健康验收见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)末尾。真实大媒体库性能验收仍待业务测试。

- 1.5（Gallery发布时间与三种日期展示/时间线，已部署）：schema v18追加人工`publish_date`与`MONTH/DAY`精度；管理页用年月或年月日控件填写，服务端严格校验真实日期。字段通过Manifest v1的`extensions.cgm.publish_date`往返，保留核心目录迁移包与媒体派生时间的既有边界。Gallery详情与其它日期在同一行显示；搜索Gallery行同时展示加入、拍摄、发布时间，缺失使用`--:--:--`。Coser时间线可按拍摄、完整媒体加入、发布时间三种依据选择，未知日期仅从对应时间线略去，月精度按当月首日排序但展示不补日，Coser默认ALL。后端产品库、API、Server与Manifest测试、前端38文件139项测试、TypeScript和685模块正式构建通过；正式部署验收见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)末尾。

- 1.5（Gallery媒体加入时间，已部署）：schema v18将旧`added_at_utc`生命周期证据复制到独立`first_activated_at_utc`，并新增逐Item来源修改时间证据与Gallery最早／最晚区间、状态和revision。DIRECTORY正常扫描复用`FileInfo.ModTime()`，ZIP/CBZ、TAR/TAR.GZ及7z复用成员目录项时间；完整扫描成功后才原子替换Gallery汇总，失败／取消保留旧值。旧媒体每分钟最多处理一个Gallery级轻量回填：目录只`stat`，存档只枚举一次且不打开成员；首次媒体处理的EXIF提取继续复用`CARD_480`物化，二者状态独立。Browse默认排序只采用`COMPLETE`媒体区间并以首次激活／创建时间稳定回退，详情只有完整可靠区间才显示，Manage Basic展示状态。迁移包和Manifest不携带这些本机派生证据。启动后的历史证据回填仍按每分钟至多一个Gallery渐进执行，未完成者不伪装为加入时间。

- 1.5（迁移实测MT-00～MT-03闭环，已部署）：产品数据库升至纯新增schema v17，可移植包升至显式档位format v3；`CORE_CATALOG`与Gallery Manifest状态彻底解耦，`GALLERY_IDENTITY_ASSISTED`保留Gallery身份、来源定位和可选owner continuity，v1/v2继续按Legacy辅助包兼容读取。共享只读身份检查器统一发现与迁移的UUID／核心引用／来源分类；媒体库候选显示稳定问题码，完整Manifest按声明Item UUID先扫描后Pull并支持失败后幂等续跑，可访问副本可显式改绑或整体分叉Gallery局部身份。迁移工作台基于最近完整发现快照解析原位、唯一迁移、重复和未解析来源，保存可失效令牌，支持逐项`ADOPT`、默认关闭的`AUTO ADOPT`、READY部分重建及独立可信自动激活请求；自动激活只交给目标库已保存的`TRUSTED + autoActivate`持久化队列且不绕过门禁。TRUSTED媒体库自动化可自动导入无冲突目录Manifest，MANUAL/ASSISTED及全部冲突仍保留复核。正式库已从v16连续升级至v18；跨机迁移流程仍待所有者实测。详细边界和验收见[迁移测试问题与改进备忘录](MIGRATION_TEST_ISSUES_AND_IMPROVEMENTS_2026-09-23.md)。

- 2026-09-22 最新部署状态：下方两项“Docker首次设置/密码恢复/路径契约”及“迁移工作台导入包选择”的“已开发、未部署”是功能提交前的阶段记录；现已从提交`f424966a77495a768e2ad054daf70e116e85aebc`正式部署。本机库从schema v15升级至v16，完整性和七类业务计数复核通过，Health/Ready均204。Docker Hub已更新不可变标签`rainbowrunner2015/cosplay-gallery-manager:sha-f424966a77495a768e2ad054daf70e116e85aebc`，远端摘要`sha256:35b8d71757de4313e90bb7fa560e123c1172f2d9e09f4e525e9961fba6c1b796`，仅支持`linux/amd64`；未修改`latest`。跨机迁移及新Docker首次设置仍待所有者实测。

- 1.5（Docker首次设置、所有者密码恢复与路径契约，已开发、未部署）：提供的Compose仅绑定宿主机loopback并显式启用无门票本机首次设置；其他Docker/非loopback部署仍需原有单次门票。Setup由服务端声明运行环境，Docker的Coser/备份路径固定在持久`/var/lib/cgm`卷，完成前校验`/var/lib/cgm`、`/var/cache/cgm`、`/media`、`/transfer`挂载及存储目录可写性；新Docker媒体库路径须在已挂载的`/media`之下。忘记密码通过本机CLI生成10分钟单次Recovery Token，schema v16只保存哈希，成功后撤销Session并关闭可信模式；便携包格式、Gallery Manifest均不变。默认Compose媒体挂载改为允许显式旁置Manifest写回，原始媒体业务处理仍不改写。见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)和[安装手册](../INSTALLATION.md)。

- 1.5（迁移工作台导入包选择，已开发、未部署）：导出目标路径与导入来源分开；导入/准备合并只能从默认`/transfer`的顶层ZIP文件下拉选择，显示文件名、大小、包内创建时间、格式版本和校验状态，并支持刷新目录及显式完整校验。未校验或校验失败时禁用导入/准备合并，执行阶段仍走原有完整检查。Docker Compose新增只读`./transfer:/transfer`挂载；无数据库schema或迁移包格式变化。测试与边界见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（Docker Hub迁移测试镜像，已发布）：公开仓库`rainbowrunner2015/cosplay-gallery-manager`新增提交标签`sha-eea756a67ad1c5d2b1c79e102d98bf3299a3f661`；远端摘要`sha256:97ec56b4a1860bec667138c63930caa32539c7e0a11bb705b8c63be087dab572`与GHCR相同，仅支持`linux/amd64`。未改动`latest`；跨机迁移测试仍待执行。详见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（迁移测试Docker镜像，已发布）：当前迁移测试分支新增专用GitHub Actions发布流程，仅构建`linux/amd64`镜像，先启动临时无业务数据容器并通过Docker健康检查，再以不可变提交标签和`migration-test`标签推送GHCR。2026-09-21远端流程[35521591466](https://github.com/Rainbow-Runner/Cosplay-Gallery-Manager/actions/runs/35521591466)完成；源码提交`eea756a67ad1c5d2b1c79e102d98bf3299a3f661`，镜像`ghcr.io/rainbow-runner/cosplay-gallery-manager:sha-eea756a67ad1c5d2b1c79e102d98bf3299a3f661`，远端摘要`sha256:97ec56b4a1860bec667138c63930caa32539c7e0a11bb705b8c63be087dab572`。本机退出GHCR登录后再次匿名拉取成功。未打包数据库、配置、媒体、Manifest或缓存；另一台机器的Linux→Docker真实迁移模拟仍待执行。

- 1.5（手动Gallery扫描后续任务优先级，已部署）：Manage手动来源扫描仍在请求中即时执行，不引入新的来源扫描队列。扫描完成后立即为该Gallery补排符合条件的日期任务，并把当前内容修订的基础派生、视频首次探测及日期补录中尚待执行的任务提升到现有worker队列前列；后发手动扫描优先于先发，运行中租约、重试等待和终态失败不被抢占或复活。保留现有工作线程数、自动扫描与schema。功能提交`9d0cd79`在独立完整回滚备份保护下完成本机增量部署；服务、About、数据库完整性及业务计数复核通过。测试与边界详见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（新增媒体拍摄日期首次处理合并，已部署）：静态图片首次`CARD_480`处理在同一已定位媒体来源上提取EXIF日期；视频首次技术探测在同一次FFprobe响应中解析容器/流日期。日期证据与派生图/视频技术结果独立发布，缺失日期明确记为`NONE`；首次处理待执行时全局日期补录不抢先入队，首次处理终态失败后补录兜底。历史媒体、首次提取失败及未走首次处理路径的媒体仍保留有界日期任务队列，不新增schema或前端功能。随功能提交`9d0cd79`一同完成备份和无schema增量部署；测试、部署与差异检查结果见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（前台搜索结果单列封面布局，已部署）：Browse搜索页各结果改为独占一行，Gallery行左侧显示已有有效封面、缺失时显示图标占位；其他实体使用类型图标。响应式紧凑布局、中英双语分组/类型/空状态及键盘焦点样式已补齐。搜索接口为Gallery命中增加可选不透明封面资源标识，复用认证资源端点及现有Browse可见性门禁；不新增schema、缓存生成或源媒体读取。正式三标签Go测试/Vet、TypeScript、前端35文件131项测试及685模块生产构建通过；从清洁提交`e9ec40c`完成停服备份与无schema增量部署，服务、资源、数据库完整性和业务计数复核通过。详见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（Gallery Media目录折叠与紧凑行，已部署）：后台Gallery详情的Media页按真实相对父目录构建逐层可折叠树；根目录默认展开，子目录默认折叠，节点显示子树媒体数量，“仅显示缺失媒体”默认展开可见节点。文件夹上下移动仅交换同级目录及其完整子树，文件内顺序、自然文件名排序及媒体类型分组保持原有边界。File/Caption改为同行，普通项目不重复显示父路径，重名文件仍显示完整父路径以便辨认；缩小表格、输入框和按钮的垂直留白。不修改后端或schema。前端TypeScript、34文件130项测试及685模块生产构建通过；从清洁提交`8c803d5`完成备份与无schema增量部署，服务、页面资源、数据库完整性及业务计数复核通过。详见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（后台帮助页说明媒体库识别与排除规则，已部署）：Manage → Help 新增中英双语主题，解释媒体库发现、已有／忽略来源、有效Manifest、内置存档根、需要启用规则的`.cosplay-root`、PATH_TEMPLATE、FIXED_DEPTH及未分配媒体的处理顺序；区分根层级新媒体默认排除、自定义EXCLUDE/INCLUDE规则与安全／格式原因跳过。明确当前没有预装文件夹名或文件名排除清单，自定义INCLUDE也不能覆盖独立的根层级默认排除。仅前端和文档变更，TypeScript、34文件127项测试及685模块生产构建通过；2026-09-19从清洁提交`2cba64d`完成停服备份及无schema增量部署，服务、资源、数据库完整性和主要业务计数复核通过。详见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（后台作品集跨页搜索，已部署）：Manage → Gallery 列表新增服务端关键词搜索，覆盖标题、别名、Slug、Set ID 和来源路径；与现有问题状态卡筛选组合，结果总数和分页以筛选结果计算，状态卡数量仍表示全库统计。搜索词保存在URL中，输入防抖并在搜索或切换状态时重置页码；清除后恢复现有列表。后端为参数化LIKE并转义通配符，限制300字符，不新增schema或索引。产品库/API/Server/CMD正式三标签测试及Go Vet、前端34文件126项测试、TypeScript与685模块生产构建通过；2026-09-19从清洁提交`970e251`完成停服备份与无schema增量部署，服务、资源、数据库完整性和主要业务计数复核通过。详见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（作品集 Source/Manifest 状态帮助说明，已部署）：保持 Gallery 列表的短语展示与交互不变，在后台帮助页新增中英双语对照，逐项解释 Source 可用性／对账、最近扫描错误码及 Manifest 巡检状态、检查时间和安全处理入口；明确两列相互独立、巡检非逐行实时读取。帮助页第二篇正文在桌面与窄屏布局均正确排列。前端 TypeScript、34 文件 124 项测试和 685 模块生产构建通过；2026-09-19 从清洁提交 `4f14073` 完成停服备份和无 schema 增量部署，服务、资源、数据库完整性及主要业务计数复核通过。详见[1.5 开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（媒体只读与元数据写回策略解耦，已部署）：产品库schema v14新增媒体库级`metadata_writeback_enabled`，由旧`read_only`精确迁移，保留正式`collection2`已启用状态和其他库的既有状态；新建库默认允许旁置Manifest写回，原始媒体仍只读取不改写。现有媒体库可在Manage页经影响数量提示和确认切换；单项/批量Push改用独立策略门禁，批量预览只读检查父目录实际可写性并显示阻断原因，全部阻断时禁用执行。2026-09-19从清洁提交`a1f3677`备份并增量部署，正式库迁移至schema v14，完整性、策略映射、业务计数与健康探针复核通过；真实浏览器设置交互及正式批量Push仍待所有者验收。详见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。
- 1.5（媒体拍摄日期自动提取，已部署）：schema v15保存当前媒体修订的日期证据及手工日期冲突复核状态。来源扫描不在事务内逐个读取大文件；现有有界worker队列分批提取图片EXIF `DateTimeOriginal`与视频QuickTime/容器创建日期。只有全部可处理媒体完成后才用最早日期更新无冲突的Gallery `shoot_date`；手工值冲突须在Manage → Gallery → Basic确认。可移植重建恢复Manifest日期后在目标机重新排队提取，不迁移源机证据。2026-09-19从清洁提交`242ad15`完成停服完整备份与正式库v14→v15迁移；完整性、业务计数和健康探针正常。582个符合条件的媒体正在后台有界回填，待完成后做真实日期与复核交互验收。详见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（Manifest定期只读巡检与手动批量Push，已部署）：产品库schema v13将巡检观察与同步基线分离；已启用自动扫描时，每轮在来源扫描后按持久游标、有界批量检查已启用媒体库的Gallery Manifest，仅更新数据库巡检结果，不写来源文件。Gallery管理列表增加Manifest状态、检查时间及待处理筛选。最多选择100个Gallery先预览实际业务内容变化和本地文件变化，再明确选择跳过或覆盖本地改动；`DB_DIRTY`且业务内容确实变化时正常Push，只有revision变化而内容不变则不重写。执行逐项复核revision、路径、文件哈希；错误、来源不可用及只读媒体库始终跳过，逐项报告结果。2026-09-18提交`4bd5588`并在独立备份后迁移正式库至schema v13、增量部署；完整性、业务计数和健康探针通过。真实浏览器批量确认交互仍待业务验收。详见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（Tag 分类容器与直接关联策略，已部署）：Tag 新增“允许直接关联到图集”独立开关；关闭后仍可作为多父层级中的分类节点并汇总子标签图集，但 Browse/Manage 的 Gallery Tag 选择器不再提供它，数据库事务及触发器也禁止直接关联。已有直接关联的 Tag 必须先移除这些关系才能关闭开关；旧 Tag 在 schema v11→v12 迁移后默认保持可直接关联。可移植数据包记录此策略，旧包缺省解释为允许，导入和合并沿用同一字段；包格式版本不变。2026-09-18 从清洁提交`aaad2a2`独立备份并增量部署，正式库已升级至 schema v12，完整性、业务计数、Tag 默认值及服务探针复核通过；真实浏览器交互和用户执行的可移植迁移模拟仍待业务验收。详见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 1.5（Browse Gallery标题下展示与编辑Tag，已部署）：Gallery详情页在媒体数量/加入时间下方新增可换行Tag行，使用参考站白底深灰字小矩形、蓝色悬浮态及末尾`+TAG`编辑按钮；无Tag仍可直接编辑。点击既有Tag按UUID进入默认ALL范围的图集列表；点击`+TAG`展开复用的搜索、气泡与保存面板，移除“更多详情”中的重复编辑器，避免双份草稿状态。Tag详情页补齐已有分页数据的可操作分页控件。仅修改前端，无数据库schema/API变化；Web 34文件119项测试、TypeScript及685模块正式构建通过。2026-09-18从清洁提交`987e48e`完成独立完整备份保护下的本机增量部署，服务、探针、深链、新资源和数据库完整性复核通过。详见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。
- 1.5（Tag层级管理UI，已部署）：Manage核心实体的Tag默认切换为树形浏览，支持根/未归类、展开折叠、名称或Alias搜索自动显示上级路径、同一多父Tag在各父节点呈现以及直接子数/直接Gallery数；保留原平铺分页列表和右侧完整编辑。树节点“新建子Tag”采用父revision乐观锁，在单事务中创建实体、父子边并递增父revision，失败不遗留孤立Tag；父关系编辑后刷新树。新增认证GraphQL轻量投影，无数据库schema变更，不新增拖拽语义。正式三标签产品数据库/API/Server/CMD测试及Go Vet、Web 34文件116项测试、TypeScript和685模块构建通过；2026-09-17从清洁提交`420bfce`完成独立回滚备份保护下的本机增量部署，服务、探针、Tag管理深链、静态资源及数据库完整性复核通过。详见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。
- 1.5（Browse Gallery快捷Tag编辑，已部署）：Gallery详情“更多详情”内新增Stash式多选Tag控件，聚焦展开现有Tag、按名称/Alias实时筛选、选择生成可删除气泡，并以草稿显式保存/取消；不在Browse创建Tag实体。新增专用`replaceGalleryTags`乐观锁事务，只原子替换直接Tag、递增Gallery revision、标记Manifest `DB_DIRTY`并审计，不回传Credit/Cast、不运行激活降级，ACTIVE/DRAFT/ARCHIVED状态严格保持。无数据库schema变化；正式标签产品数据库/API/Server/CMD测试、Web 33文件113项测试、TypeScript及684模块生产构建通过。2026-09-16从清洁提交`65dfa74`完成独立回滚备份保护下的本机增量部署，服务、探针、静态资源、数据库完整性和业务计数复核通过。详细记录见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。
- 1.5（自动化派生等待、进度与稳定轮询，已部署）：TRUSTED运行扫描后如仍有可执行基础派生任务，不再立即把`DISPLAYABLE_ITEM_REQUIRED`记为人工复核，而是保持当前Gallery游标并进入可恢复的`WAITING_FOR_MEDIA`阶段；任务成功或耗尽后再执行完整激活门禁。Manage Libraries新增发现、等待预览、应用策略、收尾等实时阶段、目标计数、当前Gallery和进度条；轮询保留已有页面与未保存策略草稿，不再反复切换加载态造成闪跳。Archive组合文件名按显式分隔符/括号边界拆分后仍只做精确唯一实体匹配；既有无Credit/无待审建议的Archive草稿可在下一次显式自动化中安全补算。进度完全由持久状态推导，无新增schema。2026-09-15从清洁提交`ab2dc80`完成独立回滚备份保护下的正式增量部署，服务、探针、日志、数据库完整性和业务计数复核通过。
- 1.5（后台帮助页及媒体库自动化语义澄清，已部署）：Manage新增可持续扩展的“帮助”入口和主题索引，首篇“媒体库与导入”说明仅扫描检查、显式自动化和计划扫描三种入口，以及媒体库发现/Gallery来源对账两层扫描和新根、既有DRAFT、ACTIVE的不同处理范围。Libraries页同步把普通发现操作明确为“仅扫描并检查”，把自动化操作明确为“扫描并按已保存规则自动处理”。策略仍须显式保存、任务仍须显式执行，且表单存在未保存改动时禁止入队，避免任务静默使用旧策略快照；MANUAL模式、单库单活动任务、取消能力与全部激活门禁保持不变。无后端、数据库schema或自动调度语义变化；Web 33文件112项测试、TypeScript及683模块生产构建通过。2026-09-15从清洁提交`243d18a`完成独立回滚备份保护下的无schema增量部署，服务、深链、About、数据库完整性和业务计数复核通过。

- 1.5（Manage Gallery七卡片筛选，已部署）：顶部五项统计扩展为全部/草稿/超限/不可用/阻断/缺失媒体/处理失败七个按钮卡片，取消重复下拉框；同一URL筛选状态与分页复用原逻辑。全部卡片统一按Gallery数计，旧API的缺失Item数继续保留但不再作为卡片数字；GraphQL只新增只读统计字段，无数据库schema迁移。正式三标签产品测试/Vet、Web 32文件109项测试、TypeScript及682模块构建通过；2026-09-14从清洁提交`7b172be`完成独立回滚备份及无schema增量部署。服务、静态资源、数据库完整性和业务计数均已复核；详细记录见[1.5开发日志](V1_5_DEVELOPMENT_LOG.md)。

- 2026-09-14部署状态补记：下方自动扫描schema v11、媒体库改根/删除、Source转移及ignored Source撤销条目的“源码完成/未部署”属于各阶段提交前快照，现统一已从清洁提交`276d7454178e4aecf031ed77fe9f00c3682a69df`完成独立停服备份、v10→v11迁移和本机部署。发现快照并发复核同批交付。正式库完整性`ok`、业务计数不变，服务active、Health/Ready均204；本机真实业务操作待所有者验收。备份位置及校验和见[专项部署记录](AUTOMATIC_SCAN_AND_SOURCE_RECOVERY_2026-09-10.md)。

- 1.5（ignored Source撤销工作台，源码完成、未部署）：Libraries可按全局/当前库查看、搜索和分页忽略路径，并逐项预览撤销影响。所有者密码、`REVEAL`确认词、事务内令牌复核和活动自动化门禁保护撤销；成功只删除CGM忽略记录并使受影响库发现快照失效，不立即扫描或触碰媒体。路径和Set ID只在认证Manage预览显示，不写入审计；无新schema。详情见[自动扫描与来源恢复](AUTOMATIC_SCAN_AND_SOURCE_RECOVERY_2026-09-10.md)。

- 1.5（媒体库变更工作台第一段，源码完成、未部署）：Libraries新增只读改根/删除影响预览，显示直接绑定的Gallery Source及标题、路径、建议归属、ignored Source数量和子库根；新增需逐项选择并二次确认的Source转移。后端用原归属做并发校验、目标路径包含校验和事务式发现快照失效；不会自动让父库接管，不触碰媒体文件、Gallery身份或属性。第一段当时尚未开放实际改根/删除，现已由第二段补齐。无新schema；详细记录见[自动扫描与来源恢复](AUTOMATIC_SCAN_AND_SOURCE_RECOVERY_2026-09-10.md)。

- 1.5（媒体库变更工作台第二段，源码完成、未部署）：改根/删除影响预览补齐ignored Source精确路径、识别/分类/排除规则、自动化策略/任务、扫描状态、迁移映射和边界冲突；执行需所有者密码、动作确认词及事务内预览令牌复核。删除前须转走所有Gallery Source；ignored Source保留为全局路径忽略，防止父库静默重导，子库保留。改根重映射Source/ignored/Manifest并标记待复扫；不移动或删除媒体文件，不新增schema。后续仍需全局忽略撤销入口及更细文件系统诊断。

- 1.5（自动扫描计划与来源异常第二闭环，源码完成、待部署）：在既有默认关闭的每日自动扫描基础上升级schema v11，新增可选启动扫描及15分钟～7天周期；调度每分钟检查持久租约，继续服从维护模式和恢复暂停，并复用现有媒体库发现、自动化策略和Source原子扫描。Manage Settings提供中英文配置。Gallery索引新增UNAVAILABLE/MISSING/处理失败/阻断/超限等服务端筛选与异常高亮；Media可只看MISSING、单项/批量Forget，并可经预览及`REPLACE`强确认把新文件事实迁入旧MISSING身份，保留旧UUID和业务状态。忘记操作会安全解除封面引用。来源读取错误细分，Source页显示最近20次扫描，Libraries可显式确认相同`set_id`来源重新绑定。扫描成员变化会把已跟踪Manifest置为`DB_DIRTY`；Manifest页提供写回成员差异预览和二次确认。媒体库改根/删除及ignored Source撤销工作台亦已补齐；发现快照的数据库判断与写入现使用同一立即写事务，并在写入前复核遍历使用的库根与子库边界。可选扫描统计扩展及更细来源诊断按专项记录继续开发。

- 1.5（双语Web迁移工作台与可选owner continuity，已部署）：Manage → Operations新增中英文可移植迁移工作台，与CLI共用既有服务和持久化会话，覆盖导出就绪报告、服务器绝对路径导出、空库导入、非空库Merge准备/逐项合法决定/应用/终止、逻辑媒体库映射、Gallery重建预检/执行、连续性范围摘要/应用及中断恢复；每项迁移动作均需所有者密码和动作确认词，失败只返回动作级稳定码。portable package升级为format v2并继续读取v1，可选`owner-continuity.json`默认不导出，只能分别选择Gallery生命周期/首次收录时间或Gallery收藏隐藏/Item收藏；仅在`GALLERIES_REBUILT`后原子应用，ACTIVE请求重新通过目标门禁。Gallery地址/Slug历史明确不迁移，Gallery/Item评分继续由Manifest负责，最后浏览时间/项目等浏览状态不迁移且应用测试证明保持目标值。修复Merge会话真实格式版本持久化；无数据库schema变化。正式三标签Go测试/Vet、TypeScript、前端31文件103项测试和682模块生产构建通过；2026-09-10从清洁提交`771b188`完成完整备份保护下的无schema增量部署，数据库完整性、业务计数、服务和资源探针通过。

- 1.5（可移植元数据迁移基础闭环，已部署）：非空库Merge现可实际执行全部持久化REVIEW决定；`MAP_TO_LOCAL`把传入UUID登记为永久Alias并重映射关系，`KEEP_LOCAL/USE_INCOMING`控制实体、帐号位置及Tag边写入，硬冲突仍不可绕过。新增或明确采用传入内容的Coser资源在维护模式中暂存发布；替换既有资源时在Merge隔离目录保留旧assets，数据库失败恢复旧目录，提交后复验并清理，进程中断可用`-recover-portable-merge`按所有权标记恢复。Merge应用同时建立兼容阶段3的重建工作流，后续直接使用Merge UUID映射新媒体根并接管set/item/link UUID；非空库目录来源端到端已验证保持Item身份和DRAFT门禁。应用前可`-abort-portable-merge`，包/目标变化使决定失效，重复Apply拒绝；每次Apply先创建完整安全备份并进入`PORTABLE_MERGING`。完整CLI运行手册已持久化；正式三标签测试、Go Vet、构建和差异检查通过，无前端/GraphQL变化。2026-09-09从清洁提交`b477ddc`完成停服备份和schema v8→v10部署，迁移前后业务计数一致，数据库完整性、维护状态、健康探针及日志门禁通过。

- 1.5（可移植元数据阶段2空库核心导入，源码完成、未部署）：数据库升级至schema v9，新增受包摘要绑定的导入会话和Gallery/GalleryItem/ExternalLink待接管身份声明；普通UUID分配不能抢占`PENDING`声明，正式重建须按`PENDING→CLAIMING→CLAIMED`推进。需所有者重新认证并输入`IMPORT`的CLI只接受绝对ZIP路径和空业务数据库，先保留隔离包并自动创建完整安全备份，再原子导入核心Registry、Alias/Tombstone、Coser/Work/Character/Tag/SocialAccount、关系、Slug历史和revision；Coser当前原件在同一托管根暂存、复验摘要、重建480/960/1600派生资源后发布。进程内失败回滚数据库和本轮已发布目录；专属所有权标记及`-recover-portable-import`可在强制中断后安全回滚未提交资源，或复验已提交资源并完成维护状态。未重建Gallery身份不会成为孤儿ACTIVE Registry行，非空库明确拒绝。本阶段不修改Gallery Manifest、媒体、缓存、机器路径，不提供媒体映射、Gallery重建、Merge或Web UI；正式三标签测试、Go Vet与二进制构建通过，尚未迁移正式数据库或部署。
- 1.5（可移植元数据阶段4非空库Merge预检首切片，源码完成、未部署）：新增需所有者重新认证的`-preflight-portable-merge <zip>`，独立Inspector通过后复核来源inode与前后SHA-256，并把包身份账本和当前Registry按UUID顺序流式归并；报告可新增/一致复用以及kind/state/target/history硬冲突，不把百万Item/Link载入内存。核心实体进一步报告同UUID内容差异、不同UUID规范化主名/Alias候选、Slug占用、SocialAccount URL候选、Tag边位置与Slug重定向冲突；只输出稳定码、kind和UUID，同名绝不自动合并。预检不复制包、不创建会话/声明、不修改业务对象、Manifest、媒体或资源，仅写无正文审计。持久化决策schema、实际Merge写入、字段级工作台与Gallery合并仍未实现。
- 1.5（可移植元数据阶段4持久化决策切片，源码完成、未部署）：数据库升至schema v10，以包SHA-256和目标Registry/核心目录指纹双重绑定Merge会话、稳定冲突身份与显式决策；不在数据库复制冲突字段正文。`-prepare-portable-merge`经`PREPARE`确认后保留并二次验证精确包副本；`-decide-portable-merge`逐项限制合法决定并要求`DECIDE`，硬冲突不可接受，REVIEW必须完整覆盖。目标或包变化使会话`STALE`，旧决定不写入。当前READY仅表示决策完整，不执行实际Merge，也不创建安全备份或修改核心/Gallery/资源。
- 1.5（可移植元数据阶段4无冲突核心Merge写入切片，源码完成、未部署）：schema v10增加Merge专属Gallery/Item/Link待接管声明并纳入普通UUID分配保护；已有未完成Import/Merge声明也会在后续Merge预检中作为占用冲突。仅零硬冲突、零REVIEW、无Coser资源且目标指纹未变的READY会话可经`-apply-portable-merge`和`MERGE`确认执行；先创建完整安全备份，再在单事务新增缺失核心Registry、实体、关系和生命周期，精确相同项无写复用，Gallery类只保留声明。后段失败全部回滚，重复Apply拒绝。人工决策、资产和Gallery业务重建尚未开放。
- 1.5（可移植元数据阶段3媒体映射与Gallery重建，源码完成、未部署）：schema v9导入会话现同时持久化不含旧绝对路径的逻辑媒体库标签、Gallery相对来源定位和逐Gallery重建游标。所有者CLI通过`-map-portable-libraries`逐库选择本机已启用目标或明确跳过；`-preflight-portable-rebuild`只读复验来源边界、非符号链接路径、来源类型、sidecar精确BLAKE3摘要/set_id/revision、安全扫描、Manifest Item路径唯一对应和全部待接管UUID，不创建Gallery、Item、扫描记录或Manifest。`-rebuild-portable-galleries`需再次输入`REBUILD`，按Gallery创建DRAFT来源、扫描并在各自事务中接管set/item/link UUID，再应用现有Manifest关系/封面/排除项；失败保留可见游标供相同包和路径重试，不覆盖或重新打包媒体。全部ACTIVE声明接管后才发布Gallery类Alias/Tombstone并把会话标记完成；跳过或不完整来源的声明继续保留且不会被误报为完成。目录与ZIP异机根迁移端到端、sidecar变化阻断、映射完整性、事务重试、正式三标签测试、Go Vet和二进制构建已通过；其他已支持存档格式继续复用统一安全扫描器，但尚未执行真实跨机/正式业务库演练，也没有Web入口、非空库Merge、提交或部署。
- 1.5（可移植元数据阶段1，源码完成、未部署）：新增独立`cgm-portable-metadata` v1包模型，从单一数据库读事务确定性导出全部Portable UUID Registry/Alias/Tombstone、Coser/Work/Character/Tag/SocialAccount、核心Slug历史、Tag关系和不含绝对路径的Gallery Manifest覆盖索引；当前Coser头像/Banner原文件通过既有安全托管资源读取进入同一包，派生图不导出。包内逐文件SHA-256、整包SHA-256、严格Entry白名单、NFC/大小写/平台保留名/路径穿越/压缩比/容量门禁及孤儿ACTIVE身份、Alias断链/环、Tag环、资源引用校验已实现。新增需所有者重新认证的`-preflight-portable`，以聚合SQL和最多Gallery/资源规模的受控迭代报告身份kind/state、目录、Gallery覆盖与资源计数；不创建包且不输出业务正文/物理路径。身份账本现从同一只读事务逐条写入v1 JSON；Inspector逐条解码并使用自动清理的0600临时磁盘索引验证全部身份，只在Go内存保留核心/Gallery对象身份。百万Item/10,000 Gallery合成库的1,010,400身份完整写包及检查耗时12.227秒，129,278,548字节身份Payload的峰值Go堆增长20,311,192字节，因此已移除400,000条临时阻断；仍保留20,000,000身份和10GiB解压硬上限。CLI支持严格导出、显式Gallery警告导出和无需数据库的离线检查；正式三标签测试、Go Vet和二进制构建通过。尚未在正式业务库执行预检/导出，不新增数据库schema，不实现导入，不修改Manifest、媒体或部署服务。
- 1.5（Character跨Work迁移与合并归属，已部署）：独立Character编辑页允许通过Primary Work选择器迁移到另一Work，保存前展示原Work→目标Work影响并要求输入`MOVE`；数据库在一个事务内保留Character UUID/Slug/Alias/Gallery Cast，只更新Work及同批名称字段、递增revision并将受影响Gallery Manifest标记为`DB_DIRTY`。目标Work不存在/失效或已有规范化同名Character时拒绝且不产生部分写入。跨Work Character合并明确以目标Character所属Work为最终归属，既有Cast迁至目标UUID，源UUID/Slug继续永久重定向；选择器与预览显示目标Work，审计记录迁移路径或合并目标Work。不新增字段、表或schema。产品数据库/API测试、前端30文件101项、TypeScript、681模块生产构建及正式三标签编译测试通过；2026-09-06从清洁提交`82cd53b`完成无schema增量部署。
- 1.5（独立Character列表紧凑布局修正，已部署）：撤销所属Work占用第三行造成的行高增长；Character列表项恢复原有两行高度，左侧保留名称与Aliases/UUID，右侧仅显示所属Work名称并省略“Work:/所属作品：”前缀。长文本保持单行截断，Work内嵌Character区域及其他实体列表不变，无接口、数据或schema变化。前端30文件99项、TypeScript和681模块生产构建通过；2026-09-05从清洁提交`7d15ed3`完成无schema增量部署。
- 1.5（独立Character列表显示所属Work，已部署）：独立Character管理列表在名称和Aliases/UUID之后增加所属Work名称，使用略高对比度的中英文标签，便于同名或近似角色快速辨认；数据直接复用现有`workName`，旧响应缺少名称时防御性回退到`workUUID`。Work、Coser、Tag列表以及Work内嵌Character气泡区保持原样，无接口、数据库或schema变化。ManageCoreEntitiesPage 12项、前端30文件99项、TypeScript和681模块生产构建通过；2026-09-05从清洁提交`7de86e7`完成无schema增量部署。
- 1.5（Work内嵌Character管理，已部署）：保留Character独立管理页，在已保存Work编辑区新增“作品属性/关联角色”双语页签；关联角色页固定显示当前Work名称，以固定高度滚动气泡区列出全部直属Character，点击气泡编辑并可随时新建。该入口锁定Primary Work，新建时自动提交当前Work UUID，继续复用既有Character查重、Alias气泡、revision、网络名称导入及合并/删除能力。新增认证只读`manageWorkCharacters`关系查询，严格按Work隔离并沿用英文名称/中文拼音与Sort name覆盖排序；不新增数据库字段、表、schema或另一套Character模型。产品数据库/API测试、前端30文件99项、TypeScript、681模块生产构建及正式三标签产品测试通过；2026-09-04从清洁提交`949c320`完成无schema增量部署。
- 1.5（Character关联Work名称恢复，已部署）：修复Manage Character数据只返回`workUUID`且表单把选择器名称固定传空，导致已有关联及新选择的Work只能辨认UUID的问题。认证Manage实体模型新增只读`workName`，由当前Work表实时解析，不新增数据库字段；Character详情重载后显示Work名称，选择Work后立即更新名称，Gallery Cast选择Character时也保留其Work名称。后端定向测试、前端30文件98项、TypeScript、681模块生产构建及正式三标签产品测试通过；2026-09-03从清洁提交`6e76fad`完成无schema增量部署。
- 1.5（Work/Character创建前重复名称复核，已部署）：将Coser已有的350ms防抖精确查重扩展到Work与Character；后端按NFC、去首尾空格和Unicode大小写折叠匹配主名称及Alias，只返回精确命中，不把包含关系误报为重复。Work命中显示名称、Aliases、UUID尾号和关联Gallery数；Character额外显示所属Work，并跨Work提供身份复核。同一Work内Character主名称硬冲突不能通过确认绕过，Create保持禁用；跨Work同名或Alias命中可在明确确认是不同实体后继续。查询失败同样要求显式确认，不新增字段、表或schema。产品数据库/API测试、前端30文件97项、TypeScript、681模块生产构建及正式三标签产品测试均通过；2026-09-03从清洁提交`1c71ad8`完成无schema增量部署。
- 1.5（可拔除Work/Character名称资料Provider，已部署）：新增站点无关`entitymetadata`契约、短期绑定预览和受控REST流程；萌娘百科适配器只在`cgm_moegirl`构建标签下注册，按公开搜索页与语义信息框提取Work/Character其他名称。启动配置`entity_metadata_scraping_enabled`默认关闭，只有认证所有者在对应Manage实体页的显式搜索、候选预览和逐项勾选应用会外联；扫描、Browse、启动、自动化及计划任务不调用Provider。应用仅追加Alias，不改主名称、Sort name或Character所属Work，继续复用revision、NFC/重复/数量门禁和Manifest dirty传播；无数据库字段、表、Schema或回填变化。Work/Character气泡状态同时改为真实字符串数组，`Fate/stay night`一类合法斜杠名称不再被UI分隔符拆开；Coser/Tag既有输入规则不变。管理页有未保存编辑时禁止应用，Provider移除/零结果/旧`null`集合/局部渲染失败均有隔离行为。合成结构、当前公开Work/Character/搜索HTML兼容校核、后端安全选择、前端30文件95项、TypeScript、681模块生产构建、正式三标签产品测试与构建均通过。2026-09-03从清洁提交`1de0b95`完成无schema正式部署，并在私有正式配置中显式启用该功能。
- 1.5（Alias气泡连续编辑切换，已部署）：修复Work/Character输入框有待提交文本时其他气泡正文被禁用的问题；点击另一气泡现原子保存当前文本、移除目标气泡并将目标原文放入输入框继续编辑。若当前文本与其余气泡重复则不切换且显示提示；与目标本身等价时只保留一个编辑副本。无接口、数据或schema变化；2026-09-02已从清洁提交`ffcc255`完成无schema增量部署。
- 1.5（Work/Character Alias气泡编辑，已部署）：两类实体不再使用难以区分单项的`/`分隔普通输入框；Aliases现以内嵌气泡展示，Enter生成，点击正文退回输入框编辑，叉号直接删除。未回车文本会显示提示并阻止Create/Save，避免静默丢失；处理中文IME合成、NFC/大小写重复、单项300字符和最多100项边界。Coser/Tag维持原控件，后端Alias数组和schema不变；2026-09-02已从清洁提交`a29eaaa`完成无schema增量部署。
- 1.5（Work/Character/Tag管理列表交互，已部署）：三类核心实体前端不再隐藏已有的全库搜索与30/60/100页量能力，现提供双语搜索、排序规则说明、清除条件、结果区间、首页/前后页/页码/末页和URL状态恢复；搜索覆盖名称、Sort name与Alias。后端把Coser已确认的英文/中文拼音及Sort name优先排序推广到全部核心实体，并在全局排序后分页。Coser图片完善度筛选保持专属，无数据库或schema变化；2026-09-02已从清洁提交`9220923`完成无schema增量部署。
- 1.5（Coser网络资料搜索完成反馈，已部署）：2026-09-02真实运行日志确认连续4次搜索均为HTTP 200且响应为规范空候选数组，不是网络失败或页面崩溃；现有前端却只渲染非空候选，导致成功零结果表现为无响应。搜索现显示候选数量或带改进关键词建议的中英文零结果状态，并在输入/Provider变化时清除旧结果；服务新增不含姓名和查询词的完成事件，仅记录request ID、Provider key及候选数。无资料写入、Provider匹配算法、数据库或schema变化；已从清洁提交`d94a45e`完成无schema增量部署。
- 1.5（GalleryEpic零账号候选黑屏修复，已部署）：真实`seya-狮砸`预览成功取得头像和Banner但账号数为0，Go nil切片被序列化为`accounts:null`；前端在已排队写入Preview后依次调用`.filter/.map`，且应用没有局部边界，最终清空整个React页面。公开预览和零候选响应现固定输出`[]`，前端将旧`null`再次标准化为空数组；Coser网络资料面板增加可重试局部错误边界，未来异常只隔离该面板。没有资料写入、schema或Provider耦合变化。2026-09-01已从清洁提交`9aee6ac`完成无schema增量部署。
- 1.5（Manage Coser中英文/拼音排序，已部署）：Coser管理列表不再使用SQLite二进制文本顺序；全库搜索和图片完善度筛选后，以项目已有`golang.org/x/text`固定zh-CN Unicode Collation，在同一不区分大小写的字母序中混排英文名称和中文拼音，再进行30/60/100项分页。人工Sort name保持最高优先级，可覆盖多音字或特殊读音；名称、原始文本和UUID依次稳定破同序。排序不依赖宿主机locale，不新增依赖、字段、表或数据库schema。2026-09-01已从清洁提交`8123ab6`完成无schema增量部署。
- 1.5（Coser管理列表大数据量交互，已部署）：Manage Coser不再只能逐页浏览固定30项；认证查询现支持在数据库分页前对主名称、Sort name和Alias执行全库包含搜索，并按缺头像、缺Banner、任一不完整或两者完整筛选。左侧列表提供300ms搜索、30/60/100页量、结果区间、首页/前页/页码输入/后页/末页和空结果/错误状态；工具区与分页区固定，人物行在中间滚动。`q/assets/pageSize/page/uuid`全部保留在URL，刷新、返回、深链及从重复身份提示打开已有Coser不会丢失列表上下文。没有新增表、索引或数据库schema。2026-09-01已从清洁提交`fb1f989`完成无schema增量部署。
- 1.5（Coser创建前重复身份复核，已部署）：新建Coser输入Name后350ms防抖调用认证Manage查询，后端使用NFC、去除首尾空格和Unicode大小写折叠，对现有Coser主名和Alias只做精确匹配，不将包含子串误报为同一人。命中时展示头像、主名、Aliases、匹配值、UUID末8位和关联Gallery数，可直接打开已有Coser；Create保持禁用，直到所有者明确确认是不同人物。查重失败也必须明确承认后才可继续；无命中时正常创建。Coser同名业务语义、UUID/Slug身份和数据库schema不变。2026-08-31已从清洁提交`4e1e43e`完成无schema增量部署。
- 1.5（Coser管理列表图片完善度，已部署）：Coser名称前新增40px圆形头像，已配置时复用认证的revision资源URL并延迟解码，未配置时保留等尺寸浅灰空白占位，资源加载失败与未设置区分为琼珀色错误状态。每行右侧增加低干扰头像/Banner图标，绿色表示已设置、灰色表示缺失，提供中英文悬浮说明和可访问名称。列表现有查询已包含`avatarURL/bannerURL`，因此不增加GraphQL请求、数据库或schema变更；Work/Character/Tag列表不受影响。2026-08-31已从清洁提交`8170e92`增量部署。
- 1.5（核心实体Alias输入修复，已部署）：修复Coser/Work/Character/Tag共用Aliases受控输入框在每次按键后立即拆分、trim并重组，导致用户无法输入`/`、分隔空格或名字内部空格的问题。编辑期现保留原始文本，仅在创建/保存时按`/`拆分、去除两端空格并忽略空项；补充可见格式示例和英文/日文/中文/名字内空格回归。2026-08-31已从清洁提交`f415c1a`增量部署，schema v8和配置不变。
- 1.5（Archive内置Gallery根与校核入口，已部署）：修复安全Archive观察结果错误复用DIRECTORY根规则的问题；ZIP/CBZ、TAR/TAR.GZ/TGZ和7Z只要通过安全校验且含受支持媒体，就以内置`ARCHIVE_FILE`方式直接成为独立Candidate，不依赖PATH_TEMPLATE/FIXED_DEPTH。有效相邻Manifest仍优先，明确DIRECTORY根拥有完整子树并压住内部Archive，避免嵌套Gallery。手工发现默认只生成Candidate；媒体库ASSISTED/TRUSTED新增默认关闭的“自动导入安全存档”策略开关并冻结进运行快照。schema v8扩展候选识别枚举及两个策略字段，v7迁移前创建来源准确快照且既有策略全部保持关闭。Libraries页将“发现覆盖率待处理”与“Gallery自动化待复核”分开，未分配项明确限定为目录并可预填精确PATH_TEMPLATE；安全Archive直接显示可创建草稿的Candidate。2026-08-31已从清洁提交`2c4bd3a`构建并迁移正式库到schema v8；业务计数不变，真实两个TAR在隔离库发现中分别产生72/74成员的无冲突Candidate。产品Go、schema v7→v8、GraphQL、正式标签组合、TypeScript、29文件83项Vitest和680模块生产构建均通过。
- 1.5（可管理媒体自动排除规则，Archive扩展已部署）：全局/媒体库规则现统一作用于 DIRECTORY 新Item和经过安全校验的 Archive 新成员，按成员内部相对路径匹配；规则支持任意父目录段、完整父目录/祖先路径、文件名、文件stem、完整相对路径及 Exact/Glob/Go RE2。Archive 无有效 sidecar 时以存档文件名去除扩展名作为 Gallery 标题保底，并基于外部路径/文件名生成唯一实体待审核建议；不新增来源范围字段或数据库迁移。2026-08-29已从清洁提交 `d9420fc` 完成增量部署，schema v5和配置保持不变。详细边界见[专项计划](MEDIA_EXCLUSION_RULES_PLAN_2026-08-28.md)和[Archive规划](ARCHIVE_RULES_AND_TITLE_PLAN_2026-08-29.md)。
- Archive后续扩展规划：已记录存档内部成员排除、文件名标题保底及实体候选建议的独立实施方案，当前不改变已部署Archive行为，详见[Archive规则与标题规划](ARCHIVE_RULES_AND_TITLE_PLAN_2026-08-29.md)。
- 1.5（自动排除扫描、审核与schema v5，源码完成、未部署）：规则只自动设置DIRECTORY新Item初始排除状态，根目录本次扫描开关继续优先；既有路径/唯一指纹Item、人工与Manifest决定不被重扫覆盖，ZIP/CBZ保持不变。存量评估仅建立PENDING EXCLUDE审核项，接受后才排除并取消未完成任务，Restore恢复当前revision任务且保留REVERSED历史；排除Item不进入1000有效成员、Browse或新任务口径。schema v5迁移不播种规则、不改既有Item，并在v4写入前生成可验证在线快照。
- 1.5（自动排除双语管理界面，源码完成、未部署）：Libraries & import新增独立自动排除区，按全局策略与当前媒体库覆盖分组，新规则默认全局；提供规则CRUD与删除二次确认、五类subject、三类operator、媒体类型、EXCLUDE/INCLUDE、后端RE2门禁、单路径测试、最多200项存量预览、显式单库评估及逐项/批量审核。目标Go与正式标签组合、TypeScript、29文件80项Vitest和680模块生产构建通过；本轮尚未提交、备份、迁移正式schema v4数据库或部署。
- 1.5（MARKER单子目录标题保底，已部署）：`.cosplay-root`所在父目录继续作为DIRECTORY来源根；新发现的MARKER根若恰好只有一个直属真实子目录，Candidate预览、手动导入及自动建DRAFT统一以该子目录名作为标题，零个或多个子目录仍取根目录名。已有Gallery不回写，根级媒体默认Exclude和ZIP/CBZ扫描规则不变。Discovery、产品数据库、API、Server及正式标签组合回归已通过；2026-08-19已从清洁提交`fbba7e2`完成本机增量部署，正式数据库保持schema v4且完整性复核通过。
- 1.5（单媒体完整元数据显示，已部署）：单媒体详情新增文件、描述、日期、相机、图像、视频及其他安全EXIF分组。详情主体先完成，独立GraphQL查询再从当前原始媒体异步实时只读作者、来源、程序名称、版权、拍摄/获取日期、制造商/型号、光圈、曝光、ISO、焦距等字段；读取失败仅影响信息区。图片安全提取结果不写数据库、不生成缓存、不排后台任务、不回填存量数据，产品schema保持v4。Manage Settings按当前浏览器保存显示选择，服务端逐次校验并过滤；GPS与设备/图像唯一标识默认关闭，嵌入缩略图、MakerNote、未知Tag、二进制块和物理路径永不进入Browse。2026-08-18已从清洁提交`2db91e9`完成本机增量部署，数据库完整性与无图片元数据表状态均已复核。
- 1.5（媒体分类规则信息架构与中文化，已部署）：将Media classification从单个媒体库工作区提升为Libraries & import页的独立管理区；有效规则按“全局规则”和“当前媒体库覆盖规则”分组，新增规则默认全局作用域，默认恢复只出现在全局区。规则作用范围与预览/评估目标明确分离，单库操作显示实际媒体库名称；媒体库、发现规则、导入发现及分类审核的可见文案已接入`react-intl`中英文消息，不改变后端叠加、优先级、建议审核或媒体写入语义。2026-08-18已从清洁提交`ef92526`完成本机增量部署。
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

- 2026-08-30新增并部署TAR、TAR.GZ/TGZ与7Z GallerySource支持：发现、扫描、安全校验、按需物化、复合扩展名标题保底和相邻Manifest统一经过顺序只读归档适配层；密码保护归档保持拒绝且不保存密码。schema v7纯新增媒体库覆盖摘要/诊断，Libraries页提供中英双语“查缺补漏”数据展示，并对照已登记来源、已入库Item与等待扫描来源。
- 本项定向Go包与正式嵌入标签组合通过；schema v6→v7来源快照、空表迁移和覆盖诊断回归通过。React TypeScript、Vitest 29文件/82项及680模块Vite构建通过；主共享JS约517.96KiB并继续存在大包提示。法律/SBOM清单已按67项实际编译依赖重新生成。
- 正式部署提交为`28e71a7`，二进制SHA-256为`07cc18ecbe5611a812170e753b8e48db2224adbbfc6d851a43556d8cce6f6729`。额外完整回滚包和自动schema v6快照均已实际校验；正式库schema v7完整性为`ok`，2/5/5/322业务计数不变，新表为空。服务保持active、`NRestarts=0`且Health/Ready为204。

- 2026-08-29新增媒体库自动化首轮实现：schema v6以纯新增表保存媒体库级`MANUAL/ASSISTED/TRUSTED`策略和运行摘要；v5升级保持全部媒体库为隐式`MANUAL`且先生成安全快照。
- 自动化执行已串联确定性发现、规则授权的自动DRAFT、来源扫描、空分级保底、精确唯一实体组合、确定性媒体分类建议和统一状态机激活；任何歧义或激活阻断均保留DRAFT并计入待复核。
- 自动化现由独立持久化后台队列执行：策略入队时冻结快照，每批最多25个Gallery且逐项保存游标；同库单活动任务、租约心跳、进程退出回队列、过期接管、人工取消和不含路径的分项失败记录均已实现。现有自动扫描总开关会定期为ASSISTED/TRUSTED库入队，ACTIVE来源对账保持原职责。
- Libraries管理页新增中英双语策略编辑、预览、后台入队、1.5秒活动态轮询、进度与取消；自动激活只对`TRUSTED`开放且要求默认分级。
- 本轮产品数据库自动化/迁移测试、productapi/productserver测试、React TypeScript检查及Vitest 29文件/82项均通过；2026-08-30已在额外完整回滚包和自动v5快照验证后迁移正式业务数据库并增量部署。
- 2026-08-30完成自动化阶段5合成验收：运行中取消、短租约持续心跳、批次游标后的过期租约接管均有回归；固定`GOMAXPROCS=4`下10,000个IN_SYNC Draft以25项一批完成400批，处理约1.802秒，低于45秒门槛。该结果不等同于真实文件扫描、媒体解码或操作系统强杀恢复验收。
- 2026-08-30完成正式二进制隔离进程门禁：10,000个真实目录/PNG文件完成发现和来源扫描；发现阶段及持久化Gallery游标5801后分别执行SIGKILL，推进测试租约过期后均由新进程接管并收敛；处理中取消约93毫秒，最终日志修复版复测约10毫秒并记录正确的CANCELLED事件。生产30分钟租约的10分钟真实心跳周期尚未等待实测。

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
- React 19 TypeScript `--noEmit`通过；Vitest当前27个文件、74项测试全部通过。
- Vite生产构建通过；2026-08-29当前构建共转换680个模块，主JS minify后、gzip前约515.15KiB并存在大于500KiB提示，其余Manage/Browse页面仍按路由生成懒加载块；需要在后续性能阶段继续拆分主共享依赖。
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

- 2026-08-19 00:32 CST已从精确提交`fbba7e2c9e2673ae652b67d2ebd748a0d6972dd4`增量部署MARKER单子目录标题保底，SHA-256为`f35b84abdab4d5c5a8c42745a88a37917af5d2c3d38d8d90b997bea8e0aa498d`；旧二进制备份于`/tmp/cgm-before-marker-title-20260819`。服务保持`enabled/active/running`、`NRestarts=0`，Health/Ready为204，About报告精确源码且`exactSourceAvailable=true`；配置校验和和产品数据库inode不变，schema v4完整性为`ok`，启动日志无异常。
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

1. 由所有者在Manage中选择一个真实媒体库明确启用ASSISTED或TRUSTED，人工核对后台进度、取消、重复运行及TRUSTED激活阻断；当前升级保持所有正式媒体库为隐式MANUAL。
2. 在可接受至少10分钟持续扫描的维护验收窗口验证生产30分钟租约的真实心跳周期；短租约续租、实际SIGKILL、过期接管和取消延迟已经通过。
3. 为视频schema v2安排本机维护窗口：先核验自动快照与独立完整备份，再部署新二进制并完成真实业务视频验收。
4. 在Linux arm64、其他目标浏览器和远端CI/夜间runner完成尚未执行的平台、无障碍、性能和发行门禁。

任何未执行的测试不得在状态记录或发布说明中标记为通过。
