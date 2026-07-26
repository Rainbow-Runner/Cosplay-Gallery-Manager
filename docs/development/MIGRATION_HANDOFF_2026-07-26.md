# 开发迁移交接记录（2026-07-26）

## 1. 交接目的

当前宿主机资源不足以高效执行原 Stash 全仓大型测试、浏览器矩阵、跨平台媒体样本与性能基线。本记录用于把 Cosplay Gallery Manager 当前源码、决策、验证证据和未完成门禁迁移到更高性能设备，并保证接手后无需重新梳理上下文。

开发规范以以下文件为准：

- `docs/COSPLAY_DEVELOPMENT_MEMO.md`：已确认的产品与架构约束。
- `docs/COSPLAY_V1_DEVELOPMENT_PLAN.md`：第一版完整流程与门禁。
- `docs/development/IMPLEMENTATION_STATUS.md`：逐工作包实施状态。
- `docs/architecture/`：数据库、Gallery 聚合、扫描、Manifest、媒体与 Browse 边界。

## 2. 当前实现范围

当前已进入 0.6 运维、安全与恢复阶段，主要完成：

- 独立产品身份、全新 SQLite WAL 数据库和 Portable UUID Registry。
- Gallery 聚合根、唯一来源、成员硬上限、状态机、个人收藏/半星评分。
- 确定性媒体库发现、两阶段导入、原子扫描、BLAKE3 同来源重绑定。
- ZIP/CBZ 安全检查、RAW/图片/GIF/Video 分类和 Gallery 级处理队列。
- Gallery/Coser Manifest、三方合并、核心实体、Tag DAG 和关系批量保存。
- Gallery 级 Browse GraphQL、React 19 BrowseShell/ManageShell、静态 Poster Scrubber。
- 单所有者认证、Setup、统一认证资源、Operations 管理页。
- 每日 SQLite 在线快照、完整备份包、SHA-256 整包校验、保留策略。
- Web/CLI 共用维护恢复、安全快照、数据库/Coser 元数据交换、失败回滚。
- 恢复后 Session 撤销、任务取消、计划暂停、人工环境校验与显式恢复。
- 默认关闭的启动/每 24 小时自动扫描，使用数据库持久化租约。
- 无用户画像管理审计，仅记录高影响操作和任务摘要。

所有产品新增源码都在当前工作树内；没有需要从宿主机复制的媒体、数据库或秘密文件。

## 3. 已验证项目

使用 Go `1.25.12`、Node `24.18.0`：

```bash
GOMAXPROCS=2 GOTOOLCHAIN=local \
  GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  go test ./internal/persistence/productdb ./internal/productapi ./internal/productserver
```

结果：通过。覆盖数据库约束、备份 SHA-256、每日调度、自动扫描、完整恢复、Session/任务撤销、维护恢复，以及数据库交换后故障自动回滚。

```bash
cd ui/web
node node_modules/typescript/bin/tsc --noEmit -p tsconfig.json
node node_modules/vitest/vitest.mjs run --maxWorkers=1
```

结果：TypeScript 通过；Vitest 3 个测试文件、7 项测试通过。

Vite 生产构建已生成成功，当前主 JS 压缩前约 590 KiB；存在需要后续通过路由级懒加载解决的分包警告。`ui/web/build` 是可再生输出，不提交仓库。

## 4. 当前宿主机限制与未完成验证

采样配置：

- 2 vCPU。
- 3.6 GiB 内存，1.9 GiB Swap；采样时约 555 MiB Swap 已使用。
- 根磁盘 59 GiB，已使用 85%，仅余约 8.5 GiB。

全仓命令：

```bash
go test ./internal/... ./cmd/cgm
```

未形成可作为发布门禁的完整成功结果：

- 原 Stash `internal/api`、`internal/api/urlbuilders`、`internal/manager` 在测试装载阶段依赖未生成的 `ui/v2.5/build`，因此先行失败；这不是 CGM 产品包失败。
- 原 Stash `internal/autotag` 单包在当前机器耗时约 186 秒，后续旧业务大型测试在低配主机上不适合作为高频迭代门禁。
- 尚未执行 Playwright 离线 E2E、截图、键盘/读屏、目标浏览器矩阵。
- 尚未执行 Linux arm64、Windows amd64、Docker 双架构构建与安装验收。
- 尚未执行真实 LibRaw/FFmpeg RAW/GIF/Video 样本矩阵。
- 尚未执行 10,000 Gallery / 1,000,000 GalleryItem 的 p95 性能基线。

这些项目必须在新设备上重新运行，不能根据当前定向测试推定通过。

## 5. 新设备建议配置

推荐至少：

- 8 个现代 CPU 核心，16 GiB 内存，100 GiB 以上可用 SSD。
- Go 1.25.12。
- Node 24、pnpm（按 `ui/web/pnpm-lock.yaml` 安装）。
- ffmpeg 与 LibRaw/dcraw 兼容工具，用于真实媒体矩阵。
- Chromium/Firefox/WebKit 的 Playwright 依赖。
- GitHub CLI `gh` 并完成 `gh auth login`。

