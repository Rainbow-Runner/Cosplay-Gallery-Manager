# Cosplay Gallery Manager

CGM 是本地优先、单所有者、自托管的 Cosplay／写真图集管理系统，基于 Stash 演进，使用 Go、SQLite 和 React。

当前版本为 `1.5.0-dev`。建议部署在可信局域网，尚未发布稳定版；不要直接暴露到公网。

## 功能

- 发现和管理目录、ZIP／CBZ、TAR／TGZ、7Z 图集，支持增量内容复用、可选文件监听和草稿批量重新识别。
- 管理 Coser、Work、Character、Tag、Credit／Cast、别名及可移植 Manifest；保留手动修改和人工复核结果。
- Coser 头像／Banner 支持浏览器上传、裁剪和用户主动触发的资料导入；Work／Character 支持主动网络名称导入。
- 首页、Cosplay／Album 分区、人物／作品／角色详情、搜索、角色筛选、月度时间线、收藏和评分。
- “随机浏览”抽取媒体；并列的“随机图集”从全部可浏览图集中等概率抽取，每批最多24套，不分配类型比例。
- “标签”之后的“相似图片”入口可按现有感知哈希手动查找同图集或跨图集的相似图片；结果供人工复核，不自动删除或合并。
- 普通静态图片使用共享 `CARD_DISPLAY_960` WebP 基础图；支持视频按需播放、可回收增强缓存和可选硬件加速。
- 全屏媒体查看器支持独立滚动的信息侧栏，按需只读展示 EXIF／XMP／IPTC、历史文件名及编辑／转换记录；属性紧凑分组，原始记录按需展开，完整时间事件从早到晚排列。随机浏览的静态图片隐藏信息入口，保留所属图集入口。
- 编辑事件优先显示，派生／转换详情直接可见，文件声明的软件独立折叠；静态大图解码完成后再替换预览，失败可重试，不预加载整套图集。
- 显式备份／恢复、默认每日数据库快照及保留清理；自动扫描和文件监听均须显式启用。

Docker 默认启用两项网络资料工具，但只有已登录所有者主动操作时才会联网，不会在扫描或空闲时自动刮削。

## Docker 部署

发布目标为 `linux/amd64`。arm64 构建配置存在，但不宣称完成真实设备验收。

在独立部署目录准备 `state`、`cache`、`transfer` 和媒体目录，给容器运行用户必要权限，然后保存为 `compose.yml`：

```yaml
services:
  cgm:
    image: rainbowrunner2015/cosplay-gallery-manager:release-20261018
    user: "1000:1000" # 按宿主机 id -u / id -g 修改
    ports:
      - "9999:9999"
    read_only: true
    security_opt:
      - no-new-privileges:true
    tmpfs:
      - /tmp:size=512m,mode=1777
    volumes:
      - ./state:/var/lib/cgm
      - ./cache:/var/cache/cgm
      - /absolute/path/to/media:/media:ro
      - ./transfer:/transfer:ro
    restart: unless-stopped
```

将媒体宿主机路径替换为实际目录。只读挂载允许浏览、扫描和元数据保存在数据库，但禁止向媒体目录写回 Manifest；需要显式 Push 时改为可写挂载并检查权限。

```bash
docker compose pull
docker compose up -d
```

访问 `http://部署机器IP:9999`，首次初始化创建所有者密码，将 Coser 根目录设置为 `/var/lib/cgm/cosers`、备份目录设置为 `/var/lib/cgm/backups`。无需一次性门票。添加媒体库时填写容器路径 `/media` 或其子目录，不填写宿主机路径。

浏览器上传图片由客户端读取并发送图片数据，容器不需要访问客户端的本地文件路径。

### Intel 核显硬件加速

CPU 带核显不等于容器自动拥有 GPU 权限。确认宿主机存在渲染节点，再在服务中添加：

```yaml
    devices:
      - /dev/dri/renderD128:/dev/dri/renderD128
    group_add:
      - "109" # 替换为 stat -c '%g' /dev/dri/renderD128 的实际结果
```

进入后台 Settings 查看探测结果，再显式选择可用的 AUTO／VAAPI 模式。无设备映射、权限不足或探测失败时使用 SOFTWARE；NVIDIA 另需宿主机驱动和容器运行时配置。具体说明见[安装手册](docs/INSTALLATION.md)。

### 更新与备份

先从后台备份并保留旧镜像标签／摘要，再执行 `docker compose pull && docker compose up -d`。不要删除持久卷或更改现有数据目录映射。跨 schema 回滚需要同时恢复对应数据库备份，不能只换旧程序。

完整备份与每日数据库快照不同，恢复前需确认原媒体仍可访问。详见[备份与恢复](docs/BACKUP_AND_RECOVERY.md)。

## 从源码构建

需要 Go 1.25.x（验证工具链1.25.12）、Node.js 20.19+、Corepack、CGO 的 C 编译器。运行时需 FFmpeg／FFprobe（含 libwebp）；RAW 另需兼容 LibRaw 的 dcraw。

```bash
cd ui/web
corepack pnpm install --frozen-lockfile
corepack pnpm build
cd ../..
CGO_ENABLED=1 go build -trimpath \
  -tags "cgm_web_embed cgm_galleryepic cgm_moegirl" \
  -ldflags "-X github.com/stashapp/stash/internal/build.githash=$(git rev-parse HEAD)" \
  -o cgm ./cmd/cgm
./cgm -config /absolute/path/cgm.json
```

配置示例见 [configs/cgm.example.json](configs/cgm.example.json)，Docker 配置及构建见 [docker/cgm](docker/cgm)。不要使用旧 Stash 构建目标替代 CGM。

## 公开源码范围与许可证

本仓库发布经过隐私清理的可构建源码快照；测试、测试夹具、真实业务清单、审计结果、设备截图和本机运维记录不公开。测试及完整开发历史单独保留，不应向本仓库直接推送未清理的开发历史。历史重写后请重新克隆，不要把旧分支合并回来。详见[发布范围](PUBLICATION.md)。

基于 Stash，遵循 GNU AGPLv3 或更新版本，保留原作者与第三方归属。参见 [LICENSE](LICENSE)、[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) 和 [依赖 SBOM](docs/legal/cgm.spdx.json)。容器还发布包含系统运行依赖的 SBOM 与构建证明。
