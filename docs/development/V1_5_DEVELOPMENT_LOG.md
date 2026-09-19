# Cosplay Gallery Manager 1.5 本地开发记录

> 状态：进行中
> 开始日期：2026-07-27
> 当前分支：`agent/cgm-migration-handoff-20260726`
> 开发版本：`1.5.0-dev`

## 范围与工作方式

- 本轮以已经确认的Gallery聚合、单来源、只读媒体、确定性发现、显式DRAFT和人工元数据审核设计为基础，不修改既有产品决策。
- 暂不执行或扩展Windows、macOS、Linux arm64、Docker多架构等平台构建任务；这些工作不作为本轮本机功能迭代门禁。
- 正式业务测试数据库继续由唯一的`cosplay-gallery-manager.service`进程访问，禁止同时启动第二个后端。
- 前端开发使用Vite HMR：`make web-ui-start`只监听loopback `127.0.0.1:3100`，并将API、Session和媒体资源同源代理到`127.0.0.1:9999`。
- 后端变更使用Go增量构建，验证后原子替换本机测试二进制并重启用户服务；数据库、配置、缓存和媒体库不随二进制替换。
- 每个功能阶段均先运行针对性测试，再执行本机增量部署和真实页面检查；结果持续追加到本文档。

## 启动基线

- 开始时源码HEAD：`18684271`（`Document local CGM business test deployment`）。
- 工作区开始时干净；本地分支比同名远端领先9个提交。
- 开始时本机业务测试服务仍运行提交`3dc86fb6be38216349fb039a6b3253fb392ae422`的嵌入式Linux amd64二进制。
- 真实媒体库和`.cosplay-root`发现已验证；此前暴露出的主要管理缺口是扫描规则只有新增入口，没有编辑或删除入口。

## 1.5-01 开发环境日志增强

### 实现

- 启动配置新增`log_level`，允许`DEBUG | INFO | WARN | ERROR`；旧配置省略该字段时保持`INFO`默认值。
- 服务日志改用Go结构化日志，增加稳定事件：
  - `CGM_SERVICE_STARTED` / `CGM_SERVICE_STOPPED`
  - `CGM_WORKERS_STARTED`
  - `CGM_HTTP_REQUEST`
  - `CGM_MANAGE_OPERATION`
  - `CGM_MEDIA_JOB_COMPLETED` / `CGM_MEDIA_JOB_FAILED`
- 每个HTTP请求生成`X-Request-ID`，DEBUG日志记录请求ID、端点类别、方法、状态、耗时和响应字节数。
- 管理Mutation日志与数据库管理审计共享事件码、技术目标Kind/ID和成功/失败结果，失败只输出稳定错误码。
- 隐私边界：不记录URL查询字符串、GraphQL正文/变量、标题、人物名称、搜索词、媒体路径或错误原文。
- Vite开发服务器增加到本机9999后端的同源代理，登录Session与认证资源可以在HMR页面中直接使用。

### 阶段验证

```text
go test ./internal/productlog ./internal/productserver ./internal/productapi ./cmd/cgm
PASS

pnpm run check
PASS

pnpm run test
8 files / 15 tests PASS
```

## 1.5-02 扫描规则编辑与删除

### 实现

- `RecognitionRuleStore`新增Update/Delete；更新继续执行MARKER、FIXED_DEPTH和PATH_TEMPLATE的既有确定性校验。
- 规则类型改变时，数据库显式清空不适用于新类型的`pattern`或`fixed_depth`，避免残留字段绕过表约束。
- GraphQL新增`updateRecognitionRule`和`deleteRecognitionRule`，成功与失败均进入管理审计和结构化运行日志。
- Libraries页面可编辑规则名称、模式、顺序、深度/RE2模板、启用状态及自动创建DRAFT开关。
- 删除使用页面内二次确认。删除只删除规则本身；已有发现快照Candidate保留并将`rule_id`置空，已经导入的DRAFT、GallerySource和源媒体不受影响。

### 阶段验证

```text
go test ./internal/persistence/productdb ./internal/productapi \
  ./internal/productserver ./internal/productlog ./cmd/cgm
PASS

pnpm run check
PASS

pnpm run test
9 files / 17 tests PASS

pnpm run build
661 modules transformed; production build PASS
```

## 1.5-03 Marker Gallery标题保底与目录名实体提示

### 实现

- 保持`.cosplay-root` Marker与`.cosplay.json` Manifest分离；本工作项不改变Manifest识别、Pull/Push或冲突处理。
- MARKER Candidate新增确定性的`title`预览值，直接取Gallery来源根目录最后一级文件夹名称，并执行NFC规范化和既有300字符限制。
- MARKER Candidate导入或`AUTO_CREATE_DRAFT`自动导入时，将文件夹名称直接写入Gallery正式标题并据此生成slug；不再为同一个保底标题创建重复的待审核元数据建议。
- PATH_TEMPLATE既有行为不变：命名捕获仍只写入待审核建议，不会因本工作项自动进入正式元数据。
- Marker Gallery编辑页根据来源文件夹名称，对数据库中已经存在的Coser、Work、Character名称和Alias执行确定性匹配：
  - 使用NFC、大小写折叠和完整名称/Alias子串匹配；
  - 歧义名称、文件夹中的非精确单字符命中和被更长命中覆盖的短名称不产生提示；
  - 已命中的Work用于限制Character范围，降低同名角色误匹配；
  - 匹配结果只显示为Cast页编辑提示。Coser/Character必须点击`Use`加入当前关系草稿并再次点击`Save all relations`后才成为正式关系；Work只作为上下文显示。
- 匹配失败不产生错误、不阻止DRAFT创建，也不会创建新Coser、Work或Character。

### 阶段验证

```text
go test ./internal/discovery ./internal/persistence/productdb ./internal/productapi
PASS

go test ./internal/persistence/productdb ./internal/productapi \
  ./internal/productserver ./internal/productlog ./cmd/cgm
PASS

pnpm run check
PASS

pnpm run test
9 files / 17 tests PASS

pnpm run build
661 modules transformed; production build PASS
```

## 1.5-04 管理表单校验与Gallery关系回显

### 问题求证

- 检查本机用户服务journal，没有发现本次Gallery关系保存对应的HTTP 500；GraphQL请求均为200。
- 只读校核实际业务数据库确认目标Gallery已持久化1项Coser Credit和1项Cast，`metadata_revision`也已递增。因此`Save all relations`实际成功，重新打开后不显示属于前端Apollo内存缓存陈旧，不是数据库丢失。
- 目录名Coser、Work、Character提示不是一次性扫描结果。每次后端读取Gallery详情时，都会根据当前来源目录名、现有实体名称/Alias和已命中的Work重新确定性计算；它们不写入正式关系，也不会自动创建实体。
- 第二次打开时，已经正式保存的匹配项现在显示`Already saved`，不再给出重复的`Use`动作。
- 既有后端允许扩展SocialAccount `platform_key`，格式为`[a-z0-9][a-z0-9_-]{0,63}`；未知平台继续使用通用显示，不需要新增schema或数据库迁移。

### 实现

- Coser社交账号的Platform key改为可选择常用项的`datalist`，内含Instagram、Twitter/X、Weibo、Bilibili、Xiaohongshu、Douyin、TikTok、YouTube、Pixiv等常用键；输入仍可直接编辑，从而保留后端既有的自定义平台能力。
- 社交账号只有在Platform key格式正确且URL为不含凭据的绝对HTTP(S)地址时才能提交。
- Gallery和核心实体详情采用Apollo `network-only`读取本机CGM GraphQL后端，不访问互联网；保存Gallery关系后还会显式refetch，并用服务端返回结果刷新编辑草稿。
- Gallery关系编辑器在任一Coser、Character或Tag空行未完成时禁用`Save all relations`并显示说明，避免把不完整草稿交给后端。
- Character创建/保存要求Name和Primary Work；缺少任一项时按钮不可用。后端仍保留防御性校验，并把缺少Work作为可理解的GraphQL校验错误返回，不再隐藏为`internal server error`。
- 同类管理表单一并增加提交门禁和进行中状态：媒体库、扫描规则、Tag父级、Gallery外部链接及运行时设置。Setup、登录和Coser托管图片原本已有相应门禁，保持不变。
- 核心实体创建/更新、Coser社交账号新增和Gallery关系批量替换增加无业务元数据的管理审计；关系审计只记录Credit、Cast、Tag数量，不记录名称、URL或路径。

### 阶段验证

```text
go test ./internal/archivecheck ./internal/browse ./internal/coserasset \
  ./internal/discovery ./internal/gallery ./internal/manage ./internal/manifest \
  ./internal/mediaaccess ./internal/mediaprocessing ./internal/mediaresource \
  ./internal/persistence/productdb ./internal/portableid ./internal/processingworker \
  ./internal/product ./internal/productapi ./internal/productauth ./internal/productlog \
  ./internal/productserver ./internal/sourcescan ./cmd/cgm ./ui/web
PASS

go test -tags cgm_web_embed ./internal/productserver ./cmd/cgm ./ui/web
PASS

make verify-cgm-ui-boundary
PASS

pnpm run check
PASS

pnpm run test
11 files / 20 tests PASS

pnpm run build
661 modules transformed; production build PASS
```

直接执行`go test ./...`仍需要先生成上游Stash遗留的`ui/v2.5/build`嵌入目录；本阶段按独立CGM CI产品包清单和`cgm_web_embed`标签完成回归，没有为绕过该遗留前置条件而重新引入旧UI依赖。

## 本机热开发与日志检查

前端功能检查：

```bash
cd /home/rainbowrunner/Dev/Cosplay-Gallery-Manager
make web-ui-start
```

浏览器打开`http://127.0.0.1:3100`。前端修改由Vite HMR即时更新，API和认证仍使用当前9999业务测试后端。

开发日志：

```bash
journalctl --user -u cosplay-gallery-manager.service -f -o cat
journalctl --user -u cosplay-gallery-manager.service --since today -o cat
```

开发实例在启动配置中使用`"log_level": "DEBUG"`；完成问题定位后恢复为`INFO`，避免长期保留高频请求日志。

## 待完成

- 通过真实Libraries页面编辑并恢复当前MARKER规则，确认下一次发现使用更新值。
- 通过真实页面执行删除二次确认；业务库中的现用规则默认不删除，只使用专门创建的临时规则验证删除路径。
- 使用一个新建的空`.cosplay-root`真实目录重新发现，确认自动创建的DRAFT标题等于目录名；已有DRAFT不会被本工作项静默改名。
- 在目录名包含已有Coser/Work/Character名称或Alias的样本上检查Cast页提示，并人工决定是否加入和保存正式关系。
- 在真实Coser资料中分别选择常用Platform key和输入自定义key，确认两者均能保存并重新显示。
- 重新打开已经保存关系的Gallery Cast页，确认Credit/Cast及`Already saved`提示与数据库一致。
- 分别尝试缺少Primary Work的Character、未完成的Gallery关系行和非法外部URL，确认提交按钮保持禁用。
- 根据实际业务测试暴露的问题继续追加1.5工作项和验证记录。

## 1.5 本机增量部署记录

- 2026-07-27 23:12 CST首次安装本轮开发二进制；23:15 CST在最终UI提示与测试重建后再次增量替换。当时的开发版本为`1.5.0-dev`，二进制SHA-256为`e0513629b7c512a57a58a5c4022a23feb3e960e9ea7887861282ab4ce28c2323`。
- 2026-07-27 23:49 CST安装Marker标题保底与目录名实体提示构建；当前二进制SHA-256为`6832f9188c3233aa69133f0fdab32aaf504aee8e453ef49177f5db041f6cc0e4`，替换前二进制另存于`/tmp/cgm-before-marker-title-fallback`。
- 2026-07-28 00:48 CST安装管理表单校验与Gallery关系回显构建；当前二进制SHA-256为`e10dacab3fc167dc536bde86fa733507c3de2532319f1fb05d1e34fa644c15cb`，替换前二进制另存于`/tmp/cgm-before-form-relations-fix`。
- 2026-07-30 02:40 CST安装UI-03～UI-08 GalleryEpic视觉改造构建；该构建二进制SHA-256为`38af89d5afe479a6f38d88d3a08735b68a03044c5648cf5a00f28a79e089dded`，替换前二进制另存于`/tmp/cgm-before-galleryepic-ui-20260730`。
- 2026-07-30 23:56 CST安装Gallery卡片Coser详情入口构建；当前二进制SHA-256为`87e87d2128f1fd2d258d6ef2768b991c152c0c41adeb74643a536bb6912c99ce`，替换前二进制另存于`/tmp/cgm-before-coser-card-links-20260730`。
- 2026-07-31 00:29 CST安装Coser详情高保真与数字分页最终构建；当前二进制SHA-256为`1aac8277d217cac3a5c5d7d0ab41970937a7802b26ee28f2555c069d17b5e2b0`，替换前功能基线二进制另存于`/tmp/cgm-before-coser-detail-20260731`，其SHA-256为`87e87d2128f1fd2d258d6ef2768b991c152c0c41adeb74643a536bb6912c99ce`。
- 本次部署入口引用`index-DQfwh2rp.js`和`index-DpMxJ2Aj.css`，Coser详情与数字分页懒加载块分别为`EntityDetailPages-EWnHhW4q.js`和`Patterns-CpDb8YDR.js`；服务保持`enabled/active`且Health/Ready均为204。
- 本次部署后配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，产品数据库inode仍为`19679716`；没有替换数据库、配置、媒体库、缓存或Manifest。
- 本次部署后用户服务保持`enabled/active`，Health/Ready均为204；入口HTML引用`index-DIaGmrZF.js`和`index-hypU1gnW.css`，已部署的`GalleryCard-BmxDwNEF.js`包含`/coser/`路由与`gallery-card__coser-link`。
- 02:40部署后用户服务为`enabled/active`，`/healthz`与`/readyz`均返回204，`/`与`/legal`均返回200；当时首页引用`index-DMxPCGgd.js`、`index-De7_OG72.css`以及对应GalleryCard、GalleryDetail和Patterns哈希Chunk。
- `/about.json`报告`version=1.5.0-dev`、`gitHash=local`、`buildTime=2026-07-30`和`exactSourceAvailable=false`。配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，产品数据库inode仍为`19679716`；重启期间SQLite文件大小随正常运行写入/检查点变化，没有替换数据库文件。
- 首次构建校核发现旧Make构建信息仍尝试调用未安装的系统Go，曾短暂把脏工作树错误标识为精确提交；该构建立即被`gitHash=local`的正确开发构建替换。最终`/about.json`为`version=1.5.0-dev`、`exactSourceAvailable=false`，没有把未提交改动声明为精确发行源码。
- 最终用户服务重启成功，`/healthz`与`/readyz`均返回204；数据库、配置、缓存、媒体库和Manifest均未随二进制替换。
- 替换前二进制和配置分别备份到`/tmp/cgm-before-1.5`与`/tmp/cgm-config-before-1.5.json`；没有复制、迁移或修改产品数据库。
- 本机启动配置增加`"log_level": "DEBUG"`；当前配置SHA-256为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`。两次用户服务重启均成功，最终`/healthz`与`/readyz`均返回204。
- `/about.json`报告`1.5.0-dev`及`exactSourceAvailable=false`。这是包含尚未提交本地修改的业务测试构建，故不会错误声明为某个精确Git提交的对应发行产物。
- journal已验证`CGM_WORKERS_STARTED`、`CGM_SERVICE_STARTED`及连续`CGM_HTTP_REQUEST`事件；请求仅显示技术请求ID、`HEALTH/ABOUT`端点类别、状态、耗时和响应大小。
- Vite开发服务器在`127.0.0.1:3100`启动成功；首页返回200，`/session/status`经同源代理返回200及真实后端Setup/认证状态。验证结束后已停止Vite，9999后端继续由唯一用户服务进程运行。

## 1.5-05 Gallery卡片媒体计数与GalleryEpic视觉改造起步

### 卡片计数规则

- Gallery卡片不再无条件输出四种媒体类型，也不再使用`P100 / S0 / G0 / V0`形式。
- 展示统一为“数量+类型缩写”，并保持PHOTO、SELFIE、GIF、VIDEO顺序；数量为0的类型被过滤。
- 示例：仅100张Photo显示`100P`；96张Photo、4张Selfie和2个Video显示`96P 4S 2V`；四类全为0时隐藏整段。
- 本次只改变Browse卡片格式化和渲染，不改变后端计数、PENDING/ERROR成员计入口径、GraphQL DTO或Gallery状态机。
- 同一规则已写入`GALLERYEPIC_FRONTEND_GAP_ANALYSIS_AND_PLAN_2026-07-30.md`的Gallery卡片建议和UI-03测试清单。

### UI-00与UI-01首个增量切片

- 将2026-07-30观察到的GalleryEpic浅色版本的基础测量值固化为本地CSS语义变量：208/256px侧栏、64px顶栏、32px桌面内容边距、16px网格Gap和8～12px圆角。
- 新增白色、中性黑灰、危险色、焦点色和表面层级Token；当前先与旧页面样式隔离，不提前把尚未迁移的页面强制切换为浅色，避免产生不可用的混合主题。
- 新增CGM自行实现的内嵌SVG线性图标，不读取远程图标、字体或媒体，也不复制GalleryEpic Logo和组件代码。
- 新增原创CGM Gallery图形字标，并在现有Browse顶栏替换旧文字品牌。
- 搜索、收藏、Legal和Manage入口已由Unicode字符替换为统一24×24视口、2px描边、`currentColor`图标。
- 新增Button、IconButton、Input、Select、Textarea、Badge和Alert首批基础组件，覆盖primary、secondary、ghost、danger、disabled和`focus-visible`状态；后续页面迁移必须复用语义Token。

### 阶段验证

```text
pnpm exec vitest run \
  src/browse/galleryMediaCount.test.ts \
  src/ui/Primitives.test.tsx --maxWorkers=1
2 files / 6 tests PASS

pnpm run check
PASS

pnpm run test
13 files / 26 tests PASS

pnpm run build
665 modules transformed; production build PASS
main JS 471.46 KiB; no large-chunk warning
```

### 后续

- UI-01仍需补齐Breadcrumb、Pagination、Tabs、Dialog、Drawer、DataTable和视觉夹具。
- 完成基础组件后进入UI-02，把Browse横向顶栏迁移为桌面固定侧栏、64px品牌顶栏和移动Drawer；切换时再统一启用浅色Token，避免半成品主题进入业务检查。

## 1.5-06 UI-02 BrowseShell与响应式导航

### 实现

- Browse从深色横向导航切换为浅色应用壳层：
  - `1024px`及以上使用固定左侧栏；
  - 侧栏宽度按Token在208px/256px之间响应；
  - 主内容区使用64px粘性品牌顶栏和32px桌面内容边距；
  - `1024px`以下隐藏侧栏并显示移动菜单按钮。
- 原Browse URL和页面组件保持不变，导航按Galleries、Library、Explore分组；Home、List、Magic、Coser、Work、Character、Tag、Timeline、Random、Favorites、History、Search、Manage和Legal全部仍可达。
- 移动Drawer使用本地组件实现：
  - 打开后初始焦点落在关闭按钮；
  - Tab/Shift+Tab在Drawer内循环；
  - Escape或点击遮罩关闭；
  - 导航或关闭后焦点返回原菜单按钮；
  - 打开期间禁止背景页面滚动。
- 顶栏使用原创CGM字标，桌面保留Search和Manage快捷入口；移动窄屏保留Search，Manage继续位于Drawer。
- Browse作用域启用浅色语义Token，不强制改写Manage、Setup和Maintenance既有主题，避免跨Shell半迁移。
- 页面标题由超大Georgia改为30px无衬线层级；Gallery卡片标题、浅色Poster占位、边框、阴影及媒体计数转为中性紧凑风格。
- 保持既定Gallery网格`6/5/4/3/2`断点，不照搬GalleryEpic的4/5列模型。

### UI-01导航前置组件补齐

- 图标集新增Home、Gallery、Magic、Coser、Work、Character、Tag、Timeline、Random、History、Menu和Close等原创内嵌SVG。
- 新增Breadcrumbs、Pagination、Tabs、Drawer、DataTable、EmptyState和ErrorState；全部使用语义Token且不加载远程资源。
- Drawer、分页、Tab和DataTable已增加Vitest语义与键盘测试；Dialog/ConfirmDialog、CardMedia、Avatar和Skeleton留到首次实际使用页面时完成，避免提前建立未验证API。

### 验证与问题修复

- 首次离线E2E暴露此前表单校验工作已把标签改为`Name (required)`，但Playwright仍按旧`Name`精确匹配；已同步更新Coser、媒体库和扫描规则选择器。
- 首次浅色axe检查发现侧栏分组标题`#a3a3a3`在`#fafafa`上只有2.41:1；改为`#737373`语义弱前景色。
- Gallery移动详情随后发现旧深色`#b7afac`文字在白底只有2.15:1；描述、日期和facts已迁移到语义前景色。
- 两项对比度修复均由最终离线E2E重新验证，不通过例外名单绕过。

```text
pnpm run check
PASS

pnpm run test
15 files / 32 tests PASS

pnpm run build
667 modules transformed; production build PASS
main JS 471.82 KiB; no large-chunk warning

playwright test e2e/offline-lifecycle.spec.ts
1 passed
```

最终Playwright覆盖Setup→导入→审核→激活→Browse→Manifest→完整备份恢复→路径映射→显式恢复→重扫，并额外验证移动Drawer打开、axe A/AA、Escape关闭和焦点恢复；1440桌面Browse与390移动Gallery基准截图已更新。

### 下一项

- 进入UI-03 Gallery卡片与索引：把媒体数量叠加到封面右下，重排Character/Work/Coser信息层级，保持R-18、Favorite、Rating和Scrubber语义。
- 在UI-03实际使用时补齐CardMedia、Avatar和Skeleton组件，并增加单/多Coser、单/多Character、Album、长标题、无封面和零媒体类型视觉夹具。

## 1.5-07 UI-03～UI-08 GalleryEpic视觉改造完成

### UI-03 Gallery卡片与索引

- Gallery卡片统一为3:4弱阴影封面、封面右下媒体计数、左下收藏操作和紧凑三层正文。
- Cosplay第一行优先Character、第二行Work、第三行显示Coser头像组与名称；Album第一行保持Gallery标题、第二行显示Album。
- P/S/G/V继续按PHOTO、SELFIE、GIF、VIDEO固定顺序输出，零值类型不渲染；全零时不生成空徽标。
- R-18、评分、Favorite Mutation和仅桌面精确指针启用的Scrubber语义保持不变。
- `CardMedia`、`Avatar`和`Skeleton`已进入共用组件层，卡片展示规则具有独立单元测试。

### UI-04 Gallery详情、媒体网格和Lightbox

- 详情增加Breadcrumb，原大封面Hero收敛为标题旁的小型3:4缩略图；标题、日期、关系、描述、Tag和facts改为紧凑无衬线信息区。
- 桌面主体采用媒体内容+Related右侧栏；窄屏Related回落到内容之后，不引入热门、下载、浏览量或评论语义。
- 媒体网格改为桌面4列、窄桌面3列、移动2列，继续保留Photo/Selfie、GIF、Video分组和每组24项手工展开。
- Lightbox改用本地SVG关闭/前后图标，打开后锁定背景并聚焦关闭按钮，支持Escape和方向键；到达首尾时对应按钮禁用且不循环。

### UI-05 实体、搜索与个人页面

- Coser、Work、Character和Tag索引改为浅色高密度头像/文字行；Coser详情保留已经确认的头像、Banner、Biography和社交账号能力。
- Search改为双列结果区，Timeline继续复用统一GalleryCard，Random、Favorites、History及媒体详情统一浅色Header、Tab、Toolbar和Sidebar节奏。
- 原LIST/MAGIC/ALL范围、URL Query、分页、关系和个人Favorite/Rating行为均未改变。

### UI-06 Manage与共用Dialog

- ManageShell使用与Browse一致的白色/浅灰Token、原创CGM标志和`Manage`上下文Badge；既有管理URL、五页签、表格密度、revision和Mutation保持不变。
- Gallery、核心实体、Libraries/Recognition Rules、Tasks、Settings和Operations统一浅色Panel、表格、表单、状态和危险色；技术ID、路径和审计仍只在Manage显示。
- 新增共用Dialog焦点管理：打开时锁定背景并把焦点送入Dialog，Tab保持在Dialog内，Escape/遮罩可安全取消，关闭后恢复原触发点。
- 数据库恢复、Gallery永久删除、核心实体合并/删除和未引用Coser托管资源清理已迁移到共用Dialog；Mutation执行期间禁止通过Escape或遮罩关闭，原确认词、密码和后端校验不变。

### UI-07 系统页面

- Setup、Login、Legal、Maintenance、Session loading和错误/空状态迁移到同一浅色Token、无衬线标题、轻边框卡片和明确焦点样式。
- Setup步骤、单密码登录、AGPL/Stash归属、恢复路径映射和Maintenance门禁不变；没有增加远程字体、图标或运行时资产请求。

### UI-08 回归收敛

- Playwright桌面Browse与390px Gallery详情基线更新为CGM自有浅色页面；新增新版媒体计数DOM断言，避免浏览器误连旧嵌入产物时产生假通过。
- 视觉差异容差从1%收紧到0.2%；原阈值曾允许局部卡片结构变化不更新截图，已强制刷新基线并在不更新模式下再次通过。
- 离线E2E继续覆盖Setup→导入→审核→激活→Browse→Manifest→完整备份恢复→路径映射→显式恢复→重扫，并执行axe A/AA、移动Drawer Escape和焦点恢复。
- 恢复期间短暂出现的`WEB_ROUTE 503`是既有Maintenance数据库交换窗口，随后工作器恢复且流程成功，不是本轮视觉改造回归。

### 最终阶段验证

```text
pnpm run check
PASS

pnpm run test
16 files / 37 tests PASS

pnpm run build
669 modules transformed; production build PASS
main JS 472.07 KiB; no large-chunk warning

playwright test e2e/offline-lifecycle.spec.ts
1 passed
```

自动化门禁完成不替代真实Firefox/WebKit、真实移动设备、读屏和200%缩放人工验收；这些仍按实施状态文档列为跨环境验收项，而不是把未执行结果标记为通过。

## 1.5-08 Gallery卡片Coser详情入口

### 实现

- Gallery卡片的Coser头像与名称改为独立可聚焦链接，鼠标点击或键盘操作均进入对应Coser详情页。
- 多Coser卡片为每个已返回Coser提供各自的详情入口；同一UUID重复关系只显示一次，不再因显示名称相同而错误合并两个不同Coser。
- 链接复用后端既有的UUID详情入口和规范slug重定向，不新增GraphQL字段、数据库查询或N+1请求；Gallery详情页原有Coser链接使用相同机制。
- 无Credit的Album保留空白布局占位，但不会生成空链接；Favorite、Rating、Scrubber和Gallery标题链接行为不变。

### 阶段验证

```text
pnpm exec vitest run \
  src/browse/GalleryCard.test.tsx \
  src/browse/galleryCardPresentation.test.ts \
  src/browse/galleryMediaCount.test.ts --maxWorkers=1
3 files / 8 tests PASS

pnpm run check
PASS

pnpm run test
17 files / 39 tests PASS

pnpm run build
669 modules transformed; production build PASS
main JS 472.07 KiB; no large-chunk warning

playwright test e2e/offline-lifecycle.spec.ts
1 passed
```

离线E2E新增“Browse卡片Coser链接→Coser详情标题→规范详情URL”真实点击验证；既有Setup、导入、激活、Manifest、备份恢复、路径映射、axe和视觉回归继续通过。

## 1.5-09 Coser详情GalleryEpic高保真复核与数字分页

### 参考冻结

- 按用户指定的`https://galleryepic.com/zh/coser/298/1`重新抓取1440×1000桌面、390×844移动和页面底部分页视图。
- 广告和广告造成的留白不纳入实现；参考截图只用于测量，不复制参考站Logo、媒体、代码或远程资产。
- 精确基线已同步到`GALLERYEPIC_FRONTEND_GAP_ANALYSIS_AND_PLAN_2026-07-30.md`：
  - 4:1 Banner；
  - 桌面128px、移动80px方形头像叠层；
  - 20/18px名称与紧凑社交标记；
  - 36px高、8px圆角控件；
  - 36px居中数字分页。

### 实现

- Coser详情增加`Home › Cosers › 当前Coser`Breadcrumb，页面顶部内容间距按参考页收敛到12px。
- Banner固定为4:1；没有托管Banner时显示中性本地占位，仍维持同一首屏几何且不产生网络请求。
- 资料区改为方形头像覆盖Banner下沿：
  - 桌面头像128px、8px白色内边距；
  - 移动头像80px、4px内边距，名称叠在Banner下沿；
  - Country继续保留为弱文字。
- 社交账号由大文字卡片收敛为20～24px紧凑平台标记；每个链接仍有可访问名称、Tooltip、ACTIVE/INACTIVE透明度和自定义Platform key缩写回退。
- Profile与Biography继续保留，使用轻边框可折叠内容，不改变元数据模型。
- LIST/MAGIC/ALL Scope与Shoot timeline保持原功能，只采用参考站筛选控件的轻边框几何；没有伪造作品/角色筛选或下载功能。
- Coser详情中的Gallery卡片隐藏重复Coser行，保留3:4封面、媒体计数、Favorite和既有CGM `6/5/4/3/2`列约束。
- `Pagination`升级为居中的Chevron+数字分页：
  - 最多连续显示5个页码；
  - 当前页为浅边框圆角按钮；
  - 点击后更新`scope/page`URL并向本机GraphQL读取对应页；
  - 不改变24项页大小、GraphQL DTO或数据库。
- 新增Coser详情桌面/移动视觉基线；离线E2E增加Coser页axe A/AA、桌面与390px截图。

### 阶段验证

```text
pnpm exec vitest run \
  src/ui/Patterns.test.tsx \
  src/browse/EntityDetailPages.test.tsx \
  src/browse/GalleryCard.test.tsx
3 files / 8 tests PASS

pnpm run check
PASS

pnpm run test
18 files / 41 tests PASS

pnpm run build
669 modules transformed; production build PASS
main JS 472.08 KiB; no large-chunk warning

playwright test --update-snapshots
1 passed; offline lifecycle, axe A/AA and Coser desktop/mobile baselines PASS
```

恢复阶段的短暂`WEB_ROUTE 503`仍为既有数据库交换窗口；随后工作器恢复、路径映射与来源重扫成功。本阶段没有后端、Schema、数据库或媒体文件变更。

### 增量部署

- 2026-07-31 00:29 CST将本阶段嵌入式Linux amd64最终构建安装到`~/.local/bin/cgm`，SHA-256为`1aac8277d217cac3a5c5d7d0ab41970937a7802b26ee28f2555c069d17b5e2b0`。
- 替换前二进制备份为`/tmp/cgm-before-coser-detail-20260731`，可用于本机回退。
- 最终CSS清理前的同功能构建另存于`/tmp/cgm-before-pagination-final-css-20260731`，SHA-256为`05d55f8e0415a3da41a2cc1e5d29de1dea79cb9e994c4418a8fa96fb60dfd58c`。
- `/about.json`正确报告`version=1.5.0-dev`、`gitHash=local`、`buildTime=2026-07-31`和`exactSourceAvailable=false`，没有把尚未提交的工作树伪装成精确发行源码。
- 用户服务保持`enabled/active`，`/healthz`与`/readyz`均为204；配置SHA-256与产品数据库inode保持不变。

## 1.5-10 缓存占用与来源访问审计、管理页只读状态

### 实测与结论

- 本机108个来源媒体逻辑大小337.75 MiB；CGM缓存216个文件逻辑大小149.02 MiB，约为来源的44.1%。
- `CARD_480`仅3.56 MiB（2.39%缓存占比）；`LIGHTBOX_4096`为145.46 MiB（97.61%），确认空间放大的根因不是卡片缩略图，而是每张静态图的扫描时4096大图。
- 当前两种Variant均为BASE；增强缓存50 GiB阈值和LRU不会回收Lightbox。按当前样本线性估算，10,000张静态图约产生13.48 GiB缓存。
- 仓库中的原Stash实现只常驻最长边640 JPEG缩略图，可关闭落盘、首次请求生成、小图跳过生成并按Checksum复用；Lightbox直接读取原图，不预存每图4096大图。
- CGM日常Browse只读取产品数据库、浏览器缓存或本机CGM派生缓存；缓存缺失返回404，不回退读取来源。显式Gallery重扫会完整读取并哈希来源，随后每个独立派生任务再次打开来源。
- 完整分析与后续优化候选记录在[缓存占用与来源访问分析](CACHE_STORAGE_AND_SOURCE_ACCESS_ANALYSIS_2026-07-31.md)。本轮不修改已确认的BASE/ENHANCED策略。

### 管理页实现

- Manage → Settings增加只读`Generated cache`区，显示实际绝对缓存路径、普通文件逻辑总大小和文件数。
- 缓存根仍来自启动级`cache_path`；页面没有路径输入、迁移、清理或重建操作，也不修改启动配置。
- GraphQL只暴露已认证Manage查询`manageCacheStorage`；实际文件系统统计由Server运维服务执行，Resolver不获得直接来源访问能力。
- 统计遍历不跟随符号链接，只计普通文件并响应请求Context取消；仅在Settings查询时执行，不进行后台轮询。

### 阶段验证

```text
go test ./internal/productapi ./internal/productserver ./internal/mediaresource \
  ./internal/mediaprocessing ./internal/processingworker \
  ./internal/persistence/productdb ./internal/sourcescan
7 packages PASS

pnpm run check
PASS

pnpm run test
19 files / 43 tests PASS

pnpm run build
669 modules transformed; production build PASS
main JS 472.08 KiB; no large-chunk warning
```

新增回归验证GraphQL运维边界、缓存普通文件计数/字节统计、符号链接不跟随，以及Settings中
路径与大小只读显示。本阶段没有执行位置修改、缓存删除、重建或来源媒体写入。

### 增量部署

- 2026-07-31 00:58 CST将本阶段嵌入式Linux amd64构建安装到`~/.local/bin/cgm`，SHA-256为
  `c1503fe9653617789e0e522a2dd122aaf4c5b146cf03392160275076e67d1007`。
- 替换前二进制备份为`/tmp/cgm-before-cache-status-20260731`，SHA-256为
  `1aac8277d217cac3a5c5d7d0ab41970937a7802b26ee28f2555c069d17b5e2b0`。
- 用户服务保持`enabled/active`，`/healthz`与`/readyz`均为204；入口引用
  `index-CPpPzbO_.js`、`index-DZkfDa2M.css`和`ManageSettingsPage-D4q1vbqz.js`，Settings
  Chunk已核对包含`manageCacheStorage`、`Generated cache`和`Occupied space`。
- `/about.json`报告`1.5.0-dev`、`gitHash=local`、`exactSourceAvailable=false`；配置SHA-256
  仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，产品数据库
  inode仍为`19679716`，没有替换数据库、配置、媒体库、缓存或Manifest。

## 1.5-11 CARD BASE、按需Lightbox与可回收缓存上限

### 已确认策略与实现

- `CARD_480`和`STATIC_POSTER`继续属于不可被LRU删除的BASE；`LIGHTBOX_4096`改为ENHANCED。
- 扫描静态图片（包括RAW）只排队`CARD_480`。首次打开Lightbox或静态媒体详情时，已认证且
  Browse可见的单Item请求使用稳定任务键幂等排队4096代理；前端立即显示480图，以750ms聚焦
  状态查询轮询，生成完成后切换大图，失败时保留480图并显示非阻断提示。
- ENHANCED回收删除派生记录和应用生成文件，但不再立即重排任务，避免“刚回收又生成”的循环；
  下一次显式查看会重新打开已完成/失败/取消的任务。
- 成功打开资源（含304校验请求）会更新完整资源身份对应的`last_accessed_at_utc`。Scheduler启动时
  及每分钟读取运行时上限、数据库ENHANCED字节与文件系统可用空间，执行最多10,000条候选的LRU。
- Manage → Settings继续只读显示缓存位置，并增加BASE/可回收占用；可回收上限以GiB输入，仍持久化
  到既有`enhanced_cache_maximum_bytes`带revision设置。位置、手工清理和来源媒体不在页面权限内。

### 兼容迁移与安全边界

- 工作器启动前执行幂等数据迁移：已有`LIGHTBOX_4096`派生记录改为ENHANCED，相关任务Payload
  改为ENHANCED；文件本身不移动、不重编码、不立即删除。
- 按需接口只允许ACTIVE、来源/Item AVAILABLE、未超限、未隐藏、无未解决阻断Issue且未排除的
  静态图片；返回值只有处理状态与不透明资源身份，不返回来源路径或缓存路径。
- 回收候选仍由数据库严格限定ENHANCED；BASE与媒体来源不属于删除能力。默认50 GiB上限下，当前
  145.46 MiB已有Lightbox不会仅因本次部署被清除。

### 阶段验证

```text
go test ./internal/mediaprocessing ./internal/persistence/productdb \
  ./internal/processingworker ./internal/mediaresource ./internal/productapi ./internal/productserver
6 packages PASS

pnpm run test
20 files / 46 tests PASS

pnpm run check
PASS

pnpm run build
670 modules transformed; production build PASS
main JS 472.13 KiB; no large-chunk warning

playwright test
1 passed; offline lifecycle, backup/restore, axe A/AA and visual baselines PASS
```

