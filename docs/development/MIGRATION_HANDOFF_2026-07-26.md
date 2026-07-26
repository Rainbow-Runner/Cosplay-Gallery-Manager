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
- React 19页面级路由分包、正式单文件二进制静态资源嵌入和旧`ui/v2.5`产品入口隔离。
- Coser未引用托管资源集中审阅、选择性清理、提交时引用重检、所有者重新认证和无路径审计。

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

结果：TypeScript 通过；Vitest 7 个测试文件、13 项测试通过。

Vite 生产构建已生成成功，共转换660个模块；当前主JS minify后、gzip前约470KiB，其余页面按路由生成懒加载块，原大于500KiB分包警告已消失。`ui/web/build`仍是可再生输出，不提交仓库。

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
cd ../..

go test ./internal/persistence/productdb ./internal/productapi ./internal/productserver
make build-cgm
```

`make build-cgm`会先生成`ui/web/build`，再以`cgm_web_embed`标签嵌入产品二进制，并检查CGM依赖图不包含旧`ui`包。之后再生成旧 UI 依赖并运行全仓测试；不要为让全仓测试变绿而把新产品数据库重新接入旧 Stash Manager、Scene、Image 或 Gallery API。

## 7. 下一批开发优先级

1. Setup→导入→审核→激活→浏览→Manifest→备份恢复的离线 Playwright E2E。
2. 真实媒体、危险归档、跨平台和性能门禁。
3. AGPLv3 发行源码对应、第三方许可证清单和安装/恢复文档。

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

随后完成了Coser头像/Banner托管上传的首个完整闭环：

- 新增同源、Session认证的multipart上传端点；按实际内容只允许JPEG、PNG和静态WebP，拒绝GIF/SVG等其他格式，并执行20MiB/50MP限制。
- 原图和头像480、Banner 960/1600派生图只写入单一`<coser_root>/<uuid>/assets/`目录；目录和资源读取逐级拒绝符号链接。
- 上传保存归一化头像裁切与Banner焦点，使用Coser独立`metadata_revision`乐观锁，并将既有Coser Manifest标为`DB_DIRTY`。
- 替换时不删除旧原图或派生图；数据库提交冲突最多留下未引用托管资源，不会触碰用户媒体来源。
- Manage资料页新增上传、预览和裁切/焦点字段；Browse Coser索引与详情使用不透明认证资源URL，无头像保持名称首字符占位，无Banner隐藏区域。
- 上传成功/失败进入不含物理路径的管理审计。

本阶段验证结果：

```bash
GOMAXPROCS=2 GOTOOLCHAIN=local \
  GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  /tmp/cgm-go1.25.12/bin/go test \
  ./internal/persistence/productdb ./internal/coserasset \
  ./internal/productapi ./internal/productserver
```

结果：通过。`go build -o /tmp/cgm-build-check ./cmd/cgm`通过。

```bash
cd ui/web
corepack pnpm run check
corepack pnpm run test
corepack pnpm run build
```

结果：TypeScript通过；Vitest 6个测试文件、12项测试通过；Vite生产构建通过，共转换658个模块。当前主JS minify后、gzip前约626 KiB，路由级分包警告仍未解决，不能标记为通过。

随后完成了React 19路由分包、正式二进制静态资源打包和旧业务UI入口隔离：

- Browse、Manage、Setup、登录和维护页面均改为`React.lazy`路由边界，URL、Shell结构和已确认交互不变。
- Vite生产构建共转换659个模块，主JS由约626KiB降至约470KiB，页面形成独立懒加载块且不再触发500KiB警告。
- 新增独立`ui/web` Go嵌入包；仅正式`cgm_web_embed`构建标签包含可再生`ui/web/build`，普通后端开发构建不要求前端产物。
- 产品Server在未设置`web_root`时服务嵌入SPA，显式`web_root`继续作为覆盖；深层路由返回`index.html`，哈希资源使用一年immutable缓存。
- `make cgm`和`make build-cgm`现在先运行新前端构建再生成嵌入式单文件二进制；边界检查确保`cmd/cgm`依赖`ui/web`而不是会嵌入`ui/v2.5/build`的旧`ui`包。
- `ui/web/build`仍被忽略且未提交，旧Stash UI源码和构建入口未被删除或改造。

本阶段验证结果：

```bash
cd ui/web
corepack pnpm run check
corepack pnpm run test
corepack pnpm run build
```

结果：TypeScript通过；Vitest 6个测试文件、12项测试通过；Vite生产构建通过，共转换659个模块，主JS约470KiB，无大包警告。

```bash
GOTOOLCHAIN=local GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  /tmp/cgm-go1.25.12/bin/go test ./internal/productserver ./ui/web
GOTOOLCHAIN=local GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  /tmp/cgm-go1.25.12/bin/go test -tags cgm_web_embed \
  ./internal/productserver ./ui/web
