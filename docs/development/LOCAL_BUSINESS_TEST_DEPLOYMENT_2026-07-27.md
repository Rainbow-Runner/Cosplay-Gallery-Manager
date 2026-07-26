# 本机实际业务应用测试部署

## 部署基线

- 运行形态：Linux amd64 原生单所有者服务。
- 源码提交：`3dc86fb6be38216349fb039a6b3253fb392ae422`。
- 产品版本：`0.1.0-dev`，仅作为第一版正式发布前的本机业务测试实例。
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
