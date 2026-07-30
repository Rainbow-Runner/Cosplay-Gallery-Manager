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
- 2026-07-30 02:40 CST安装UI-03～UI-08 GalleryEpic视觉改造构建；当前二进制SHA-256为`38af89d5afe479a6f38d88d3a08735b68a03044c5648cf5a00f28a79e089dded`，替换前二进制另存于`/tmp/cgm-before-galleryepic-ui-20260730`。
- 本次部署后用户服务为`enabled/active`，`/healthz`与`/readyz`均返回204，`/`与`/legal`均返回200；首页实际引用`index-DMxPCGgd.js`、`index-De7_OG72.css`以及本轮GalleryCard、GalleryDetail和Patterns哈希Chunk。
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