```

结果：默认开发模式和正式嵌入模式均通过；正式模式额外验证产品首页、深层路由、哈希资源缓存和嵌入内容不含旧`v2.5`路径。

```bash
GOTOOLCHAIN=local GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  make CGM_GO=/tmp/cgm-go1.25.12/bin/go \
  CGM_OUTPUT=/tmp/cgm-embedded-check \
  BUILD_DATE=20260726 GITHASH=b2573ec5 \
  STASH_VERSION=0.1.0-dev OFFICIAL_BUILD=false build-cgm
```

结果：通过，生成约24MiB单文件产品二进制；依赖边界检查只发现`github.com/stashapp/stash/ui/web`，未发现旧`github.com/stashapp/stash/ui`包。

随后完成了Coser未引用托管资源的集中审阅和人工清理闭环：

- Operations新增集中审阅面板，按已替换、已合并Coser、已删除Coser显示资源组；只返回不透明组ID、技术Coser UUID、头像/Banner类型、文件数量/容量和最后修改时间。
- 扫描只接受`avatar|banner-<uuid>`原图和既定480/960/1600 JPEG派生命名；未知文件、上传/清理临时文件、目录、符号链接和未注册UUID目录只计入忽略摘要，不返回路径且不能选择。
- 现用头像/Banner组始终排除；清理提交在SQLite immediate事务中重新装载全部Coser引用，审阅过期即返回冲突，防止上传或Manifest写入在最终检查与删除之间发布旧路径。
- 文件删除先在同一托管目录隔离重命名并复核inode，确认仍为同一普通文件后才删除；Coser Manifest、资料目录和全部用户媒体不读取、不移动、不删除。
- UI必须人工选择最多100组、重新输入所有者密码和精确确认词`CLEAN`；HTTP端点要求Session和同源，失败后界面重新审阅且不自动重试。
- 清理成功/失败均进入管理审计；摘要只记录选择/删除的组数、文件数和字节数，不记录Coser名称、文件名或物理路径。

本阶段验证结果：

```bash
GOTOOLCHAIN=local GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  /tmp/cgm-go1.25.12/bin/go test \
  ./internal/persistence/productdb ./internal/coserasset \
  ./internal/productapi ./internal/productserver
GOTOOLCHAIN=local GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  /tmp/cgm-go1.25.12/bin/go test -tags cgm_web_embed \
  ./internal/coserasset ./internal/productserver ./ui/web
```

结果：通过。覆盖现用资源排除、替换/删除分组、未知文件与符号链接跳过、过期审阅冲突、选择性清理、Manifest保留、认证/同源/密码/确认词门禁及无路径审计。

```bash
cd ui/web
corepack pnpm run check
corepack pnpm run test
corepack pnpm run build
```

结果：TypeScript通过；Vitest 7个测试文件、13项测试通过；Vite生产构建通过，共转换660个模块，主JS约470KiB，无大包警告。

```bash
GOTOOLCHAIN=local GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  /tmp/cgm-go1.25.12/bin/go build -tags cgm_web_embed \
  -o /tmp/cgm-embedded-check ./cmd/cgm
```

结果：通过。`ui/web/build`仍为可再生忽略输出，未加入提交范围。

随后完成了恢复异机路径映射与完整备份跨平台损坏演练：

- 恢复最终化写入独立的`RESTORE_PATH_MAPPING_REQUIRED`维护门槛；在所有媒体库完成决策前，Web与CLI均不能恢复任务或自动计划。
- 维护页逐库显示恢复根，要求映射到本机现存、无符号链接的绝对目录，或明确在本机禁用；提交要求恢复旧根并发校验、所有者密码及精确确认词`MAP`。
- 路径映射事务支持Windows盘符、UNC和POSIX旧路径，同步改写GallerySource、IgnoredGallerySource与Gallery Manifest，清空发现快照，并把来源标记为`MISSING / NEEDS_RESCAN`；事务不读取媒体、不触发发现或扫描。
- 恢复包中的Coser Manifest路径自动映射到当前机器保留的Coser元数据根，并进入重新校验状态。
- CLI新增`-map-restored-paths`交互动作，与Web共用同一映射服务、路径校验和无路径审计；逐库输入新根或留空禁用，最后输入`MAP`。
- 完整备份写入端与恢复端共用平台中立ZIP路径规则，拒绝路径穿越、反斜杠/盘符路径、非NFC名称、大小写碰撞、Windows保留名、符号链接、特殊文件与未知Entry。
- 恢复解包额外校验条目总量/总大小/单项大小/压缩比、精确版本清单、必要Entry及单一JSON对象，并继续在替换前验证SQLite产品身份。
- 表驱动损坏样本覆盖路径、重复项、JSON、缺项、无效SQLite、压缩炸弹和截断ZIP；前端回归覆盖逐库决策、密码与`MAP`门禁及提交载荷。

本阶段验证结果：

```bash
GOMAXPROCS=2 GOTOOLCHAIN=local \
  GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  /tmp/cgm-go1.25.12/bin/go test \
  ./internal/persistence/productdb ./internal/coserasset \
  ./internal/productapi ./internal/productserver ./ui/web ./cmd/cgm