新增回归覆盖扫描只排BASE、RAW主代理、Lightbox按需任务幂等/终态重开、既有层级迁移、回收不
立即重建、分层容量统计、GiB换算和前端按需Hook。`go test ./...`除CGM无关的旧Stash
`ui/v2.5/build`未生成setup failure及沙箱禁止`httptest`监听IPv6端口外，其余已运行包通过；上列
六个产品相关包与正式`build-cgm`链全部通过。

### 增量部署与本机迁移核验

- 2026-07-31 23:54 CST将嵌入式Linux amd64构建原子安装到`~/.local/bin/cgm`，SHA-256为
  `72e10e6c53b77c5bc942ae182acd41d6238e4d9f1ec8efd39f0e003b3953386b`。
- 替换前二进制备份为`/tmp/cgm-before-on-demand-lightbox-20260731`，SHA-256为
  `c1503fe9653617789e0e522a2dd122aaf4c5b146cf03392160275076e67d1007`。
- 用户服务为`active`，`/healthz`与`/readyz`均为204；启动日志确认2个工作器正常启动，无迁移或
  缓存维护错误。
- 迁移后只读数据库核验：108条`CARD_480`仍为BASE（3,733,456 bytes）；108条
  `LIGHTBOX_4096`均为ENHANCED（152,529,881 bytes），108条相关任务Payload也全部为ENHANCED。
- 缓存物理文件仍为216个、156,263,337 bytes，说明部署没有立即清理或重建；配置SHA-256仍为
  `1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，数据库inode仍为
  `19679716`。本阶段没有修改媒体来源、Manifest或缓存位置。

## 1.5-12 Cosplay/Album双分区与GalleryEpic实体页面层级

### 需求确认与边界

- Browse侧栏主结构调整为Cosplay（Lists/Cosers/Parodies/Magic）与Album（Lists/Models）。
- Cosplay Lists只显示`COSPLAY + NON_ADULT`，Magic只显示`COSPLAY + ADULT`；Album Lists与Models按用户确认展示全部分级ALBUM。
- Model不进入核心实体模型。GalleryCredit继续统一引用Coser，Gallery仍以Cast非空/空实时推导COSPLAY/ALBUM；同一UUID可以在两个人物索引和详情视图中出现。
- 保持既定Gallery/Coser/Work-Character分页24/30/60、Gallery网格6/5/4/3/2、无远程字体/CDN和原创本地图标约束。

### 查询与路由实现

- Browse Gallery、Coser索引和Coser详情增加可选`collectionType`参数；旧客户端不传时保持原有混合查询行为。
- 实体索引增加最多300字符的名称、Sort name和Alias包含搜索；筛选在数据库分页前完成，不使用只过滤当前页的前端假搜索。
- 新增`/albums`、`/models`、`/model/:slug`；Model详情复用Coser DTO和规范Slug重定向，但Gallery查询固定为`ALL + ALBUM`。
- Cosers固定查询`ALL + COSPLAY`，Models固定查询`ALL + ALBUM`；Work和Character仍只能由Cast关系进入Cosplay层级。
- Gallery卡片按派生类型生成Coser或Model人物链接。Characters不再位于主侧栏，旧路由仍可直接访问；Timeline、Random、Favorites、History、Tags和管理入口保留在次级区域。

### 视觉实现

- Coser/Model索引使用Breadcrumb、居中搜索、四列36px圆形头像文字行和数字分页。
- Parodies/作品来源使用四列高密度文字索引；Work详情先显示Character，Character详情再显示Gallery网格。
- Coser详情保留4:1 Banner、资料、LIST/MAGIC/ALL和Timeline；Model详情不伪造独立资料，按参考页直接显示Breadcrumb与Album网格。
- 新增原创Palette和Lock线性SVG，调整侧栏标题、字号、行距和白色表面；没有复制GalleryEpic Logo、媒体、代码或远程资产。

### 阶段验证

```text
go test ./internal/persistence/productdb ./internal/productapi
2 packages PASS

pnpm run check
PASS

pnpm run test
21 files / 49 tests PASS

pnpm run build
670 modules transformed; production build PASS
main JS 473.18 KiB; no large-chunk warning

playwright test e2e/offline-lifecycle.spec.ts
1 passed; Album/Cosplay隔离、Models归属、Model/Coser桌面与移动基线、axe A/AA、备份恢复和重扫PASS
```

E2E的ALBUM样本明确验证：不出现在`/list`或`/magic`，不出现在`/cosers`，会出现在`/albums`与`/models`，卡片人物入口进入`/model/:slug`。恢复期间的短暂`WEB_ROUTE 503`仍是既有数据库交换窗口，随后工作器恢复并完成路径映射和来源重扫。

### 增量部署

- 2026-08-01 01:54 CST将Cosplay/Album双分区构建安装到`~/.local/bin/cgm`，SHA-256为`c1ffa2edc8f9086b8e6e160c1bd17c325dbe4db9b10dfb5b4be68ac482ef6a0b`。
- 替换前二进制备份为`/tmp/cgm-before-cosplay-album-20260801`，SHA-256为`72e10e6c53b77c5bc942ae182acd41d6238e4d9f1ec8efd39f0e003b3953386b`。
- 用户服务为`active`，`/healthz`与`/readyz`均为204；`/about.json`报告`version=1.5.0-dev`、`gitHash=local`、`buildTime=2026-08-01`和`exactSourceAvailable=false`。
- 嵌入入口引用`index-DXmm-gQE.js`与`index-WDjoyUJX.css`；导航、Gallery索引、实体索引、实体详情和Gallery卡片Chunk均来自本轮670模块构建。
- 配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，数据库inode仍为`19679716`。重启时SQLite将既有WAL正常检查点到主文件，未替换数据库，也未修改媒体来源、Manifest、缓存位置或启动配置。

## 1.5-13 Coser/Model索引头像与主名称垂直居中

### 问题与修复边界

- Cosplay的Coser索引和Album的Model索引共用人物索引组件。原样式让36px头像跨越两条隐式Grid行；即使人物没有显示Alias，主名称仍位于第一行，因此名称中心高于头像中心。
- 按本轮确认，人物索引只展示主名称，不考虑Alias展示；名称、Sort name和Alias的后端搜索能力不变，Work、Character等纯文字索引的Alias展示也不受影响。
- 人物索引显式固定为36px单行Grid，头像只占第一行，主名称使用同一行的`align-items: center`和`align-self: center`完成垂直居中。

### 阶段验证

```text
pnpm run test
21 files / 49 tests PASS

pnpm run check
PASS

pnpm run build
670 modules transformed; production build PASS
main JS 473.18 KiB; no large-chunk warning

playwright test e2e/offline-lifecycle.spec.ts
1 passed; Coser/Model人物行头像与主名称中心点误差小于1px，完整离线生命周期PASS
```

组件测试额外使用带Alias的人物夹具确认列表不渲染Alias；E2E使用浏览器实际布局盒验证头像与主名称中心线，不只检查CSS类名。

### 增量部署

- 2026-08-01 02:26 CST将人物索引对齐修复安装到`~/.local/bin/cgm`，SHA-256为`53bfdbc288f8e9aecd7f89167fab877234b3edc53e88f5c878e58ad4dfe3f041`。
- 替换前二进制备份为`/tmp/cgm-before-person-index-alignment-20260801`，SHA-256为`c1ffa2edc8f9086b8e6e160c1bd17c325dbe4db9b10dfb5b4be68ac482ef6a0b`。
- 用户服务保持`enabled/active`，`/healthz`与`/readyz`均为204；入口引用`index-B-wD7SgW.js`、`index-pRt2Qbb1.css`和人物索引块`EntityIndexPage-ikuTtgbU.js`。
- 配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，产品数据库inode仍为`19679716`；没有替换数据库，也没有修改媒体来源、Manifest或缓存。

## 1.5-14 Coser/Model人物名称字型对齐

### 参考证据与根因

- 2026-08-01重新读取用户指定的GalleryEpic Coser公开列表页。人物名称节点明确使用`text-sm leading-none font-medium`，其公开CSS计算值为`14px / 14px / 500`；36px头像与名称之间使用8px左间距。
- CGM人物名称此前先继承通用实体索引的`18px / 1.35 / 550`，再仅把字号覆盖为16px，因此最终计算值是`16px / 21.6px / 550`。字号、字重和行高都大于参考站，视觉上明显偏大、偏厚。

### 修复与验证

- Coser/Model人物索引专属样式改为`14px / 14px / 500`，头像与名称间距改为8px；Work、Character等通用实体文字索引不变。
- Playwright直接读取浏览器`getComputedStyle`并严格断言上述三个值，同时继续约束头像与名称中心点误差小于1px。

```text
pnpm run test
21 files / 49 tests PASS

pnpm run check
PASS

pnpm run build
670 modules transformed; production build PASS

playwright test e2e/offline-lifecycle.spec.ts
1 passed; computed font 14px / 14px / 500 and vertical alignment PASS
```

### 增量部署

- 2026-08-01 02:39 CST将人物名称字型对齐修复安装到`~/.local/bin/cgm`，SHA-256为`5bbf3557bd3e56ed0eb3839797561f70336db2ebde4258e58ff457020d80ec55`。
- 替换前二进制备份为`/tmp/cgm-before-person-name-typography-20260801`，SHA-256为`53bfdbc288f8e9aecd7f89167fab877234b3edc53e88f5c878e58ad4dfe3f041`。
- 用户服务保持`enabled/active`，`/healthz`与`/readyz`均为204；入口引用`index-Dagulhjn.js`与`index-DUKMwGwd.css`。
- 配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，产品数据库inode仍为`19679716`；未替换数据库或修改媒体、Manifest与缓存。

## 1.5-15 项目README产品化改写

- 删除原Stash README中的上游构建徽章、下载入口、StashDB、网络Scraper、插件、社区、Windows/macOS安装和旧产品宣传，避免把CGM不存在或已延期的能力误写成当前功能。
- README改为CGM中文项目入口，覆盖Gallery优先定位、候选/DRAFT/扫描/激活闭环、核心实体、Cosplay/Album双分区、媒体派生缓存、Manifest、备份恢复、单所有者安全模型和明确排除能力。
- 增加真实平台状态矩阵，区分Linux amd64已部署验证、Linux arm64仅构建通过、Docker amd64已运行验证、Docker arm64待runner和Windows/macOS原生延期。
- 增加可执行的源码构建、原生配置、首次Setup、Docker Compose、数据安全、测试入口和文档导航；明确不能把CGM指向原Stash数据库，不能并发运行两个实例，完整备份也不包含媒体或Gallery sidecar。
- 使用现有离线E2E Browse基线作为项目截图，不新增远程品牌或媒体资产；保留Stash衍生归属、GalleryEpic仅作视觉参考且无隶属关系的说明，以及AGPL、第三方通知和SPDX链接。

### 文档验证

```text
README local-link validation
All local README links exist

git diff --check -- README.md
PASS
```

本阶段只修改仓库文档，不改变应用二进制、运行服务、配置、数据库、媒体、Manifest或缓存，因此无需执行增量部署。

## 1.5-16 Gallery详情页高保真与单媒体操作

### 已冻结范围

- 2026-08-01用户确认采用GalleryEpic作品集详情页的紧凑标题、详情媒体2/3/4/6列和256px相关推荐栏布局。
- 混合媒体继续按Photo/Selfie、GIF、Video既定顺序依次展示并按组每次展开24项，但所有媒体分类标题均隐藏。
- 本阶段补齐V1原定的单媒体收藏、静态图片设封面与Lightbox URL/返回/定位行为；不增加浏览数、下载、评论或热门能力，不修改媒体、实体和Gallery数据模型。

### 实施状态

- 已完成：移除独立封面Hero，将标题、非零媒体计数、拍摄/收录日期、Gallery收藏和“更多详情”收敛到紧凑页头；详细元数据继续可访问，但不再挤占首屏媒体区域。
- 已完成：媒体区按Photo/Selfie、GIF、Video后端顺序连续排布，视觉上使用统一2/3/4/6列网格和12px间距，不显示媒体分类标题；各后端分组仍独立保留每次24项的增量展开边界。
- 已完成：相关推荐改为256px右栏的紧凑缩略图行；窄屏自动回落到主内容下方，不改变既有推荐查询和分级口径。
- 已完成：媒体卡片补齐收藏/取消收藏、静态图片设为封面、当前封面标识和媒体详情入口；封面Mutation继续携带Item的`metadata_revision`并在成功后重新读取Gallery与成员数据。
- 已完成：Lightbox由`?item=<uuid>`驱动，支持浏览器前进/后退、当前筛选范围内首尾不循环导航、关闭后滚回原媒体、记录`last_item_id`，并提供单媒体收藏、半星评分、静态图片设封面和媒体详情入口。
- 已完成：新增Gallery详情组件测试与桌面视觉基线；离线E2E实际执行单媒体收藏、封面切换、Lightbox深链打开/关闭和关闭后可视区定位。

### 验证结果

```text
go test ./internal/persistence/productdb ./internal/productapi
PASS（2个包）

pnpm run check
PASS

pnpm run test
PASS（22个文件，54项测试）

pnpm run build
PASS（670个模块；主JS 474.24 KiB；无大分包警告）

pnpm exec playwright test e2e/offline-lifecycle.spec.ts
PASS（1项；实际Mutation、URL/返回、定位、axe与视觉基线均通过）

git diff --check
PASS
```

### 增量部署

- 2026-08-01 03:57 CST将包含本阶段改动的嵌入式Linux amd64开发构建安装到`~/.local/bin/cgm`，SHA-256为`66b235a1f93babc839fd3245c9c8305ff578c1f6b9ba02b1b75128504e49901f`。
- 替换前二进制备份为`/tmp/cgm-before-gallery-detail-20260801`，SHA-256为`5bbf3557bd3e56ed0eb3839797561f70336db2ebde4258e58ff457020d80ec55`。
- 服务重启后保持`enabled/active`，Health/Ready均为204；入口HTML引用`index-BB00ESCj.js`与`index-B5AaBjOr.css`。
- 配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，产品数据库inode仍为`19679716`；没有替换配置、数据库、媒体、Manifest或缓存。

## 1.5-25 可拔除Coser网络资料导入

### 架构决策

- 将网络资料导入拆成站点无关契约、产品导入编排和GalleryEpic站点适配器三层。核心数据库、资产、Manifest和产品Server不包含GalleryEpic域名、路由、DOM选择器或远端ID语义。
- `cmd/cgm`只在`cgm_galleryepic`构建标签存在时注册GalleryEpic；默认核心构建完全不导入站点包。正式CGM构建包含适配器，但启动配置`metadata_scraping_enabled`默认`false`。
- 无Provider时Provider集合为空，Manage资料页自动隐藏导入面板；普通Coser编辑、社交账号、托管资源、Manifest和Browse保持完整可用。删除适配器不需要数据库迁移。
- 架构和拆除步骤持久化到`docs/architecture/COSER_METADATA_PROVIDER_BOUNDARY.md`；Make增加依赖图门禁，验证默认构建不含适配器且显式标签构建才包含。

### 实现

- Manage → Cosers → Profile新增“导入网络Coser资料”：默认用当前名称搜索候选人，用户必须选择具体候选、预览头像/Banner/账号并逐项勾选后才能应用。
- 已存在的相同URL账号显示为已保存且不能重复选择；已有头像/Banner默认不覆盖，必须额外勾选替换确认。不会自动修改Coser名称、Alias、简介或其他核心资料。
- GalleryEpic适配器兼容`.com/.xyz`页面域名和当前Next.js流式HTML：以`/coser/<id>/<page>`识别候选，以资料头部数字alt图片顺序提取Banner/头像，并从主HTML及隐藏流式模板中收集受支持社交平台。
- 账号按URL Host映射为Twitter/X、Instagram、Weibo、Patreon、Facebook、YouTube、Pixiv、Bilibili、TikTok或Website/Linktree；广告、下载及未知域名不进入建议。
- 出站客户端不使用系统代理；仅允许固定HTTPS页面/图片域名，限制5次重定向、15秒请求、30秒客户端操作、4MiB HTML和20MiB图片，检查响应为图片并以实际字节探测JPEG/PNG/WebP（GalleryEpic CDN当前会把JPEG标为WebP），检查最终重定向资源域名，并拒绝环回、私网、链路本地、组播和未指定地址。
- 预览使用随机不透明令牌，仅保存在进程内15分钟，最多8份/96MiB；前端只从认证同源端点显示短期图片，不直接热链接第三方资源。
- 应用时先通过现有Coser资产规则校验全部选中图片并生成托管派生资源，再在一个SQLite事务中发布头像、Banner和SocialAccount。Coser metadata revision只递增一次，既有Coser Manifest标记`DB_DIRTY`；事务失败最多留下可集中审阅的未引用应用资源，不产生半套数据库资料。
- 日志/审计只记录Provider key、成功/失败、头像/Banner布尔值和账号数量，不记录搜索词、Coser名称或完整远端URL。启动、Browse、Gallery扫描、自动计划和社交账号目标网站检查均不触发外联。

### 阶段验证

```text
go test ./internal/cosermetadata/... ./internal/coserasset \
  ./internal/persistence/productdb ./internal/productserver ./cmd/cgm
PASS

go build -o /tmp/cgm-without-providers ./cmd/cgm
PASS（默认核心依赖图不含GalleryEpic）

go build -tags cgm_galleryepic -o /tmp/cgm-with-galleryepic ./cmd/cgm
PASS（显式标签组合GalleryEpic）

make verify-cgm-metadata-provider-boundary
PASS

pnpm run check
PASS

pnpm exec vitest run src/manage/ManageCoserMetadataImport.test.tsx \
  src/manage/ManageCoserAssetsPanel.test.tsx \
  src/manage/ManageCoreEntitiesPage.test.tsx --maxWorkers=1
PASS（3个文件，5项测试）
```

- 后端测试覆盖无Provider/配置关闭、候选预览、托管头像与账号应用、单事务单revision、失败不改变聚合、预览过期、内存上限基础行为、当前HTML形态、隐藏流式账号、未知社交域过滤和私网IP拒绝。
- 前端测试覆盖无Provider自动隐藏、候选人工选择、已存在账号去重和仅提交勾选项。
- 使用`CGM_LIVE_GALLERYEPIC_TEST=1`显式执行受控在线兼容性测试，已知公开Coser `298`的名称搜索、详情、头像、Banner及不少于2个社交账号全部通过。该测试默认Skip，CI和离线发布门禁不依赖第三方站点。
- 在线校核发现GalleryEpic CDN当前对已知头像返回`Content-Type: image/webp`但实际字节是JPEG；实现保留图片响应要求，并以实际字节安全探测JPEG/PNG/WebP后再交给现有资产解码管线，不盲信错误标头。
- 最终产品包定向回归（archivecheck、browse、资产、Provider、discovery、gallery、manifest、媒体、数据库、API、认证、日志、Server、扫描、cmd及新UI）全部通过；带`cgm_web_embed cgm_galleryepic`的正式组合标签回归通过，UI/Provider两项依赖边界门禁和`git diff --check`通过。
- 前端最终全量结果为24个Vitest文件、61项测试通过；TypeScript及Vite生产构建通过，共转换672个模块，主JS 476.59 KiB，无大包警告。
- 产品默认升级仍保持`metadata_scraping_enabled=false`和零外联；本机实际业务测试部署已按所有者本次明确要求单独启用，详见下方部署记录。
- 启动日志只有正常的`CGM_SERVICE_STOPPED`、`CGM_WORKERS_STARTED`与`CGM_SERVICE_STARTED`事件，没有迁移或运行错误。

### 增量部署与实装验证

- 2026-08-12 23:37 CST完成包含`cgm_web_embed cgm_galleryepic`组合标签的本机Linux amd64增量部署。新二进制安装到`/home/rainbowrunner/.local/bin/cgm`，SHA-256为`c1ae56fd42f79935d5e5b8ed6916af0a926374f61eb38bb11e0f8d0d846d4d6e`；`go tool nm`确认产物包含GalleryEpic Provider的`Search`、`FetchProfile`与`OpenAsset`实现。
- 替换前二进制备份为`/tmp/cgm-before-galleryepic-scraper-20260812`，SHA-256为`9e9477e36d1572150a7fe0502d7981e3fcd0dcff533168254e34baf06a43bf35`。替换前配置备份为`/tmp/cgm-config-before-galleryepic-scraper-20260812.json`，SHA-256为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`。
- 本机启动配置已显式加入`"metadata_scraping_enabled": true`并继续保持`0600`，新配置SHA-256为`ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`。该值只启用认证Manage页面中的人工操作入口，不产生启动、扫描、Browse或计划任务外联。
- 用户服务重启后保持`enabled/active`；Health/Ready均为204，About为200并报告`version=1.5.0-dev`、`gitHash=local`与`buildTime=20260812`。入口实际引用本轮`index-C2e4nvxI.js`和`index-BaLfZSme.css`；未认证访问Provider端点返回401，确认认证边界未被放宽。
- 重启journal只出现正常停止、两个工作器启动与服务启动事件，没有配置解析、数据库迁移、Provider初始化或任务错误。产品数据库SHA-256仍为`797b2aa899ac7cd85e1e05f3c4d90b8088b7afde4be26e0124da2c6ef5fd6828`，inode仍为`19679716`；没有替换数据库、媒体、Manifest或缓存。
- 部署后显式联网测试第一次在`static.galleryepic.xyz`发生瞬时TLS握手超时；随后直连资源返回HTTP 200，完整重试通过，确认已知公开Coser `298`的名称搜索、候选选择、头像、Banner及不少于2个关联账号可正常取得。第三方站点或网络异常仍会作为人工导入错误显示，不影响其他本地业务。

## 1.5-26 Browse人物头像链路与社交视觉对齐

### 问题求证

- 只读校核真实业务数据库和Coser托管目录确认，目标Coser已经保存头像/Banner，`avatar-480`、`banner-960`和`banner-1600`派生资源均存在；运行日志也确认Coser详情页对这些认证资源的请求返回200。因此问题不在网络资料导入、图片生成或资源服务。
- Coser/Model索引仓储已经计算`AvatarAvailable`和`AssetRevision`，但GraphQL `entityPage`转换手工重建条目时遗漏了`avatarURL`；列表因此只能渲染名称首字母占位。
- Gallery卡片的Credit原来使用只有UUID和名称的通用`EntitySummary`，前端也没有给`Avatar`传入`src`；该链路从契约层开始就不具备展示头像的能力。
- 参考站实际人物行使用32px头像/行高、6px图文间距、14px字号、14px行高和500字重。CGM原卡片采用约21.6px头像和更小文字，视觉密度明显偏小。Coser详情社交账号虽然布局已收紧，但仍是文字缩写，不是参考站的20～24px平台图标。

### 实现

- Browse领域新增专用、无物理路径的`PersonSummary`，Gallery卡片和Gallery Credit查询一次联表读取头像是否存在及Coser revision，不产生逐人物N+1查询。GraphQL据此只生成带revision的同源认证URL`/resource/coser/{uuid}/{revision}/avatar-480`，不会返回头像文件名、托管根目录或源站URL。
- `entityPage`统一复用既有`entityIndexItem`转换，恢复Coser/Model索引已有的头像URL；Gallery卡片将人物URL传给通用Avatar组件。无头像人物继续稳定回退为名称首字符。
- Gallery卡片人物行调整为32px头像和高度、6px图文间距、14px/14px、500字重名称；多人物仍按原有顺序展示并进入对应Coser/Model详情。字号被限定在人物链接内，不改变同一行评分摘要的既有11px视觉。
- Coser详情新增本地`SocialPlatformIcon`组件，覆盖Twitter/X、Facebook、Instagram、Weibo、Bilibili、YouTube、Pixiv、TikTok/Douyin、Bluesky、Patreon、FANBOX和Xiaohongshu；未知自定义Platform key继续显示通用网站图标。图标不复用参考站文件、不访问互联网，外链、ACTIVE/INACTIVE状态、`aria-label`、`title`和键盘焦点语义保持不变。

### 阶段验证

```text
go test ./internal/browse ./internal/coserasset ./internal/persistence/productdb \
  ./internal/productapi ./internal/productserver ./cmd/cgm ./ui/web -count=1
PASS

go test -tags "cgm_web_embed cgm_galleryepic" \
  ./internal/productserver ./cmd/cgm ./ui/web -count=1
PASS

pnpm run check
PASS

pnpm run test
PASS（24个文件，61项测试）

pnpm run build
PASS（673个模块；主JS 476.59 KiB）

pnpm exec playwright test e2e/offline-lifecycle.spec.ts
PASS（1项；32px人物行/头像、6px间距、14px/500名称计算样式及0.2%截图门禁）

make build-cgm
PASS（正式单文件验证产物：/tmp/cgm-avatar-social-check）
```

- SQLite及GraphQL回归验证Coser/Model索引和Gallery卡片均返回相同的认证头像URL，并显式拒绝物理`avatar_path`及媒体根路径泄露；前端组件回归覆盖两处头像`img src`和Coser详情SVG社交图标。

### 增量部署

- 2026-08-13 00:47 CST完成包含本阶段修复的本机Linux amd64增量部署。正式产物使用`cgm_web_embed cgm_galleryepic`组合标签构建并安装到`/home/rainbowrunner/.local/bin/cgm`，SHA-256为`91ab21dbb979544d8fb5beb195c6c690c4c25bb182edfaa932c60ae95002cd51`；`go tool nm`确认仍包含GalleryEpic Provider的`Search`、`FetchProfile`和`OpenAsset`。
- 替换前二进制备份为`/tmp/cgm-before-avatar-social-20260813`，SHA-256为`c1ae56fd42f79935d5e5b8ed6916af0a926374f61eb38bb11e0f8d0d846d4d6e`。新文件先写入同目录临时名称，再以`mv`原子替换，服务只重启一次。
- 用户服务保持`enabled/active`，新进程启动于00:47:02 CST；`/healthz`和`/readyz`均为204，`/session/status`、`/legal`及新JS/CSS资源均为200。入口实际引用`index-B65da5eI.js`和`index-DM9Wuy91.css`；`/about.json`报告`version=1.5.0-dev`、`gitHash=local`、`buildTime=20260813`和`exactSourceAvailable=false`。
- 启动journal确认2个工作器、LibRaw和服务正常启动，未发现WARN、ERROR、panic、fatal或迁移错误。配置SHA-256仍为`ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`且权限为0600，产品数据库inode仍为`19679716`；没有替换配置、数据库、媒体、Manifest或缓存。

### 社交图标二次视觉复核与部署

- 2026-08-13重新读取用户指定参考页`/zh/coser/26/1`的实时DOM、SVG与浏览器计算样式，确认首轮只对齐尺寸和间距，图标本体仍是手工近似轮廓。参考站实际固定显示X、Facebook、Instagram、微博、Patreon、Linktree六项：X为20px，其余24px，间距4px、行高24px；已有账号为`#101828`链接，缺失账号仍以`#99a1af`不可点击图标占位。
- 新增独立`SocialAccounts`展示组件：按固定六项匹配并消费已有账号，缺失项输出无链接的灰色`span`，剩余自定义平台继续按数据库顺序追加。`website + Linktree`以及URL Host为`linktr.ee`的现有记录显示精确Linktree图标，无需迁移或重写业务数据；INACTIVE已有账号仍可显示其已保存链接但采用灰色状态。
- X、Facebook、Instagram、微博、Patreon与Linktree改为参考页逐路径实心SVG；参考集合之外的既有平台图标和未知平台地球回退继续由本地组件提供，运行时不请求GalleryEpic静态资源。
- 定向组件回归验证固定顺序、四个缺失占位、Linktree历史映射、INACTIVE自定义平台追加和缺失项无链接；完整结果为24个Vitest文件、61项测试通过，TypeScript及674模块生产构建通过。离线Chromium更新Coser移动视觉基线后连续两次通过，计算样式精确锁定24px行高、4px间距、20/24px图标尺寸、六项标题顺序及`rgb(153, 161, 175)`缺失颜色，并继续通过axe A/AA和0.2%截图门禁。
- 2026-08-13 01:17 CST完成本机增量部署。正式`cgm_web_embed cgm_galleryepic`产物SHA-256为`c9143fe559281b826104f0f73252b57edc045be39818feba5d3cb5404387ce71`；替换前二进制备份为`/tmp/cgm-before-social-exact-20260813`，SHA-256为`91ab21dbb979544d8fb5beb195c6c690c4c25bb182edfaa932c60ae95002cd51`。
- 服务保持`enabled/active`，新进程启动于01:17:47 CST；Health/Ready均为204，入口引用`index-DCePnCFw.js`与`index-D_Nx6NG6.css`且资源返回200，About报告`buildTime=20260813`、`gitHash=local`和`exactSourceAvailable=false`。启动journal确认2个工作器与LibRaw正常，没有WARN、ERROR、panic、fatal或迁移错误；配置SHA-256和数据库inode保持不变，未修改配置、数据库、媒体、Manifest或缓存。

## 1.5-27 Coser详情头像直达管理编辑器

### 实现

- Coser详情资料区头像由纯展示容器改为React Router内部链接，目标为`/manage/cosers?uuid=<coser UUID>`；使用不可变UUID而不是名称或Slug，避免重名、改名和Slug历史影响管理定位。
- Manage Coser页原有`uuid`查询参数会直接以`network-only`读取目标实体，因此目标无需出现在后台当前30项分页中，点击头像后也无需再从大量Coser列表中人工翻找。
- 入口只应用于具有完整人物资料区的Coser详情；Model详情本来不渲染该头像，不改变其既有页面结构。后台仍受现有SessionBoundary保护，未认证访问不会绕过所有者登录。
- 链接保持头像原有尺寸、位置、图片回退和视觉结构，补充中英文`Manage/管理 {name}`可访问名称及`title`，并增加轻量悬浮边框和清晰的`focus-visible`键盘焦点反馈。

### 阶段验证

```text
pnpm run check
PASS

pnpm exec vitest run src/browse/EntityDetailPages.test.tsx \
  src/manage/ManageCoreEntitiesPage.test.tsx
PASS（2个文件，4项测试）

pnpm test
PASS（24个文件，61项测试）

pnpm run build
PASS（674个模块；生产构建完成）

pnpm run e2e
PASS（1项离线所有者完整业务链路）
```

- 组件回归锁定头像链接的可访问入口及精确UUID目标；浏览器回归实际点击Coser头像，验证进入`/manage/cosers?uuid=...`后显示Coser管理页并加载目标人物名称，同时继续通过既有axe A/AA和0.2%视觉门禁。

### 增量部署

- 2026-08-13 01:34 CST完成本机Linux amd64增量部署。正式产物继续使用`cgm_web_embed cgm_galleryepic`组合标签，安装到`/home/rainbowrunner/.local/bin/cgm`，SHA-256为`0674c59a06dcb305ab2c6f46c2e5758e5855009a2a7e7534ee02f6486ef11d8d`；`go tool nm`确认GalleryEpic Provider的`Search`、`FetchProfile`和`OpenAsset`仍在产物中。
- 替换前二进制备份为`/tmp/cgm-before-coser-manage-link-20260813`，SHA-256为`c9143fe559281b826104f0f73252b57edc045be39818feba5d3cb5404387ce71`。构建产物先写入同目录临时名称再以`mv`原子替换，未改动配置、数据库、媒体、Manifest或缓存。
- 首次通用Make产物短暂将脏工作树误标为当前HEAD精确源码；发现`exactSourceAvailable=true`后立即以相同源码和功能重建，并显式标记`gitHash=local`。最终`/about.json`报告`version=1.5.0-dev`、`buildTime=20260813`和`exactSourceAvailable=false`，没有把未提交累计修改声明为精确提交产物。
- 用户服务保持`enabled/active`，最终进程启动于01:34:01 CST；Health/Ready均为204，入口引用`index-ZJow6_NO.js`与`index-DX30qN0Z.css`且两项资源均返回200。启动journal确认2个工作器和LibRaw正常启动，没有WARN、ERROR、panic、fatal或迁移错误。

## 1.5-28 Manage设置输入控件浅色主题收敛

### 问题与修复

- Manage Shell已经声明`color-scheme: light`并对普通`input/textarea/select`应用浅色设计Token，但Settings遗留规则`.settings-form input[type="number"]`具有更高CSS选择器优先级，继续覆盖为旧主题的`#100e10`黑底和白字；这解释了为什么主要是数字输入框残留，而同页下拉框通常已正常变白。
- Settings数字输入框和下拉框现在直接使用`--cgm-surface`、`--cgm-foreground`与`--cgm-border`，不再依赖低优先级的外层补丁；键盘聚焦时使用主色边框、`--cgm-focus`轮廓和1px偏移，与当前浅色Manage设计系统一致。
- 修改范围限定在`.settings-form`，不改变Browse、Lightbox、Setup、Login、Maintenance或其他刻意保留深色舞台的页面。

### 阶段验证

```text
pnpm run check
PASS

pnpm exec vitest run src/manage/ManageSettingsPage.test.tsx
PASS（1个文件，3项测试）

pnpm test
PASS（24个文件，61项测试）

pnpm run build
PASS（674个模块；生产构建完成）

pnpm run e2e
PASS（1项离线所有者完整业务链路）
```

- Chromium在真实Manage Settings页面读取全部数字输入和下拉控件的计算样式，统一得到白色背景`rgb(255, 255, 255)`及深色文字`rgb(10, 10, 10)`；同页axe A/AA检查及既有完整视觉/业务回归继续通过。

### 增量部署

- 2026-08-13 01:44 CST完成本机Linux amd64增量部署。正式`cgm_web_embed cgm_galleryepic`产物安装到`/home/rainbowrunner/.local/bin/cgm`，SHA-256为`a42fb73f919daf1aa8abef6e8f1bf24936f17684ab09f7b08de4cc7ae9c52e0a`；替换前二进制备份为`/tmp/cgm-before-settings-light-controls-20260813`，SHA-256为`0674c59a06dcb305ab2c6f46c2e5758e5855009a2a7e7534ee02f6486ef11d8d`。
- 服务保持`enabled/active`，最终进程启动于01:44:14 CST；Health/Ready均为204，入口引用`index-fsxEioKQ.js`与`index-OhZ8A7Bj.css`且两项资源均返回200。About正确报告`gitHash=local`、`buildTime=20260813`和`exactSourceAvailable=false`；启动journal确认2个工作器与LibRaw正常，没有WARN、ERROR、panic、fatal或迁移错误。
- 部署只原子替换本机测试二进制并重启一次用户服务；未改动启动配置、业务数据库、媒体、Manifest或缓存。

## 1.5-17 单媒体操作菜单点击外部关闭

### 问题与修复

- 原实现直接使用浏览器原生`<details>`，它只负责点击自身`<summary>`时切换开关，不会在用户点击组件外部时自动关闭，因此媒体三点菜单会持续停留。
- 每个媒体菜单现在只在打开期间注册文档级`pointerdown`与`keydown`监听；指针落在菜单外时关闭，菜单内部点击保持原行为，`Escape`也可关闭。菜单关闭或组件卸载后立即移除监听，不为未打开的媒体卡片常驻全局监听器。
- 进入媒体详情和“设为封面”操作会显式关闭当前菜单；封面Mutation、revision校验、混合媒体排序和视觉布局均未改变。

### 验证与增量部署

```text
pnpm run check
PASS

pnpm run test
PASS（22个文件，55项测试）

pnpm run build
PASS（670个模块；主JS 474.24 KiB）

pnpm exec playwright test e2e/offline-lifecycle.spec.ts
PASS（1项；真实浏览器执行打开菜单、点击Gallery标题关闭、重新打开并设封面）

git diff --check
PASS
```

- 2026-08-01 04:06 CST安装增量构建，SHA-256为`30c9a63ec9c176043d8b92956e8a7283abcdf4cc77dd4344583ea9d61064ede7`；替换前二进制保存在`/tmp/cgm-before-media-menu-dismiss-20260801`，SHA-256为`66b235a1f93babc839fd3245c9c8305ff578c1f6b9ba02b1b75128504e49901f`。
- 服务保持`enabled/active`且Health/Ready为204，入口引用`index-BzI7BpIi.js`与`index-B5AaBjOr.css`。
- 配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，数据库inode仍为`19679716`；没有替换业务数据、媒体、Manifest或缓存。

## 1.5-18 单媒体菜单悬浮提示误定位（已撤销）

### 实施与验证

- 本阶段曾把“单个媒体右上角菜单按钮”误解为Gallery列表卡片的三点按钮，并为其添加本地化`title`。用户随后明确目标是点击媒体后Lightbox右上角的一组操作按钮。
- 该误定位版本仅在本机于04:13短暂部署；没有数据模型、Mutation、数据库、媒体、Manifest或缓存变更。
- 04:20纠正版本已移除列表三点按钮的`title`并替代本构建，详见1.5-19。本段保留用于记录判断修正和二进制回退链，不代表当前产品行为。

```text
pnpm run check
PASS

pnpm run test
PASS（22个文件，55项测试）

pnpm run build
PASS（670个模块；主JS 474.24 KiB）

pnpm exec playwright test e2e/offline-lifecycle.spec.ts
PASS（1项）

git diff --check
PASS
```

### 增量部署