## 6. 新设备首轮续跑

```bash
git clone https://github.com/Rainbow-Runner/Cosplay-Gallery-Manager.git
cd Cosplay-Gallery-Manager
git switch <本交接分支>

go mod download
cd ui/web
pnpm install --frozen-lockfile
pnpm run validate
pnpm run build
cd ../..

go test ./internal/persistence/productdb ./internal/productapi ./internal/productserver
go build ./cmd/cgm
```

之后先生成旧 UI 依赖或明确隔离旧业务包，再运行全仓测试。不要为让全仓测试变绿而把新产品数据库重新接入旧 Stash Manager、Scene、Image 或 Gallery API。

## 7. 下一批开发优先级

1. Coser 头像/Banner 托管上传。
2. React 路由级分包、正式二进制静态资源打包和旧业务 UI 入口隔离。
3. Setup→导入→审核→激活→浏览→Manifest→备份恢复的离线 Playwright E2E。
4. 真实媒体、危险归档、跨平台和性能门禁。
5. AGPLv3 发行源码对应、第三方许可证清单和安装/恢复文档。

## 8. 安全与仓库注意事项

- 不提交 `ui/web/node_modules`、`ui/web/build`、媒体、数据库、备份、缓存或日志。
- 应用不得删除任何用户媒体来源文件。
- 第一版拒绝原 Stash/未知非空数据库，不做原地迁移。
- Browse DTO 和资源 URL 不得泄漏物理路径。
- 恢复、合并、删除和物理来源转移继续保持显式预览/确认与审计。

## 9. 迁移后续跑记录

2026-07-26在新环境继续完成了核心实体生命周期的Manage闭环：

- 新增Coser/Work/Character/Tag删除预览，按关系类型返回阻断引用数量；提交事务仍会重新检查引用和`metadata_revision`。
- 新增核心实体合并预览/提交GraphQL契约和Manage UI，显示双方revision、受影响Gallery及全部冲突；任何冲突都会禁用提交，必须先通过普通编辑解决。
- 合并和删除均要求前端明确确认词，并记录无名称、无路径的高影响管理审计。
- 合并继续使用既有永久UUID Alias、Slug重定向和Manifest待Push规则；删除继续只允许无引用实体并永久Tombstone UUID。
- Coser合并在数据库提交后写`redirect_to_uuid`；若文件系统收尾失败，API返回待处理警告而不声称数据库事务已回滚。

本轮验证结果：

```bash
GOMAXPROCS=2 GOTOOLCHAIN=local \
  GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  /tmp/cgm-go1.25.12/bin/go test \
  ./internal/persistence/productdb ./internal/productapi ./internal/productserver
```

结果：通过。`go build ./cmd/cgm`通过。

```bash
cd ui/web
corepack pnpm run check
corepack pnpm run test
corepack pnpm run build
```

结果：TypeScript通过；Vitest 4个测试文件、9项测试通过；Vite生产构建通过。当前主JS minify后、gzip前约612 KiB，路由级分包警告仍未解决，不能标记为通过。

随后完成了Gallery归档后永久删除闭环：

- 数据库只允许删除`ARCHIVED` Gallery，并在事务中重新检查`metadata_revision`；DRAFT/ACTIVE和过期revision均拒绝。
- 删除前取消并解除相关处理任务引用，使任务诊断记录保留；随后删除Gallery业务/个人/Item记录，永久Tombstone set/item/link UUID。
- 有绑定来源时原子建立`IgnoredGallerySource`，防止发现流程静默重建；无来源Gallery不伪造路径记录。
- 删除服务不读取、删除、移动或改写来源媒体、Gallery Manifest、托管来源资源和衍生缓存文件。
- GraphQL由产品Server注入所有者密码验证服务；Mutation同时要求重新输入密码和精确确认词`DELETE`，成功与失败进入无路径管理审计。
- Manage Gallery基本信息页新增删除影响预览和双重确认弹窗；非`ARCHIVED`状态永久禁用提交。

本阶段验证结果：

```bash
GOMAXPROCS=2 GOTOOLCHAIN=local \
  GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  /tmp/cgm-go1.25.12/bin/go test \
  ./internal/persistence/productdb ./internal/productapi ./internal/productserver
```

结果：通过。`go build ./cmd/cgm`通过。

```bash
cd ui/web
corepack pnpm run check
corepack pnpm run test
corepack pnpm run build
```

结果：TypeScript通过；Vitest 5个测试文件、11项测试通过；Vite生产构建通过，共转换657个模块。当前主JS minify后、gzip前约620 KiB，路由级分包警告仍未解决，不能标记为通过。

迁移备忘录中的下一项未完成开发任务现为Coser头像/Banner托管上传；必须继续遵守独立Coser Manifest、单一托管元数据根、裁切/焦点同步和绝不写入用户媒体来源的既有约束。