GOMAXPROCS=2 GOTOOLCHAIN=local \
  GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  /tmp/cgm-go1.25.12/bin/go test -tags cgm_web_embed \
  ./internal/coserasset ./internal/productserver ./ui/web
```

结果：通过。覆盖异机路径映射门槛、Windows/UNC/POSIX路径改写、Coser Manifest根迁移、显式禁用、不自动扫描，以及15类跨平台损坏备份样本。

```bash
cd ui/web
corepack pnpm run test
corepack pnpm run check
corepack pnpm run build
```

结果：Vitest 8个测试文件、14项测试通过；TypeScript与Vite生产构建通过，共转换660个模块，主JS约470KiB。

```bash
GOTOOLCHAIN=local GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  /tmp/cgm-go1.25.12/bin/go build -tags cgm_web_embed \
  -o /tmp/cgm-embedded-check ./cmd/cgm
```

结果：通过，生成约24MiB单文件验证产物。下一项未完成开发任务现为Setup→导入→审核→激活→浏览→Manifest→备份恢复的离线Playwright E2E及无障碍矩阵。

2026-07-27继续完成阶段10/11本地发行加固：

- 新增完全离线的Playwright主流程，覆盖Setup→导入→审核→激活→Browse→Manifest→完整备份恢复→路径映射→显式恢复→重扫；Linux Chromium下axe A/AA检查、键盘焦点和1440/390截图回归通过。
- 新增运行时合成DNG/GIF/MP4/JPEG真实媒体矩阵，实际执行dcraw和FFmpeg；修正dcraw TIFF输出以及原子临时文件丢失目标扩展名导致的格式选择问题。
- 扩展危险归档边界样本；新增10,000 Gallery/1,000,000 Item固定4核性能门禁，全部p95目标通过，最接近上限的是Gallery列表约493ms和时间线约477ms。
- 固定Go 1.25.12的CGO交叉构建通过Linux amd64、Linux arm64和Windows amd64。新增Docker发行配置，Linux amd64镜像实际构建、非root启动、Docker健康检查和宿主`/healthz` 204通过。
- 新增About/Legal与公开构建信息、精确提交源码入口、AGPL/无担保/Stash归属、52项应用依赖清单、SPDX 2.3 SBOM、安装/升级/反向代理与恢复文档、干净提交源码对应校验脚本。
- 当前仍不得宣称G9或1.0完成：Firefox/WebKit/Edge/真实移动浏览器、真实Linux arm64与Windows安装运行、arm64 Docker健康启动、网络挂载、真实读屏/200%缩放、正式CI/RC产物和容器平台SBOM尚未执行。宿主机arm64 binfmt注册需要特权且被安全策略拒绝，本轮只记录真实已通过的交叉构建结果。

下一项未完成任务更新为：在正式目标runner建立CI/夜间/RC矩阵，执行真实Linux arm64、Windows amd64、Docker arm64和完整浏览器/无障碍验收；所有通过后才能冻结0.9并运行正式发行源码对应门禁。

2026-07-27产品范围调整：第一版取消Windows平台构建、安装和运行验收，Windows原生支持整体移入后续版本；已完成的Windows交叉构建仅保留为历史技术证据，不再属于G9或1.0门禁。平台脚本和交叉工具链已移除MinGW/Windows目标。当前下一项改为真实Linux arm64、Docker arm64和完整浏览器/无障碍验收。

同日新增独立`CGM CI`与`CGM Nightly Gates`定义：提交门禁覆盖产品Go/React、真实媒体、归档、法律资料确定性、源码对应、Chromium离线E2E、Linux双架构CGO与Docker多架构构建；夜间门禁覆盖固定4核百万Item性能和Firefox/WebKit离线E2E。工作流尚未在远端runner执行，首次结果仍需逐项核实，不能在当前本地记录中标记为通过。