- 2026-08-01 04:13 CST安装增量构建，SHA-256为`543c40f5995bf2d812d37ce23020514cad94bbe7e8796d8c3d8b65eeb1ccbdd5`；替换前二进制保存在`/tmp/cgm-before-media-menu-tooltip-20260801`，SHA-256为`30c9a63ec9c176043d8b92956e8a7283abcdf4cc77dd4344583ea9d61064ede7`。
- 服务保持`enabled/active`且Health/Ready为204，入口引用`index-BrpLuX8R.js`与`index-B5AaBjOr.css`。
- 配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，数据库inode仍为`19679716`；没有替换配置、业务数据、媒体、Manifest或缓存。

## 1.5-19 Lightbox右上角操作提示

### 纠正后的范围

- Gallery列表卡片三点按钮恢复为仅有`aria-label`、没有悬浮`title`；点击外部或按`Escape`关闭菜单的1.5-17交互保留。
- 点击单个媒体后，Lightbox右上角工具栏的收藏/取消收藏、媒体评分、设为封面/当前封面、打开媒体详情和关闭媒体查看器均显示与当前状态及界面语言一致的悬浮提示。
- 新增`gallery.closeViewer`中英文文案，关闭按钮不再硬编码英文可访问名称；所有提示与各控件`aria-label`使用同一变量，避免屏幕阅读器名称和视觉提示不一致。

### 验证与增量部署

```text
pnpm run check
PASS

pnpm run test
PASS（22个文件，55项测试）

pnpm run build
PASS（670个模块；主JS 474.33 KiB）

pnpm exec playwright test e2e/offline-lifecycle.spec.ts
PASS（1项；验证列表按钮无title及Lightbox五项提示）

git diff --check
PASS
```

- 2026-08-01 04:20 CST安装纠正后的增量构建，SHA-256为`d482e86092d1831b00266b431ad52e3430902ad07c11c4af781e2f94a3cdfb1c`；替换前误定位版本保存在`/tmp/cgm-before-lightbox-toolbar-tooltips-20260801`，SHA-256为`543c40f5995bf2d812d37ce23020514cad94bbe7e8796d8c3d8b65eeb1ccbdd5`。
- 服务保持`enabled/active`且Health/Ready为204，入口引用`index-BPUu1Roj.js`与`index-B5AaBjOr.css`。
- 配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，数据库inode仍为`19679716`；没有替换配置、业务数据、媒体、Manifest或缓存。

## 1.5-20 核心实体选择后自动关闭候选列表

### 问题与修复

- Character编辑器的Primary Work使用通用`ManageEntitySelector`。原组件选择候选项后只清空搜索文本，但Apollo `useLazyQuery`仍保留`called=true`和上一批`data`，候选列表的渲染条件继续成立，导致选择成功后下拉列表不关闭。
- 选择器现在用独立`open`状态管理弹层可见性：输入框聚焦或继续输入时打开，选择任一实体后清空搜索并显式关闭；再次聚焦仍按原查询规则加载最近候选，不清除已写入父表单的UUID。
- 修复位于通用实体选择器，因此Character Primary Work、Gallery的Coser/Character/Tag关系、Tag父级和实体合并目标选择均获得一致行为；GraphQL查询、实体校验和保存Mutation未改变。

### 验证与增量部署

```text
pnpm run check
PASS

pnpm run test
PASS（22个文件，55项测试）

pnpm run build
PASS（670个模块；主JS 474.33 KiB）

pnpm exec playwright test e2e/offline-lifecycle.spec.ts
PASS（1项；真实浏览器验证选择后listbox从DOM移除）

git diff --check
PASS
```

- 2026-08-08 16:27 CST安装增量构建，SHA-256为`6766a0c08c714af46face98239953d614cdaa12bdc5c97cdd78aa2848cc56817`；替换前二进制保存在`/tmp/cgm-before-entity-selector-dismiss-20260808`，SHA-256为`d482e86092d1831b00266b431ad52e3430902ad07c11c4af781e2f94a3cdfb1c`。
- 服务保持`enabled/active`且Health/Ready为204，入口引用`index-D45_orVP.js`与`index-B5AaBjOr.css`；实体选择器Chunk为`ManageEntitySelector-Be-gqGJ9.js`。
- 配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，数据库inode仍为`19679716`；没有替换配置、数据库、媒体、Manifest或缓存。

## 1.5-21 Gallery媒体完整父目录排序与根目录默认排除

### 已确认业务语义

- Manage媒体页以`relative_path`的完整父目录分组，不引入新的Folder业务实体；同名文件通过父目录区块、单独文件名和完整路径提示区分。
- 固定媒体类型顺序继续为PHOTO、SELFIE、ANIMATED_IMAGE、VIDEO。Gallery根目录是每个类型中的独立分组并固定排在全部文件夹之前；文件夹排序只在各自媒体类型组中生效。
- 移动文件夹保留其内部既有Item顺序；用户可对具体文件夹显式执行“按文件名自然排序”，比较basename而不是完整相对路径。文件夹内仍允许逐项上移/下移，但不能越过当前完整父目录。
- 默认扫描策略仅把本次真正新建的根目录Item设为excluded，人工扫描页提供默认开启的本次开关。按路径命中或唯一指纹重绑定的既有Item保留人工Exclude/Restore决定，不会被重扫覆盖。

### 实现

- GraphQL新增`reorderGalleryItems`，客户端提交一个媒体类型组的完整Item UUID顺序。仓储在同一SQLite事务中校验完整集合、重复、跨Gallery/跨组与`metadata_revision`，先迁移到临时高水位，再复用原组Position槽位，最终只递增一次revision并把Manifest标为`DB_DIRTY`。
- Manage媒体页新增完整父目录区块、根目录固定标识、文件夹上移/下移、具体文件夹文件名自然排序、文件夹内Item上移/下移，以及文件名/父目录分行和同名文件提示。排序完成后清除Browse成员索引缓存。
- `scanGallerySource`新增默认值为`true`的`excludeNewRootMedia`参数；产品默认扫描和自动计划均采用根目录新Item排除策略，人工扫描可关闭。有效成员上限在提交前按同一选项计算，排除Item不生成基础派生任务。
- `SetItemExcluded(false)`现在会在同一事务中立即为所有可用、PENDING且未排除的成员补排基础派生任务，避免自动排除Item被Restore后等待下一次扫描。
- 离线E2E夹具包含两个根目录媒体，因此主流程在首次扫描时明确关闭新开关，既验证开关可选，也保留原3成员生命周期断言；恢复后的重扫继续验证既有选择不会被默认策略覆盖。

### 阶段验证

```text
go test ./internal/persistence/productdb ./internal/productapi ./internal/productserver
PASS

pnpm run check
PASS

pnpm run test
PASS（23个文件，58项测试）

pnpm run build
PASS（671个模块；主JS 474.33 KiB）

go test ./internal/gallery ./internal/media ./internal/mediaprocessing ./internal/sourcescan ./internal/persistence/productdb ./internal/productapi ./internal/productserver ./ui/web
PASS

go test -tags cgm_web_embed ./internal/productserver ./ui/web
PASS

pnpm exec playwright test e2e/offline-lifecycle.spec.ts
PASS（1项；首次扫描关闭根目录自动排除，恢复后默认开启重扫仍保留既有Item选择）

make ... build-cgm
PASS（正式单文件产物：/tmp/cgm-folder-order-check）

git diff --check
PASS
```

- 新增SQLite回归覆盖根目录新Item默认排除、子目录默认纳入、仅为纳入Item排任务、Restore立即补任务、重扫保留Restore、关闭选项后根目录新Item纳入，以及完整组排序/跨组/缺项/重复拒绝和其他媒体组Position不变。
- 新增前端回归覆盖完整父目录分组、根目录置顶、文件夹内部顺序保留、basename数字自然排序，以及人工扫描开关默认开启和不覆盖既有选择的说明。
- 生产构建、产品Go包、正式嵌入模式和离线Chromium E2E已经通过。首次替换后发现构建把脏工作树误标为精确提交，随即只修正版本元数据并于2026-08-09 17:30 CST完成最终增量部署：新二进制SHA-256为`007347be19cd1058d94bb554a0dcfd55df5fbcbd90493f613eb44f9ba65cc90b`，`about.json`正确报告`gitHash=local`、`buildTime=20260809`和`exactSourceAvailable=false`。旧版本备份为`/tmp/cgm-before-gallery-media-folders-20260809`且校验和为`6766a0c08c714af46face98239953d614cdaa12bdc5c97cdd78aa2848cc56817`。服务保持`enabled/active`，Health/Ready均为204，入口引用`index-Br2kDKX5.js`与`index-dwLJ7y5E.css`；启动日志确认两个工作器正常启动且没有迁移、启动或任务错误。配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，数据库inode仍为`19679716`，没有替换配置、业务数据库、媒体、Manifest或缓存。

## 1.5-22 Browse桌面滚动与Gallery更多详情目录定位

### 已确认边界

- 桌面Logo/品牌栏随页面滚动离开视口，左侧Cosplay/Album分区菜单保持固定；1024px以下继续保留粘性顶部栏，确保Drawer入口可用。
- Gallery“更多详情”点击外部或按`Escape`关闭；弹层内显示所有非MISSING实际媒体的去重绝对父目录，并提供直达该Gallery Manage媒体页的入口。
- `GalleryDetail.mediaParentDirectories`是用户确认的认证单所有者有限路径例外。DIRECTORY包含excluded但仍存在的Item，根目录优先、其余按自然序；ARCHIVE只显示归档所在目录。不得返回文件名、Item相对路径、指纹、缓存路径、`file://`或文件管理器/外部命令入口。

### 实现

- Browse仓储只在Gallery详情查询聚合父目录；Gallery卡片、列表、轻量成员索引和媒体资源身份保持无路径。GraphQL、前端类型和中英文文案同步新增目录摘要字段。
- `GalleryDetailPage`使用受控`details`、按需document监听器和组件ref处理外部点击/Escape；Manage入口直接使用既有`setID`路由并定位`media`页签。
- 桌面`.browse-topbar`恢复普通文档流，固定`.browse-sidebar`不变；移动断点显式恢复`position: sticky`，桌面Related吸附偏移从Topbar高度加16px收敛为16px。

### 阶段验证

```text
go test ./internal/persistence/productdb ./internal/productapi
PASS

pnpm run check
PASS

pnpm run test
PASS（23个文件，59项测试）

pnpm run build
PASS（671个模块；主JS 474.48 KiB）

go test ./internal/browse ./internal/media ./internal/persistence/productdb ./internal/productapi ./internal/productserver ./ui/web
PASS

go test -tags cgm_web_embed ./internal/productserver ./ui/web
PASS

pnpm exec playwright test e2e/offline-lifecycle.spec.ts
PASS（1项；真实验证目录摘要、Manage直达、外部关闭、桌面Topbar滚离/Sidebar固定及移动Topbar sticky）

make ... build-cgm
PASS（正式单文件产物：/tmp/cgm-gallery-detail-locations）

git diff --check
PASS
```

- SQLite回归覆盖根目录、自然排序子目录、excluded、UNREADABLE、MISSING忽略及ARCHIVE仅返回容器目录；GraphQL集成回归确认认证详情返回父目录但不返回媒体文件名。
- 前端回归覆盖目录与Manage链接呈现、弹层内部点击保留、外部点击/Escape关闭；离线Chromium继续通过axe/截图/备份恢复完整业务闭环。
- 2026-08-09 20:28 CST完成本机增量部署：新二进制SHA-256为`e3e595503e80082d15d0c6c49e225b47ce159aa5c34b47b239567f7360685041`，旧版本备份为`/tmp/cgm-before-gallery-detail-locations-20260809`且SHA-256为`007347be19cd1058d94bb554a0dcfd55df5fbcbd90493f613eb44f9ba65cc90b`。服务保持`enabled/active`，Health/Ready均为204，入口引用`index-9hJr0i02.js`与`index-hDZ83M8Y.css`；`about.json`正确报告`gitHash=local`、`buildTime=20260809`和`exactSourceAvailable=false`。配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，数据库inode仍为`19679716`；没有替换配置、业务数据库、媒体、Manifest或缓存，启动journal没有迁移、启动或任务错误。

## 1.5-23 Gallery详情标题响应式修复

### 问题与实现

- Gallery详情主标题没有组件级硬换行，但CSS以`max-width: 34ch`限制标题宽度；Header右侧操作区继续占位，700px以下仍使用两列和固定30px标题，导致中等长度中文标题在实际空间充足时也提前分行。
- 移除`34ch`上限，为标题容器补充`min-width: 0`，并用`overflow-wrap: break-word`保护允许最长300字符的异常长标题。桌面继续使用标题/操作两列，正常标题可以使用左列全部宽度。
- 700px以下Header切换单列，收藏/更多详情操作在标题与统计区下方右对齐；标题字号使用24～30px响应式范围。没有使用`nowrap`、省略号或行数截断，完整业务标题仍然可见。

### 阶段验证

```text
pnpm run check
PASS

pnpm run test
PASS（23个文件，59项测试）

pnpm run build
PASS（671个模块；主JS 474.48 KiB）

pnpm exec playwright test e2e/offline-lifecycle.spec.ts --update-snapshots
PASS（1项；桌面中等长度标题单行、移动操作区分行、300字符标题无横向溢出）

pnpm exec playwright test e2e/offline-lifecycle.spec.ts
PASS（1项；更新后的0.2%视觉基线稳定通过）
```

- Playwright按计算样式确认桌面主标题`max-width: none`，扣除padding后的内容高度不超过一个line-height；390px下操作区纵坐标位于完整标题/统计区之后，300个连续`W`仍满足`scrollWidth <= clientWidth`。
- 更新后的390px Gallery截图已人工检查：标题、统计、右对齐操作区与两列媒体网格间距正常，没有重叠、截断或横向滚动。
- 按用户要求，本阶段只完成源码、测试与开发记录，未构建安装本机服务、未重启systemd，也未修改配置、数据库、媒体、Manifest或缓存；当前部署仍是2026-08-09 20:28 CST的`e3e595503e80082d15d0c6c49e225b47ce159aa5c34b47b239567f7360685041`版本。

## 1.5-24 Gallery卡片参考站密度与弱化覆盖层

### 已确认范围与实现

- 以2026-08-12实测`galleryepic.xyz/zh`公开页面为准，用户确认将Gallery索引从CGM原6/5/4/3/2列改为参考站2/3/4/5列：默认2列、768px起3列、1024px起4列、1536px起5列，横纵间距统一16px。所有复用`GalleryCard`的首页、分区、实体详情、收藏、历史和时间线保持一致；Gallery详情媒体与Random媒体的独立网格不变。
- 右下媒体计数保持8px定位和现有非零P/S/G/V格式，视觉收敛为Inter 12px/16px、400字重白字；移除76%黑底、内边距、圆角、背景模糊和额外字间距。
- Gallery收藏按钮继续调用既有Mutation；在`hover:hover + pointer:fine`设备默认透明且不接收指针，封面悬浮或`focus-within`时显示；触屏或粗指针设备保持常驻。键盘仍能Tab聚焦隐藏按钮并触发显示，`aria-pressed`收藏状态不变。
- 上一阶段Gallery详情标题响应式修复与本阶段将作为同一个增量构建部署，不修改后端模型、GraphQL、数据库、配置、媒体、Manifest或缓存。

### 阶段验证

```text
pnpm run check
PASS

pnpm run test
PASS（23个文件，59项测试）

pnpm run build
PASS（671个模块；主JS 474.48 KiB）

go test ./internal/browse ./internal/media ./internal/persistence/productdb ./internal/productapi ./internal/productserver ./ui/web
PASS

go test -tags cgm_web_embed ./internal/productserver ./ui/web
PASS

pnpm exec playwright test e2e/offline-lifecycle.spec.ts --update-snapshots
PASS（1项；更新Browse及Model详情桌面/移动视觉基线）

pnpm exec playwright test e2e/offline-lifecycle.spec.ts
PASS（1项；计算样式验证计数、四档列数、16px间距及鼠标/键盘收藏可见性）
```

- 已人工检查更新后的Browse、Model详情桌面与390px截图：卡片尺寸和间距已按目标放大收敛，计数无底色且视觉弱化，文字与卡片没有重叠或溢出。

### 增量部署

- 2026-08-12 00:29 CST完成包含1.5-23 Gallery标题响应式修复和本阶段Gallery卡片调整的本机增量部署。新二进制SHA-256为`9e9477e36d1572150a7fe0502d7981e3fcd0dcff533168254e34baf06a43bf35`；旧版本备份为`/tmp/cgm-before-gallery-cards-20260812`，SHA-256为`e3e595503e80082d15d0c6c49e225b47ce159aa5c34b47b239567f7360685041`。
- 用户服务保持`enabled/active`，重启后Health/Ready均为204；首次紧随`systemctl restart`的Health请求发生在监听端口就绪前并返回连接失败，随后带连接重试的核验立即通过。journal确认两个工作器和服务正常启动，没有迁移、启动或任务错误。
- `/about.json`正确报告`version=1.5.0-dev`、`gitHash=local`、`buildTime=20260812`和`exactSourceAvailable=false`；入口实际引用`index-BN0Udqs0.js`与`index-qdWPUXcG.css`。
- 配置SHA-256仍为`1beb3770cf5f84098fad3d10b87965ed7421227f605aebdda0f7f6220c3c6dd7`，产品数据库inode仍为`19679716`；没有替换配置、数据库、媒体、Manifest或缓存。

## 1.5-29 视频处理第一、第二阶段功能规划

### 现状核验

- 当前产品可以发现VIDEO、计算指纹并排队`STATIC_POSTER`，但Poster固定取第0秒；`DIRECT | REMUX | TRANSCODE`目前只是未接入技术元数据、资源路由和前端的规划函数。
- `VIDEO_PLAYBACK`已声明为ENHANCED variant，媒体详情前端也能渲染`video/*`，但没有生成器、请求入口或原视频直读路由，因此尚未形成实际播放闭环。
- 本机业务部署仍未配置FFmpeg/FFprobe，当前不能把Video Poster或播放真实业务验收记录为通过。

### 已冻结规划

- 第一阶段：FFmpeg/FFprobe成对诊断、受控数据库前向迁移、GalleryItem 1:1视频技术元数据、确定性主轨选择、有界回填，以及约20%位置/960px/快速seek失败精确回退的BASE Poster。
- 第二阶段：经认证且不泄漏路径的DIRECT Range路由；Lightbox或媒体详情实际打开时才按需Remux/转码；H.264/AAC Fast Start MP4代理归入ENHANCED并沿用现有LRU。
- Gallery卡片和Scrubber继续只使用静态Poster；不增加字幕、多音轨、画质档、时间轴Sprite、HLS/DASH、播放进度、硬件编码或Windows构建任务。
- 完整数据字段、错误码、资源授权、缓存生命周期、测试矩阵和实施顺序记录于[视频处理第一、第二阶段功能规划](VIDEO_PROCESSING_PHASE_1_2_PLAN_2026-08-15.md)。

### 本阶段结果

- 本阶段只修改开发规范和计划，没有修改业务代码、数据库、配置或部署，也没有运行媒体功能测试。
- 后续从VP1-01开始按阶段实施；首次schema升级部署必须先创建并校验完整安全备份，不能按普通无schema热替换处理。

## 1.5-30 视频处理第三阶段条件式功能规划

### 阶段定位

- 第三阶段不是第一、第二阶段的完成条件，也不改变当前第一版明确延期范围；只有基础探测、Poster、DIRECT、Remux和MP4代理真实业务稳定后才考虑实施。
- 第三阶段A独立规划按需Storyboard Sprite/WebVTT及媒体详情/Lightbox辅助时间轴。Gallery卡片、网格和Gallery卡片Scrubber继续只显示静态Poster。
- 第三阶段B先执行无观看画像的本地大视频冷启动/seek基准；只有第二阶段完整MP4代理反复不能满足实际业务，才通过ADR启用单清晰度渐进HLS。

### 已冻结边界

- Storyboard与HLS均属于ENHANCED，受容量、磁盘余量和LRU约束，不参与Gallery可展示或BASE Poster判定。
- 渐进HLS采用受认证的短期会话和完成后整体bundle；不同时实现DASH，不做多清晰度、硬件编码、字幕、多音轨或观看进度。
- 视频pHash和重复建议属于独立未来项目，不并入播放第三阶段。
- 完整启用门槛、资源身份、采样、会话、缓存、安全、回退和测试规格见[视频处理第三阶段功能规划](VIDEO_PROCESSING_PHASE_3_PLAN_2026-08-15.md)。

### 本阶段结果

- 本阶段只持久化后续规划，没有修改业务代码、数据库、配置或部署，也没有运行视频媒体测试。
- 后续开发顺序仍从VP1-01开始；不得越过第一、第二阶段直接实现Storyboard或HLS。

## 1.5-31 视频处理第一阶段：探测、技术元数据与可靠Poster

### 实现结果

- 启动配置保留`ffmpeg_path`并新增向后兼容的可选`ffprobe_path`；解析时优先使用FFmpeg同目录FFprobe，分别校验可执行性和最低主版本5。Manage → Settings新增路径无关的可用性、来源、版本和稳定错误码诊断，缺失工具不阻止图片/RAW业务启动。
- 产品数据库schema提升至v2。旧v1库打开时先通过SQLite Online Backup创建权限0600、独占命名的`pre-schema-v1`快照，重新验证完整性、产品身份与v1结构后才迁移；新增GalleryItem 1:1的`video_technical_metadata`和独立`ITEM_TECHNICAL_METADATA`任务。
- FFprobe适配器使用固定参数、30秒超时、context取消和4MiB/64KiB输出上限；忽略attached picture与字幕，主视频/音频按default后first选取，只持久化规范技术字段，不保存原始JSON、stderr、标题、文件名或路径。
- 新发现/替换VIDEO只先排探测，成功后再排STATIC Poster；存量DIRECTORY视频每分钟低优先级回填最多25项，已记录ERROR只允许人工重试，避免后台无限读取。内容revision变化会删除旧技术状态、取消旧任务并HARD_INVALID旧派生。
- Poster改为有效时长20%，快速seek失败后精确seek、再以第0秒保底；固定960px JPEG、不放大，使用已选视频轨并显式处理旋转与HDR到SDR。Manage媒体行支持探测状态/错误/摘要与重试，MediaDetail仅返回白名单技术摘要。

### 阶段验证

- SQLite迁移测试确认v1约束真实不含新任务种类，迁移后v2表/约束存在，自动快照仍保持v1身份；回填有界、幂等、不自动重试ERROR且不改变Gallery metadata revision。
- 单元/Worker/API覆盖轨道选择、WebM容器规范化、旋转/HDR、20%时间点、未知/短时长、探测→Poster顺序、错误码、Manage重试和路径不泄漏。
- 本机真实`/usr/bin/ffmpeg`、`/usr/bin/ffprobe`、`/usr/bin/dcraw`合成门禁通过MP4、MOV、MKV、WebM、无音频、双音轨、旋转、HDR技术探测和960px Poster；测试实际发现并修复`format_name=matroska,webm`被错误归类为MKV的问题。

## 1.5-32 视频处理第二阶段：认证直放与按需兼容代理

### 实现结果

- 播放矩阵以当前revision READY技术元数据为唯一输入：只有MP4/H.264/AAC、MP3或无音频，且无需旋转、非HDR时DIRECT；H.264容器/音频不兼容优先Remux，其余转CPU libx264/AAC Fast Start MP4。WebM/VP9/Opus暂不进入共同浏览器直放白名单。
- 新增`/resource/video/<item_uuid>/<content_revision>/direct`：无路径参数，未认证/无权限统一404；授权复核ACTIVE、Scope、Hidden、Source/Item AVAILABLE、Exclude、Over-limit、Blocking Issue、当前revision和DIRECT方案。仅DIRECTORY可用，路径逐级拒绝符号链接并保持同一已打开普通文件描述符完成GET/HEAD、单/多Range、416和ETag响应。
- GraphQL新增路径无关播放状态和请求契约。DIRECT立即就绪且不创建任务；Remux/Transcode按item/revision/profile并发汇聚，profile包含矩阵、轨道、FFmpeg版本、目标编码、分辨率、旋转/HDR和参数。VIDEO_PLAYBACK固定为ENHANCED，生成前执行缓存容量/磁盘余量检查，失败返回`DISK_SPACE_LOW`。
- FFmpeg生成器显式map一条视频与可选音频、去除字幕/附件/metadata；Remux复制H.264并按需音频转AAC，Transcode使用libx264 medium/CRF20/high/yuv420p与AAC 192k、不放大、旋转清理和确定性HDR到SDR。滤镜不可用返回`VIDEO_TONEMAP_UNAVAILABLE`，命令和stderr不进入公共错误。
- Lightbox当前视频和媒体详情挂载时才发请求并750ms轮询；准备中保留Poster，完成后无刷新切换原生播放器，失败可重试。切换成员以item UUID卸载旧Lightbox/播放器；卡片、网格、Scrubber、推荐和相邻媒体均不触发播放。直接与代理响应清除普通2分钟写超时，但客户端断开仍通过请求context终止读取。
- 增加`CGM_VIDEO_PROBE_*`、`CGM_VIDEO_POSTER_*`、`CGM_VIDEO_PLAYBACK_REQUESTED`、`CGM_VIDEO_DIRECT_SERVED`和`CGM_VIDEO_PROXY_*`事件；默认只包含技术状态、job ID、短item前缀、耗时和稳定错误码，不形成观看Audit或保存进度。

### 阶段验证与边界

- 播放规划、输出尺寸、并发请求幂等、DIRECT不排任务、代理任务/Worker/ENHANCED发布、来源size/mtime/字节不变、GraphQL路径不泄漏均有回归；DIRECT HTTP覆盖未认证、Scope、Hidden、旧revision、HEAD、Range、多Range和416。
- 真实工具门禁已实际生成并重新FFprobe验证H.264 MP4 Remux与Transcode代理，并验证HDR标记输入可完成固定SDR流程；自动化门禁结果不替代Chrome/Firefox/Safari真实业务媒体播放体验。
- 最终验证结果：视频相关Go/SQLite/API/Server目标包全部PASS；`CGM_TEST_FFMPEG=/usr/bin/ffmpeg CGM_TEST_FFPROBE=/usr/bin/ffprobe CGM_TEST_LIBRAW=/usr/bin/dcraw`真实媒体矩阵PASS；TypeScript检查PASS；Vitest 25个文件、64项PASS；Vite生产构建675模块PASS；`cgm_web_embed`产品Server/UI回归PASS；现有Chromium离线完整业务Playwright 1项PASS且没有更新视觉快照。
- `go test ./internal/...`额外探测只有原Stash遗留`internal/api`、`internal/api/urlbuilders`和`internal/manager`因项目明确移除的`ui/v2.5/build`嵌入目录而setup failed，其他执行到的内部包通过；CGM不以恢复旧UI来规避该隔离边界，产品入口已由上述`cgm_web_embed`门禁替代验证。
- 本轮没有构建安装或重启本机服务，没有修改正式配置、数据库、媒体、Manifest或缓存。正式部署会触发schema v2维护迁移，必须先额外完整备份并保留自动v1快照；部署与人工浏览器验收另行执行。

## 1.5-33 累计提交、额外完整备份与视频阶段正式部署

### 部署基线

- 将当前127个累计变更文件完整提交为`135174fc3a17d76c8887ebfba99120b55dfb976a`（`Complete CGM 1.5 frontend metadata and video workflows`）；提交前`git diff --cached --check`通过，提交后工作树清洁。
- 从该清洁提交以`cgm_web_embed cgm_galleryepic`构建`1.5.0-dev`正式产物；二进制报告完整提交且`go version -m`确认`vcs.modified=false`，SHA-256为`e9dc5dc4fe3f1143cdecad8a6a2c86934015a4b63310423bd28d43b96959200c`。
- `scripts/verify-cgm-release.sh`重新构建产品、生成提交源码归档并确认二进制版本/完整提交和AGPL源码对应关系；Vite生产构建仍为675个模块。

### 额外完整回滚备份

- 2026-08-15 21:29 CST停止`cosplay-gallery-manager.service`并确认MainPID为0；没有让第二个CGM进程与正式SQLite并行运行。
- 在真实备份根创建`/home/rainbowrunner/cos/bk/cgm-predeploy-20260815T132857Z-135174f.tar.gz`，包含SQLite一致快照、启动配置、Coser托管元数据、替换前二进制和systemd用户服务定义；媒体来源、可重建缓存、日志和既有备份未递归打包。
- 归档及校验文件权限均为`0600`；归档SHA-256为`e94a134919ec0b1e334096eb798c00e59e472accaf6386f21ee65be6707dce4c`。实际解包后，源库与备份库`integrity_check`均为`ok`，规范化SQL dump SHA-256同为`15cbb03765bb65d352f4bcc890b5b294693d3b5a7691ae8bcbee4547f400aa3`，54表/1648行、产品身份和Setup存储根一致；配置、旧二进制、服务定义及Coser文件树逐项一致。

### 安装、迁移与运行验证

- 经校验的新二进制先写入同目录临时名、复核SHA-256后原子替换`/home/rainbowrunner/.local/bin/cgm`，再启动用户服务；配置文件保持SHA-256 `ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`且没有修改媒体库、Manifest或来源文件。
- 正式数据库inode保持`19679716`并由schema v1前向迁移至v2。迁移前自动快照`product.sqlite.pre-schema-v1-1786800729629296221.bak`权限`0600`、SHA-256 `5572e531562c6d77333af522fdf2267b2185a11d8b54d1919495367c48f924f6`，单独验证`integrity_check=ok`和schema v1；迁移后主库`integrity_check=ok`并报告schema v2。
- 服务保持`enabled/active`、`NRestarts=0`；Health/Ready为204，首页为200。`about.json`报告`1.5.0-dev`、完整提交、`buildTime=2026-08-15 20:15:00`及`exactSourceAvailable=true`。
- 启动日志确认`libraw_enabled=true`、`ffmpeg_enabled=true`和`ffprobe_enabled=true`；11个既有DIRECTORY视频完成`ITEM_TECHNICAL_METADATA`并全部READY，随后11个`STATIC_POSTER`任务全部COMPLETED/READY。部署后journal未检出ERROR、WARN、FAILED、panic或fatal。
- 新主JS/CSS、Gallery详情Chunk与`useOnDemandVideoPlayback` Chunk均返回200。缓存因新版Poster从约161 MiB轻微增加到约162 MiB，符合BASE派生写入预期。

### 保留验收与回退边界

- 本轮验证覆盖备份可解包、逻辑内容等价和迁移安全，但没有实际把正式库恢复回v1；恢复演练仍必须在隔离副本或下一次明确维护窗口执行。
- 仍需所有者在已登录目标浏览器中使用真实DIRECT、Remux和Transcode样本核对首次等待、拖动、暂停、全屏、长时播放与代理缓存重用；自动化合成媒体和服务健康检查不能替代这部分人工业务验收。

## 1.5-34 Gallery详情Video标识取消黑色背景

### 问题与实现

- Gallery详情媒体卡片右上角`.media-tile__kind`使用75%不透明黑色背景和`.25rem .35rem`内边距，VIDEO文字形成突兀的小型黑色块，与当前GalleryEpic参考风格不一致。
- 移除背景与内边距，类型文字继续固定在卡片右上角；使用82%白色和轻量文字阴影兼顾明暗Poster可读性，但不产生新遮罩。VIDEO/GIF文案、媒体顺序、菜单层级、点击区域和Lightbox逻辑保持不变。

### 验证与增量部署

- `GalleryDetailPage.ui.test.tsx`专项Vitest 7项PASS；TypeScript检查PASS；Vite生产构建675模块PASS；Chromium离线完整业务Playwright 1项PASS，现有视觉基线无需更新。
- 样式修复提交为`4b3c1ec982544da46dd48791d5ad17057b09d930`；`cgm_web_embed cgm_galleryepic`正式构建报告`vcs.modified=false`，SHA-256为`2561c13c628f3fef8a244ac74e866fa62b059b05989e511fb3f4660314b813e2`。
- 2026-08-15 21:46 CST完成本机增量部署。旧二进制保存在`/tmp/cgm-before-video-label-20260815-2145`，SHA-256为`e9dc5dc4fe3f1143cdecad8a6a2c86934015a4b63310423bd28d43b96959200c`；此前额外完整回滚包及schema v1迁移快照继续保留。
- 服务保持`enabled/active`、`NRestarts=0`，Health/Ready为204、首页为200；`about.json`报告完整提交和`exactSourceAvailable=true`。实际CSS资源`index-Btj_g1dP.css`确认使用`padding:0;background:transparent`，启动后journal未发现错误或警告。本轮未修改数据库、配置、媒体、Manifest或缓存。

## 1.5-35 静态原图混合直读与完整时长动画预览

### 已确认策略

- 静态Lightbox/媒体详情采用DIRECT/PROXY混合策略：JPEG、PNG、静态WebP仅在来源不超过20MiB且宽高均不超过4096时直读原图，其他图片继续使用按需`LIGHTBOX_4096`；RAW不进入原图端点。
- Gallery详情网格的GIF/动态WebP按需生成最长边480、最高15FPS、循环的动画WebP，但播放时长必须与原动画完整时长相同，不采用此前备忘录中的6秒截断。
- 只有视口内按Gallery顺序选出的前4个动画项占用播放槽；离开视口、Lightbox打开或`prefers-reduced-motion: reduce`时切回长期BASE Poster。Gallery卡片、Scrubber、Related和其他索引不自动播放。
- 点击动画进入Lightbox或单媒体详情时，通过认证不透明URL播放未经重编码的完整GIF/动态WebP原字节；支持DIRECTORY和ZIP/CBZ，不新增物理路径DTO或数据库schema。

### 后端实现与安全边界

- 新增`/resource/image/<itemUUID>/<contentRevision>/original` GET/HEAD端点。数据库再次校验认证、ACTIVE、分级scope、未隐藏/排除/超限、来源/Item可用、无阻断Issue和revision；服务端读取实际签名与尺寸后决定是否允许，未知/伪装格式统一404。
- DIRECTORY继续使用非符号链接、同一文件身份复核后的已打开描述符；ZIP/CBZ使用现有`Materializer`限制和私有`cache/tmp`临时文件，响应结束清理。响应使用`private, no-cache`、ETag、`nosniff`和`inline`，GraphQL不返回来源路径。
- 新增幂等`ANIMATED_PREVIEW` GraphQL状态/请求，任务键包含Item、revision和FFmpeg版本化Profile；派生固定为ENHANCED、`.webp`和300优先级。FFmpeg命令只包含`fps=15`与480缩放，不含`-t`、`-to`或帧数截断。
- 修正工作器技术元数据Profile重排队使用调用方基准时间，消除测试/恢复时由墙钟时间造成的任务暂不可领取问题；不改变正式视频业务契约。

### 前端实现

- `useImageDisplay`先以同源认证HEAD让服务端判定DIRECT资格；合格即使用原图URL，不合格静态图透明回落到现有4096任务/轮询，探测和生成期间保留Card/Poster。
- `IntersectionObserver`只观察Gallery详情当前已渲染的动画Tile，按后端展示顺序最多激活4项；动画任务完成后替换Poster，失去播放槽即移除动画资源并恢复Poster。
- Lightbox与媒体详情统一使用混合图片显示Hook；GIF/动态WebP不再停留在静态Poster。Video的DIRECT/Remux/Transcode链路没有改变。

### 验证与部署状态

- 原图Handler测试覆盖合格静态HEAD、完整GIF精确字节、4097px静态回退和ZIP/CBZ动画Entry；动画任务测试覆盖FFmpeg不可用、ENHANCED稳定任务键与并发幂等。
- 真实`/usr/bin/ffmpeg`生成测试验证20帧/2秒GIF输出仍为2秒完整ANMF序列、最长边不超过480并含动画WebP标记；参数测试锁定15FPS且不存在任何时长截断参数。
- 产品相关Go包全部PASS；`go test ./internal/...`中本阶段涉及包全部PASS，但旧Stash `internal/api`、`internal/api/urlbuilders`、`internal/manager`仍因仓库未提供`ui/v2.5/build`嵌入目录而在setup阶段失败，此为既有旧UI门禁限制。
- 前端26个Vitest文件、67项测试全部PASS，TypeScript检查与677模块Vite生产构建PASS。
- 隔离的离线Chromium完整生命周期/备份恢复/axe矩阵1项PASS（30.1秒）；运行日志实际完成一条`ANIMATED_PREVIEW`任务，证明Gallery详情可视动画请求、FFmpeg派生、认证资源与现有业务流程共同工作。维护恢复窗口中的预期503由用例覆盖并最终恢复就绪，不属于正式服务日志。
- 源码、测试与第一版记录提交为`4c0dadacee45b47850f3d3a4074f804838b16925`（`Add mixed original image and animated preview playback`）；提交后工作树清洁。带`cgm_web_embed cgm_galleryepic`的Server/cmd/UI回归PASS，精确构建的`go version -m`报告`vcs.modified=false`，二进制SHA-256为`c195f0e16b4c56628e241fa54ef4e113b2db1bbce2d1873830dc4abc373f68bf`。
- 2026-08-16 22:49 CST完成本机Linux amd64增量部署。替换前二进制备份为`/tmp/cgm-before-image-animation-20260816`，SHA-256为`2561c13c628f3fef8a244ac74e866fa62b059b05989e511fb3f4660314b813e2`；新产物原子安装到`/home/rainbowrunner/.local/bin/cgm`后只重启一次用户服务。
- 服务保持`active/running`、`NRestarts=0`，Health/Ready均为204；`about.json`报告`version=1.5.0-dev`、完整提交、`buildTime=2026-08-16`和`exactSourceAvailable=true`，入口引用`index-anzNnO_M.js`与`index-Btj_g1dP.css`且均来自本轮嵌入构建。
- 启动journal只记录正常停止、2个工作器以FFmpeg/FFprobe/LibRaw全部可用启动及健康请求，没有WARN、ERROR、panic、fatal或数据库迁移事件。配置SHA-256仍为`ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`、inode仍为`19681854`；产品数据库inode仍为`19679716`。本轮没有schema变更，也没有替换配置、数据库、媒体、Manifest或既有缓存。

## 1.5-36 Gallery动画可配置播放窗口与锁定节流

### 已确认策略与实现

- Gallery详情动画安全上限默认12，可在Manage Settings设置1～16。动画总数小于等于N时不安装悬浮切换处理，所有进入视口的动画均有播放资格；超过N时初始窗口为排序前N项。
- 精确鼠标在动画Tile持续悬浮150ms后锁定新窗口。窗口以目标项为中心连续取N项：奇数左右均分，偶数左`N/2-1`、右`N/2`，靠近首尾时夹紧为前N项或后N项。鼠标移开只取消尚未确认的候选，不改变已经锁定的窗口。
- 两次有效锁定之间采用后台可配置节流：默认800ms，范围700～1000ms；冷却期内进入新目标时，只有目标持续停留到冷却结束才接受切换，避免相邻Tile造成高频动画资源替换。
- 播放窗口与`IntersectionObserver`可见集合求交后才真正加载动画；打开Lightbox或系统要求`prefers-reduced-motion`时全部回退Poster。无悬浮能力设备以当前可见动画的中位项自动移动窗口，保证触屏滚动到后段仍能播放。
- 新增产品数据库schema v3，在`runtime_settings`持久化`gallery_animated_playback_limit`和`gallery_animated_lock_interval_ms`。迁移器支持v1→v2→v3及正式环境v2→v3，写入前生成带真实来源版本名的SQLite Online Backup；新库直接建立到v3。

### 自动验证与部署门禁

- 数据库/API专项测试覆盖新库默认值、1～16和700～1000边界、数据库CHECK、乐观并发、v1→v3、v2→v3、迁移前快照身份及旧设置保留；相关Go包全部PASS。
- 前端纯函数覆盖奇偶窗口、首尾夹紧、视口求交、reduced-motion、150ms确认和配置冷却；Manage Settings越界时禁用保存。Vitest 26个文件70项、TypeScript检查及677模块生产构建PASS；隔离Chromium完整业务生命周期、备份恢复与axe矩阵1项PASS，实际完成一条`ANIMATED_PREVIEW`任务，维护恢复阶段的预期503最终恢复就绪。
- 本项包含schema迁移，正式增量部署前必须先提交清洁源码、创建并校验额外完整回滚包；启动迁移后还须验证自动`.pre-schema-v2-*`快照、主库schema v3、Health/Ready、About精确源码及服务日志。部署结果在完成维护窗口后补记。

### 提交、备份与正式增量部署

- 源码、测试和部署前记录提交为`f22ca41d237def7d70e489522422dd4f7a3819a5`（`Add configurable gallery animation playback windows`），提交后工作树清洁。正式产物使用`cgm_web_embed cgm_galleryepic`、Go 1.25.12和完整提交构建；`go version -m`确认`vcs.modified=false`，二进制SHA-256为`a66ab60a43cc5a9a572afca8b9767cf529f6e32b3bcfa0a6a36598bd8c252cfc`。
- 2026-08-17 00:03 CST停止用户服务并确认MainPID为0，在真实备份根创建`/home/rainbowrunner/cos/bk/cgm-predeploy-20260816T160239Z-f22ca41.tar.gz`。归档包含一致SQLite schema v2快照、启动配置、Coser托管资源、替换前二进制和systemd用户服务定义；媒体来源、缓存、日志及既有备份未递归打包。
- 回滚包权限为`0600`、SHA-256为`8b7e99b449da6a765b8f77b63fc7da0c08da3b9606a1c4c2163b4cc27c1d2493`。实际解包后数据库`integrity_check=ok`、产品身份/schema v2、55表/1920行；配置、旧二进制、服务定义及Coser资源与正式来源逐项一致。
- 新二进制先写同目录临时文件并复核SHA-256一致后原子替换，旧二进制另保留于`/tmp/cgm-before-animation-window-20260817`。只启动一次服务即完成v2→v3迁移；配置SHA-256保持`ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`，未修改媒体、Manifest或缓存。
- 自动迁移快照`product.sqlite.pre-schema-v2-1786896235945493562.bak`权限`0600`、SHA-256为`8aff930f6ceca6999c7ac9b404ae47a30fdf1e3231e1608a4d535087a6be3afa`，独立校验`integrity_check=ok`、schema v2、55表/1920行。迁移后主库同样`integrity_check=ok`、schema v3、55表/1920行，默认动画上限12、锁定间隔800ms。
- 正式服务保持`active/running`、`NRestarts=0`；Health/Ready均为204。About报告`version=1.5.0-dev`、完整`f22ca41...`、`buildTime=2026-08-17`和`exactSourceAvailable=true`；2个工作器以FFmpeg/FFprobe/LibRaw全部可用启动，本轮journal未检出WARN、ERROR、FAILED、panic或fatal。

## 1.5-37 Coser详情跨COSPLAY与ALBUM作品筛选

### 已确认边界与实现

- 只修改`/coser/:slug`作品区域；`/model/:slug`继续固定查询ALBUM并保留现有简洁页面，Coser/Model索引、侧栏、Gallery人物链接、Timeline及其他Browse详情的LIST/MAGIC/ALL逻辑均不改变。
- Coser详情原LIST/MAGIC/ALL控件替换为“全部作品 / COSPLAY / ALBUM”。默认“全部作品”向现有可选`collectionType`传null并返回该Coser的混合Gallery；COSPLAY传`collectionType=COSPLAY, scope=ALL`，明确等于LIST与MAGIC合集；ALBUM传`collectionType=ALBUM, scope=ALL`，维持全部分级Album口径。
- 筛选状态使用`?type=ALL|COSPLAY|ALBUM&page=N`持久化；切换类型重置到第1页，分页保留当前类型。中英文标签和可访问分组名称已补齐，现有三段控件视觉与Timeline布局复用且不新增样式分叉。

### 自动验证与部署状态

- 产品数据库回归新增同一Coser同时拥有NON_ADULT COSPLAY、ADULT COSPLAY和ADULT ALBUM样本，锁定混合查询返回3项、COSPLAY合并LIST/MAGIC返回2项、ALBUM只返回1项。
- Coser组件回归覆盖默认null混合查询、三种筛选、切换重置分页、URL持久化及Model无新控件；产品数据库专项、TypeScript检查和Vitest 26个文件70项全部PASS。677模块Vite生产构建及带`cgm_web_embed cgm_galleryepic`的Server/API/数据库/cmd组合回归PASS。
- 功能、测试和部署前记录提交为`edf4bf30c8090e70cbe2624487d65d4648853a10`（`Show all gallery types on coser details`）；清洁提交以`cgm_web_embed cgm_galleryepic`和Go 1.25.12构建，`go version -m`确认`vcs.modified=false`，正式二进制SHA-256为`f34233ad63c524cb60564c7fe19ba03f628819d70343a58aaa90671d4edc7c8f`。
- 2026-08-17 00:36 CST完成本机增量部署。替换前二进制保留于`/tmp/cgm-before-coser-types-20260817`；新产物同目录临时安装、SHA-256复核一致后原子替换，只重启一次用户服务。现有schema v3完整回滚包和自动schema v2迁移快照继续保留，本轮没有schema或配置变更。
- 正式服务保持`active/running`、`NRestarts=0`，Health/Ready为204、首页为200；About报告完整`edf4bf3...`、`buildTime=2026-08-17`和`exactSourceAvailable=true`。入口使用新`index-DSKoTdcB.js`与既有`index-Btj_g1dP.css`；配置与数据库inode分别保持`19681854`和`19679716`，未替换数据库、媒体、Manifest或缓存，启动journal未发现异常。

## 1.5-38 可管理媒体分类规则与RE2校验

### 已确认边界与数据模型

- 新增独立“媒体分类规则”，不复用Gallery根发现的MARKER/PATH_TEMPLATE/FIXED_DEPTH。规则可为全局或单一媒体库，匹配父目录段、文件名、文件stem或完整相对路径，支持Exact、Glob和Go RE2，路径统一NFC与`/`且默认不区分大小写。
- 有效规则按较小order、同order媒体库专属优先、ID依次执行并在首个命中后停止；PHOTO结果可明确阻断后续SELFIE规则。只有AVAILABLE的STATIC_IMAGE参与，动画与视频不再经过旧自拍语义判断。
- 产品数据库升级为schema v4，新增规则表与独立建议表。规则编辑递增revision；同一Item/规则/revision的接受或拒绝保持稳定，规则更新或赢家变化会把旧PENDING标记SUPERSEDED。保存规则和扫描只产生建议，只有人工接受才修改分类及Gallery revision/Manifest dirty状态。
- schema v4一次性播种启用的多语言自拍目录Exact规则及关闭的文件名Glob规则；两者均可编辑、禁用或删除，并通过显式“Restore defaults”恢复。v3迁移会把旧`DIRECTORY_SEMANTIC`自拍建议连同接受/拒绝状态复制进revision 1，避免升级后重复提示。

### 校验、管理界面与安全边界

- 后端纯规则包对名称、枚举、4000字节/100行限制、Glob语法和RE2执行编译校验；Create/Update持久化入口无条件重复校验，因此直接GraphQL调用不能绕过。Go `regexp`提供RE2线性时间边界，不调用外部命令、不遍历来源目录。
- Manage → Libraries & import新增Media classification区块：规则CRUD/作用域/优先级/匹配对象/操作符/大小写/结果编辑，单相对路径测试，最多200条现有数据库媒体预览，显式存量评估，以及显示Gallery、相对路径、规则revision和命中值的逐项/批量审核。
- RE2界面使用当前完整输入的指纹作为校验凭证；表达式、作用域、匹配对象、大小写或其他输入一旦变化即废止旧结果，当前表达式未获得后端valid响应时Save保持禁用。服务端仍作为最终安全门禁；审计只记录规则/建议ID、操作符和数量，不记录路径或模式正文。

### 自动验证与部署状态

- 纯Go规则测试覆盖多语言目录、大小写折叠、文件Glob、stem/相对路径RE2以及无效RE2/Glob；SQLite测试覆盖新库默认值、v1/v2/v3→v4在线快照迁移、CRUD revision、静态媒体边界、拒绝稳定性、规则新revision重提、默认规则删除恢复和扫描接入。
- GraphQL测试证明校验返回稳定`RULE_RE2_INVALID`且绕过前端直接Create仍无法写入；React测试锁定无效/过期RE2校验下Save禁用、当前表达式通过后才启用。产品身份/API/Server/SQLite/扫描器/Gallery/规则包Go回归全部PASS；TypeScript检查、Vitest 27个文件71项和678模块Vite生产构建全部PASS。隔离的离线Chromium完整业务生命周期/备份恢复/axe矩阵1项PASS，临时schema v4服务完成媒体库、发现、扫描、媒体处理、关系、激活与Manifest流程，未连接正式实例。
- 本阶段包含正式数据库schema v3→v4迁移；部署前必须先提交清洁源码，创建并完整校验额外回滚包；迁移后复核自动`.pre-schema-v3-*`快照、`integrity_check`、默认规则、正式服务Health/Ready/About及journal。以下部署记录确认该门禁已经完整执行。

### 提交、备份与正式增量部署

- 功能、测试和部署前记录提交为`6e61b9a6b0864a9619c74cfbe14c9f87210b33f4`（`Add configurable media classification rules`），提交后工作树清洁。正式产物使用Go 1.25.12、`cgm_web_embed cgm_galleryepic`及完整提交构建；`go version -m`确认`vcs.modified=false`，GalleryEpic Provider的Search/FetchProfile/OpenAsset均存在，二进制SHA-256为`5aa52944757232585f36effcb2f62b0f932e15be8f0b18c6f785c77bee83c7e0`。
- 2026-08-17 01:22 CST停止用户服务并确认MainPID为0，在真实备份根创建`/home/rainbowrunner/cos/bk/cgm-predeploy-20260816T172208Z-6e61b9a.tar.gz`。回滚包权限`0600`、SHA-256为`63cb688b8520c2159dcd76dbb95f7b76e285ef0b6fc66922d43271ae3ead6d3f`，包含SQLite一致schema v3快照、启动配置、Coser托管元数据、替换前二进制和systemd用户服务定义，不包含媒体来源、缓存、日志或既有备份。
- 回滚包已实际解包；数据库`integrity_check=ok`、产品身份/schema v3、55张表/1930行，解包后的旧二进制、配置、服务定义和全部Coser元数据与正式来源逐项一致。旧二进制另保留于`/tmp/cgm-before-media-classification-20260817`，SHA-256为`f34233ad63c524cb60564c7fe19ba03f628819d70343a58aaa90671d4edc7c8f`。
- 候选产物先写入正式目录临时文件并逐字节比对后原子替换；服务只启动一次，于2026-08-17 01:24:15 CST完成schema v3→v4迁移。自动快照`product.sqlite.pre-schema-v3-1786901055992698276.bak`权限`0600`、SHA-256为`a7f2842136e05904490c042dd2ed7b5d994cf84d377ce39a2b9bc6e34eaa22a0`，独立验证`integrity_check=ok`、schema v3、55张表/1930行且不含v4表。
- 迁移后主库保持原inode `19679716`，`integrity_check=ok`、schema v4、57张表/1932行。两条`system_default=1`规则已创建：`Default selfie folders`启用，`Optional selfie filenames`关闭；正式库原有`SELFIE_CATEGORY`建议为0，因此新建议表为空，迁移未丢失或重复生成建议。
- 正式服务保持`enabled/active/running`、`NRestarts=0`；Health/Ready为204，首页、Setup、Legal和Session端点为200。About报告`version=1.5.0-dev`、完整提交、`buildTime=2026-08-17`和`exactSourceAvailable=true`；入口使用`index-CvDrpo1q.js`与`index-8_Nt1C6e.css`。两个工作器以FFmpeg/FFprobe/LibRaw全部可用启动，本轮journal未检出WARN、ERROR、FAILED、panic、fatal或迁移错误。
- 配置SHA-256保持`ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`且未替换。除产品数据库前向迁移和两条默认规则外，本轮没有修改媒体、Gallery/Coser Manifest、缓存或业务配置。

## 1.5-39 媒体分类规则信息架构与中英文管理界面

日期：2026-08-18

### 问题确认

- 后端既有模型是“全局规则 + 所选媒体库覆盖规则”，并不要求为每个媒体库重复配置；但分类规则组件原先嵌在每个媒体库工作区中，新建规则又默认`Selected library`，视觉层级与默认值共同造成了“每库必须单独配置”的误导。
- 原页面还把规则作用范围、现有媒体预览/评估目标、全局默认恢复和当前库建议审核混在同一区块中；其中全局规则作用于所有库，而预览、评估与待审核建议实际以当前所选媒体库为目标，两种概念缺少明确边界。

### 实现

- 将Media classification从单个媒体库工作区提升到Libraries & import页的独立一级管理区，媒体库的根发现规则和最近发现结果仍留在所选媒体库工作区，未改变两套规则的业务边界。
- 将有效分类规则拆分为“全局规则”和“当前媒体库覆盖规则”两个并列区块；全局区明确说明自动应用于所有媒体库，单库区明确说明只是可选例外，并显示实际媒体库名称。规则标签改为`GLOBAL`/`LIBRARY OVERRIDE`语义，空状态也按范围分别说明。
- 新建分类规则默认选择“所有媒体库（推荐）”；只有用户明确选择时才创建单库覆盖。`Restore global defaults`只保留在全局规则区，避免被误认为会恢复当前库专属配置。
- 独立增加“评估目标”说明和操作区；单路径测试仍不依赖媒体库，现有媒体预览、显式评估和待审核建议均明确显示所选媒体库名称。规则作用范围与执行目标在界面上分离，但GraphQL、数据库结构、order优先级、首命中停止、建议审核及媒体写入语义均未改动。
- Libraries & import整页及分类工作流的可见文本接入既有`react-intl`消息表，覆盖媒体库新增/扫描、根发现规则CRUD、发现结果、分类范围/匹配项/校验/预览/评估/审核和成功/回退错误消息。中文环境下新建发现规则和分类规则也使用中文默认名称，不再残留英文种子文本；技术枚举、`.cosplay-root`、RE2、PHOTO/SELFIE及DRAFT保持稳定产品术语。
- 新增响应式双栏规则分组、作用范围提示、范围Badge和评估目标样式；窄屏回落为单栏且操作头部垂直排列，继续复用Manage浅色Token与既有键盘焦点规则。

### 验证与交付状态

- `corepack pnpm@10.33.0 --dir ui/web exec vitest run src/manage/MediaClassificationRules.test.tsx src/manage/ManageLibrariesPage.test.tsx`通过：2个测试文件、5项测试覆盖RE2保存门禁、根发现规则编辑删除、全局/单库分组、新规则默认全局作用域和简体中文实际渲染。
- `corepack pnpm@10.33.0 --dir ui/web run test`通过：27个测试文件、74项测试全部通过；除页面行为外，应用级回归还锁定英文与简体中文消息目录键集合完全一致；`corepack pnpm@10.33.0 --dir ui/web run check`通过TypeScript检查。
- `corepack pnpm@10.33.0 --dir ui/web run build`通过，Vite完成678模块生产构建；本阶段没有Schema、GraphQL、数据库、媒体、Manifest或缓存变更。
- 页面重构、双语消息、测试与部署前记录提交为`ef92526332ae175a5e5f6d4aba2c3ac5ed14893f`（`Clarify media classification rule scopes`）；提交后工作树清洁。
- 清洁提交以Go 1.25.12和`cgm_web_embed cgm_galleryepic`组合标签构建；`go version -m`确认`vcs.modified=false`，GalleryEpic Provider的Search/FetchProfile/OpenAsset均保留，正式候选二进制SHA-256为`023810836a04045e56eda97fe8e857ea8539555bdfea7ff53bb9f1d516c7eae0`。组合标签下`productapi`、`productserver`与`cmd/cgm`回归通过。
- 2026-08-18 22:36 CST完成本机Linux amd64增量部署。替换前正式二进制备份为`/tmp/cgm-before-classification-scope-i18n-20260818-2233`，SHA-256为`5aa52944757232585f36effcb2f62b0f932e15be8f0b18c6f785c77bee83c7e0`；候选文件写入正式目录后与构建产物逐字节一致，再原子替换`/home/rainbowrunner/.local/bin/cgm`并只重启一次用户服务。
- `cosplay-gallery-manager.service`保持`enabled/active/running`、`NRestarts=0`，Health/Ready均为204、首页为200；About精确报告`ef92526332ae175a5e5f6d4aba2c3ac5ed14893f`、`buildTime=2026-08-18T14:33:12Z`和`exactSourceAvailable=true`。新主资源`index-CYbhbCJu.js`与`index-rVfgypDI.css`均返回200。
- 启动日志仅包含正常停止、启动、2个工作器以LibRaw/FFmpeg/FFprobe全部可用启动及验证请求，没有WARN、ERROR、FAILED、panic、fatal或迁移事件。配置SHA-256部署前后均为`ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`；本阶段没有Schema变更，也未替换数据库、媒体、Manifest或缓存。

## 1.5-40 单媒体内嵌元数据与可见性管理

日期：2026-08-18

### 需求与边界

- 单媒体详情不再只显示所属Gallery关系和视频摘要；新增基础文件、描述、日期、相机、图像、视频及其他安全EXIF分组，覆盖作者、来源、程序名称、获取/数字化日期、版权、制造商、型号、光圈、曝光时间、ISO、焦距、曝光补偿、闪光灯、测光与白平衡等常用字段。
- 展示型内嵌元数据不自动写入Gallery、Coser、Work、Character或Cast业务字段，原始媒体继续只读。Browse仍不返回来源/缓存绝对路径、相对文件名或指纹。
- “尽量完整”受安全和资源上限约束：保留全部已知且可安全文本化的EXIF项，但嵌入缩略图、IFD偏移、MakerNote、未知Tag、超过64值的数组、不透明二进制块和超长值直接丢弃。单值最多1,024字符、单次最多192项。
- GPS及设备/图像唯一标识被划入独立敏感开关，产品默认关闭。Manage设置控制的是服务端GraphQL白名单，不是前端CSS隐藏；关闭项不会传到浏览器。

### 数据、任务与API实现

- 经产品确认撤销图片元数据表、schema v5和后台回填方案。产品数据库保持schema v4；扫描只保留原有BASE派生与视频技术任务，图片不排`ITEM_TECHNICAL_METADATA`，启动和调度器也不枚举存量图片。
- 保留CGM安全提取器：只读前32MiB用于尺寸/EXIF解析，丢弃嵌入缩略图、IFD偏移、MakerNote、未知Tag、过大数组和不透明二进制块，单值最多1,024字符、单次最多192项。
- `MediaDetail`继续只读产品数据库并快速返回；新增独立`mediaEmbeddedMetadata(itemUUID, visibleFields)`查询。前端在主详情到达后异步发起，图片经安全`Materializer`即时读取DIRECTORY或ZIP/CBZ当前内容，同时最多处理2项；来源或解析失败只返回稳定信息区错误状态，不使主详情失败。
- 基础文件大小/类型/CGM收录时间来自现有Item记录；图片尺寸与EXIF每次实时读取且不写数据库/缓存，视频尺寸/时长/封装/编解码/帧率继续复用现有视频技术表。服务端先复核Browse可见性，再解析内部路径，响应不含任何物理路径或指纹。
- 可见字段键由当前浏览器随独立查询发送，后端拒绝未知/重复键并在返回前过滤，因此关闭项不会进入GraphQL响应；GPS和设备/图像唯一标识仍默认关闭。

### 管理界面与验证状态

- Manage Settings保留中英双语“媒体详情元数据”区，按文件、描述、日期、相机、图像/视频和敏感信息分组提供独立复选框。选择明确保存到当前浏览器`localStorage`，不写数据库、不递增`settings_revision`、不进入完整备份；后端仍拒绝未知键和重复键。
- 单媒体详情新增中英文分组、字段名、实时读取中/读取失败/无可见字段状态，并保持长值安全换行；普通字段默认开启，GPS和唯一标识明确标记为默认关闭。
- Go目标回归覆盖真实合成JPEG的安全EXIF解析，以及改写原始图片后第二次查询立即得到新尺寸，证明未使用图片元数据持久化或缓存；productdb、productapi、productserver目标包均PASS。带`cgm_web_embed cgm_galleryepic`标签的Product API、Server与`cmd/cgm`组合回归PASS。
- `corepack pnpm run test`通过28个测试文件、77项测试；`corepack pnpm run check`通过TypeScript检查；生产构建完成679模块转换。完整`go test ./internal/...`仍只有既有原Stash `internal/api`、`internal/api/urlbuilders`和`internal/manager`因仓库明确不提供`ui/v2.5/build`旧UI嵌入目录而在setup阶段失败，本轮没有恢复旧UI绕过隔离门禁。
- 功能、测试及部署前记录提交为`2db91e94f6fcf85be60465bde94a898976d57d10`（`Read image metadata on demand`）。清洁提交以Go 1.25.12和`cgm_web_embed cgm_galleryepic`组合标签重新构建；`go version -m`确认`vcs.modified=false`，候选二进制SHA-256为`4a75dfc5edb5a20fd2406f39b7a48c95d4debd8558ac40711975715fc8cfd36a`。
- 2026-08-18 23:51 CST完成本机Linux amd64增量部署。替换前正式二进制备份为`/tmp/cgm-before-metadata-ondemand-20260818-2351`，SHA-256为`023810836a04045e56eda97fe8e857ea8539555bdfea7ff53bb9f1d516c7eae0`；候选先写入同目录并逐字节校验，再原子替换`/home/rainbowrunner/.local/bin/cgm`且只重启一次用户服务。
- `cosplay-gallery-manager.service`保持`enabled/active/running`、`NRestarts=0`；Health/Ready均为204、首页为200。About精确报告`gitHash=2db91e9`、`buildTime=2026-08-18 23:50:36`和`exactSourceAvailable=true`。
- 正式数据库身份仍为`cosplay-gallery-manager / schema 4`，`image_embedded_metadata`表数量为0且`PRAGMA integrity_check=ok`；启动日志只有正常停止、启动、2个工作器及健康验证请求，没有迁移、回填、WARN、ERROR、FAILED、panic或fatal。配置SHA-256部署前后均为`ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`，未替换数据库、媒体、Manifest或缓存。

## 1.5-41 MARKER单子目录标题保底

日期：2026-08-19

### 已确认规则与实现

- `.cosplay-root`所在父目录继续是GallerySource根；标题规则不改变发现根、媒体相对路径或后续扫描范围。
- 新文件系统发现会统计MARKER根内直属、真实且非符号链接的子目录：恰好一个时标题取该子目录名；零个或多个时继续取根目录名。只统计直属目录，普通文件和根级媒体不影响目录数量。
- 最终标题在发现快照阶段完成NFC、空值和300字符边界校验并保存为Candidate title建议；手动导入和AUTO_CREATE_DRAFT改为使用该快照值，避免预览与创建结果不一致。旧快照仍保持其发现时结果，已经导入的Gallery不会被重新计算或改名。
- 根级媒体仍由Gallery扫描的`excludeNewRootMedia=true`默认选项处理；本项不改变Exclude/Restore持久选择、媒体计数、GallerySource扫描或派生任务。MANIFEST优先级及ZIP/CBZ ARCHIVE候选流程完全不变。

### 验证与交付状态

- 产品数据库集成测试覆盖MARKER根同时含根级媒体和唯一子目录时来源根保持不变、媒体总数不变且标题取子目录；新增双直属子目录样本锁定标题继续取根目录。既有Archive两阶段导入和扫描Exclude回归继续纳入相关包测试。
- `internal/discovery`、`internal/persistence/productdb`、`internal/productapi`与`internal/productserver`回归通过；带`cgm_web_embed cgm_galleryepic`标签的Product API、Server和`cmd/cgm`组合回归通过。产品数据库全包测试同时覆盖既有Archive两阶段导入和根级媒体默认Exclude。
- 功能、测试及部署前记录提交为`fbba7e2c9e2673ae652b67d2ebd748a0d6972dd4`（`Refine marker gallery title fallback`）。清洁提交以Go 1.25.12和`cgm_web_embed cgm_galleryepic`组合标签构建；`go version -m`确认`vcs.modified=false`，正式二进制SHA-256为`f35b84abdab4d5c5a8c42745a88a37917af5d2c3d38d8d90b997bea8e0aa498d`。
- 2026-08-19 00:32 CST完成本机Linux amd64增量部署。替换前正式二进制备份为`/tmp/cgm-before-marker-title-20260819`，SHA-256为`4a75dfc5edb5a20fd2406f39b7a48c95d4debd8558ac40711975715fc8cfd36a`；候选先安装至同目录临时路径并逐字节校验，再原子替换`/home/rainbowrunner/.local/bin/cgm`且只重启一次用户服务。
- `cosplay-gallery-manager.service`保持`enabled/active/running`、`NRestarts=0`，Health/Ready均为204、首页和Session为200。About精确报告完整提交、`buildTime=2026-08-18T16:31:05Z`和`exactSourceAvailable=true`；首次健康探针发生在监听就绪前，自动重试后立即通过。
- 正式数据库仍为`cosplay-gallery-manager / schema 4`、inode `19679716`且`PRAGMA integrity_check=ok`；配置SHA-256仍为`ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`。启动日志仅有正常停止、启动、两个工作器和验证请求，没有迁移、WARN、ERROR、FAILED、panic或fatal。本项未替换数据库、配置、媒体、Manifest或缓存。

## 1.5-42 可管理媒体自动排除规则

日期：2026-08-28

### 已确认边界与数据模型

- 本功能与Gallery根发现规则及PHOTO/SELFIE媒体分类保持业务解耦，只复用新的纯路径匹配器。规则支持全局或单媒体库范围、`PARENT_FOLDER/PARENT_PATH/FILE_NAME/FILE_STEM/RELATIVE_PATH`、Exact/Glob/Go RE2、媒体类型过滤及`EXCLUDE/INCLUDE`结果；较小order优先，同order时单库规则优先，再按ID，首个命中决定。
- 第一阶段只应用于DIRECTORY扫描中新建Item；现有根目录媒体本次扫描开关继续拥有最高优先级。按路径存在或唯一完整指纹重绑定的Item保留当前Exclude状态，规则编辑、禁用、删除或普通重扫均不自动恢复或覆盖；ZIP/CBZ扫描、安全校验和成员排除行为不变。
- 产品数据库目标升级为schema v5，新增规则与决策历史表。`gallery_items.excluded`继续是当前状态事实来源；规则命中保存规则名称、revision、subject、matched value和`PENDING/APPLIED/REJECTED/SUPERSEDED/REVERSED`历史，删除规则通过`ON DELETE SET NULL`保留快照。迁移不播种规则、不扫描来源、不改变既有Item。
- v4→v5继续执行来源版本准确的SQLite Online Backup，事务内建表并更新产品身份，随后验证schema和完整性。集成测试实际重开自动快照，证明它保持有效v4、既有排除状态未变且主库新表初始为空。

### 扫描、审核与任务语义

- 扫描提交事务在1000有效成员判断前一次加载并编译来源媒体库的有效规则；同一结果用于硬上限、新Item初始`excluded`和任务排队。新规则排除Item仍创建UUID与业务记录并写APPLIED来源，但不计入1000有效成员，也不排基础派生/视频技术任务。
- 存量预览最多返回200条数据库样本；显式评估只读取AVAILABLE、当前未排除的DIRECTORY Item，并只为最终胜出的EXCLUDE建立PENDING项。接受时以Gallery metadata revision乐观锁排除Item并取消其未完成处理任务；拒绝只记录REJECTED。人工Restore把当前APPLIED历史改为REVERSED，恢复当前content revision被取消的任务并按既有规则补排。
- 新的共享`internal/mediarules`匹配器无数据库、文件系统和Shell依赖，统一NFC、正斜杠和Unicode大小写折叠；Exact/Glob最多100行、总pattern最多4,000字节，RE2单表达式由Go引擎编译。原媒体分类规则迁移到同一匹配器后仍限制原有subject与STATIC_IMAGE业务语义。

### API、管理界面与安全

- Manage GraphQL新增规则查询/校验/CRUD、单路径和媒体类型测试、单库预览、显式评估、待审核查询及接受/拒绝；预览会把当前草稿替换进完整有效规则链后统一排序，只统计该草稿成为最终胜出规则的Item。全部入口沿用所有者认证、同源/CSRF、Gallery revision和管理审计。审计只记录规则ID、枚举、revision与数量，不记录pattern、matched value或媒体路径；Browse DTO未增加字段。
- Libraries & import页在媒体分类区旁新增独立中英双语“自动排除规则”，按全局策略和当前媒体库覆盖分组，新规则默认全局。界面解释首匹配、单库优先和INCLUDE例外，提供五类subject、三类operator、媒体类型、结果、启停、编辑及删除二次确认。
- RE2必须获得当前完整输入的后端valid响应后才能保存；路径测试要求选择媒体类型。已有媒体预览和评估明确绑定当前所选库，逐项或批量操作均提示“保存/删除规则不会修改既有Item”。

### 自动验证与交付状态

- 通用匹配器测试覆盖Unicode/大小写、五类subject、Exact/Glob/RE2、无效表达式和父路径祖先语义；SQLite测试覆盖v4→v5快照、CRUD/revision/范围优先级、预览、显式审核、删除保留历史、任务取消/恢复、根开关优先、INCLUDE例外、既有Item保持、1000上限及ARCHIVE不应用。
- `internal/mediarules`、`internal/mediaexclusion`、`internal/mediaclassification`、`internal/persistence/productdb`、`internal/productapi`、`internal/productserver`、`internal/sourcescan`和`internal/manifest`目标回归全部PASS；带`cgm_web_embed cgm_galleryepic`标签的Product API、Server和`cmd/cgm`组合PASS。
- `corepack pnpm run check`通过；`corepack pnpm run test`通过29个测试文件80项；生产构建完成680模块转换。仓库级`go test ./...`中的CGM目标包均通过，但总命令仍因仓库不包含旧Stash `ui/v2.5/build`以及受限沙箱不允许`pkg/scraper`的`httptest`监听IPv6端口而失败，未把它记录为全仓通过。
- 本阶段到此只完成源码、生成代码、测试和开发记录。尚未创建提交或正式候选，也没有触碰/迁移正式schema v4数据库，没有备份、重启或部署。后续部署必须先形成清洁提交，再建立并解包验证额外完整回滚包，随后核验自动v4快照、schema v5、`integrity_check`、规则表初始状态、正式服务Health/Ready/About和journal。

### 1.5-42 部署记录

- 2026-08-28 从清洁提交 `18d6485b34cb52bc0db77dca835142c2d4714bc6` 构建并部署带 `cgm_web_embed cgm_galleryepic` 的 Linux amd64 二进制，SHA-256 为 `ab1156db14e21c6f50f7cbe91a92be6c5df65021f6e2f3a2a086854a59412825`。
- 停止服务后创建额外完整回滚包 `/home/rainbowrunner/cos/bk/cgm-predeploy-20260828T123000Z-18d6485.tar.gz`，SHA-256 为 `4aa68c94c9361a86b3422726fadc57acf34fae5510e0baec4dd15e766b3da913`；归档已实际列举校验，未包含媒体、缓存或日志。
- 启动时自动完成 schema v4→v5 迁移；`cgm_product_identity.schema_version=5`、`PRAGMA integrity_check=ok`，规则与决策表初始为空，既有 Item 状态保持不变。
- 用户服务保持 `active`、`NRestarts=0`，Health/Ready 均为 204；About 报告完整提交号与 `exactSourceAvailable=true`。journal 未发现迁移错误、WARN、ERROR、FAILED、panic 或 fatal。

## 1.5-43 Archive 内部规则与标题扩展规划

- 经现状核对，PHOTO/SELFIE 媒体分类规则已对可用静态 Archive 成员生成待审核建议；本阶段将可管理自动排除规则扩展为统一匹配 DIRECTORY 新Item和经过安全校验的 Archive 新成员。Archive 的 `.cosplay-root` 不生效，现有有效相邻 Manifest 仍是标题和实体元数据首选来源。
- 已实现 Archive 成员内部相对路径排除、无 Manifest 文件名标题保底，以及基于外部路径/文件名的唯一 Coser/Work/Character 待审核候选；复用 schema v5 的 `relative_path`，不新增作用范围字段或数据库迁移。定向产品数据库回归已通过，前端/API正式部署待后续重新构建验证。
- 2026-08-29 从清洁提交 `d9420fce4cf9584229ec5bcd31ef87e991d14b74` 完成增量部署；正式二进制 SHA-256 为 `835597af497f93042633e9742a42301432df3f8ba51d9e8b1d6a21e982639b8c`，旧二进制保存于 `/tmp/cgm-before-archive-rules-20260829`。正式标签组合、29文件80项Vitest、TypeScript检查和680模块生产构建通过。
- 服务保持 `active`、`NRestarts=0`，Health/Ready 为 204，About 精确对应源码提交；数据库保持 schema v5 且 `integrity_check=ok`，配置校验和未变，journal 未发现异常。
## 1.5-44 媒体库自动化处理（本地开发，未部署）

日期：2026-08-29

- 将批量处理设计持久化到`LIBRARY_AUTOMATION_PLAN_2026-08-29.md`，采用媒体库级`MANUAL/ASSISTED/TRUSTED`，保留全部既有人工入口和激活阻断器。
- 产品数据库版本推进至schema v6，纯新增策略表和运行摘要表；v5迁移回归确认先生成快照、不写入策略、不自动启用、不修改已有业务记录。
- 新增可重复自动化执行：确定性发现、发现规则授权的自动DRAFT、来源扫描、默认内容分级、精确唯一的一组Coser/Character关系、确定性媒体分类建议，以及TRUSTED可选激活。
- 实体匹配拒绝模糊、多候选、多关系和不完整组合；激活失败或派生资源尚未READY时保留DRAFT并计入待复核，后续运行可继续处理。
- GraphQL新增策略/预览/最近运行查询、策略保存和显式执行Mutation，成功与失败均进入无路径管理审计。
- Libraries管理页新增中英双语自动化面板。自动激活要求TRUSTED和默认分级，运行按钮只有保存非MANUAL策略后可用。
- 已通过：`go test ./internal/productapi ./internal/persistence/productdb -count=1`；`pnpm run check`；Vitest 29文件/81项；Vite 680模块生产构建。主共享JS为514.78KiB并触发既有500KiB提示，已如实列入状态文档后续拆分项。正式schema v6迁移、备份和增量部署均未执行。

### 后台批次强化

继续日期：2026-08-30

- 将GraphQL运行Mutation从同步处理改为只创建持久化QUEUED任务，新增独立自动化工作器；每批最多25个Gallery，每完成一项即保存游标和累计计数，进程中断最多重复当前这一项的幂等处理。
- 运行记录冻结入队时的策略revision及全部开关，之后编辑策略只影响下一次运行。同一媒体库由部分唯一索引限制为一个QUEUED/RUNNING任务；RUNNING任务使用owner、过期时间和心跳租约，服务正常退出主动回队列，异常过期后由新工作器接管。
- 新增QUEUED直接取消、RUNNING请求取消及Gallery边界检查；管理页在活动态每1.5秒查询进度，提供取消按钮，任务结束后停止轮询。
- 新增不含业务文本或路径的分项问题表，只记录Gallery/来源技术ID、SCAN/POLICY/ACTIVATION阶段与稳定错误码；单项失败继续推进整批。
- 现有自动扫描总开关成为计划触发总门槛：MANUAL库保持原发现/扫描；已保存ASSISTED/TRUSTED的库由计划任务入队自动化，其ACTIVE来源仍由原扫描计划对账，DRAFT来源交给策略快照处理，避免重复扫描和根级排除开关失真。
- 后端回归覆盖持久化入队/取消、同库并发拒绝、租约过期接管、策略快照、阻断项问题记录、GraphQL入队/取消和自动扫描计划入队；TypeScript通过，Vitest 29文件/82项通过，Vite 680模块构建通过。主共享JS为515.15KiB并继续触发500KiB提示。正式schema v6迁移、备份、提交和部署均未执行。

### 阶段5合成验收

日期：2026-08-30

- 新增运行中取消回归，确认已领取任务在进入发现前读取取消请求并以`CANCELLED`终止，不创建候选或Draft。
- 将租约心跳生命周期提取为独立可测边界；心跳失败以具体原因取消处理上下文，测试用90毫秒短租约可跨过原到期点而不被替代工作器错误接管。最小心跳间隔由1秒修正为10毫秒，生产30分钟租约仍按三分之一周期续期。
- 新增批次间崩溃恢复回归：首批保存2项游标后模拟下一工作器丢失，替代工作器在租约过期后接管并完成剩余3项，最终累计5项且不重算已提交游标。
- 新增独立`scripts/test-cgm-automation-gate.sh`门禁。固定4核的临时SQLite中，10,000个已发现、来源IN_SYNC的Draft按25项完成400批，夹具约165毫秒、处理约1.802秒，低于45秒门槛，因此不调整生产批量值。
- 合成门禁只覆盖队列、租约、游标、策略与逐项持久化成本。在该小节记录时尚未触碰正式数据库或服务；获授权后的schema v6部署及进程级验收结果记录于下一小节。

### schema v6部署与隔离进程验收

日期：2026-08-30

- 自动化主体以清洁提交`ed275690d5f13dd60e868557eba6ef3327953f06`构建；正式标签组合和VCS元数据验证通过。停服且MainPID为0后创建额外完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260830T125712Z-ed27569.tar.gz`，权限0600、SHA-256为`62c388e2c62e2d5fd662e82a220ca7095fc9de19eeb90f5c6fe8bba6380c60a5`。归档实际解包后，旧二进制、配置、systemd单元和Coser资源逐项一致；数据库仍为schema v5、`integrity_check=ok`及2/5/5/322业务计数。
- 首次启动生成自动快照`product.sqlite.pre-schema-v5-1788094705533130987.bak`，SHA-256为`937aafa399e01f1b1ea8aa578941ec85c55699d3efb1bf784b895f7fcdf2251d`；独立只读验证保持schema v5、无v6表、完整性及业务计数一致。正式库迁移到schema v6后仍为`integrity_check=ok`，2个媒体库、5个Gallery、5个来源和322个Item不变，新增策略/运行/问题表均为0行。
- 使用正式候选二进制、端口10099、正式库隔离副本和79MiB临时夹具完成真实进程门禁；夹具包含10,000个MARKER目录和实际PNG。发现阶段SIGKILL后，新进程在测试时间推进至租约过期后重复发现并只新增剩余8,000个Draft，最终总数10,000。
- 第二轮将10,000个来源置为NEEDS_RESCAN，在游标5801、已计数扫描5796时SIGKILL；推进隔离租约过期后，新进程从已保存游标继续并完成至10005。最终10,000项均已处理且无问题记录；扫描摘要为9999属于被SIGKILL时恰有1个来源已经提交IN_SYNC、但累计计数尚未保存，恢复时不会为日志计数重复扫描真实来源。
- 运行中取消从游标4683发起，约93毫秒后在4685成为CANCELLED。验收发现业务状态正确但旧工作器把所有`done=true`误记为COMPLETED；修复提交`ce953f24b9033855647305f7805ca00a9b344588`按COMPLETED/CANCELLED/FAILED分别记录事件，最终隔离复测约10毫秒并实际输出`CGM_LIBRARY_AUTOMATION_CANCELLED`。
- 最终正式二进制SHA-256为`c049b359fb05b040c9448de463b4b34f52e5ab46291c148e713870c40547f585`，旧schema v6二进制另存`/tmp/cgm-before-automation-log-20260830`。服务`active/running`、`NRestarts=0`，Health/Ready均为204，About精确指向`ce953f24`且`exactSourceAvailable=true`；正式数据库与配置未被隔离验收修改。
- 生产租约仍为30分钟、心跳周期10分钟。本轮实际扫描未持续超过10分钟；只有90毫秒短租约并发测试验证了续租，因此生产周期长扫描心跳仍保留为后续人工门禁，不虚报通过。正式媒体库策略继续由所有者在Manage明确启用。

## 1.5-45 TAR/7Z来源与媒体库查缺补漏（本地开发，未部署）

日期：2026-08-30

- 新增独立`internal/archivefile`顺序只读适配层，统一支持ZIP/CBZ、TAR、TAR.GZ/TGZ和7Z；发现、正式来源扫描、按需媒体物化与归档安全检查均复用该边界。纯Go 7Z运行时不调用外部命令，固实压缩流按成员顺序消费，避免反复重放。
- 归档标题保底可正确去除`.tar.gz`等复合扩展名；成员路径继续经过NFC、路径穿越、大小写碰撞、符号链接/特殊Entry、嵌套存档、容量、压缩比和图片像素门禁。密码保护的ZIP/7Z拒绝导入，不保存密码、不自动猜测密码。
- 产品数据库目标推进到schema v7，纯新增发现快照覆盖摘要和诊断表，不改变Gallery、来源、Item、规则、自动化策略或Manifest。每次媒体库发现保存普通文件、松散媒体、支持/不支持存档、CGM控制文件和其他忽略文件计数；查询时同时显示当前已登记Gallery来源、已入库Item和等待扫描来源数量，便于对照“发现”与“实际入库”进度。
- “查缺补漏”诊断记录未匹配Gallery根的媒体目录、不支持的存档格式、密码保护、损坏/不可读、未通过安全校验和不含受支持媒体的存档，并保留Manage所有者可见的定位路径。发现遇到单个损坏或加密存档时不再中止整个媒体库扫描。
- Libraries & import页面新增中英双语覆盖摘要和待校核路径清单；现有候选和未分配媒体界面继续保留。本阶段只完成源码与测试，尚未提交、备份、迁移正式schema v6数据库或增量部署。
- 定向Go回归通过`archivefile/archivecheck/sourcescan/mediaaccess/productdb/productapi/productserver`，并通过`cgm_web_embed cgm_galleryepic`标签组合；schema v6→v7测试实际创建并核验来源准确的`pre-schema-v6`快照，主库新表为空。TypeScript检查与Vitest 29文件/82项通过，Vite生产构建转换680模块；主共享JS约517.96KiB，保留既有大于500KiB提示。
- 新增运行时编译依赖后使用项目确定性生成器更新Third-Party Notices与SPDX 2.3应用清单，共67项依赖。该记录不代表已完成正式发行或部署验证。

### schema v7提交与增量部署

- 功能、测试和部署前记录提交为`28e71a7a4c095b095b58d9c742ccc48941d30769`（`Add archive formats and library coverage reporting`）。清洁提交以Go 1.25.12及`cgm_web_embed cgm_galleryepic`标签构建，`go version -m`确认VCS revision一致且`modified=false`；正式二进制SHA-256为`07cc18ecbe5611a812170e753b8e48db2224adbbfc6d851a43556d8cce6f6729`。
- 2026-08-30 22:35 CST停止用户服务并确认`MainPID=0`，创建额外完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260830T142000Z-28e71a7.tar.gz`，权限0600、SHA-256为`2e2cb02cca5d2227bd1b30d74c5355418b739a4030733d7b42bd4ad29d68ff7b`。归档包含一致schema v6数据库、旧二进制、启动配置、systemd单元和Coser托管资源，不包含媒体、缓存或日志；实际解包后数据库、文件和Coser资源逐项一致。
- 首次启动生成自动快照`product.sqlite.pre-schema-v6-1788100566661475656.bak`，权限0600、SHA-256为`bc3330368f500efe20118ba18bec3a21aca2f22e51491f73b6fcd0148a38e6f8`；独立验证保持schema v6、62张表、`integrity_check=ok`和2/5/5/322业务计数。正式库迁移到schema v7后为64张表、`integrity_check=ok`，媒体库/Gallery/来源/Item仍为2/5/5/322，新增覆盖摘要和诊断表均为空。
- 候选文件逐字节校验后原子替换`/home/rainbowrunner/.local/bin/cgm`，旧二进制保存在`/tmp/cgm-before-archive-coverage-20260830`。服务只启动一次，保持`active/running`、`NRestarts=0`，Health/Ready为204，Root/Legal/Session为200，About精确指向`28e71a7`且`exactSourceAvailable=true`。配置SHA-256继续为`ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`；部署日志未发现迁移、WARN、ERROR、FAILED、panic或fatal。

## 1.5-46 Archive内置Gallery根与校核语义修复

日期：2026-08-31

- 真实媒体库排查确认两个可正常读取、分别含72和74个媒体成员的TAR被统计为支持的Archive，却因没有命中MARKER/PATH_TEMPLATE/FIXED_DEPTH而进入未分配目录诊断；自动化运行因此得到`candidates=0`，没有创建Draft。问题位于发现阶段共用了DIRECTORY根匹配器，不是TAR解析、扫描任务或前端刷新失败。
- 安全且含受支持媒体的ZIP/CBZ、TAR、TAR.GZ/TGZ和7Z现在以内置`ARCHIVE_FILE`识别方式直接成为独立Candidate，来源根就是Archive文件本身。有效相邻Manifest继续使用`MANIFEST`优先级；PATH_TEMPLATE可继续提供待审核元数据建议，但不再决定Archive能否成为Gallery根。
- 已绑定/忽略来源继续优先；明确的DIRECTORY Candidate拥有完整子树并删除其内部Archive Candidate，保持Gallery不嵌套。加密、损坏、不安全或不含受支持媒体的Archive保留覆盖诊断，超限Archive保留受阻Candidate；它们都不会自动导入。
- 手工发现默认只产生可点击“创建草稿”的Archive Candidate。媒体库自动化策略新增默认关闭的`auto_import_archives`；只有已保存的ASSISTED/TRUSTED运行会冻结该授权并在发现阶段自动创建Archive Draft，MANUAL和既有策略不会因升级改变。自动化预览把已授权Archive计入可自动创建数量。
- 产品数据库目标推进到schema v8：重建候选及建议表以增加准确的`ARCHIVE_FILE`枚举，并给策略和运行快照增加默认0字段。v7→v8集成测试实际创建、重开和校验来源准确的`pre-schema-v7`快照；历史候选模型、Gallery、来源、Item、Manifest和媒体文件均不发生业务迁移。
- Libraries & import将“发现覆盖率待处理项”与“Gallery自动化待复核”改为不同文案；未分配列表明确只表示散装媒体目录，安全Archive在Candidate区显示。真正未分配目录可点击“准备精确目录规则”，界面只预填并启用精确PATH_TEMPLATE，默认不自动建Draft，仍需所有者检查并保存。
- 验证通过：目标Go包`archivefile/archivecheck/discovery/productdb/productapi/productserver/cmd/cgm`；schema v7→v8、无规则Archive、显式自动导入授权和DIRECTORY嵌套优先回归；带`cgm_web_embed cgm_galleryepic`正式标签组合；TypeScript；Vitest 29文件83项；Vite生产构建680模块。主共享JS约518.74KiB并保留既有大于500KiB提示。

### 提交、备份、schema v8迁移与增量部署

- 功能、测试与部署前记录提交为`2c4bd3a24da56365d180682501966ddb17a629d3`（`Recognize archive files as gallery roots`）。清洁提交使用Go 1.25.12和`cgm_web_embed cgm_galleryepic`构建；`go version -m`确认revision一致且`vcs.modified=false`，正式二进制SHA-256为`1f5addf1b1b30c7538d8eabd61a4c88fcd773aa956af775394987a77f9e53b9d`。
- 2026-08-31 21:22 CST停止用户服务并确认`MainPID=0`，创建0600额外完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260831T132134Z-2c4bd3a.tar.gz`，SHA-256为`8c1d236e5c4dacabe511bd883fd3ace027a8f7c0bfb1cc6fea6e902be05ae5c3`。包含一致schema v7数据库、旧二进制、启动配置、systemd单元和Coser托管资源，不含媒体、缓存或日志；实际解包后文件与Coser树逐项一致，源库与备份库均为`integrity_check=ok`及2/5/5/322业务计数。
- 首次启动生成自动快照`product.sqlite.pre-schema-v7-1788182665735158828.bak`，权限0600、SHA-256为`ce3b13e2e535c18e960171c7255dd4aefb8104a7e652828ec848dadbb4ef155e`；独立只读校验保持schema v7、`integrity_check=ok`和2/5/5/322。正式库迁移到schema v8后仍为`integrity_check=ok`且业务计数不变；现有1条自动化策略的`auto_import_archives=0`，升级没有暗中开启存档自动导入。
- 旧二进制保存于`/tmp/cgm-before-archive-root-v8-20260831`；候选文件逐字节校验后原子替换，服务只启动一次并保持`active/running`、`NRestarts=0`。Health/Ready为204，Root/Legal/Session为200；About精确指向`2c4bd3a`且`exactSourceAvailable=true`。配置SHA-256继续为`ca5c8aa1c695546cfe95689f6491ee3ef841d08098114cc27a410958b95dd82d`，本次启动journal未检出WARN、ERROR、FAILED、panic或fatal。
- 为避免验收改写正式发现快照，从停服备份制作隔离数据库副本，再以真实媒体库路径执行完整发现。两个TAR分别生成72与74成员的`ARCHIVE_FILE / PENDING`候选，`has_conflict=0`且`over_limit=0`；证明原先的“未分配媒体”问题已在真实输入上修复，且正式业务库没有因验收扫描产生新快照。

## 1.5-47 核心实体Aliases受控输入修复

日期：2026-08-31

- 实际业务测试确认Coser的Aliases输入框无法输入英文斜杠或空格。原因是输入框每次`onChange`都立即执行`split("/")`、`trim`、删除空项和`join(" / ")`；用户刚输入的分隔符尚未有右侧名字就被删除，名字内部正在输入的空格也会被trim。
- EntityForm现在编辑期保留原始Alias文本，只在Create/Save提交边界按`/`拆分、清理两端空格并忽略空项；后端仍收到原有字符串数组，数据模型、GraphQL和schema均不变。Coser、Work、Character和Tag共用该修复。
- 输入框新增可见格式提示与多语言示例；定向Vitest覆盖`Komachi / こまち / 小 丁`完整保留及提交解析，3项通过，TypeScript检查通过。
- 修复及部署前记录提交为`f415c1a1a11c087fd68cb0a9767cf9a3f4403b2d`（`Fix core entity alias editing`）。完整Vitest 29文件84项、TypeScript和680模块生产构建通过；清洁提交以Go 1.25.12及`cgm_web_embed cgm_galleryepic`构建，`vcs.modified=false`，二进制SHA-256为`af7232e77ebef6b331313b5055e0ed6cb93878cbe8426e8dbc8fbfcb4ada92d8`。
- 2026-08-31完成无schema增量部署；替换前二进制保存于`/tmp/cgm-before-alias-input-20260831`，新候选逐字节校验后原子替换并只启动服务一次。服务保持`active/running`、`NRestarts=0`，Health/Ready均为204，About精确指向`f415c1a`且`exactSourceAvailable=true`。正式库保持schema v8和`integrity_check=ok`，本次启动journal未检出WARN、ERROR、FAILED、panic或fatal。

## 1.5-48 Coser管理列表头像与Banner完善状态

日期：2026-08-31

- Coser管理列表每页已有的`ManageCoreEntityFields`本就返回`avatarURL`与`bannerURL`，因此本轮不新增API查询、DTO、数据表或schema。只在COSER行前增加40px固定圆形头像；已设置头像用现有revision化认证URL、`loading=lazy`和异步解码，未设置时显示等尺寸浅灰空白占位，文件加载失败时使用独立错误色而不留破损图标。
- 每行右侧增加人物与图片两个低干扰小图标，分别表示Avatar与Banner；绿色表示已设置，灰色表示缺失，琼珀色表示Avatar URL存在但资源加载失败。中英文title和`aria-label`明确说明具体Coser及状态，不使用姓名首字母伪装成已有头像。Work/Character/Tag保持旧列表布局。
- 定向Vitest新增“完整头像+Banner”与“两者均缺失”对照，校验真实图片、空白占位、双状态及可访问名称；ManageCoreEntitiesPage 4项与TypeScript检查通过。
- 功能、测试和部署前记录提交为`8170e92c232dc75928ba08e5c0e5fe06c3739e19`（`Show Coser image completeness in management`）。完整Vitest 29文件85项、TypeScript和680模块生产构建通过，仅保留既有主共享包超过500KiB提示。清洁提交以Go 1.25.12及`cgm_web_embed cgm_galleryepic`构建，`vcs.modified=false`，二进制SHA-256为`a9163ff447fe8d64245a8f11b68e7b10d72bbbee90ffe5db5bb99fd2861e3c71`。
- 2026-08-31完成无schema增量部署；旧二进制保存于`/tmp/cgm-before-coser-completeness-20260831`，候选逐字节校验后原子替换并只启动服务一次。服务保持`active/running`、`NRestarts=0`，Health/Ready均为204，About精确指向`8170e92`且`exactSourceAvailable=true`；入口资源为`index-BfrLW-Cd.js`与`index-DfSgBTN-.css`。正式库保持schema v8、`integrity_check=ok`，配置、媒体、Manifest和缓存均未替换；本次启动journal未检出WARN、ERROR、FAILED、panic或fatal。

## 1.5-49 Coser创建前同名/别名复核

日期：2026-08-31

- 保留“Coser可以真实同名、UUID决定身份”的已确认模型，不新增名称唯一索引或schema迁移。新增Manage专用`manageCoserNameConflicts`查询，后端使用与核心实体一致的NFC、首尾清理和Unicode大小写折叠，对所有现有Coser主名及Alias做精确匹配；不执行子串、简繁、标点或罗马音推断。
- 查询最多返回10个审核候选，每项包含已有Coser编辑DTO、实际命中值和关联Gallery数。前端在新建Coser的Name停止350ms后查询，展示头像、名称、Aliases、匹配值、UUID末8位和Gallery数，并可直接打开已有实体。
- 命中时Create按钮保持禁用，直到所有者勾选“已复核且确认为不同人物”；查询失败时也需要显式承认后才能继续，避免网络/API错误静默绕过复核。修改Name会立即清除上次确认；无精确命中时正常创建。
- 定向回归已通过：后端覆盖`Straße/STRASSE`的Unicode折叠、Alias命中、子串不误报、Gallery计数与上限；GraphQL覆盖认证查询数据；前端覆盖禁用Create、显式确认和打开已有实体。`productdb/productapi`、ManageCoreEntitiesPage 5项和TypeScript检查通过。
- 功能、测试和部署前记录提交为`4e1e43e9830e55c51821f8aefecd3875c12ceba1`（`Add Coser duplicate identity review`）。完整前端回归为29文件86项，TypeScript与680模块生产构建通过，仅保留既有主共享包超过500KiB提示；相关Go包和`cgm_web_embed cgm_galleryepic`正式标签组合均通过。清洁提交以Go 1.25.12构建，VCS revision一致且`modified=false`，正式二进制SHA-256为`74ffa8ca423faac040e5ca21843976734d7e3496c2ecbec7fb057c93cd52ee90`。
- 2026-08-31完成无schema增量部署。旧二进制保存于`/tmp/cgm-before-coser-duplicate-20260831`，SHA-256为`a9163ff447fe8d64245a8f11b68e7b10d72bbbee90ffe5db5bb99fd2861e3c71`；候选逐字节校验后原子替换并只启动服务一次。服务保持`active/running`、`NRestarts=0`，Health/Ready均为204，About精确指向`4e1e43e`且`exactSourceAvailable=true`；入口资源为`index-CxHtflRY.js`与`index-CzquyQXG.css`。正式数据库保持schema v8、`integrity_check=ok`及2个媒体库、6个Gallery、6个来源、322个Item；启动journal未检出迁移、WARN、ERROR、FAILED、panic或fatal。

## 1.5-50 Coser管理列表大数据量交互

日期：2026-09-01

- 保留Coser默认30项服务端分页和左侧列表/右侧编辑器结构，不采用会丢失编辑定位的无限滚动。`manageCoreEntities`新增可选`pageSize/query/coserAssetFilter`参数；默认参数维持既有Coser 30项及其他核心实体60项行为，页量只接受30/60/100。
- 名称查询在数据库分页和总数计算前统一匹配主名称、Sort name及Alias；图片完善度直接使用Coser已持久化的`avatar_path/banner_path`是否为空，支持缺头像、缺Banner、任一不完整和全部完整，不读取资源文件、不生成缓存。该功能只作用于Manage认证边界，不改变Browse人物索引。
- Coser列表顶部新增300ms防抖全库搜索、完善度筛选、页量选择和一键清除；底部新增结果区间、首页/上一页/页码输入/下一页/末页。工具和分页保持在滚动区域上下端，提供中英文文案、加载/失败/空结果状态及可访问标签。
- `q/assets/pageSize/page/uuid`均保存在URL；搜索和筛选变化重置到第1页，选择、新建、合并、删除以及重复身份复核的直达入口保留当前列表条件。没有新增数据表、索引、写入任务或schema迁移。
- 定向验证通过：产品数据库覆盖Alias搜索、完整度组合、60/100页量及非法参数；GraphQL覆盖认证参数和返回总数；ManageCoreEntitiesPage 6项覆盖URL状态恢复、结果区间和末页直达；TypeScript检查通过。
- 完整验证通过：前端29文件87项、TypeScript及680模块Vite生产构建；`productdb/productapi/productserver/cmd/cgm`普通组合和带`cgm_web_embed cgm_galleryepic`正式标签组合。主共享JS约522.95KiB，仅保留既有大于500KiB提示。
- 功能、测试和部署前记录提交为`fb1f989ee5c29054fbbd460139f47e02b94d1cf8`（`Improve Coser management navigation`）。清洁提交以Go 1.25.12和`cgm_web_embed cgm_galleryepic`标签构建，VCS revision一致且`modified=false`；正式二进制SHA-256为`851cb8b572d89502786ec1fd58452c5412c5fe311c0bd5b4f0cae40a98a74799`。
- 2026-09-01完成无schema增量部署。旧二进制保存于`/tmp/cgm-before-coser-navigation-20260901`，SHA-256为`74ffa8ca423faac040e5ca21843976734d7e3496c2ecbec7fb057c93cd52ee90`；候选逐字节校验后原子替换并只启动服务一次。服务保持`active/running`、`NRestarts=0`，Health/Ready均为204，About精确指向`fb1f989`且`exactSourceAvailable=true`；入口资源为`index-CpiIQcmi.js`与`index-Yl_GFxuJ.css`。正式数据库保持schema v8、`integrity_check=ok`及2个媒体库、6个Gallery、6个来源、322个Item；启动journal未检出迁移、WARN、ERROR、FAILED、panic或fatal。

## 1.5-51 Manage Coser中英文与拼音排序

日期：2026-09-01

- 原Manage Coser使用SQLite默认二进制文本顺序`COALESCE(NULLIF(sort_name,''),name),uuid`，中文按UTF-8编码位置而非拼音排列，英文大小写也不属于同一自然字母序。
- Coser专用Manage分页现先取得已通过名称/Alias搜索和图片完善度筛选的轻量`uuid/name/sort_name`集合，使用项目既有`golang.org/x/text/collate`及固定`zh-CN` Unicode Collation不区分大小写排序，再截取当前30/60/100项页面；只对页面内人物读取完整编辑DTO。
- 英文和中文拼音进入同一字母序；验证样例为`Aardvark(Sort name覆盖) / Alice / bob / 李四 / 小丁 / 张三`。人工Sort name继续最高优先，明确用于多音字、人名特殊读音或自定义位置；同序时按显示名称、原始排序文本和UUID稳定破同序。
- 固定语言标签使结果不依赖Linux locale或部署机器设置；不新增第三方依赖、数据列、索引、回填任务或schema迁移。Manage工具区新增中英文排序说明，Browse人物索引和其他核心实体排序保持不变。
- 定向验证通过：产品数据库新增中英文混排、大小写、中文拼音和Sort name覆盖回归；`productdb/productapi`、ManageCoreEntitiesPage 6项和TypeScript检查均通过。
- 完整验证通过：前端29文件87项、TypeScript及680模块Vite生产构建；`productdb/productapi/productserver/cmd/cgm`普通组合和带`cgm_web_embed cgm_galleryepic`正式标签组合。主共享JS约523.17KiB，仅保留既有大于500KiB提示。
- 功能、测试和部署前记录提交为`8123ab668acfe9450120178feb5d99ca235a9159`（`Sort managed Cosers by Pinyin`）。清洁提交以Go 1.25.12和`cgm_web_embed cgm_galleryepic`标签构建，VCS revision一致且`modified=false`；正式二进制SHA-256为`0e580dc662d7c21c610400dbdb7740ac8e6fee8ccf7a23b7f7466a56f7cf96c3`。
- 2026-09-01完成无schema增量部署。旧二进制保存于`/tmp/cgm-before-coser-pinyin-20260901`，SHA-256为`851cb8b572d89502786ec1fd58452c5412c5fe311c0bd5b4f0cae40a98a74799`；候选逐字节校验后原子替换并只启动服务一次。服务保持`active/running`、`NRestarts=0`，Health/Ready均为204，About精确指向`8123ab6`且`exactSourceAvailable=true`；入口资源为`index-4FAUgmfD.js`与`index-Bb_578R2.css`。正式数据库保持schema v8、`integrity_check=ok`及2个媒体库、6个Gallery、6个来源、322个Item；启动journal未检出迁移、WARN、ERROR、FAILED、panic或fatal。

## 1.5-52 GalleryEpic零账号候选黑屏修复

日期：2026-09-01

- 真实日志与只读审计确认服务始终`active/running`且Provider请求返回200。Coser`seya-狮砸`连续4次预览均成功取得头像和Banner，`account_count=0`，随后没有Apply事件；故障不是GalleryEpic超时、后端失败、GraphQL或拼音排序，也没有产生不完整资料写入。
- 根因是`PreparedPreview.Accounts`为空时经Go nil切片复制后JSON输出`accounts:null`。前端先`setPreview(value)`，再调用`value.accounts.filter`触发被异步catch捕获的异常；已排队的Preview随后进入渲染并调用`preview.accounts.map`，由于网络资料面板没有错误边界，异常清空整个React页面。
- `publicPreview`现使用非nil空切片，零账号稳定输出`[]`；搜索服务同样把nil候选规范为`[]`。前端搜索候选和预览账号在进入状态前均以`Array.isArray`标准化，兼容旧版本、代理缓存或异常Provider响应。
- `ManageCoserMetadataImport`外层新增以Coser UUID重置的局部React错误边界；未预见的渲染异常只替换网络资料Panel为中英文错误和重试按钮，Coser表单、图片管理及生命周期功能不再随之黑屏。边界只写浏览器控制台技术事件，不向服务端发送候选或人物资料。
- 定向验证通过：后端锁定空账号公开JSON为`accounts:[]`；前端新增`accounts:null`仍显示头像/Banner导入和空账号列表回归，并用畸形数据证明局部边界保留页面。`cosermetadata/productserver`、ManageCoserMetadataImport 4项和TypeScript检查通过。
- 完整验证通过：前端29文件89项、TypeScript及680模块Vite生产构建；`cosermetadata/productdb/productapi/productserver/cmd/cgm`普通组合和带`cgm_web_embed cgm_galleryepic`正式标签组合。主共享JS约523.56KiB，仅保留既有大于500KiB提示。
- 功能、测试和部署前记录提交为`9aee6ac2d45288e8145613155f64acda33550a6b`（`Prevent empty Coser metadata preview crashes`）。清洁提交以Go 1.25.12和`cgm_web_embed cgm_galleryepic`标签构建，VCS revision一致且`modified=false`；正式二进制SHA-256为`587457b293dcf0b34c95cea54418172aad79140f940cd526b2cbef63d4e8b581`。
- 2026-09-01完成无schema增量部署。旧二进制保存于`/tmp/cgm-before-metadata-null-fix-20260901`，SHA-256为`0e580dc662d7c21c610400dbdb7740ac8e6fee8ccf7a23b7f7466a56f7cf96c3`；候选逐字节校验后原子替换并只启动服务一次。服务保持`active/running`、`NRestarts=0`，Health/Ready均为204，About精确指向`9aee6ac`且`exactSourceAvailable=true`；入口资源为`index-Dgv8eFKL.js`与`index-CU533R9Y.css`。正式数据库保持schema v8、`integrity_check=ok`及2个媒体库、6个Gallery、6个来源、322个Item；启动journal未检出迁移、WARN、ERROR、FAILED、panic或fatal。

## 1.5-53 Coser网络资料搜索完成反馈

日期：2026-09-02

- 对已部署服务的只读排查确认进程保持`active/running`且没有重启。20:52:14、20:52:19、20:52:27和20:52:32的四次`/manage/coser-metadata/search`均为HTTP 200，耗时343～409ms、响应18字节，对应规范的`{"candidates":[]}`；同一会话此前多次搜索和候选准备成功，证明Provider通路与解析器整体可用。日志按隐私约束不记录查询词，因此不能仅凭服务日志判断具体名称本应命中与否。
- 根因是前端只在`candidates.length > 0`时渲染候选区；成功零候选不会设置提示，也不是异常，现有错误消息因此不出现，最终表现为点击搜索后页面毫无反馈。
- 搜索现保存本次已规范化查询词和完成状态；成功后有结果显示候选数量，零结果显示中英文明确提示并建议尝试完整名称、Alias或更短的特征名称。修改查询词或Provider会立即清除旧候选、Preview及完成状态，防止旧结果与新条件混淆；请求期间继续使用既有`正在搜索…`状态，失败继续进入既有错误反馈。
- 服务新增`CGM_COSER_METADATA_SEARCH_COMPLETED`结构化事件，只包含request ID、Provider key和candidate count，不记录Coser姓名、查询词、候选姓名或来源URL。该变更不改变Provider搜索算法、候选排序、人工审核/应用边界、数据库或schema。
- 定向验证通过：ManageCoserMetadataImport 5项覆盖成功空结果可见性，`internal/productserver`及TypeScript检查通过。完整验证通过：前端29文件90项、TypeScript、680模块生产构建，以及`cosermetadata/productdb/productapi/productserver/cmd/cgm`正式`cgm_web_embed cgm_galleryepic`标签组合；仅保留既有主共享包超过500KiB提示。
- 功能、测试和部署前记录提交为`d94a45ea057df49d9590d49218170c2d4fa5b2dd`（`Show Coser metadata search outcomes`）。清洁提交使用Go 1.25.12和`cgm_web_embed cgm_galleryepic`构建，Go VCS信息确认revision一致且`modified=false`。
- 2026-09-02完成无schema增量部署。首次候选虽功能与VCS信息正确，但未注入产品About字段，验收发现`exactSourceAvailable=false`后立即以同一清洁提交和正式ldflags重建并替换；缺元数据的临时候选保存在`/tmp/cgm-search-feedback-missing-build-metadata-20260902`，没有作为最终交付。最终正式二进制SHA-256为`1b6ef274d1576a095eb88a615592bb81715f529a80195bf525a0c54434498d34`，原`9aee6ac`二进制保存在`/tmp/cgm-before-search-feedback-20260902`。
- 最终服务保持`active/running`、`NRestarts=0`，Health/Ready均为204，Root/Legal/Session为200；About精确指向`d94a45e`、`buildTime=2026-09-02T13:10:04Z`且`exactSourceAvailable=true`。入口资源为`index-ByfZSH2f.js`与`index-DaCBCKeP.css`。正式数据库保持schema v8、`integrity_check=ok`及2个媒体库、6个Gallery、6个来源、322个Item；配置、数据库、媒体、Manifest和缓存均未替换，最终启动journal未检出WARN、ERROR、FAILED、panic或fatal。

## 1.5-54 Work、Character与Tag管理列表交互

日期：2026-09-02

- 排查确认`manageCoreEntities`及产品数据库早已支持所有核心实体按主名称、Sort name和Alias执行全库包含搜索，也统一接受30/60/100页量；但React页面用`coserOnly`把查询词、页量选择和工具栏全部限制在Coser，Work、Character和Tag只能固定60项翻页。
- 三类列表现与Coser共用双语工具栏：300ms防抖搜索、名称/Sort name/Alias提示、排序规则说明、30/60/100项页量和清除条件；底部分页统一显示结果区间、首页、前页、页码输入、后页和末页。`kind/q/pageSize/page/uuid`保存在URL，切换实体类别重置旧类别条件，选择和编辑实体不丢失当前列表上下文。头像/Banner完善度继续只显示在Coser。
- 原Work、Character与Tag使用SQLite二进制`ORDER BY`后分页，中文不按拼音。管理查询现对四类实体统一读取轻量`uuid/name/sort_name`候选，以固定zh-CN Unicode Collation不区分大小写排序后再分页；人工Sort name优先，名称、原始键和UUID稳定破同序。这样保证跨页顺序正确且不依赖宿主机locale，不改变Browse索引。
- 不新增GraphQL字段、数据表、索引、回填任务或数据库schema。定向验证通过：产品数据库覆盖三类实体的Alias搜索、30/100页量、英文/中文拼音和Sort name覆盖；ManageCoreEntitiesPage 7项覆盖Work工具栏、URL参数、页量、排序说明及分页，TypeScript检查通过。完整验证通过：前端29文件91项、TypeScript、680模块生产构建，以及`productdb/productapi/productserver/cmd/cgm`正式`cgm_web_embed cgm_galleryepic`标签组合；仅保留既有主共享包超过500KiB提示。
- 功能、测试和部署前记录提交为`92209236507f51115caa81eafe44293a547f9870`（`Improve core entity list navigation`）。清洁提交使用Go 1.25.12及正式标签构建，Go VCS信息确认revision一致且`modified=false`；最终二进制SHA-256为`9f1a6b96a7c9c02a44573534fb4bb079aae30f73d37674afc35e8ff2ba3e9d30`，旧`d94a45e`二进制保存在`/tmp/cgm-before-entity-navigation-20260902`。
- 2026-09-02完成无schema增量部署。服务保持`active/running`、`NRestarts=0`，Health/Ready均为204，Root/Legal/Session为200；About精确指向`9220923`、`buildTime=2026-09-02T13:32:22Z`且`exactSourceAvailable=true`。入口使用主资源`index-DAOrDXWT.js`、页面分块`ManageCoreEntitiesPage-UESjSQZQ.js`及`index-DaCBCKeP.css`。正式数据库保持schema v8、`integrity_check=ok`及2个媒体库、6个Gallery、6个来源、322个Item；配置、数据库、媒体、Manifest和缓存均未替换，启动journal未检出WARN、ERROR、FAILED、panic或fatal。

## 1.5-55 Work与Character Alias气泡编辑器

日期：2026-09-02

- 现有控件并非弹窗，而是Coser、Work、Character和Tag共用的普通单行输入框；用户必须用`/`分隔，无法直接识别、编辑或删除某一个Alias。本轮按需求只修改Work与Character，Coser和Tag继续使用已确认的斜杠输入及提交解析。
- 新控件在同一输入边框内混排Alias气泡和文本输入：输入后按Enter生成气泡；点击气泡正文会从集合移除并把原文退回输入框，编辑后再次Enter即可重建；右侧独立叉号立即删除该气泡。正文与叉号使用两个并列button而非嵌套交互元素，并提供中英文可访问名称。
- 输入法合成阶段的Enter不触发提交；空文本不生成气泡。前端以NFC与不区分大小写检查当前集合重复，保留后端更严格的Unicode标准化最终门禁；单项沿用300字符、集合沿用100项上限。输入框仍有未回车文本时显示明确提示并禁用Create/Save，防止用户误以为该文本已进入Alias数组。
- 气泡编辑只改变现有GraphQL `aliases: [String!]!`提交前的前端呈现，不新增接口、字段、表、迁移或回填。定向验证通过：ManageCoreEntitiesPage 8项覆盖Work的生成、待提交禁用、正文回填编辑、重新生成与叉号删除，并确认Character使用同一新控件；TypeScript检查通过。完整验证通过：前端29文件92项、TypeScript、680模块生产构建，以及`productdb/productapi/productserver/cmd/cgm`正式`cgm_web_embed cgm_galleryepic`标签组合；仅保留既有主共享包超过500KiB提示。
- 功能、测试和部署前记录提交为`a29eaaa61ccfcc5d3a8ad394753370c62196e33f`（`Add Alias chips for Works and Characters`）。清洁提交使用Go 1.25.12及正式标签构建，VCS revision一致且`modified=false`；最终二进制SHA-256为`6dc671c3872fd29fdc14e5a4686f7a199c5382d8d35e86f0c3721a182fe7f071`，旧`9220923`二进制保存在`/tmp/cgm-before-alias-chips-20260902`。
- 2026-09-02完成无schema增量部署。服务保持`active/running`、`NRestarts=0`，Health/Ready为204，Root/Session为200；About精确指向`a29eaaa`、`buildTime=2026-09-02T14:07:21Z`且`exactSourceAvailable=true`。入口使用主资源`index-DVo45rwQ.js`、页面分块`ManageCoreEntitiesPage-BJ-Xl0qs.js`与`index-CCgdgV2C.css`。正式数据库保持schema v8、`integrity_check=ok`及2个媒体库、6个Gallery、6个来源、322个Item；配置、数据库、媒体、Manifest和缓存均未替换，启动journal未检出WARN、ERROR、FAILED、panic或fatal。

## 1.5-56 Alias气泡连续编辑切换

日期：2026-09-02

- 业务复测发现输入框已有待提交文本时，上一版将所有气泡正文按钮设为disabled，以防编辑文本被覆盖；这保护了数据但阻断了用户直接切换编辑对象的预期交互，叉号仍可用但正文无法点击。
- 气泡正文不再因输入框非空而禁用。点击目标气泡时，控件在一次状态变更中移除目标、把当前待提交文本生成新气泡，再将目标原文放入输入框并保持焦点；因此可连续编辑多个Alias而不必每次手工按Enter。
- 切换前仍执行NFC/大小写重复检查：若当前文本与其他未点击气泡重复，集合和输入框均保持不变并显示错误；若当前文本只与被点击目标等价，则删除目标气泡并保留单一输入副本，避免制造重复。100项上限在一进一出的切换中保持不变。
- 定向ManageCoreEntitiesPage 8项已覆盖“编辑FGO期间点击Fate Series，自动生成Fate Grand Order并把Fate Series退回输入框”的完整链路，TypeScript和diff检查通过。完整验证通过：前端29文件92项、TypeScript、680模块生产构建，以及`productdb/productapi/productserver/cmd/cgm`正式标签组合；仅保留既有主共享包超过500KiB提示。
- 功能、测试和部署前记录提交为`ffcc25540c454c56f4481dc552b2c1e27f9d24c0`（`Allow continuous Alias chip editing`）。清洁提交使用Go 1.25.12及正式标签构建，VCS revision一致且`modified=false`；正式二进制SHA-256为`e15785dbf1281038e69e42eb937ba498ddd8123eb20962e1d07fc50bc6f89c0a`，旧`a29eaaa`二进制保存在`/tmp/cgm-before-alias-switch-20260902`。
- 2026-09-02完成无schema增量部署。服务保持`active/running`、`NRestarts=0`，Health/Ready为204，Root/Session为200；About精确指向`ffcc255`、`buildTime=2026-09-02T14:20:19Z`且`exactSourceAvailable=true`。入口使用主资源`index-I48NmJXz.js`、页面分块`ManageCoreEntitiesPage-CwXv8K-z.js`与`index-1hXP2LRJ.css`。正式数据库保持schema v8、`integrity_check=ok`及2个媒体库、6个Gallery、6个来源、322个Item；配置、数据库、媒体、Manifest和缓存均未替换，启动journal未检出WARN、ERROR、FAILED、panic或fatal。

## 1.5-57 Work与Character可拔除名称资料Provider

日期：2026-09-03

- 本轮按已确认范围为Work和Character增加人工网络名称导入，不建设通用运行时插件系统。`internal/entitymetadata`只定义实体类型、候选、Alias建议、Provider注册表和15分钟预览令牌；令牌绑定Provider、实体类型、目标UUID和候选引用，公开响应不暴露绑定字段。Provider不能访问数据库、Manifest或产品Server。
- 首个`internal/entitymetadata/moegirl`适配器只知道`zh.moegirl.org.cn`、公开搜索路由和HTML结构。安全客户端禁用环境代理，只允许固定HTTPS主机与公共解析地址，并限制超时、重定向、Content-Type和8MiB响应。提取按“原名/官方译名/本名/外文名/别名/别号”等字段语义兼容flex信息框和表格，排除删除线、黑幕/隐藏、脚注与ruby注音；适配当前页面正文位于`MOE_SKIN_TEMPLATE_BODYCONTENT`的结构。
- 主程序仅在`cgm_moegirl`标签下注册适配器，默认核心构建不导入该包；完整CGM、Docker、Linux双架构脚本、发布校验与SBOM枚举已加入该标签。独立配置`entity_metadata_scraping_enabled`默认`false`；禁用时Manage路由不存在，启用但未编译Provider时返回`providers: []`。删除适配器不需要数据库迁移，既有Alias仍是普通本地数据。
- Work/Character管理页提供双语Provider、名称、Character可选作品上下文、候选数量/零结果、来源链接和字段级复核。已有名称不可重复选择；官方/原名类可默认勾选，常用名/别号默认不勾选。本地实体表单存在未保存修改时禁止应用，避免外部revision更新覆盖正在编辑的文本；面板有独立错误边界。
- 集成复核发现旧气泡组件仍把Work/Character Alias数组临时拼成`/`分隔文本，合法名称`Fate/stay night`会在下一次保存时被错误拆分。本轮把两类气泡的编辑状态改为真实`string[]`并增加斜杠名称回归；Coser/Tag仍保留已确认的斜杠分隔普通输入框，后端Alias数组契约不变。
- 应用API再次校验实体类型/UUID、当前revision、令牌绑定以及每个选中值属于预览；数据库层只把新值合并进Alias并复用`UpdateWork`/`UpdateCharacter`，因此保留300字符/100项/NFC重复规则、revision递增和关联Gallery Manifest dirty传播，不改主名称、Sort name或Character所属Work。本轮不新增表、字段、Schema版本或回填任务。审计不记录查询、名称、Alias、URL或页面正文。
- 定向Go测试覆盖空Provider数组、预览绑定/过期、非法选项注入、旧revision、规范化重复、Work Alias合并和站点安全解析；合成HTML覆盖flex/table/隐藏与ruby结构。另用只存于`/tmp`的当前公开“鸣潮”、搜索结果和“碧蓝航线:大凤”页面完成只读兼容校核，均通过，页面正文未加入仓库。
- 完整前端测试30文件95项、TypeScript和681模块生产构建通过；产品Go包在`cgm_web_embed cgm_galleryepic cgm_moegirl`正式标签组合下通过并成功生成验证二进制，Provider构建边界检查通过。`go test ./...`中CGM相关包通过；旧Stash目标仍因未生成的`ui/v2.5/build`失败，旧`scraper`测试在受限沙箱中因禁止监听IPv6 loopback失败，均与本轮CGM变更无关。
- 功能源码提交为`1de0b952d746d45bdf6f0656aacebcd0e49c91ee`（`Add removable Work and Character metadata provider`）。以Go 1.25.12和`cgm_web_embed cgm_galleryepic cgm_moegirl`从清洁提交构建，`go version -m`确认revision一致且`vcs.modified=false`；正式二进制SHA-256为`723d93361cb8098e443aaf3249d07573380d6fc9c00bbc5a4363a38735574db6`。
- 2026-09-03停服后创建并实际解包复验0600完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260903T114606Z-1de0b95.tar.gz`，SHA-256为`1c14863f779c9ece9cbc9b53cde5318fa7d167932af08c4922330a683b4ef296`。包内schema v8数据库、旧二进制、旧配置、systemd单元和Coser托管资源均与正式来源一致，不含媒体、缓存或日志。
- 私有正式配置显式启用`entity_metadata_scraping_enabled`，并保留既有Coser资料开关；配置SHA-256变为`fee095a8642c53278838475549251a48956d3058cd50fc652e4d0b392b877899`。服务只启动一次并保持`active/running`、`NRestarts=0`；Health/Ready为204，Root/Legal/Session为200，About精确对应源码且`exactSourceAvailable=true`。数据库保持inode `19679716`、schema v8、`integrity_check=ok`及2个媒体库、6个Gallery、6个来源、322个Item、135个Coser、4个Work和5个Character；启动journal无WARN、ERROR、FAILED、panic或fatal。

## 1.5-58 Work与Character创建前重复名称复核

日期：2026-09-03

- 复用Coser已确认的创建前复核语义：仅在新建实体时对去首尾空格后的Name执行350ms防抖查询，按项目统一`normalizedKey`规则进行NFC与Unicode大小写折叠，并同时检查主名称和Alias；不做模糊、前缀或包含匹配。编辑既有实体不触发本流程。
- 新增认证Manage GraphQL查询`manageCoreEntityNameConflicts`，只接受WORK或CHARACTER及1～20结果上限。响应返回完整实体、实际匹配值、关联Gallery数、Character所属Work和是否命中主名称；Work主名称因既有schema没有规范化列而按Coser同类方式安全比较，Alias及Character主名称使用既有规范化索引，不新增schema。
- Work精确命中时，Create保持禁用，直到所有者查看名称、Aliases、UUID尾号和关联Gallery数并明确确认这是不同Work；打开已有实体可直接切换到其编辑器。查询失败也必须明确承认后才能继续，避免静默跳过检查。
- Character会展示所有Work中的精确命中及各自所属Work。跨Work主名称或任意Alias命中属于人工复核项，确认不同角色后可继续；若所选Primary Work内已有相同主名称，则与数据库既有唯一约束一致，不能以确认框绕过，Create持续禁用并提示打开已有角色或更名。切换Primary Work会清除先前确认。
- 定向数据库测试覆盖Work Alias、Character主名/Alias、跨Work上下文、大小写/空格规范化、非精确值和非法Kind；API测试覆盖关联Work与主名命中标识。前端完整30文件97项、TypeScript、681模块生产构建，以及`productdb/productapi/productserver/ui/web/cmd/cgm`正式三标签组合测试通过；仅保留既有主共享包超过500KiB提示。
- 功能与部署前记录提交为`1c71ad84bcaa552c2efb2030e795203db62552c3`（`Add Work and Character duplicate review`）。清洁提交以Go 1.25.12和`cgm_web_embed cgm_galleryepic cgm_moegirl`构建，`go version -m`确认revision一致且`vcs.modified=false`；正式二进制SHA-256为`7f5a8ef6a602aa249276d90d753488a24c314640c410bb9b850f365ddc92fa56`。
- 停服后创建并实际解包复验0600完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260903T122433Z-1c71ad8.tar.gz`，SHA-256为`234814d52733d33ed244a9ebc6e586204e8c5c6a3365905a6de9a97120a14775`。包内schema v8数据库、旧二进制、配置、systemd单元和Coser托管资源均与正式来源一致，不含媒体、缓存或日志。
- 2026-09-03完成无schema增量部署，只原子替换正式二进制，配置SHA-256继续为`fee095a8642c53278838475549251a48956d3058cd50fc652e4d0b392b877899`，数据库inode保持`19679716`。服务只启动一次并保持`active/running`、`NRestarts=0`；Health/Ready为204，Root/Legal/Session为200，About精确对应源码。正式库`integrity_check=ok`并保持2个媒体库、6个Gallery、6个来源、322个Item、135个Coser、16个Work和5个Character；启动journal无WARN、ERROR、FAILED、panic或fatal。

## 1.5-59 Character关联Work名称恢复

日期：2026-09-03

- 排查确认数据库关系仍完整，问题位于Manage展示链路：`ManageCoreEntity`对Character只暴露`workUUID`，没有可显示的Work名称；`EntityForm`又把`ManageEntitySelector.name`固定传为空字符串，并在选择后只保存UUID。因此已有关联重新打开后和新选择后都无法稳定显示名称，只能看到UUID输入框。
- 产品数据库Manage读取现按Character的外键实时取得当前Work名称，并通过认证GraphQL `ManageCoreEntity.workName`只读返回；没有复制或持久化冗余名称，不新增表、字段、schema或回填。Work重命名后下一次读取自然显示新名称。
- Character表单把`workName`传给选择器，并在选择Work时同时更新本地UUID与名称；Gallery关系编辑器选择Character时也使用选项随附的`workName`，避免保存前短暂退化为仅角色名。手工UUID编辑仍清空无法验证的旧名称，防止显示与UUID不一致。
- 回归覆盖选择Work后立即显示名称、打开既有Character后恢复名称，以及API/数据库返回当前所属Work。后端定向测试、前端30文件98项、TypeScript、681模块生产构建和正式三标签产品测试通过；仅保留既有主共享包超过500KiB提示。
- 功能与部署前记录提交为`6e76fad5dc071598cf7a2e65d01199f2560783a2`（`Restore Character Work names in management`）。清洁提交以Go 1.25.12和`cgm_web_embed cgm_galleryepic cgm_moegirl`构建，`go version -m`确认revision一致且`vcs.modified=false`；正式二进制SHA-256为`e863633ea480a631613be7e4cfeaae05c3f46935387331dfe3159a799c5ad642`。
- 停服后创建并实际解包复验0600完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260903T131441Z-6e76fad.tar.gz`，SHA-256为`4524a05337a170859b62bd5d55ac2ce7d1f0a7641deef475dd71948380527a1c`。包内schema v8数据库、旧二进制、配置、systemd单元和Coser托管资源均与正式来源一致，不含媒体、缓存或日志。
- 2026-09-03完成无schema增量部署，只原子替换正式二进制；配置SHA-256继续为`fee095a8642c53278838475549251a48956d3058cd50fc652e4d0b392b877899`，数据库inode保持`19679716`。服务只启动一次并保持`active/running`、`NRestarts=0`；Health/Ready为204，Root/Legal/Session为200，About精确对应源码。正式库`integrity_check=ok`并保持2个媒体库、6个Gallery、6个来源、322个Item、135个Coser、99个Work和6个Character；启动journal无WARN、ERROR、FAILED、panic或fatal。

## 1.5-60 Work内嵌Character管理

日期：2026-09-04

- 保留现有Character独立管理入口、URL与Primary Work选择流程，不改变已经确认的实体模型。已保存Work的原编辑区域新增“作品属性/关联角色”顶部页签；未保存Work必须先创建，因此不会产生缺少Work身份的临时Character。
- 关联角色页顶部持续显示当前Work主名称；其下以固定7.5rem高度、可纵向滚动的气泡区展示全部直属Character和总数，避免角色数量增加时挤占表单空间。点击气泡进入该Character编辑，活动项有明确状态；“新建角色”恢复空白表单。
- Work内入口将Primary Work显示为锁定值而非选择器，创建Mutation始终从当前Work注入UUID，省去逐个搜索和选择Work的重复操作，也不能意外关联到其他Work。名称、Sort name、Alias气泡、350ms精确查重、同Work主名称硬阻断、revision保存、可拔除网络名称导入、合并与删除仍复用原Character能力。
- 后端新增认证只读查询`manageWorkCharacters(workUUID)`。产品数据库先确认Work存在，再只读取`characters.work_uuid`匹配项；返回当前Work名称，并复用管理列表的英文名称/中文拼音排序和Sort name人工覆盖。该查询没有写入能力，不向Browse开放，不新增数据库字段、表、索引、schema版本、回填任务或第二套Character所有权关系。
- 回归覆盖不同Work不能串项、Work名称/UUID恢复、拼音排序与不存在Work错误；GraphQL覆盖认证关系响应。前端覆盖页签切换、固定名称、关联气泡、锁定Work、查重就绪、自动注入Work UUID、创建及列表刷新。定向`productdb/productapi`、ManageCoreEntitiesPage 12项和TypeScript通过；完整前端30文件99项、681模块生产构建及`cgm_web_embed cgm_galleryepic cgm_moegirl`正式标签产品测试通过，仅保留既有主共享包超过500KiB提示。
- 功能与部署前记录提交为`949c3208d72340d76d0b1433516a51f45cc4464e`（`Add Work-scoped Character management`）。清洁提交以Go 1.25.12和`cgm_web_embed cgm_galleryepic cgm_moegirl`构建，`go version -m`确认revision一致且`vcs.modified=false`；正式二进制SHA-256为`3366a0ebd40bc96a7661bab3c879b03686ddc5c9fb2de50c872a7ed077687250`，构建时间为`2026-09-04T01:05:43Z`。
- 停服后创建并实际解包复验0600完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260904T010841Z-949c320.tar.gz`，SHA-256为`37867b75809e58d0de5a45665ecba064beaf32da85ac105d16108977f6139dab`。包内schema v8数据库、旧二进制、配置、systemd单元和Coser托管资源均与正式来源一致，不含媒体、缓存或日志；正式、暂存和解包数据库均为`integrity_check=ok`且计数一致。
- 2026-09-04完成无schema增量部署，只原子替换正式二进制；配置SHA-256继续为`fee095a8642c53278838475549251a48956d3058cd50fc652e4d0b392b877899`，数据库inode保持`19679716`。服务只启动一次并保持`active/running`、`NRestarts=0`；Health/Ready为204，Root/Legal/Session为200，About精确对应源码且`exactSourceAvailable=true`。正式库保持2个媒体库、6个Gallery、6个来源、322个Item、135个Coser、99个Work和12个Character；2个工作器及FFmpeg/FFprobe/LibRaw正常启用，启动journal无WARN、ERROR、FAILED、panic或fatal。

## 1.5-61 独立Character列表显示所属Work

日期：2026-09-05

- 独立Character管理列表原先只显示角色主名称和Aliases（无Alias时为UUID），虽然编辑表单已经能恢复Primary Work名称，但用户必须逐项打开才能区分不同作品中的同名或近似角色。
- Character列表行现增加第三层“Work/所属作品”文本并提高到正文对比度；名称和Aliases/UUID的既有层级、点击选择、分页、搜索及英文/中文拼音排序均不改变。显示值直接使用现有Manage实体响应的`workName`，若连接旧响应则回退`workUUID`，最终才显示占位符。
- 变更只作用于独立Character列表；Work、Coser、Tag列表以及上一阶段Work内嵌Character气泡区不变。不新增GraphQL字段、数据库字段、表、schema版本或回填任务。
- 定向ManageCoreEntitiesPage 12项回归确认列表和编辑表单同时显示Work名称；完整前端30文件99项、TypeScript及681模块生产构建通过，仅保留既有主共享包超过500KiB提示。
- 功能与部署前记录提交为`7de86e78c568b8779d4ae07519a3ec7efdcfec71`（`Show Work names in Character list`）。清洁提交以Go 1.25.12和`cgm_web_embed cgm_galleryepic cgm_moegirl`构建，`go version -m`确认revision一致且`vcs.modified=false`；正式二进制SHA-256为`8ef94cb55ea3f51c69f03b1858313177758281d0782df9e832b336216eb9ead1`，构建时间为`2026-09-04T17:09:35Z`。
- 停服后创建并实际解包复验0600完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260904T171031Z-7de86e7.tar.gz`，SHA-256为`94e8b119b6be6ac3703e0990969371aefbfdfcdffd902e48ca56a28991565bd2`。包内schema v8数据库、旧二进制、配置、systemd单元和Coser托管资源逐项一致，不含媒体、缓存或日志；正式、暂存与解包数据库均为`integrity_check=ok`并保留42个Character。
- 2026-09-05完成无schema增量部署，只原子替换正式二进制；配置SHA-256继续为`fee095a8642c53278838475549251a48956d3058cd50fc652e4d0b392b877899`，数据库inode保持`19679716`。服务只启动一次并保持`active/running`、`NRestarts=0`；Health/Ready为204，Root/Legal/Session及本轮Character管理分块和样式资源均为200，About精确对应源码且`exactSourceAvailable=true`。正式库保持2个媒体库、6个Gallery、6个来源、322个Item、135个Coser、99个Work和42个Character；启动journal无WARN、ERROR、FAILED、panic或fatal。

## 1.5-62 独立Character列表紧凑布局修正

日期：2026-09-05

- 业务复核发现所属Work作为第三层文本使每个Character列表项由原有两行增至三行，降低长列表的信息密度；本轮保持功能目标但修正信息排布，不改变已经确认的实体关系。
- Character列表项改为左右两区：左侧仍是主名称与Aliases/UUID两行，右侧与其垂直居中，仅显示Work名称本身，不再渲染“Work:”或“所属作品：”前缀。作品名和左侧文本均保持单行省略，列表项继续使用原有padding和两行内容高度。
- 变更仅涉及独立Character列表的React标记与CSS；Work内嵌Character气泡、编辑表单Primary Work、Coser/Work/Tag列表、查询接口、数据库和schema均不变。
- 回归新增Character列表专属布局类及无前缀断言；完整前端30文件99项、TypeScript和681模块生产构建通过，仅保留既有主共享包超过500KiB提示。
- 功能与部署前记录提交为`7d15ed32fc59e70b6f8fc95af1a9fe48fea9d435`（`Keep Character list rows compact`）。清洁提交以Go 1.25.12和`cgm_web_embed cgm_galleryepic cgm_moegirl`构建，`go version -m`确认revision一致且`vcs.modified=false`；正式二进制SHA-256为`72761317a7a908c3e9539dd02c8e477f90b17f4c3b50c8358522d4e61a8b6c81`，构建时间为`2026-09-04T17:25:32Z`。
- 停服后创建并实际解包复验0600完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260904T172737Z-7d15ed3.tar.gz`，SHA-256为`c943e760eb23124180e4bcb7ca119e814a6b524ee0382d8eda5cd09314a9323a`。包内schema v8数据库、旧二进制、配置、systemd单元和Coser托管资源与正式来源逐项一致，不含媒体、缓存或日志；正式、暂存和解包数据库均为`integrity_check=ok`并保持业务计数一致。
- 2026-09-05完成无schema增量部署，只原子替换正式二进制；配置SHA-256继续为`fee095a8642c53278838475549251a48956d3058cd50fc652e4d0b392b877899`，数据库inode保持`19679716`。服务只启动一次并保持`active/running`、`NRestarts=0`；Health/Ready为204，Root/Legal/Session、本轮Character管理分块和样式资源均为200，About精确对应源码且`exactSourceAvailable=true`。正式库保持2个媒体库、6个Gallery、6个来源、322个Item、135个Coser、99个Work和42个Character；2个工作器及LibRaw、FFmpeg、FFprobe正常启用，启动journal无WARN、ERROR、FAILED、panic或fatal。

## 1.5-63 Character跨Work迁移与合并归属

日期：2026-09-05

- 此前独立Character表单允许选择其他Primary Work，但`updateCoreEntity`会以`Character Work cannot be changed by this update`拒绝；这既缺少实际迁移能力，也使UI与后端行为不一致。Work内嵌Character编辑继续锁定当前Work，避免该快捷入口意外跨边界。
- 独立Character编辑现将Primary Work变化视为受保护迁移：首次Save只打开双语确认窗口，明确显示原Work→目标Work、UUID/Slug身份保持、Gallery Cast保留、Manifest待同步和目标同名阻断规则；必须输入`MOVE`才提交。名称、Sort name、Alias与Work变更在同一个现有`updateCoreEntity`请求内原子保存，不会先写一半。
- 数据库`UpdateCharacter`现接收目标Work UUID，在事务内先检查Character revision、目标Work的active便携UUID身份，以及目标Work内规范化主名称唯一性；随后同时更新`work_uuid`与名称字段、重建Alias、递增revision，并按Character UUID把有关Gallery Manifest从`CLEAN`置为`DB_DIRTY`。失败回滚全部内容；冲突返回可操作的`Character name already exists in target Work`，不再退化为Internal Server Error。
- 成功迁移使用独立审计事件`CHARACTER_WORK_MOVE`，摘要持久化previous/target Work UUID；普通Character元数据更新仍记为`CORE_ENTITY_UPDATE`。没有新增GraphQL字段、数据库字段、表、索引、schema版本或回填任务。
- Character跨Work合并沿用既有安全合并事务并明确语义：目标Character及其Work始终为最终权威，源Gallery Cast改指目标Character，源名称/Alias合入目标，源UUID与Slug成为永久重定向；重复Gallery Cast仍是必须先处理的阻断项。合并审计新增`target_work_uuid`，Character目标搜索结果、预览和永久确认窗口均展示最终Work。
- 数据库回归覆盖成功迁移、UUID/Slug稳定、revision门禁、目标Work同名冲突全量回滚，以及跨Work合并后Gallery Cast经目标Character解析到目标Work并保留源UUID重定向；GraphQL回归覆盖迁移响应和审计路径。前端覆盖`MOVE`确认门禁、迁移提交、目标Work显示与跨Work合并说明。产品数据库/API测试、完整前端30文件101项、TypeScript、681模块生产构建和`cgm_web_embed cgm_galleryepic cgm_moegirl`正式标签编译测试通过，仅保留既有主共享包超过500KiB提示。
- 功能与部署前记录提交为`82cd53b8f26e8ef27f70a877bd8b7c09211fa378`（`Add Character Work migration`）。清洁提交以Go 1.25.12和正式三标签构建，`go version -m`确认revision一致且`vcs.modified=false`；正式二进制SHA-256为`dfb191477f20b63b7a83d6cbb91c2219fa67742d3c6bc8fb01519cfa43888661`，构建时间为`2026-09-06T07:16:41Z`。
- 2026-09-06 15:18 CST停服并确认`MainPID=0`后创建、列举、解包复验0600完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260906T071811Z-82cd53b.tar.gz`，SHA-256为`fa9b2c5304b47f6e3cad3c3d535694336f71c76fb503d21913d8cbd1d2d59c15`。包内schema v8数据库、旧二进制、配置、systemd单元和Coser托管资源与正式来源逐项一致，不含媒体、缓存或日志；暂存及解包数据库均为`integrity_check=ok`且计数一致。
- 本次无schema迁移，只原子替换正式二进制并只启动服务一次。正式配置SHA-256和inode保持`fee095a8642c53278838475549251a48956d3058cd50fc652e4d0b392b877899`/`19679670`，数据库inode保持`19679716`；停服一致快照与部署后数据库均为schema v8、`integrity_check=ok`及2个媒体库、6个Gallery、6个来源、322个Item、135个Coser、99个Work、403个Character。服务保持`active/running`、`NRestarts=0`，Health/Ready为204，Root/Legal/Session及新JS/CSS资源为200，About精确对应源码且`exactSourceAvailable=true`；启动journal无WARN、ERROR、FAILED、panic或fatal。

## 1.5-64 可移植核心目录与Gallery重建长期方案

日期：2026-09-07

- 完整复核开发备忘录、第一版计划、当前实施状态和迁移交接记录后，确认长期异机迁移不应继续把核心实体、媒体根绝对路径和Gallery运行状态绑定为一个不可拆分数据库恢复动作；同时现有完整备份必须保留，继续承担同机回滚与整套灾难恢复。
- 新增长期专项方案`PORTABLE_CATALOG_AND_GALLERY_REBUILD_PLAN_2026-09-07.md`。方案把数据划分为全局Portable UUID身份账本、Coser/Work/Character/Tag核心目录、媒体侧Gallery Manifest、可选所有者连续性状态和机器本地绑定五层；核心目录包不保存媒体绝对路径、Session、任务、缓存、日志或凭据。
- 明确Gallery Manifest v1已经保存`set_id`、核心实体引用以及可选`item_uuid/link_uuid`，但旧Manifest缺UUID、目标UUID跨kind占用、同名异UUID、Character所属Work冲突、重复set_id和首次Item UUID接管仍需专门的预检/冲突流程，不能按名称自动合并或重生成UUID掩盖问题。
- 全种类身份账本中的Gallery/Item/Link在业务行重建前只进入受包摘要约束的待接管声明；所有UUID分配必须检查声明，Manifest验证通过后再原子注册业务身份，避免预写Registry形成孤儿ACTIVE记录。
- 规划按确定性只读导出与inspector、空库核心目录导入、媒体映射与Gallery重建、非空库Merge冲突工作台、可选所有者连续性状态、产品化恢复演练六个实施阶段推进；第一切片不导入正式数据库、不新增schema、不部署、不改变Gallery Manifest。
- 本轮仅修改开发文档，没有业务代码、数据库、配置、媒体、Manifest、构建产物或正式服务变化；因此只执行文档结构、链接和Git差异检查，不宣称产品测试或部署通过。

## 1.5-65 可移植元数据阶段1只读导出与离线检查

日期：2026-09-07

- 按长期方案完成阶段0数据边界与阶段1首个纵向切片。新增站点无关`internal/portablecatalog`，格式`cgm-portable-metadata`与数据库schema、Gallery/Coser Manifest和媒体处理版本独立；固定包含`package.json`、`checksums.json`、`identity-ledger.json`、`core-catalog.json`、`gallery-index.json`及可选`coser-assets/`。
- 数据库层在单一只读事务中导出全部Portable UUID Registry、Alias和Tombstone，以及Coser/Work/Character/Tag/SocialAccount、Alias、核心Slug重定向、Character→Work、Tag DAG；Gallery索引只保存导出内逻辑媒体库key、名称、库内相对来源、set_id和已有Manifest同步摘要，不保存媒体库根、来源路径或Manifest绝对路径。
- Coser帐号Link作为结构化目录数据导出；当前头像/Banner只记录包内引用，并由Server通过既有`coserasset.OpenManaged`逐级拒绝符号链接、复核普通文件和inode后持有文件描述符写入包。原始资源每项限制20MiB，派生480/960/1600图、已替换未引用资源和Coser Manifest同步文件不进入此切片。
- 写包按UUID和稳定关系顺序规范化NFC并生成确定性JSON；每个有效载荷文件记录大小、种类及SHA-256，`package.json`绑定`checksums.json`摘要，完成后再次运行离线Inspector并计算整包SHA-256。目标使用同目录0600临时文件原子发布且拒绝覆盖已有文件。
- 离线Inspector不加载启动配置或打开数据库，严格拒绝未知/缺失Entry、路径穿越、绝对/反斜杠路径、非NFC、大小写碰撞、控制/平台保留字符、Windows保留名、符号链接/目录、异常总量/单项/压缩比、摘要或计数不符、JSON未知字段/多值、UUID kind冲突、Alias断链/环、孤儿ACTIVE核心身份、Character Work错误、Tag环及缺失/未引用Coser资源。
- Server默认发现任一Gallery Manifest不是`CLEAN`或来源为`UNBOUND/OUTSIDE_LIBRARY`时阻断导出；CLI只有显式`-allow-incomplete-portable`才生成带计数警告的包。`-export-portable`要求交互式所有者重新认证，`-inspect-portable`可完全离线使用；成功/失败审计不记录目标路径、实体名称、帐号URL或业务正文。
- 本切片不增加数据库表、字段或schema版本，不提供导入/身份声明，不修改Gallery/Coser Manifest、媒体、缓存、配置或正式服务，也没有增量部署。后续应先完成真实正式库的只读导出预检与大规模流式内存门禁，再进入空业务数据库导入。
- 单元/集成回归覆盖相同快照字节级确定性、帐号与头像引用、Payload篡改、平台保留名、未知UUID kind、核心关系与Slug历史、数据库/根路径排除、安全原图导出、不生成派生图、默认Gallery警告阻断及显式覆盖。以下正式构建标签组合测试通过：

```bash
GOMAXPROCS=2 GOTOOLCHAIN=local \
  GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  /tmp/cgm-go1.25.12/bin/go test \
  -tags 'cgm_web_embed cgm_galleryepic cgm_moegirl' \
  ./internal/portablecatalog ./internal/persistence/productdb \
  ./internal/productapi ./internal/productserver ./cmd/cgm
```

- 同一正式标签组合的`go build -o /tmp/cgm-portable-stage1-check ./cmd/cgm`通过，验证二进制可执行且报告`Cosplay Gallery Manager 1.5.0-dev`。未运行前端测试，因为本切片没有前端代码或GraphQL schema变更；未执行正式业务库导出、部署、导入、百万Item身份账本内存门禁或真实跨机重建，不得据此标记这些项目通过。

## 1.5-66 可移植导出只读预检与大库安全门禁

日期：2026-09-07

- 阶段1继续补齐正式导出前的独立只读预检。新增需所有者交互式重新认证的CLI入口`-preflight-portable`；它检查当前数据库及Coser托管原件，但不创建ZIP、不修改数据库业务状态、Manifest、媒体、资源或缓存。只输出身份kind/state、核心目录、Gallery覆盖、资源数量/字节及稳定问题码，不输出姓名、帐号URL、媒体标题或绝对路径。
- 数据库预检在一个只读事务内使用聚合SQL统计全部Portable UUID，并验证ACTIVE核心/Gallery/SocialAccount身份和业务对象双向对应、Alias端点与kind、Tombstone kind、Character→Work、SocialAccount→Coser、Tag边与DAG、核心Slug重定向。Gallery只按库内相对路径判定`CLEAN/UNBOUND/OUTSIDE_LIBRARY`，按问题类别计数；资源引用沿用既有Manifest相对路径规则。
- Server通过`coserasset.OpenManaged`逐级拒绝符号链接并锁定当前文件描述符，再复用离线Inspector的静态JPEG/PNG/WebP、20MiB、50MP和动画拒绝规则。预检结果及成功/失败审计只含计数和稳定码。正式导出先复用同一预检，阻断项永远不能由`-allow-incomplete-portable`绕过；完成一致性快照后仍二次核对Gallery警告，避免预检与快照间变化被静默放过。
- 现有Writer/Inspector仍会把身份账本JSON载入内存，且单JSON上限128MiB。为防止尚未完成的百万身份导出导致OOM或生成中途失败，当前身份总数超过400,000时返回`PORTABLE_IDENTITY_LEDGER_REQUIRES_STREAMING`硬阻断；该门禁是临时安全边界，不代表删除百万身份，也不改变包格式。下一阶段必须实现流式Writer/Inspector后才能解除。
- 扩展已有可选百万Item门禁：10,000 Gallery、1,000,000 Item、合计1,010,400身份的503,365,632字节合成数据库构建后，完整预检耗时1.704秒、累计分配1,245,080字节，低于5秒/64MiB门禁，并正确返回流式能力阻断；原有Browse/Manifest全部性能子门禁同期通过。
- 常规`portablecatalog/productdb/productserver/cmd`定向测试通过；正式`cgm_web_embed cgm_galleryepic cgm_moegirl`标签下的`portablecatalog/productdb/productapi/productserver/cmd`测试、相关Go Vet及`/tmp/cgm-portable-preflight-check`二进制构建通过，二进制报告`Cosplay Gallery Manager 1.5.0-dev`。本轮没有前端或GraphQL变化，因此不重复运行前端测试。尚未在正式业务数据库执行预检/导出，未提交、未部署、未进入空库导入。

## 1.5-67 百万身份账本流式导出与检查

日期：2026-09-08

- 阶段1解除上一切片为内存型JSON设置的临时400,000身份阻断。产品数据库新增显式生命周期的`PortableCatalogReader`：在同一只读事务中先读取受控规模的核心目录、关系和Gallery索引，再按UUID顺序逐条读取Registry/Alias/Tombstone；导出完成后提交只读事务，失败或取消统一Rollback。原有小规模快照API保留为兼容包装，但正式Server导出不再调用它加载全量身份。
- `cgm-portable-metadata`格式版本和`identity-ledger.json`对象/数组结构不变。流式Writer逐条校验UUID、kind、state、时间和严格顺序，直接编码到ZIP Entry并同步累计SHA-256、未压缩字节及记录数；核心目录、Gallery索引、Coser资源、确定性顺序、原子目标发布和完成后整包复验规则不变。
- 离线Inspector不再以128MiB内存读取身份JSON，而是使用`json.Decoder`逐条解析并写入当前用户临时目录中的0600短期SQLite索引。索引校验全局UUID唯一性、kind/state、Alias目标、跨kind/退休目标、链终点和环；只把Coser/Work/Character/Tag/SocialAccount/Gallery对象身份保留在Go内存供既有对象关系验证，百万Item/ExternalLink身份不驻留。临时索引在成功、格式失败、摘要失败、取消或数据库错误路径均关闭删除，不写产品数据库。
- 身份Payload独立上限调整为最多20,000,000条，并继续受10GiB整包未压缩大小、逐Entry摘要、压缩比和ZIP安全边界约束；核心目录与Gallery JSON仍保留128MiB限制，Coser资源门禁不变。`-allow-incomplete-portable`依然只能覆盖Gallery清单警告，不能覆盖身份、关系、资源或安全错误。
- 新增20,006身份流式回归，确认Inspector只返回6条核心/Gallery对象身份而不保留20,000条Item；新增恶意Item Alias环回归，确认磁盘索引拒绝循环。既有内存Writer、确定性包、篡改、路径及资源测试继续兼容。百万性能Fixture中的旧`.000Z`测试时间改为与正式数据库一致的规范RFC3339表示，避免测试数据本身被新流式校验正确拒绝。
- 重新运行10,000 Gallery、1,000,000 Item、1,010,400身份门禁：493,797,376字节合成数据库的只读预检耗时1.768秒、分配1,242,576字节；完整“同一事务读取→ZIP写入→离线Inspector”耗时12.227秒，`identity-ledger.json`未压缩129,278,548字节，采样峰值Go堆增长20,311,192字节，低于90秒/128MiB门禁；原有全部Browse与Manifest性能子门禁继续通过。
- 本轮不新增产品表、字段、索引或schema版本，不实现导入、声明或Web界面，不修改Manifest、媒体、缓存、配置或正式服务。正式`cgm_web_embed cgm_galleryepic cgm_moegirl`标签下`portablecatalog/productdb/productapi/productserver/cmd`测试、相关Go Vet及`/tmp/cgm-portable-stream-check`构建通过，二进制报告`Cosplay Gallery Manager 1.5.0-dev`；没有前端/GraphQL变化，未重复前端测试。尚未提交、部署或在正式业务库执行导出。

## 1.5-68 可移植元数据阶段2空库核心目录导入

日期：2026-09-08

- 按长期迁移方案进入阶段2。产品数据库schema由v8前向升级至v9；迁移前沿用现有自动一致性快照，新增`portable_import_sessions`和`portable_identity_claims`，并让维护状态识别`PORTABLE_IMPORTING`。会话绑定导入ID、导出ID、隔离包相对位置、整包SHA-256、格式版本和对象计数；Gallery、GalleryItem及ExternalLink在业务来源尚未重建时只保存生命周期声明，不写入普通Registry。
- 待接管UUID具有`PENDING→CLAIMING→CLAIMED`单向状态机，源kind/state/目标/时间字段不可变；Registry触发器阻止任何普通创建流程抢占`PENDING` UUID，后续Gallery重建必须先在同一事务进入`CLAIMING`，再注册Registry及业务行并完成`CLAIMED`。这避免导入后出现没有Gallery/Item/Link业务对象的伪ACTIVE身份。
- 新增需所有者重新认证并输入确认词`IMPORT`的`-import-portable <absolute.zip>`。Server只接受正常维护状态和空业务数据库，先把源包复制到Backup根下0600隔离目录，复用完整离线Inspector并再次流式读取身份；随后自动创建并登记完整`SAFETY_SNAPSHOT`，确保schema迁移后的空目标环境仍有可恢复基线。非空目标返回明确拒绝，不进入Merge语义。
- 核心Registry、Coser/Work/Character/Tag/SocialAccount、Alias、Character→Work、Tag DAG、帐号关系、Slug重定向、revision及Alias/Tombstone依赖在一个数据库事务内按外键顺序恢复。Coser当前头像/Banner原件先写入同一托管根下的导入专属暂存目录，逐项复核大小和SHA-256，并从原件重建avatar-480及banner-960/1600派生图；数据库提交前才原子移动UUID目录。
- 每个待发布Coser目录包含导入专属所有权标记。普通错误会回滚数据库并只删除本轮已移动目录；若进程在文件发布与SQLite提交边界被强制终止，需所有者认证的`-recover-portable-import`会复验保留包：未提交会仅删除带匹配标记的目录并把会话记为`FAILED`，已提交则逐项复验落盘原件摘要、清理标记并恢复`NORMAL`。无标记或标记不匹配的既有目录绝不自动删除。
- 数据库回归覆盖v8快照后迁至v9、声明抢占阻断及强制两步接管、完整核心事务和资源发布失败回滚；Server真实ZIP闭环覆盖自动安全备份、UUID/revision保持、原图相同、派生重建、Gallery声明、重复导入拒绝、隔离包保留、已提交中断恢复和无所有权目录保护。正式`cgm_web_embed cgm_galleryepic cgm_moegirl`标签下`portablecatalog/productdb/productapi/productserver/cmd`测试、相关Go Vet、`git diff --check`和`/tmp/cgm-portable-stage2-check`构建通过，版本为`Cosplay Gallery Manager 1.5.0-dev`。
- 本阶段没有修改前端或GraphQL，因此未重复运行前端测试；没有修改Gallery Manifest、媒体、缓存或机器本地媒体库映射，也未实现Gallery业务行重建、Item/Link正式接管、非空库Merge及Web工作台。所有变更尚未提交、未部署，正式业务数据库仍为schema v8；未来部署本累计变更必须按schema升级流程先做并复验额外完整备份，不能按普通热替换执行。

## 1.5-69 可移植元数据阶段3媒体映射与Gallery重建

日期：2026-09-09

- 在既有schema v9导入会话中补齐`portable_library_mappings`与`portable_gallery_rebuilds`：前者只保存来源逻辑key/name、本机目标库ID和明确跳过决定，旧机器绝对根不会进入目标数据库；后者保存set_id、库内相对来源、目录/存档类型、导出时Manifest状态/版本/revision/BLAKE3摘要、稳定问题码及逐Gallery可重试游标。映射必须一次覆盖全部逻辑库，且只能指向当前已启用媒体库。
- 新增所有者CLI三段式入口：`-map-portable-libraries <import-id>`交互选择本机目标；`-preflight-portable-rebuild <import-id>`只读输出READY/BLOCKED/SKIPPED/REBUILT及稳定问题码；`-rebuild-portable-galleries <import-id>`先重复预检，再要求输入`REBUILD`才写入。除映射交互中供所有者辨认本机目标外，报告不输出旧机器绝对路径或业务正文。
- 只读预检逐项重建目标路径并确认不逃逸映射根，拒绝路径任一级符号链接、来源类型变化、缺失/非CLEAN/摘要变化或身份不符的sidecar；随后复用当前目录或存档安全扫描限制，要求扫描完整且不超过1,000项、每个带UUID的Manifest Item路径恰好对应一个观察项，并核验Gallery/Item/Link声明仍属于当前导入。所有预检状态持久化错误现会向上返回，不再被吞掉。
- 正式重建按Gallery建立DRAFT及本机Source，在扫描事务中按精确相对路径执行Item声明`PENDING→CLAIMING→CLAIMED`，再由Manifest应用事务接管ExternalLink并恢复标题、分级、说明、Credit/Cast/Tag、Item排序/排除和封面。普通扫描仍生成本机新UUID，不受可移植参数影响。目录和存档媒体本体均只读，存档不重新打包，sidecar也不被Push覆盖。
- 重试安全边界补强：已经CLAIMED的Gallery/Item/Link除了kind和Registry状态，还必须实际归属于当前重建Gallery；错误归属会阻断而不是继续。扫描或Manifest应用中途失败保留`REBUILDING`游标，后续相同输入可核对已有Item再继续。扫描要求的Manifest路径有重复时明确阻断，避免map覆盖后遗漏声明。
- 会话只有在全部非跳过重建完成且所有ACTIVE声明已接管时才进入`GALLERIES_REBUILT`；Gallery类Alias/Tombstone随后在同一完成事务中按约束注册并发布。跳过或不完整来源的ACTIVE声明保持保留和可见，会话不会错误宣称完整。声明状态变更和重建游标状态变更均检查实际影响行数。
- 新增目录异机根端到端测试：源库真实扫描PNG、创建ExternalLink和CLEAN sidecar并导出，目标空库导入后映射到完全不同根；先篡改sidecar验证`PORTABLE_MANIFEST_CHANGED`且数据库仍无Gallery，再恢复并重建，确认set/item/link UUID、标题和新Source路径正确且状态为DRAFT。新增ZIP相邻sidecar异机重建测试，确认存档不改写并保持Item UUID；另覆盖映射完整决定、禁用目标拒绝和延迟Alias/Tombstone发布。
- 正式标签组合`cgm_web_embed cgm_galleryepic cgm_moegirl`下`portablecatalog/productdb/productapi/productserver/cmd/cgm`测试通过（productdb约13.9秒、productserver约6.3秒），同范围Go Vet和`/tmp/cgm-portable-stage3-check`构建通过，二进制报告`Cosplay Gallery Manager 1.5.0-dev`。本阶段无前端或GraphQL变更，因此未运行前端测试。
- 本轮累计工作区仍未提交、未部署，未打开或迁移正式业务数据库，未读取或修改正式媒体、Manifest、配置、缓存和服务。真实TAR/TGZ/7Z跨机演练、Web管理入口、非空库Merge冲突工作台和可选所有者连续性状态仍属于后续阶段。

## 1.5-70 可移植元数据阶段4非空库Merge只读预检

日期：2026-09-09

- 阶段4先实现不具有业务写入能力的纵向切片，没有放宽阶段2空库导入门禁。新增需所有者重新认证的`-preflight-portable-merge <absolute.zip>`；Server只接受绝对ZIP普通文件，完整运行离线Inspector，并在检查前后复核文件inode、非符号链接属性与整包SHA-256，避免检查结果与后续报告绑定到不同字节。
- 当前数据库使用单一只读事务取得核心目录快照；本机Registry和包内身份账本分别保持UUID顺序，通过两个受Context取消控制的流生产器做归并比较。实现不会把百万GalleryItem/ExternalLink身份整体载入Go内存，也不会对每条输入身份执行一次数据库随机查询。身份报告区分ADD、精确REUSE和UUID kind/state/target/history硬冲突。
- 核心目录比较覆盖Coser、Work、Character、Tag和SocialAccount。相同UUID且全部可移植字段一致才计为复用；字段不同形成`PORTABLE_CORE_ENTITY_CONTENT_CONFLICT`人工复核。不同UUID但规范化主名称或Alias相同只形成`PORTABLE_CORE_NAME_MATCH_REVIEW`，绝不自动认定同一实体；Character名称仍以其Work UUID为作用域。另报告同kind Slug占用、SocialAccount URL候选、Tag父子边位置差异和Slug重定向目标冲突。
- CLI与审计只输出计数、稳定问题码、entity kind及incoming/local UUID，不输出名称、Alias、URL、媒体路径、字段正文或包路径。硬冲突返回专门阻断码；REVIEW明确表示后续必须做显式决策，但本切片没有实际Merge按钮或“全部接受”行为。
- 集成回归使用真实导出包和非空目标库同时覆盖：新UUID可新增、完全一致UUID/实体可复用、同UUID字段差异、不同UUID大小写规范化同名候选以及同UUID跨kind硬冲突；并确认预检前后Registry、核心实体和导入会话计数不变。正式三标签`portablecatalog/productdb/productapi/productserver/cmd/cgm`测试、同范围Go Vet、`git diff --check`及`/tmp/cgm-portable-stage4-preflight-check`构建通过，版本为`Cosplay Gallery Manager 1.5.0-dev`。
- 本切片没有新增schema版本、持久化Merge会话或决策，没有复制/隔离包、创建安全备份、导入资产、注册UUID、修改Gallery/Manifest/媒体或部署服务。下一切片必须先冻结`KEEP_LOCAL/USE_INCOMING/MAP_TO_LOCAL/SKIP`等决策对每种冲突的合法性，再以包SHA-256绑定持久化决策；在此之前不会开放非空库写入Merge。

## 1.5-71 可移植元数据阶段4持久化Merge决策

日期：2026-09-09

- 数据库schema由v9升至v10，新增只承载技术工作流的`portable_merge_sessions`和`portable_merge_conflicts`。会话不可变绑定merge/export UUID、Backup根内受控包相对路径、包SHA-256、目标指纹、格式版本和预检计数；冲突不可删除且其issue key/code、severity、kind、双方UUID与field key不可修改，只允许受控更新decision。没有把名称、Alias、URL、字段值或旧机器路径复制进数据库。
- 目标指纹由当前全部Portable Registry身份记录和规范化核心目录JSON按稳定顺序计算；Gallery/Item/Link数量也通过Registry参与，因此决策期间任何相关UUID分配都会使指纹变化。冲突issue key由稳定码、severity、kind和双方UUID确定性SHA-256生成，重复预检不依赖扫描顺序或随机ID。
- 新增`-prepare-portable-merge <zip>`：先执行纯预检并要求输入`PREPARE`，再用安全普通文件复制逻辑把包保留到`portable-merges/<merge-id>/package.zip`，校验复制摘要后对受控副本和当前目标重新完整预检，最后原子写入会话与冲突。硬冲突会话直接为`BLOCKED`；无冲突为`READY`；只有人工项时为`DECISIONS_PENDING`。此阶段不为仅保存技术状态创建完整安全备份，实际写入前仍必须创建。
- 新增`-decide-portable-merge <merge-id>`：逐项显示稳定码、kind和双方UUID，内容/Tag边位置差异仅允许`KEEP_LOCAL`或`USE_INCOMING`，同名/SocialAccount URL候选仅允许`KEEP_SEPARATE`或`MAP_TO_LOCAL`；硬冲突没有可接受决定。必须一次覆盖全部REVIEW并输入最终确认词`DECIDE`，任一遗漏、重复、类型不兼容或已处理项都会使整个事务回滚。
- 决策提交前再次从受控Backup根打开保留包并执行完整Merge预检。包摘要或目标指纹与会话不一致时拒绝全部决定，并把尚待决定或READY的会话标为`STALE`；已有冲突仍保持原始`UNRESOLVED`/决策值，不能把旧判断套到新数据库状态。
- schema回归覆盖新库初始化及v8连续迁至v10；Store测试覆盖完整决策、缺项、错误决策类型、硬冲突不可决策、冲突不可删除/改写。Server真实包回归覆盖BLOCKED包保留字节一致、稳定冲突持久化，以及纯REVIEW会话在目标新增实体后因指纹变化转为STALE且不保存决定。
- 正式标签组合`cgm_web_embed cgm_galleryepic cgm_moegirl`下`portablecatalog/productdb/productapi/productserver/cmd/cgm`测试通过（productdb约14.6秒、productserver约6.8秒），同范围Go Vet及`/tmp/cgm-portable-stage4-decisions-check`构建通过，版本为`Cosplay Gallery Manager 1.5.0-dev`。无前端/GraphQL变化，未运行前端测试。
- 当前READY只表示无硬冲突且人工决策完整，尚无执行Merge的业务写入入口；没有导入身份/实体/资源、创建写入前安全备份、修改Gallery/Manifest/媒体、提交或部署。正式库仍为schema v8，未来部署累计schema v9/v10必须走停服、额外完整备份、迁移及完整性复验流程。

## 1.5-72 可移植元数据阶段4无冲突核心Merge写入

日期：2026-09-09

- schema v10补齐`portable_merge_identity_claims`，为Merge包中新出现但尚未重建业务行的Gallery、GalleryItem和ExternalLink保存不可删除、源字段不可变、`PENDING→CLAIMING→CLAIMED`单向声明。Registry分配触发器现同时检查空库Import声明和Merge声明；Merge声明写入也拒绝已被Import保留的UUID。
- Merge只读占用流现由正式Registry、未CLAIMED Import声明及未CLAIMED Merge声明按UUID共同组成；声明使用本机内部`RESERVED_*`状态参与比较，因此另一个包即使kind和来源生命周期相同也会得到硬冲突，不能把其他会话的保留误判为精确复用。空库Import门禁也计入Merge会话/声明，不能在准备中的非空Merge状态上并行启动空库导入。
- 新增`-apply-portable-merge <merge-id>`及最终确认词`MERGE`。Server仅接受`READY`、零硬冲突、零REVIEW的会话，重新检查受控包摘要、目标指纹和Issue仍为空；当前明确拒绝任何带Coser头像/Banner资源的包，也拒绝已经做过人工决策的会话，不会把未实现的资产替换或`USE_INCOMING/MAP_TO_LOCAL`语义伪装成可执行。
- 实际写入前使用现有Full Backup服务创建并登记完整`SAFETY_SNAPSHOT`。随后单一SQLite事务按依赖顺序新增缺失核心Registry、Work、Coser、Tag、Character、SocialAccount、Alias、Tombstone、Tag边及Slug重定向；包与本机完全相同的身份/实体只计为复用、不更新revision或时间。Gallery类新身份只写Merge声明，不创建Gallery/Item/Link或伪ACTIVE Registry。
- Merge事务再次逐条核对复用身份的kind/state/target/created/retired/reason以及预检ADD/REUSE总数；后段关系唯一约束、生命周期或计数出现变化时，Registry、核心实体、关系、Gallery声明和会话`APPLIED`状态整体回滚。成功会话记录安全备份UUID并永久进入APPLIED，重复Apply明确拒绝。
- 预检补充跨库合并关系边界：SocialAccount同Coser位置占用、Tag同Child位置占用进入REVIEW；把本机和输入Tag边合并后形成的环作为硬阻断，避免两个各自合法DAG合并成循环。
- Server端到端回归从真实非空目标准备零冲突包，执行自动完整备份和核心Merge，确认Work/Character/Coser/SocialAccount/Tag UUID保留、Gallery声明为PENDING、普通UUID分配被阻止、安全备份READY、会话APPLIED及重复执行拒绝；再次预检同包可观察到声明冲突。数据库回归人为制造后段Slug唯一约束失败，确认所有输入Registry与会话状态完整回滚。
- 正式标签组合`cgm_web_embed cgm_galleryepic cgm_moegirl`下`portablecatalog/productdb/productapi/productserver/cmd/cgm`测试通过（productdb约14.3秒、productserver约7.2秒），同范围Go Vet和`/tmp/cgm-portable-stage4-apply-check`构建通过，版本为`Cosplay Gallery Manager 1.5.0-dev`。
- 本切片无前端/GraphQL变化，未运行前端测试；没有执行人工冲突决策、Coser资产Merge或Merge来源的Gallery业务重建，没有修改正式数据库/媒体/Manifest、提交或部署。正式库仍为schema v8，累计v9/v10部署必须使用停服、额外完整备份、迁移和完整性复验流程。

## 1.5-73 可移植元数据迁移关键闭环

日期：2026-09-09

- Merge Apply不再只接受零REVIEW包。每次应用重新校验受控包SHA-256、目标指纹及全部Issue身份，并读取不可变的完整决定集合；`MAP_TO_LOCAL`将传入UUID注册为同kind永久Alias，核心关系随后解析到本地目标；`KEEP_LOCAL/USE_INCOMING`实际控制同UUID实体、SocialAccount排序冲突和Tag边冲突，硬冲突仍没有绕过入口。新增字段key进入Issue摘要，避免同一实体多条关系问题键碰撞。
- Gallery/Item/ExternalLink不再使用无法接入后续流程的Merge专属声明。核心Merge事务原子创建阶段3兼容的技术重建会话、逻辑媒体库映射、逐Gallery游标及统一待接管声明；已存在Gallery清单记为SKIPPED，新Gallery保持PENDING。因此应用后可直接以Merge UUID运行映射、只读预检和DRAFT重建，统一UUID分配触发器继续阻止抢占。
- Coser资源Merge已覆盖新增和显式`USE_INCOMING`替换。包内原图先隔离复验并重建派生图；新增Coser使用UUID目录原子发布，既有Coser只替换assets子目录并把旧目录暂存于该Merge隔离根，保留Coser Manifest边界。数据库事务或发布失败按相反顺序恢复；成功后清除所有权标记和回滚目录。
- schema v10把维护状态扩展为`PORTABLE_MERGING`。Apply创建完整安全备份后进入维护态，成功才恢复NORMAL；`-recover-portable-merge`只处理与当前Merge UUID标记一致的资源，READY会话恢复旧资源/删除本轮新目录，APPLIED会话复验包内资源摘要后完成清理。`-abort-portable-merge`只允许在业务写入前关闭会话，保留包及冲突审计；重复Apply继续拒绝。
- 新增真实包端到端回归：人工同名映射产生Alias、同UUID Work采用传入内容；新Coser头像原图和480派生图发布；同UUID既有Coser头像替换；非空目标库映射完全不同媒体根后重建Gallery并保持set/item UUID及DRAFT状态；应用后恢复和应用前取消。数据库后段约束失败测试继续证明Registry、实体、声明、会话和文件发布无部分结果。
- 新增[可移植元数据迁移操作手册](PORTABLE_MIGRATION_RUNBOOK.md)，明确完整恢复/空库导入/非空Merge的选择、确认词、恢复命令、媒体映射与验收顺序。当前基础闭环为CLI；双语Web工作台及可选owner continuity明确保留为后续增强。
- 正式`cgm_web_embed cgm_galleryepic cgm_moegirl`标签下`portablecatalog/productdb/productapi/productserver/cmd/cgm`测试通过（productdb约15.3秒、productserver约10.1秒），同范围Go Vet、`git diff --check`及`/tmp/cgm-portable-migration-closure`构建通过，版本为`Cosplay Gallery Manager 1.5.0-dev`。没有前端或GraphQL变化，因此未重复前端测试。
- 累计功能提交为`b477ddcfb020b6bad60c4012021ffdbbe70c5d64`（`Add portable catalog migration workflow`）。清洁提交以Go 1.25.12和正式三标签构建，`go version -m`确认revision一致且`vcs.modified=false`；正式二进制SHA-256为`3365d282fd9b69a35169b446b3f4736bc471fad24c715e44cd87644488db40fd`。
- 2026-09-09 20:08 CST停服并确认`MainPID=0`后，创建并实际解包逐项复验0600回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260909T120810Z-b477ddc.tar.gz`，SHA-256为`aeda830f024c7aba2a1b4e72cd4918e9e2288acb959211cc4f79d143c3419c00`。包内schema v8 SQLite、旧二进制、配置、systemd单元和Coser托管资源与正式来源一致，不含媒体、缓存、日志或既有备份；解包数据库`integrity_check=ok`。
- 新二进制经同目录候选逐字节校验后原子替换，并只启动服务一次。应用自动创建`product.sqlite.pre-schema-v8-1788955809181123991.bak`，该快照仍为schema v8且`integrity_check=ok`；正式数据库升级为schema v10、`integrity_check=ok`、maintenance=`NORMAL`，inode仍为`19679716`。迁移前后均保持2个媒体库、6个Gallery、6个来源、322个Item、135个Coser、108个Work、697个Character和0个Tag，新增迁移会话表为空。
- 服务保持`active/running`、`MainPID=23429`、`NRestarts=0`；Health/Ready为204，Root/Legal/Session为200，2个Worker及LibRaw/FFmpeg/FFprobe正常启用，启动及探针日志无WARN、ERROR、FAILED、panic或fatal。配置SHA-256继续为`fee095a8642c53278838475549251a48956d3058cd50fc652e4d0b392b877899`。About因本地新提交尚未位于远端可访问源码而报告`exactSourceAvailable=false`，二进制自身VCS revision已独立核验为`b477ddc`；本轮未执行远端推送。

## 1.5-74 双语Web迁移工作台与可选所有者连续性

日期：2026-09-10

- 按所有者最终边界取消Gallery地址连续性；Gallery/Item评分继续由`.cosplay.json` Manifest负责；最后浏览时间、最后浏览项目及其他Gallery浏览历史保持目标系统现有轻量状态，不进入迁移包，也不由迁移应用覆盖。
- portable package升级为format v2并继续兼容读取v1。新增默认不存在的`owner-continuity.json`，导出时只能分别显式选择Gallery生命周期/首次收录时间和Gallery收藏隐藏/Item收藏；Entry进入逐文件checksums并记录独立计数，只允许引用身份账本中的ACTIVE `set_id`/`item_uuid`。默认导出行为与旧流程一致。
- 连续性应用只能在技术会话达到`GALLERIES_REBUILT`后执行。整份分区在一个事务内解析到目标机已重建的Gallery/Item；ACTIVE仅作为请求并重新运行`DISPLAYABLE_ITEM_REQUIRED`、`CONTENT_RATING_REQUIRED`等当前激活门禁。任一引用或门禁失败整批回滚，但不撤销此前完成的核心导入与Gallery重建，可在修复后独立重试。
- 数据库写入只更新已选择的生命周期或收藏隐藏字段。Gallery/Item评分列和Gallery最后浏览时间/项目列不在SQL写集合中；回归测试先写入这些目标机状态再应用收藏连续性，确认它们保持不变。没有新增数据库表、字段或schema版本。
- CLI新增`-include-owner-lifecycle`、`-include-owner-flags`和`-apply-owner-continuity <workflow-id>`；后者要求所有者重新认证及`CONTINUITY`确认。离线Inspector继续严格校验完整包并显示可选分区计数。
- Manage → Operations新增中英文Portable migration workbench，与CLI直接共用Server服务和schema v9/v10持久化会话。界面覆盖服务器绝对路径导出、空库导入、非空库Merge准备、合法REVIEW决定、应用/终止、逻辑媒体库映射、Gallery重建预检/执行和可选连续性应用；每个写动作均要求所有者密码和动作专属确认词。界面明确说明服务器路径语义及不迁移地址、评分和浏览历史的边界。
- 工作台收口补齐导出就绪检查及问题计数、所选保留包的生命周期/收藏隐藏范围与Gallery/Item计数，以及`PORTABLE_IMPORTING`/`PORTABLE_MERGING`维护态的显式安全恢复入口。会话列表固定最近200条，所选历史会话按UUID直接读取；包摘要只读取受控`package.json`，实际动作仍执行完整Inspector。服务失败只返回动作级稳定码，不把文件系统细节暴露给浏览器。
- 修复format v2 Merge保留会话曾错误写成v1的问题：预检报告现在携带Inspector识别的真实格式版本，准备会话原样持久化；回归锁定v2写入和v1读取兼容，确保Merge后仍能识别并应用可选owner continuity。
- GraphQL只返回会话计数、状态、稳定问题码和不透明身份，不暴露保留包路径或旧媒体绝对路径；媒体库映射只在认证Manage界面显示本机目标根。新增会话列表读取，没有把包业务正文复制进数据库。
- 当前源码定向`portablecatalog/productdb/productapi/productserver`测试、GraphQL所有者重新认证/稳定失败码测试、format v1兼容和v2 Merge格式回归通过；正式`cgm_web_embed cgm_galleryepic cgm_moegirl`标签下`portablecatalog/productdb/productapi/productserver/cmd/cgm`测试及同范围Go Vet通过。TypeScript检查、前端31文件103项测试和682模块生产构建通过；主共享chunk约535.61KiB，继续保留既有大包提示。`git diff --check`通过。
- 功能与部署前记录提交为`771b1889c7a123044ce16879c73278b4e9c4e8ee`（`Complete portable migration workbench`）。清洁提交以Go 1.25.12和`cgm_web_embed cgm_galleryepic cgm_moegirl`构建，`go version -m`确认revision一致且`vcs.modified=false`；正式二进制SHA-256为`e7de3fd1805048321c2660fa9ca9c3a2819b402424f197ff95799f8f6364ad9b`，构建时间为`2026-09-10T01:33:38Z`。
- 部署前停止服务并创建额外完整回滚包`/home/rainbowrunner/cos/bk/cgm-predeploy-20260910T093759-771b188.tar.gz`，包含旧二进制、配置、用户systemd unit、SQLite一致快照和完整Coser托管根；包为70MiB，SHA-256为`8a7e371bfb87c667874adc5012f55537f87673289ddd7fa69169bff8d017ef89`。实际解包后，全部文件清单、旧二进制/配置/unit逐项比较及快照产品身份、schema v10和`integrity_check=ok`均通过。
- 本次无schema迁移，只原子替换正式二进制并启动服务一次。配置SHA-256保持`fee095a8642c53278838475549251a48956d3058cd50fc652e4d0b392b877899`，数据库inode保持`19679716`；正式库仍为schema v10、`integrity_check=ok`、maintenance=`NORMAL`及2个媒体库、6个Gallery、6个来源、322个Item、135个Coser、108个Work、697个Character和0个Tag。服务保持`active/running`、`MainPID=234086`、`NRestarts=0`，Health/Ready为204，Root/Legal/Session为200，2个Worker及LibRaw、FFmpeg、FFprobe正常启用，启动及探针日志无WARN、ERROR、FAILED、panic或fatal。About因提交尚未位于远端可访问源码而保持`exactSourceAvailable=false`；本轮未推送远端，也未执行迁移模拟，后者由所有者继续。
# 2026-09-10 自动扫描计划与来源异常恢复

- 自动扫描从固定24小时扩展为数据库配置的总开关、可选启动扫描和15～10080分钟周期；默认仍关闭且默认周期1440分钟。调度复用持久租约、维护门禁、恢复暂停、媒体库自动化策略与原子Source扫描，不增加实时文件监听。
- 产品数据库升级为schema v11，只增加`runtime_settings.automatic_scan_on_startup`和`automatic_scan_interval_minutes`，旧库迁移保留原每日行为且不自动开启启动扫描。
- Manage Gallery新增全库异常筛选、高亮与MISSING优先；Media支持MISSING-only和显式Forget；Forget只删除应用记录并Tombstone UUID，不触碰源文件。
- 来源读取错误稳定区分不存在、权限不足及通用读取失败，Manage显示最近错误与完成时间。Libraries的`SOURCE_REBIND_CANDIDATE`补齐显式确认入口，新旧来源都可访问时再次确认，绑定后仍要求完整扫描。
- 扫描新增Item或唯一指纹路径重绑定会把已跟踪Manifest置为`DB_DIRTY`，解决成员变化只增加scan revision而Manifest误报CLEAN的问题；MISSING条目继续保留到显式Forget。
- 异常第二闭环新增显式媒体替换：所有者从同组AVAILABLE Item中选择新文件，预览后输入`REPLACE`；事务保留旧UUID及Caption/排序/Exclude/收藏/评分/封面意图，新临时UUID永久Tombstone并重新排队派生处理。带编辑或个人/封面引用的新记录会被拒绝，系统不做启发式自动迁移。
- 新增输入`FORGET N`的Gallery内批量MISSING清理；单项和批量操作均在删除前安全解除封面引用，不接触源文件。Source页新增最近20次扫描状态、时间和稳定错误码；现有schema不保留提交后的观察明细，因此不虚构历史技术计数。
- Manifest页新增基于Portable Item UUID和上次同步基线的Push成员预览，显示新增/删除/保留/更新数量；Push前二次显示摘要，实际写入继续重新校验文件哈希，不能覆盖外部修改。
- 回归覆盖正式schema v10→v11独立快照与默认值、v7连续迁移、配置周期与启动触发、持久租约、来源缺失不批量制造MISSING、Gallery异常全库筛选、Manifest成员变化/Push预览、显式替换及单项/批量Forget；产品数据库/API/Server/Manifest/SourceScan测试通过。前端TypeScript、31文件105项测试和682模块生产构建通过；正式`cgm_web_embed cgm_galleryepic cgm_moegirl`标签组合测试及验证二进制构建通过，仅保留既有主共享chunk超过500KiB提示。`git diff --check`通过。
- 当前仅完成源码与发布构建验证，未提交、未部署，也未触碰正式schema v10业务数据库；部署时必须先做独立完整备份，再由启动迁移生成schema v10快照并升级到v11。
- 详细边界和后续阶段见`AUTOMATIC_SCAN_AND_SOURCE_RECOVERY_2026-09-10.md`。

# 2026-09-14 媒体库变更影响预览与Source转移（第一段）

- Libraries新增只读媒体库改根/删除影响预览，逐项列出直接绑定的Gallery Source、Gallery标题、路径和建议归属，同时显示ignored Source数量及子媒体库根；此阶段不执行改根或删除。
- 新增逐Source显式转移工作台，用户选目标并二次确认；事务比较原库归属、校验目标路径包含关系和两端无活动自动化任务，成功后失效相关发现快照，防止旧预览静默生效；不移动媒体文件或Gallery身份。允许显式转未分配，但不会自动由父库接管。
- 目标外路径与过期归属的数据库回归通过；GraphQL与Web工作台交互测试通过。仍需完成改根/删除执行、ignored Source和规则/自动化影响预览及执行前并发复核，详见同日开发备忘录。

# 2026-09-14 媒体库改根/删除工作台第二段

- 影响预览扩展为ignored Source路径/原因、三类库专属规则、自动化策略与历史/活动任务、扫描中的Source、迁移映射及目标库边界冲突；令牌绑定本次预览全部关键状态。
- 实际改根/删除须所有者密码与`MOVE ROOT`/`DELETE LIBRARY`确认词，事务内重算并比对令牌；过期预览、活动任务/扫描、迁移映射、目标冲突均拒绝。删除还要求全部Gallery Source已显式转移。
- 删除只清理CGM媒体库及库专属规则/任务，子库保留；ignored Source改为全局精确路径忽略，防止父库静默重导。改根按相对路径映射Source/ignored/Manifest并标记待复扫；都不触碰源媒体文件。
- 数据库回归覆盖旧预览、改根路径映射、Source转移门禁、规则/策略影响、活动任务及扫描中Source阻断和删除后忽略路径保留；无新增schema。正式三标签产品数据库/API/Server/CMD测试及同范围Go Vet、Web 31文件107项测试、TypeScript、682模块生产构建和差异检查均通过。当前未提交、未部署。

# 2026-09-14 全局/库级ignored Source撤销

- Libraries补充独立忽略路径工作台：全局/当前库切换、路径子串搜索、50项分页、单项影响预览及明确的`REVEAL`确认。预览展示受影响库、活动自动化任务、同路径Gallery Source和可选Set ID。
- 撤销动作要求所有者密码，事务内复核预览令牌；旧预览和受影响库活动任务拒绝。成功只删除CGM忽略记录并失效受影响库发现快照，不访问媒体或自动扫描；下一次发现可能重新出现。审计仅记录动作和不透明ID，不包含路径/Set ID。
- 数据库回归覆盖全局/库级分页检索、旧预览、活动自动化阻断、带Set ID忽略的跨库影响/快照失效与媒体目录保留；GraphQL认证、确认词和成功路径及前端操作测试已新增。正式三标签产品数据库/API/Server/CMD测试及同范围Go Vet、Web 31文件108项测试、TypeScript、682模块生产构建和差异检查通过。无新schema，未提交/部署。

# 2026-09-14 发现快照并发复核与schema v11正式部署

- 原发现流程先在事务外读取规则、Source、忽略和Gallery身份，后开启事务写快照，可能保存并发变更前的判断。现数据库状态读取、候选计算和快照写入共用`BEGIN IMMEDIATE`事务；文件遍历后复核媒体库根和子库边界，变更则拒绝旧发现并要求重试。回归覆盖改根、新子库及忽略路径变化。
- 累计源码与开发记录提交为`276d7454178e4aecf031ed77fe9f00c3682a69df`；正式标签Go测试和Vet、TypeScript、Web 31文件108项、682模块生产构建及差异检查均通过。未执行远端推送。
- 先停止本机systemd单实例，确认正式库schema v10、`integrity_check=ok`及`135/108/697/6/6/322`业务计数。独立回滚目录`/home/rainbowrunner/cos/bk/cgm-pre-v11-276d745-VOW1cX`包含SQLite一致快照、配置、完整Coser托管资源和旧二进制；两份文件SHA-256与验证细节见[专项部署记录](AUTOMATIC_SCAN_AND_SOURCE_RECOVERY_2026-09-10.md)。原始媒体不在此包中且此次未修改。
- 原子替换新二进制并启动一次；程序另生成自动v10迁移前快照，已验证完整性。正式库为schema v11、完整性`ok`、六类计数不变，启动扫描默认0、周期1440分钟。新二进制SHA-256=`db8ae3f62e495ea17e7b56663d0d96f768d8f207e016f1e05d601edc43857419`；About精确报告提交，服务`active/running`且`NRestarts=0`，Health/Ready均204，启动日志无错误。真实媒体库上的新管理动作尚需所有者人工验收。

# 2026-09-14 Manage Gallery七卡片筛选

- 作品集列表顶部的纯展示五项扩展为“全部、草稿、超过上限、来源不可用、阻断问题、缺失媒体、处理失败”七个可点击筛选卡片，移除重复的筛选下拉框。卡片使用原有`issue` URL参数，切换时回到第一页，显示当前选中态并支持原生按钮键盘操作。
- 顶部所有数字统一代表符合条件的**作品集数量**：新增全库总数和存在处理失败Item的Gallery数，`MISSING`改为存在至少一个MISSING Item的Gallery数；旧API的`missingItem`文件数继续保留供兼容，不再作为卡片数字。多个异常可以落在同一个Gallery，因此卡片数字不应相加。GraphQL仅添加只读字段，不新增数据库表或schema。
- 数据库测试覆盖一个Gallery有多个缺失Item时的计数分离、处理失败筛选及全局计数；Web测试覆盖七卡片、无下拉框、URL切换及分页重置。正式`cgm_web_embed cgm_galleryepic cgm_moegirl`标签的产品数据库/API/Server/CMD测试及同范围Go Vet通过；Web 32文件109项测试、TypeScript和682模块生产构建通过（保留既有主共享chunk超过500KiB提示）。此段不需要数据库schema迁移；提交和部署结果在后续记录补充。
- 功能源码提交为`7b172be5d733806e0aac3b088586536af8a35554`。在正式服务停机状态创建独立回滚目录`/home/rainbowrunner/cos/bk/cgm-pre-gallery-filter-7b172be-U3UM0F`，包含schema v11的SQLite一致快照`database.sqlite`（SHA-256 `2f96fbf092a9370c5c66da13ef59907a52109cb4482e0a2f87f1e180e7e5985a`）及启动配置、systemd unit、全部Coser托管资源、旧二进制的`application-state.tar`（SHA-256 `ef6726ebbfb651dfbba336c473705a9a57e6352fe2dbd10cfbb3ad17d9449e75`）。快照完整性为`ok`；归档全量可读，旧二进制与配置内容哈希和现存文件一致。此目录是本机应用状态回滚材料，不包含原始媒体，也不是CGM原生Web可直接导入的完整备份ZIP。
- 无schema迁移；原子替换后正式二进制SHA-256为`b6d4174f948937ce6fb428fd2d6a267851828fba3ec6fd9c0011c8ae203f1e9d`，About精确指向上述功能提交。服务`active/running`、`NRestarts=0`，Health/Ready均204，Web首页和Manage深链均200，本轮Gallery索引JS与CSS哈希资源均200；启动日志无错误。正式数据库仍为schema v11、`integrity_check=ok`，Coser/Work/Character/Gallery/Source/Item计数维持`135/108/697/6/6/322`，用户媒体未修改。未执行远端推送；真实浏览器中的七卡片点击行为仍待所有者验收。

# 2026-09-15 媒体库扫描与自动化入口语义澄清

- Libraries页将顶部普通发现按钮改为“仅扫描并检查”，明确该入口只刷新候选、未分配目录和覆盖诊断，不会继续创建/扫描DRAFT或尝试激活。
- 自动化按钮改为“扫描并按已保存规则自动处理”，说明其后台任务会先执行新一轮媒体库扫描，再按冻结的策略快照创建合资格DRAFT、扫描来源、应用复核策略，并只在TRUSTED策略明确启用且通过全部现有门禁时尝试激活。
- 保持策略保存与任务执行两个显式动作。新增未保存策略门禁：任何表单改动都会禁用执行按钮并提示先保存，防止用户看到新值、后台却按旧快照运行；MANUAL模式仍不能入队，同库单活动任务和取消逻辑不变。
- Manage侧栏新增独立“帮助”页，采用可扩展的主题索引与正文结构。首篇“媒体库与导入”对照三种执行入口、媒体库发现与Gallery来源扫描、不同Gallery状态处理结果，并提供Libraries及自动扫描设置直达入口；明确显式自动化不扫描ACTIVE来源，而计划任务会在独立来源对账阶段覆盖ACTIVE。
- 帮助页和新增内容提供完整中英文消息，使用语义化标题、导航、表格和安全门禁列表，并针对窄屏把主题区和操作卡片收为单列。新增页面回归锁定两层扫描、ACTIVE差异、入口链接和显式执行语义。
- 本轮仅调整Web交互、帮助内容与文案，不改变GraphQL、后台队列、自动扫描调度、数据库schema、媒体或Manifest。Web 33文件112项测试、TypeScript检查及683模块生产构建通过；帮助页保持独立懒加载chunk，构建仅保留既有共享chunk超过500KiB提示。部署前`git diff --check`通过。
- 功能与部署前记录提交为`243d18a3216fc03697a8a940bb7ca7fafa4184fc`（`Clarify library automation and add Manage help`）。清洁提交以Go 1.25.12和`cgm_web_embed cgm_galleryepic cgm_moegirl`构建，二进制报告完整提交及`vcs.modified=false`，SHA-256为`6d6b63117db5d83b0ac0e8c4ed481fda1377cf445813fa84f73928563b5840b4`。
- 首次尝试CGM原生完整备份时，CLI按设计因缺少所有者重新认证返回`CGM_OWNER_REAUTH_FAILED`；没有生成备份、登记记录或替换文件，保护脚本恢复服务。随后停服建立额外回滚目录`/home/rainbowrunner/cos/bk/cgm-pre-help-243d18a-SV1wNr`，包含SQLite一致副本、完整Coser托管根、旧二进制、配置和用户systemd单元；数据库副本SHA-256为`b8a018543dd0a143ebd91b329b101cfa4267fcd3cbe2cde39e3b3299d5f7b313`，`integrity_check=ok`、schema v11且业务计数为`135/108/697/6/6/322`。目录权限0700、敏感文件0600，Coser根部署时为空且源/备份逐项一致。
- 原子替换正式二进制并启动后，Health/Ready为204，Root和`/manage/help`深链为200；About精确报告上述提交且`exactSourceAvailable=true`。服务保持`active/running`、`NRestarts=0`，Worker、LibRaw、FFmpeg和FFprobe正常启动，日志无WARN、ERROR、FAILED、panic或fatal。
- 本次无schema迁移；正式库仍为schema v11、`integrity_check=ok`及`135/108/697/6/6/322`业务计数，数据库inode保持`19679716`，配置SHA-256保持`fee095a8642c53278838475549251a48956d3058cd50fc652e4d0b392b877899`。未修改媒体、Manifest或缓存，未执行远端推送。

# 2026-09-15 自动化派生等待、进度展示与稳定轮询

- 修复TRUSTED自动化扫描与媒体派生任务之间的竞态：来源扫描只负责原子登记Item并排队基础派生，若当前Gallery仍存在`PENDING/RUNNING/RETRY_WAIT`任务，运行现在保留游标、释放租约并进入`WAITING_FOR_MEDIA`，由独立媒体Worker继续处理；任务成功或重试耗尽后再运行现有完整激活门禁，不再把瞬时`DISPLAYABLE_ITEM_REQUIRED`误记为人工问题。PAUSED/FAILED/CANCELLED不伪装成可自动推进状态，真正的Credit、分级、来源等阻断仍进入待复核。
- 运行阶段、已处理目标、动态总目标和当前Gallery全部由既有持久运行游标、Gallery及任务状态重建，进程重启后仍可恢复；没有增加数据库字段或schema版本。自动化Worker每次重排后让出500ms，避免等待媒体时热循环争用SQLite。
- Manage Libraries新增队列、发现、等待基础预览、应用策略、收尾和取消阶段，以及可访问的确定/不确定进度条、目标计数与当前Gallery标题。Apollo轮询只在首次无数据时显示加载占位，同revision响应不再覆盖本地策略草稿，因此页面不会每1.5秒整体闪跳，用户未保存的编辑也不会丢失。
- Archive外部路径/文件名实体建议新增保守展示名拆分：仅识别带空格的横线、竖线和中英文方括号边界，保留完整片段，不做任意空格切词、模糊或子串匹配。下一次显式自动化还会为完全没有Credit且没有待审实体建议的既有Archive草稿补算一次，使此前已创建的草稿也能受益；自动接受仍要求唯一精确Coser与Character，Work可选且必须唯一。
- 帮助页同步说明运行期进度与可信激活等待边界。后端`productdb/productserver/productapi`测试通过；Web 33文件112项测试、TypeScript检查及683模块生产构建通过，仅保留既有共享chunk超过500KiB提示。`git diff --check`通过。本段未新增schema、未提交、未部署，也未直接修改正式业务数据；现有已完成运行不会被后台静默重放，部署后须由所有者再次显式执行对应媒体库自动化。
- 功能源码提交为`ab2dc807aee30f4bd09168728fb77947c01febdc`（`Improve library automation progress`）。清洁提交以正式`cgm_web_embed cgm_galleryepic cgm_moegirl`标签重新构建，二进制报告完整提交、`vcs.modified=false`，SHA-256为`087c9070c56fc9d871e2b5d5fc48c3790faf34ef7a891a79fabb89213a36724a`。
- 2026-09-15 23:26 CST平滑停服且确认`MainPID=0`后，创建独立回滚目录`/home/rainbowrunner/cos/bk/cgm-pre-automation-ab2dc80-0CbOr8`，保存schema v11 SQLite一致副本、旧二进制、配置、用户systemd单元及完整Coser托管目录。停服关闭连接时WAL已归并；副本`integrity_check=ok`，业务计数为`135/108/697/8/8/570`，数据库SHA-256为`4a53879b846bb3cf698e1566ba33678863e802aacc3da62163462433736bface`，旧二进制SHA-256为`6d6b63117db5d83b0ac0e8c4ed481fda1377cf445813fa84f73928563b5840b4`。
- 本次无schema迁移。正式二进制原子替换后报告提交`ab2dc807`且哈希与候选一致；服务`active/running`、`MainPID=642508`、`NRestarts=0`，Health/Ready为204，首页及`/manage/libraries`深链为200。正式库仍为schema v11、`integrity_check=ok`及上述业务计数；Worker、LibRaw、FFmpeg和FFprobe正常启用，启动与探针日志无WARN、ERROR、FAILED、panic或fatal。首次探针紧跟systemd启动、早于端口监听而返回连接失败，服务完成约1秒初始化后复检全部通过，未发生重启或回滚。未修改媒体、Manifest或缓存，未执行远端推送。

# 2026-09-16 Browse Gallery快捷Tag编辑

- Gallery详情“更多详情”的现有Tag区域增加类似Stash的多选输入：点击控件空白区域或聚焦输入框即展开最多20个现有Tag，输入主名或Alias由服务端实时筛选；选择后在同一控件生成可移除气泡，空输入退格可移除最后一项，并支持上下方向键、回车选择和Escape关闭候选列表。
- 编辑采用页面内草稿和显式保存/取消；候选列表排除已选择Tag，最多保留既有200个直接Tag上限。第一段不允许在Browse创建、改名或配置Tag实体，没有结果时只显示无匹配提示。保存成功原地更新Tag和Gallery revision，不刷新页面、跳转后台、关闭Lightbox或重置媒体筛选。
- 新增Gallery范围`replaceGalleryTags` GraphQL Mutation及专用数据库事务。事务以`expectedMetadataRevision`复核并校验ACTIVE Tag身份、UUID/Position唯一性，原子替换`gallery_tags`、递增Gallery metadata revision、把已跟踪Manifest从CLEAN置为DB_DIRTY并写聚合审计；它不读取或重写Credit/Cast，也不调用`demoteInvalidActiveGallery`，因此即使存在与本次Tag无关的来源异常，Gallery生命周期状态也保持原值。
- 数据库回归覆盖ACTIVE Gallery在无关来源异常下保存Tag仍保持ACTIVE、Credit不变、顺序/revision/Manifest状态正确以及过期revision无部分写入；GraphQL回归覆盖专用返回契约。前端回归实际执行展开、名称筛选、气泡选择、revision保存、局部结果更新；Web 33文件113项测试、TypeScript检查及684模块生产构建通过，仅保留既有共享chunk超过500KiB提示。
- 本段没有数据库schema迁移，不访问或改写正式媒体、Manifest文件及业务数据库。功能与部署前记录提交为`65dfa746e3561ccf058ce439b3a4e4e0617b93c3`（`Add Browse gallery tag editing`）；该提交同时持久化上一轮已确认但暂不实施的并发调度规划。
- 2026-09-16 08:49 CST停服并确认`MainPID=0`后，创建权限0700的独立回滚目录`/home/rainbowrunner/cos/bk/cgm-pre-tag-editor-65dfa74-20260916T084935`，保存schema v11 SQLite一致副本、旧二进制、配置、用户systemd单元及完整Coser托管目录。数据库副本与正式来源SHA-256均为`9b41f1e1de3c81deee42528f299def3e1b15aabd8d66c287b68a6053e88daa3f`，旧二进制SHA-256为`087c9070c56fc9d871e2b5d5fc48c3790faf34ef7a891a79fabb89213a36724a`；配置、单元和Coser目录逐项一致。
- 本次无schema迁移，只原子替换正式二进制并启动服务一次。新二进制SHA-256为`b034d34e4d26464c51cc6ad7ab5d735fae14c54e542c476cb3e8b9c7ec035154`，About精确报告提交`65dfa746e3561ccf058ce439b3a4e4e0617b93c3`、`buildTime=2026-09-15T16:43:34Z`及`exactSourceAvailable=true`。服务保持`active/running`、`NRestarts=0`，Health/Ready为204，首页、Gallery深链及新JS/CSS资源均为200；2个Worker与LibRaw、FFmpeg、FFprobe正常启用，启动日志未检出WARN、ERROR、FAILED、panic、fatal或迁移错误。
- 部署后正式库仍为schema v11、`integrity_check=ok`，Coser/Work/Character/Tag/Gallery/Source/Item计数保持`135/108/697/0/8/8/570`；配置、媒体、Manifest和缓存均未替换，本次未执行远端推送。真实浏览器中的Tag候选筛选、气泡增删和ACTIVE状态保持仍待所有者业务验收。

# 2026-09-17 Tag层级管理UI

- 参考Billfish与Lightroom Classic的标签/关键词树，并借鉴Eagle的标签组筛选和快速检索，但保持CGM已确认的多父DAG数据模型。Manage核心实体中的Tag默认显示树形层级，仍可切回原分页列表；树节点展开/折叠、根/未归类筛选、名称/Alias搜索及全部上级路径定位、直接子Tag数/直接关联Gallery数均可查看。同一UUID的多父Tag可在各父路径下出现，右侧只保留一个实体编辑入口；不采用含义含混的拖拽迁移。
- 新增只读、认证的轻量`manageTagTree`查询，返回UUID、名称、Alias、直接父UUID、revision及直接计数，不返回完整实体编辑数据。投影设置20,000 Tag硬保护上限，超过即显式失败，不截断成假树；既有平铺列表仍服务端分页。树上“新建子Tag”向现有创建输入可选传入父UUID和预期父revision，数据库在单事务内校验活动父身份与版本、创建Tag和父子边、递增父revision；冲突或约束失败整体回滚。普通创建及已存在Tag父关系修改沿用原语义。
- 数据库回归覆盖原子子Tag创建、父版本递增、树投影Alias/父边/直接计数和过期版本无残留；GraphQL回归覆盖查询与创建契约。前端回归覆盖多父双位置、Alias搜索展开双路径、子Tag入口及列表切换；正式三标签产品数据库/API/Server/CMD测试与Go Vet、Web 34文件116项测试、TypeScript及685模块生产构建通过，仅保留既有共享chunk超过500KiB提示。`git diff --check`通过。本轮无schema迁移，未修改媒体或Manifest。源码提交为`420bfce3d268b6f9253efa48aa3c919cb443e42b`。
- 2026-09-17 22:38 CST停服并确认`MainPID=0`，在独立0700目录`/home/rainbowrunner/cos/bk/cgm-pre-tag-tree-420bfce-20260917T2238`保存schema v11 SQLite一致副本、旧二进制、配置、用户systemd单元及完整Coser托管目录；原件与副本逐项一致。备份数据库`PRAGMA integrity_check=ok`，Coser/Work/Character/Tag/Gallery/Source/Item计数为`135/108/697/2/8/8/570`。备份数据库SHA-256为`eea4550d9935da4f4476baaf74c6536f30ef8ee02da13e617fcf2883785c19e1`，旧二进制为`b034d34e4d26464c51cc6ad7ab5d735fae14c54e542c476cb3e8b9c7ec035154`。
- 从清洁提交按正式三标签重新构建内嵌Web二进制，Go构建信息确认`vcs.modified=false`；原子替换并启动一次，新二进制SHA-256为`16fd3115d4682e6391fbf43bc38465ab4ade620d7989c03eefca3234100eb410`。`/about.json`精确报告提交`420bfce3d268b6f9253efa48aa3c919cb443e42b`、`buildTime=2026-09-17T14:36:43Z`、`exactSourceAvailable=true`。服务`active/running`且`NRestarts=0`，Health/Ready均204，首页、Tag管理深链和本次JS/CSS资源均200；两个Worker与LibRaw、FFmpeg、FFprobe正常启用，启动日志未见错误。部署后正式库仍为schema v11、`integrity_check=ok`，上述业务计数不变；未执行远端推送。真实浏览器中的多父Tag树与子Tag创建交互待所有者业务验收。

# 2026-09-18 Browse Gallery详情Tag展示与跳转

- Gallery详情标题中的媒体数量、拍摄/加入时间下方新增Tag行。参考`https://www.wn10.cfd/photos-index-aid-385692.html`公开HTML/CSS中的`.addtags`/`.tagshow`：白底深灰字、12px/20px小矩形、悬浮蓝底白字；因CGM正文为白底，增加细边框确保普通态可辨识。多Tag自动换行，长名称可断行。
- 每个Tag以UUID链接至现有`/tag/:slug`详情页，其默认Scope为ALL，即全部图集；末尾`+TAG`仿照参考页作为编辑按钮，即使无Tag也常显。按钮展开原有搜索、气泡选择与显式保存面板；保存后标题Tag行即时同步。移除“更多详情”中重复的编辑器，避免两个入口分别持有未保存草稿。Tag详情页原本虽请求分页数据但未传分页回调，本次补齐分页控件，切页时保留当前Scope。
- 仅前端改动，不新增API、schema或迁移，不触碰正式业务数据。Web 34文件119项Vitest、TypeScript检查及685模块生产构建通过；构建仅有既存共享chunk超过500KiB提示。源码提交为`987e48eda922fe13a948bc09a054bd5ece0f26a1`。
- 2026-09-18 20:02 CST停服并确认`MainPID=0`，在独立0700目录`/home/rainbowrunner/cos/bk/cgm-pre-inline-tags-987e48e-20260918T2002`备份schema v11 SQLite一致副本、旧二进制、配置、用户systemd单元及完整Coser托管目录，原件与副本逐项一致。备份数据库`PRAGMA integrity_check=ok`，Coser/Work/Character/Tag/Gallery/Source/Item计数为`135/108/697/18/8/8/570`；备份数据库SHA-256为`e9f10055122444904c7fcb7cce89758efc48ea2bb04a3043d53e626e46459958`，旧二进制为`16fd3115d4682e6391fbf43bc38465ab4ade620d7989c03eefca3234100eb410`。
- 从清洁提交按正式三标签重建内嵌Web二进制，Go构建信息确认`vcs.modified=false`；原子替换并启动一次，新二进制SHA-256为`f3e37e91f15064f769977e33e1c14803e0764032e6bb8ee778893bfeddc275ed`。`/about.json`精确报告提交`987e48eda922fe13a948bc09a054bd5ece0f26a1`、`buildTime=2026-09-18T12:01:36Z`、`exactSourceAvailable=true`。服务`active/running`且`NRestarts=0`，Health/Ready均204，首页、Gallery/Tag深链及新JS/CSS资源均200；两个Worker与LibRaw、FFmpeg、FFprobe正常启用，启动日志未见错误。部署后正式库仍为schema v11、`integrity_check=ok`，上述业务计数不变；配置、媒体、Manifest和缓存均未替换，未执行远端推送。真实浏览器中的`+TAG`编辑与按Tag跳转待所有者业务验收。

# 2026-09-18 Tag分类容器与数据迁移兼容

- Tag 新增独立的`allow_direct_assignment`策略，不借用`use_in_recommendation`。新建Tag默认允许直接关联，旧数据经产品库schema v11→v12迁移后也统一默认允许；分类Tag仍可作为层级节点、承载子Tag并通过现有Tag详情汇总后代Gallery。Manage Tag编辑显示开关和直接关联计数，树上显示“分类”标识；Browse及Manage的Gallery Tag候选只列可直接关联者。关闭开关前必须清除直接Gallery关系；事务入口及SQLite触发器共同阻断绕过UI的直接写入。
- 可移植元数据包的Tag记录新增可选布尔属性，但保持现有包格式版本；导出总是记录实际值，导入、合并按该值写入。旧包缺少该属性时视为`true`，读取时在校验原始压缩包校验和之后归一化，以免合并将语义等价的旧包与本地数据误判为内容冲突。完整数据库备份仍按已有机制保存整个SQLite文件；将来恢复到新版本时自动执行v12升级。没有改变Gallery Manifest格式或媒体根目录路径逻辑。
- 源码回归：产品库v11→v12迁移与旧Tag默认值、Browse/Manage/原始SQL三层关联门禁、已有关系的关闭限制、可移植导出/导入和旧包读取通过；正式三标签产品数据库/API/可移植包/Server测试及Go Vet通过。Web TypeScript、34文件120项Vitest及685模块生产构建通过，仅见既存主chunk大小提示。真实浏览器交互和所有者执行的跨机可移植迁移模拟仍待业务验收。
- 功能源码提交`aaad2a2748474a4340312c41813c9dd80938f92e`。部署前正式服务`active/running`、Health 204；正式库schema v11、`integrity_check=ok`，Coser/Work/Character/Tag/Gallery/Source/Item为`135/108/697/18/8/8/570`。停服确认`MainPID=0`，在独立回滚目录`/home/rainbowrunner/cos/bk/cgm-pre-tag-category-aaad2a2-P4r3YPbM`保存SQLite一致副本、全部Coser托管文件、旧二进制、配置和用户systemd单元；数据库及文件逐项复验一致，副本schema v11、`integrity_check=ok`，数据库SHA-256为`81a62f6c4622d0d51ed443fed8ebc2bc452edba768d5c2d80e9a62693568f84c`，旧二进制为`f3e37e91f15064f769977e33e1c14803e0764032e6bb8ee778893bfeddc275ed`。
- 首次安装功能正确但未注入About提交元数据的候选构建后，正式库经一次启动自动迁移至schema v12；验收发现About `exactSourceAvailable=false`，随即从同一清洁提交以正式`githash/buildstamp`重新构建并原子替换，服务再次启动但无二次迁移。最终二进制SHA-256为`8f31c00adafca2ade12da2a652b9bde88cdd67962cde824296f39486212933e6`，Go VCS元数据`modified=false`，About精确报告上述提交、`buildTime=2026-09-18T12:53:00Z`及`exactSourceAvailable=true`。服务`active/running`、`NRestarts=0`，Health/Ready均204，首页和Tag管理深链200；启动日志未见WARN、ERROR、FAILED、panic或fatal。
- 部署后正式SQLite schema v12、`integrity_check=ok`，上述七项业务计数不变；18个既有Tag的`allow_direct_assignment`均为1，三项v12保护触发器存在。自动预迁移快照`product.sqlite.pre-schema-v11-1789735842537795786.bak`也已生成。配置SHA-256仍为`fee095a8642c53278838475549251a48956d3058cd50fc652e4d0b392b877899`；未替换配置、媒体、Manifest或缓存，未远端推送。

# 2026-09-18 Manifest定期只读巡检与手动批量Push

- 实施已确认的定期只读巡检方案，保留Gallery Manifest单文件16MiB限制。产品库schema v13新增独立`gallery_manifest_inspections`观察表与持久进度游标，不把尚未建立基线的`NONE/FILE_DIRTY`强行写入既有同步基线表。打开单Gallery Manifest页和后台批量预览时同样刷新观察结果；列表查询仅使用数据库观察结果，不对分页中的来源逐个实时读文件。列表显示`UNCHECKED/STALE`及最近检查时间，增加Manifest待处理计数和筛选。来源不可用单独记录，不误作文件缺失。
- 自动扫描在来源发现和已有来源扫描后，仅对已启用媒体库的Gallery执行最多100项/轮、最多30秒的Manifest读取与状态对比，并逐项保存游标；取消后可从下一Gallery继续，单项错误有计数但不阻断后续项目。不执行自动Push、Pull或冲突决策。新建Draft时从已有Manifest Pull的旧业务仍保留。
- Gallery列表支持跨页保持最多100个手动选择，先对每项重新检查并展示数据库快照相对基线的实际业务内容变化、本地文件变化及阻断原因。执行模式为“跳过本地更改”或“覆盖本地更改”；`DB_DIRTY`且实际业务内容变化正常写回，只有revision变化但内容未变不重写文件，并在校验文件哈希后确认同步基线为`CLEAN`。覆盖只适用于可解析且`set_id`相符的Manifest，执行前重新复核数据库revision、目标路径和文件哈希；来源不可用、只读库、无效Manifest必跳过，单项失败不影响其他项，结果逐项回报。既有单Gallery Push仍保留原有文件改动阻断。批量确认不扩大定期巡检的写入权限。
- 定向回归覆盖schema v12→v13、观察状态持久化、生命周期revision误报去重、数据库单方变更、预览后本地文件变化的失败保护、用户选择跳过/明确覆盖，以及定时扫描不写Manifest；Web新增列表筛选和批量预览/跳过交互测试。正式三标签Go测试及Vet、Web 34文件121项测试、TypeScript和685模块生产构建通过，仅见既存主chunk大小提示。本轮未提交、迁移正式库或增量部署；正式业务库仍为schema v12。下一次部署前需先做一致性完整备份，并在迁移后核对数据库完整性、业务计数、健康探针及真实浏览器批量确认交互。

### 提交、备份、schema v13迁移与增量部署

- 功能与部署前记录提交为`4bd55881522afe0bf3f863682a2891c604cb76ec`（`Add Manifest inspection and batch push workflow`）。清洁提交使用Go 1.25.12及`cgm_web_embed cgm_galleryepic cgm_moegirl`构建，Go VCS元数据确认`modified=false`且revision匹配；正式二进制SHA-256为`e9ea968c0a7dd64c050f63338c23fbe00dc52cdf7e7a06b847758c142d0ba4b9`，About构建时间为`2026-09-18T14:46:30Z`。
- 2026-09-18 22:47 CST停服并确认`MainPID=0`，在独立0700目录`/home/rainbowrunner/cos/bk/cgm-pre-manifest-4bd5588-rfepLp`保存schema v12 SQLite一致副本、旧二进制、配置、用户systemd单元和完整Coser托管根；旧程序、配置、单元和Coser树与原件逐项比较一致。备份库为0600，`integrity_check=ok`，Coser/Work/Character/Tag/Gallery/Source/Item计数为`135/108/697/23/8/8/570`。备份数据库SHA-256为`cfe7efd85b85912a9fb6f688b2980e5ccf0da3e940b86d1d17777baecc85b183`，旧程序为`8f31c00adafca2ade12da2a652b9bde88cdd67962cde824296f39486212933e6`。不包含原始媒体、缓存或日志。
- 逐字节校验候选后原子替换程序并于22:48 CST启动一次。正式库成功迁移至schema v13，两个新巡检表及游标行存在，`integrity_check=ok`，上述七类业务记录计数不变。自动生成的迁移前快照`product.sqlite.pre-schema-v12-1789742911708134619.bak`独立检查为schema v12且完整性`ok`。服务`active/running`、`NRestarts=0`，Health/Ready为204，首页和Gallery后台深链200；About精确报告提交`4bd5588`且`exactSourceAvailable=true`。本次启动日志未见WARN、ERROR、FAILED、panic或fatal；原配置、媒体、Manifest和缓存未替换，未远端推送。真实浏览器的跨页勾选、预览、跳过与覆盖确认仍待所有者业务验收，不曾用正式媒体执行批量Push。

### collection2 的 Manifest 写回权限调整

- 2026-09-18 22:53 CST首次正式批量Push审计为6项选中、0项写入、6项跳过、0项失败；六项均绑定于`collection2`，其媒体库`read_only=1`，所以在写入前被`READ_ONLY_LIBRARY`门禁跳过。另一媒体库没有绑定Gallery。宿主机实际挂载为可写，八个已绑定来源的Manifest目标父目录均具备写权限；沙箱内只读挂载探测结果不代表正式宿主机权限。
- 按所有者要求，仅将`collection2`媒体库改为允许Manifest sidecar写回，保留另一个`cosplay_collection`媒体库的只读设置。停服确认`MainPID=0`，在独立0700目录`/home/rainbowrunner/cos/bk/cgm-pre-collection2-write-hoxRmM`保存并核对schema v13 SQLite一致副本、旧程序、配置、用户systemd单元和完整Coser托管根；备份数据库`integrity_check=ok`，SHA-256为`3a11347d626de8a97b29b8dfbcfaaed42b6f887468687762da60f95f54f7f356`。在限定库ID、名称、根路径和原只读值的事务中，仅更新`media_libraries.read_only`及更新时间；库1仍为1，库2为0，正式库完整性`ok`，其八个来源绑定不变。随后服务`active/running`、`NRestarts=0`，Health/Ready为204。没有修改原始图片、视频、存档、媒体权限或Manifest，也没有自动重试批量Push。当前“媒体只读”是CGM不重写媒体内容的应用行为，不等于操作系统对媒体文件的写权限隔离。

# 2026-09-19 媒体只读与元数据旁置文件写回策略解耦（本地源码）

- schema v14在`media_libraries`新增`metadata_writeback_enabled`，迁移对每个旧库执行`NOT read_only`映射；不改变旧列或原始媒体/Manifest。正式库当前`collection2.read_only=0`，部署后将得到写回开启；`cosplay_collection.read_only=1`将得到写回关闭。新建库的数据库与GraphQL默认值为开启，Manage创建页也默认开启；旧`read_only`字段退为兼容遗留，不再决定Push。
- Media Libraries页显示当前写回状态和已绑定Gallery数，开启/关闭均需要显式确认，API以库`updated_at_utc`作为乐观并发令牌，记录管理审计；原始媒体仍由既有只读扫描和物化路径读取。单项和批量Manifest Push均查新策略；批量预览额外只读检查目标父目录真实路径及当前进程写权限，显示稳定阻断原因，所有选中项被阻断时禁用执行。实际写入仍以原子写入器及执行时复核为准；预览不能保证之后挂载/权限不变化。全阻断的批量执行不再以成功审计掩盖零写入。
- 定向回归覆盖v13→v14旧库策略继承、新库默认、过期设置拒绝、单项/批量门禁、写入sidecar不修改原始媒体、GraphQL鉴权及Web设置/预览交互。正式三标签Go产品库/API/Server/PortableCatalog/Product/CMD测试及同范围Go Vet、Web 34文件123项测试、TypeScript和685模块生产构建均通过；仅保留既有主chunk超过500KiB的提示，`git diff --check`通过。正式服务、业务库、媒体和Manifest尚未由本轮源码修改；部署需先提交、停服备份、迁移，再核对两个库写回策略及健康探针。

### schema v14提交、备份与正式部署

- 功能与部署前记录提交为`a1f36778427a4b07b6e5a5125d3fdd1e1b1e7c1e`（`Separate media reads from metadata writeback policy`）。清洁提交使用Go 1.25.12与`cgm_web_embed cgm_galleryepic cgm_moegirl`构建，Go VCS元数据确认`vcs.modified=false`、revision一致；正式二进制SHA-256为`200b3e7bb4b11978c54b96771909c0ebab80621affea8d3f4bd734bf3b1dcfcf`，About构建时间为`2026-09-19T04:09:22Z`。
- 2026-09-19 12:09 CST停服并确认`MainPID=0`，在独立0700目录`/home/rainbowrunner/cos/bk/cgm-pre-writeback-v14-a1f3677-pYln6T`备份schema v13 SQLite一致副本、旧二进制、配置、用户systemd单元和完整Coser托管根；程序、配置、单元及Coser树与原件逐项比较一致。备份数据库`integrity_check=ok`，SHA-256为`d958d1b2c6bda8d15d5fc7f7c8d1c7360490c69017f1e33ef2211a4e6541ce1d`，旧二进制为`e9ea968c0a7dd64c050f63338c23fbe00dc52cdf7e7a06b847758c142d0ba4b9`；备份时两个媒体库旧`read_only`依次为1和0，Coser/Work/Character/Tag/Gallery/Source/Item为`135/108/697/23/8/8/570`。回滚目录不含原始媒体、缓存或日志。
- 候选二进制逐字节校验后原子替换，12:11 CST启动一次并成功自动迁移至schema v14。正式库`integrity_check=ok`，上述业务计数不变；旧库1继承为`metadata_writeback_enabled=0`，`collection2`继承为1。自动迁移前快照`product.sqlite.pre-schema-v13-1789791067361095306.bak`独立检查为schema v13且完整性`ok`。服务`active/running`、`NRestarts=0`，Health/Ready为204，媒体库与Gallery后台深链200；About精确指向`a1f3677`且`exactSourceAvailable=true`，本次启动日志无WARN、ERROR、FAILED、panic或fatal。未自动执行Manifest Push、未修改媒体/Manifest/配置/缓存，未远端推送。真实浏览器的媒体库开关和正式批量Push由所有者继续验收。

# 2026-09-19 作品集 Source/Manifest 状态帮助说明（已部署）

- 按用户要求保持 Gallery 列表中的 Source、Manifest 短语与现有交互不变，只在 Manage → Help 新增独立的“作品集列表：Source 与 Manifest”中英双语指南和跳转列表入口。Source 分开解释来源可用状态（`AVAILABLE/MISSING/UNREADABLE`）、成员扫描对账状态（`NEVER_SCANNED/SCANNING/IN_SYNC/NEEDS_RESCAN/ERROR`）及最近扫描错误码；Manifest 逐项解释`UNCHECKED/STALE/NONE/CLEAN/DB_DIRTY/FILE_DIRTY/CONFLICT/MISSING/ERROR/SOURCE_UNAVAILABLE`。
- 说明列表 Manifest 取最近一次已保存巡检、不是每行实时读取；`IN_SYNC`不等于`CLEAN`，`STALE`不直接证明文件损坏，`NONE`与已建立基线后的`MISSING`不同。增加检查时间、批量 Push 复核、媒体库写回策略及父目录可写条件的说明。帮助页第二篇文章固定在桌面正文列，窄屏恢复单列；不修改任何产品数据或后台 Gallery 列表。
- 回归：新增中英文帮助页渲染及状态说明断言；TypeScript检查、Vitest 34文件124项和Vite生产构建（685模块）通过，仅见既存主chunk大小提示；`git diff --check`通过。纯前端与开发文档变更，无后端、数据库schema、业务库、Manifest或媒体写入。

### 提交、备份与无 schema 增量部署

- 功能和部署前记录提交为`4f14073d28c8d2ae00972eec09e43edf5e93bc5d`（`Explain Gallery Source and Manifest statuses in Help`）。正式三标签单文件构建`vcs.modified=false`且revision一致，候选SHA-256为`012c8d64a8103d87b0faef5f09b3c31bbeceb92064a3af3bb4d4bb0e64bbd726`；仅见既存主chunk大小提示。部署前服务`active/running`、Health 204。
- 2026-09-19 12:35 CST停服确认`MainPID=0`，在0700回滚目录`/home/rainbowrunner/cos/bk/cgm-pre-help-4f14073-RmOG3z`保存一致SQLite副本、旧二进制、配置、用户服务单元和完整Coser托管目录；原件与副本逐项核对，Coser目录递归比较无差异。备份库`integrity_check=ok`，SHA-256为`98ee99c787626851a0800e9b97472293052bd15ac7a6125824c3dcaaf0ea1c20`；旧程序SHA-256为`200b3e7bb4b11978c54b96771909c0ebab80621affea8d3f4bd734bf3b1dcfcf`。备份不含原始媒体、缓存或日志。
- 候选复制至正式程序同目录并逐字节校验后原子替换，12:36 CST启动一次。服务`active/running`、`NRestarts=0`，Health/Ready均204，`/manage/help`及新增Help JS、CSS资源均200；About精确报告`4f14073`且`exactSourceAvailable=true`。正式库`integrity_check=ok`，备份与部署后Coser/Work/Character/Tag/Gallery/Source/Item计数均为`135/108/697/23/8/8/570`；本次启动无warning以上日志。未迁移schema，未修改正式配置、原始媒体、Manifest或缓存；未远端推送。

# 2026-09-19 后台作品集跨页搜索（已部署）

- Manage → Gallery 现有状态卡和每页24项分页均保留，新增搜索框，对全库Gallery标题、别名、Slug、Set ID及已绑定来源路径执行服务端关键词匹配；结果数量、页数和列表行按状态卡条件与关键词的交集计算。顶部状态卡仍展示全库统计，用于随时切换问题类别；搜索词持久于URL，切换卡片或翻页不会丢失，修改搜索词与切换卡片均重置页码。输入350ms防抖、最长300字符，清除按钮恢复未搜索列表，零结果明确提示；批量Manifest选择与执行流程保持原样。
- 产品库查询沿用既有Manage可见边界和固定排序，不读取额外媒体文件；`LIKE`模式转义`%`、`_`、反斜杠并参数化，避免把用户文本作为SQL条件。GraphQL新增默认空串的`search`参数并重新生成执行层，旧调用保持兼容。不新增表、索引、schema或数据迁移。
- 定向产品库测试覆盖标题/别名/路径/ID、状态交集、空结果、字面百分号和超长输入；前端测试覆盖跨页重置、URL保留、状态组合、清除和空结果提示。正式三标签产品库/API/Server/CMD测试及Go Vet、Web 34文件126项测试、TypeScript和685模块生产构建通过，仅见既存主chunk大小提示。部署前正式业务服务仍运行上一轮`4f14073`二进制。

### 提交、备份与无 schema 增量部署

- 功能与部署前记录提交为`970e251b8260c8742d043dbd0ce369208ecfcdb6`（`Add server-side search to Manage Gallery list`）。正式三标签二进制已核对`vcs.modified=false`、revision一致、构建时间`2026-09-19T05:06:55Z`；候选SHA-256为`61ad515a36b1628cf4ee0a2abe9da86dca8b7948d47b837f2a5b655725d5ae09`。部署前服务`active/running`、Health 204。
- 2026-09-19停服确认`MainPID=0`，在0700回滚目录`/home/rainbowrunner/cos/bk/cgm-pre-gallery-search-970e251-TPV9xS`保存一致SQLite副本、旧二进制、配置、用户服务单元和完整Coser托管目录；原件与副本逐项核对，Coser目录递归比较无差异。备份库`integrity_check=ok`、SHA-256为`f9a7b9052b72d68c4692399c2f109111b415ed23fdd1245b065a951ffdab0dd2`；旧程序SHA-256为`012c8d64a8103d87b0faef5f09b3c31bbeceb92064a3af3bb4d4bb0e64bbd726`。备份不含原始媒体、缓存或日志。
- 候选复制至正式程序同目录并逐字节校验后原子替换，13:09 CST启动一次。服务`active/running`、`NRestarts=0`，Health/Ready均204，`/manage/gallery`、新Gallery列表JS和CSS资源均200；About精确报告`970e251`且`exactSourceAvailable=true`。正式库`integrity_check=ok`，备份与部署后Coser/Work/Character/Tag/Gallery/Source/Item计数均为`135/108/697/23/8/8/570`；本次启动无warning以上日志。未迁移schema，未修改正式配置、原始媒体、Manifest或缓存；未远端推送。
