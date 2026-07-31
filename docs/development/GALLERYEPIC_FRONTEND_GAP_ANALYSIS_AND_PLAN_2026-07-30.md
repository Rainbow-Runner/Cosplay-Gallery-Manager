# GalleryEpic风格前端差距分析与改造计划

> 文档状态：后续前端改造指导基线
>
> 分析日期：2026-07-30
>
> 适用范围：Cosplay Gallery Manager 1.5后续本地增量开发
>
> 本轮工作：只分析、记录和规划，不修改现有页面代码
>
> 参考对象：2026-07-30可公开访问的GalleryEpic英文站桌面端与移动端

## 1. 结论摘要

当前CGM已经具备第一版主要Browse和Manage功能，但视觉实现没有达到开发备忘录要求的“GalleryEpic风格”：

- 当前BrowseShell采用深色杂志/作品集风格：黑色背景、暖灰和棕色强调色、Georgia衬线大标题、横向顶部导航、大面积装饰性留白。
- GalleryEpic当前公开站采用浅色应用/图库风格：白色背景、中性黑灰、Inter无衬线字体、固定左侧导航、64px顶部品牌栏、紧凑3:4图片卡片、Lucide式线性图标。
- 当前差距不是只替换背景色即可解决。页面骨架、信息密度、字体层级、导航方向、卡片内部结构、图标体系、控件形态和移动端导航都需要统一调整。
- 现有GraphQL、路由、Gallery聚合、响应式列数、Scrubber、LIST/MAGIC、收藏评分、Manage工作流和离线约束可以保留。建议把改造限制在设计Token、共用组件和页面结构层，避免重写已经验证的业务逻辑。
- GalleryEpic没有CGM的完整管理后台、Manifest、扫描、备份、任务和恢复功能。这些扩展功能不能机械复制参考站页面，而应使用同一浅色Token、侧栏、顶部栏、按钮、表格、表单、Badge、Alert和Dialog语言进行“同风格扩展”。

建议把当前GalleryEpic公开站冻结为本轮视觉参考版本，以高保真还原其页面几何和视觉语言为目标；但继续严格遵守“不复制GalleryEpic品牌、代码或媒体资产”的确认约束。

## 2. 规范依据与证据范围

### 2.1 已确认的本地规范

开发备忘录和第一版计划已经明确：

- 新建GalleryEpic风格的Browse前台，同时重建Gallery聚合管理后台。
- 建立GalleryEpic风格的卡片、文字索引、详情布局和媒体网格设计Token。
- Gallery卡片固定3:4，并按6/5/4/3/2列响应。
- 卡片Scrubber、R-18徽标、收藏和评分不能破坏卡片布局。
- 前端资产全部本地化，服务端零主动外联，断网时核心功能完整可用。
- 不复制GalleryEpic品牌、代码或媒体资产。
- 不引入热门、浏览次数、趋势、下载、评论或社交互动等已经明确排除的产品语义。

对应规范：

- [COSPLAY_DEVELOPMENT_MEMO.md](../COSPLAY_DEVELOPMENT_MEMO.md)，重点为第18、20、25、31、33节。
- [COSPLAY_V1_DEVELOPMENT_PLAN.md](../COSPLAY_V1_DEVELOPMENT_PLAN.md)，重点为P06、P07和P08。

### 2.2 GalleryEpic公开页面取证

本次直接观察以下公开页面：

- [GalleryEpic首页](https://galleryepic.com/en)
- [GalleryEpic Cosplay列表](https://galleryepic.com/en/cosplays/1)
- [GalleryEpic Coser列表](https://galleryepic.com/en/cosers/1)
- [GalleryEpic公开Gallery详情示例](https://galleryepic.com/en/cosplay/10238)

观察视口：

- 桌面：1440×1000
- 移动：390×844

同时读取了公开页面加载的CSS变量和DOM class，用于核对颜色、字体、侧栏宽度、顶部栏高度、网格断点、间距和圆角。

### 2.3 证据边界

- GalleryEpic是持续变化的外部站点。本文件只描述2026-07-30观察到的版本；后续站点改版不应自动改变CGM设计。
- 本次没有登录GalleryEpic，因此无法观察其登录后页面或私有管理功能。
- 广告、下载、热门、浏览量、下载量和评论属于参考站业务，不属于CGM产品范围，只能观察其版面几何，不能复制其产品语义。
- 为避免复制第三方媒体和品牌资产，本次参考截图只保存在`/tmp`用于分析，不提交仓库。仓库后续只提交CGM自有测试夹具和视觉回归图。

## 3. GalleryEpic当前视觉基线

### 3.1 页面骨架

#### 桌面

- 左侧固定导航栏：
  - `lg`宽度约13rem；
  - `xl`宽度约16rem；
  - 白色/极浅灰背景；
  - 右侧1px浅灰分隔线；
  - 导航按Home、Cosplay、Album等分组纵向排列。
- 主内容区：
  - 宽度为视口减去侧栏；
  - 自身滚动，而不是整页与侧栏一起滚动；
  - 顶部固定/粘性64px品牌栏；
  - 品牌图标和“Gallery Epic”文字水平居中。
- 内容边距：
  - 桌面左右约32px；
  - 页面不使用超大居中标题和大面积顶部留白；
  - 内容从顶部栏后快速进入通知、面包屑、搜索或卡片网格。

#### 移动

- 左侧栏收起为抽屉。
- 顶部栏保留64px高度：
  - 左侧菜单图标；
  - 中间品牌；
  - 右侧主操作。
- 内容左右约32px。
- Gallery卡片保持两列，文字、Coser头像行和卡片操作仍完整显示。
- 不使用可横向滚动的完整桌面导航代替移动菜单。

### 3.2 精确基础Token

GalleryEpic公开CSS在默认浅色模式中使用以下核心值：

| 语义 | GalleryEpic观察值 | 视觉作用 |
| --- | --- | --- |
| 页面背景 | `#ffffff` | 主画布 |
| 主文字 | `#0a0a0a` | 标题、正文、图标 |
| 主要深色 | `#171717` | 主按钮和强调 |
| 次级表面 | `#f5f5f5` | Hover、Skeleton、弱背景 |
| 弱文字 | `#737373` | 副标题、日期、说明 |
| 边界 | `#e5e5e5` | 侧栏、输入框、分隔线 |
| 侧栏背景 | `#fafafa` | 侧栏或弱分区 |
| 危险色 | `#e40014` | 仅危险/破坏操作 |
| 焦点Ring | `#a1a1a1` | 键盘焦点 |
| 基准圆角 | `.625rem` | 约10px |

参考站也定义了暗色Token，但当前公开页面默认使用浅色模式。本轮如果目标是高保真还原，不应继续以当前CGM深色主题作为默认基线。

### 3.3 字体与文字层级

- 全站使用Inter可变字体及Arial兼容Fallback。
- 标题、卡片标题、侧栏标题和正文均为无衬线，不混用衬线展示字体。
- 常见层级：
  - 页面/详情主标题：约28～32px、600～700；
  - 侧栏分组标题：18px、600；
  - 卡片标题：约16px、500；
  - Coser行：约14px、500；
  - 副标题、作品来源、计数：12px、常规字重；
  - 导航和按钮：14px、500。
- 文字基本不使用全大写和宽字距装饰。
- 信息层级主要依靠字号、字重和中性灰，而不是暖色强调和衬线字体。

### 3.4 图标

- 导航主要使用Lucide风格：
  - 24×24 viewBox；
  - `currentColor`；
  - 2px描边；
  - 圆角线帽和连接。
- 图标与文字通常间隔8px。
- 图标直接承担Home、Gallery、Coser、Parody、语言和邮件等语义。
- 下载图标使用更实的向下箭头，但整体仍保持单色黑色。
- 没有使用`⌕`、`♡`、`ⓘ`、`⚙`等依赖字体字形的符号作为主图标系统。

### 3.5 Gallery列表卡片

- 图片固定3:4。
- 桌面首页在1440视口为4列；公开CSS在更宽断点可到5列，移动为2列。
- 卡片之间约16px横向间距。
- 图片约8px圆角，无厚边框，阴影很弱或没有明显阴影。
- Hover时图片在固定裁切容器内轻微放大，卡片整体尺寸不变化。
- 媒体数量在图片右下角用12px白字叠加。
- 图片下方结构紧凑：
  1. Character/主标题行，右侧保留一个图标操作位；
  2. Work/Parody灰色副标题；
  3. 32px圆形Coser头像+Coser名称。
- 没有独立的“COSPLAY/ALBUM”装饰眉题，也没有大面积卡片阴影。

### 3.6 Gallery详情

桌面详情采用三段式结构：

1. 固定左侧导航；
2. 中央主内容；
3. 右侧窄辅助栏。

中央内容：

- 顶部面包屑；
- 紧凑的粗体主标题；
- 标题下方一行弱化日期和统计信息；
- 主操作位于标题区域右侧；
- 媒体直接以4列紧密网格开始展示；
- 图片使用小圆角，不先显示独立大封面Hero。

右侧辅助栏：

- 参考站使用“Hottest”列表；
- 每项为横向小缩略图、两行标题和弱化日期。

CGM不能引入热门语义，但可复用这一几何位置显示“相关作品”或“近期收录”。

### 3.7 Coser列表

- 页面顶部为面包屑和居中的搜索框。
- 主体使用4列高密度文字列表。
- 每项为约36px圆形头像+名称，没有大边框卡片。
- 分页为居中的紧凑数字分页，当前页使用浅边框圆角按钮。

### 3.8 指定Coser详情页冻结基线（2026-07-31）

本轮按用户指定的[GalleryEpic Coser详情页](https://galleryepic.com/zh/coser/298/1)重新取证，桌面使用1440×1000视口，移动使用390×844视口。只观察页面自身内容，广告及广告造成的额外留白不作为CGM实现依据。

桌面测量：

- 主内容在256px侧栏之后从`x=288`开始，可用宽度1120px；顶部栏仍为64px。
- Breadcrumb位于`y=77`，14px常规字重、20px行高，距4:1 Banner顶部12px。
- Banner为1120×280px，严格4:1；资料白底区高约94px。
- 头像为128×128px方形图，白色内边距8px、1px浅灰边界，向上覆盖Banner约51px。
- Coser名称为20px/600/28px；社交图标为20～24px单色图标，紧跟在名称下方。
- 筛选控件高36px、8px圆角、1px浅灰边界；资料区到控件约24px，控件到Gallery网格约24px。
- 参考站此视口为4列，每列268px、横向间距16px；CGM继续服从已经确认的6/5/4/3/2列产品约束，只复用卡片比例、文字密度与间距语言。
- 分页居中显示：36×36px左右箭头与页码按钮，按钮间距4px；当前页使用1px浅灰边界、8px圆角和极弱阴影，参考样本显示连续`1 2 3 4 5`。

移动测量：

- 内容左右各32px，可用宽度326px；Breadcrumb后12px进入Banner。
- Banner约326×81.5px，头像为80×80px并向上叠入Banner；名称为18px，白字叠在Banner下沿。
- 社交图标落在Banner下方白色资料区；整个Banner+资料区约124px高。
- 控件改为纵向两行，Gallery为2列；卡片约155×206px，横向间距16px。
- 移动分页继续使用36px数字按钮，不改成大号“上一页/下一页”文本按钮。

实现边界：

- 不复制参考站Logo、媒体、广告、代码或远程图标；截图只保存在`/tmp`用于测量。
- 不引入参考站的下载、作品/角色筛选、浏览量或广告功能。CGM现有Scope与Shoot timeline继续保持原语义，只采用相同的控件几何。
- Coser头像、Banner、Country、社交账号、Profile、Biography与Timeline能力全部保留；没有Banner时使用本地中性占位，不访问互联网。
- Gallery分页继续由本机GraphQL的`page/totalPages`驱动，并写入现有`scope/page`查询参数，不改变24项页大小、DTO或数据库。

## 4. 当前CGM前端实现基线

### 4.1 当前设计Token

[index.css](../../ui/web/src/styles/index.css)当前核心值为：

| 语义 | 当前CGM值 |
| --- | --- |
| 页面背景 | `#111012`并叠加紫棕色径向渐变 |
| 表面 | `#181619` |
| 提升表面 | `#211d21` |
| 主文字 | `#eee9e3` |
| 弱文字 | `#9f9795` |
| 强调色 | `#b69a7c` |
| 成人色 | `#a43144` |
| 分隔线 | `rgba(255,255,255,.09)` |

这组Token形成暖黑、棕灰、画册式视觉，与GalleryEpic默认白底中性灰基线相反。

### 4.2 当前BrowseShell

[BrowseShell.tsx](../../ui/web/src/browse/BrowseShell.tsx)使用：

- 横向粘性顶部栏；
- 左侧`CGM COLLECTION`字标；
- 中部9个横向导航项；
- 右侧4个圆形字符图标按钮。

移动端仍保留横向导航并允许滚动，没有参考站的菜单抽屉。

### 4.3 当前文字系统

- 正文使用Inter/System Sans。
- 品牌、页面主标题、Gallery标题、Coser详情、Section标题、Manage标题和大量表格重点字段使用Georgia/Times New Roman。
- 页面标题最高达到约6rem～7rem。
- 大量眉题使用全大写、较宽字距和暖色。

这使当前页面更像杂志封面或摄影作品集，而不是GalleryEpic的紧凑图库应用。

### 4.4 当前Gallery卡片

[GalleryCard.tsx](../../ui/web/src/browse/GalleryCard.tsx)具备正确的：

- 3:4封面；
- Poster Scrubber；
- R-18三角标；
- 收藏和评分摘要；
- P/S/G/V计数；
- 响应式6/5/4/3/2列。

但卡片视觉结构差异明显：

- 图片有明显深色阴影；
- 收藏为图片内圆形悬浮按钮；
- 图片下先显示全大写ALBUM/COSPLAY眉题；
- Gallery标题使用Georgia；
- Coser/Character/Work只合并成一行文字，没有参考站的“主标题+作品来源+Coser头像行”三层结构；
- P/S/G/V位于正文底部，而不是参考站的封面右下角；
- 卡片垂直留白更大、信息密度更低。

### 4.5 当前Gallery详情

[GalleryDetailPage.tsx](../../ui/web/src/browse/GalleryDetailPage.tsx)当前采用：

- 左侧大封面+右侧超大标题Hero；
- Hero之后再进入内容区；
- 媒体桌面8列，移动3列；
- 相关Gallery放在页面底部；
- 深色全屏Lightbox。

与参考站相比：

- 缺少面包屑；
- 首屏被Hero和大标题占据，媒体到达位置太晚；
- 标题字号和衬线风格差距很大；
- 媒体网格密度过高，缩略图明显更小；
- 没有桌面右侧辅助栏；
- 详情分区线和标题更像章节式长页。

### 4.6 当前实体页

- Coser索引使用带四边框的3/4列大文字Tile，参考站使用无边框头像文字行。
- Work、Character和Tag采用同一种边框矩阵，缺少参考站的搜索框、面包屑、轻量分页和留白节奏。
- Coser详情使用大头像、可选Banner和超大Georgia标题；功能完整，但视觉与参考站不一致。

### 4.7 当前ManageShell

[ManageShell.tsx](../../ui/web/src/manage/ManageShell.tsx)已经使用固定左侧栏，这是现有页面中最接近参考站骨架的部分。

主要差距：

- 仍为深色背景和暖色强调；
- 侧栏标题使用大Georgia；
- 页面主标题过大；
- 表单和Panel为深色大边框块；
- 表格字重、状态和操作缺少统一浅色组件Token；
- BrowseShell与ManageShell虽然共享变量，但视觉上不是参考站的同一产品家族。

### 4.8 当前移动端

现有视觉回归：

- [Browse桌面截图](../../ui/web/e2e/offline-lifecycle.spec.ts-snapshots/browse-desktop-linux.png)
- [Gallery移动截图](../../ui/web/e2e/offline-lifecycle.spec.ts-snapshots/gallery-mobile-linux.png)

主要问题：

- 顶部导航在移动端变成多行/横向滚动文本，而不是菜单抽屉；
- 黑底和巨大标题占用大量纵向空间；
- Gallery详情移动端先显示大封面和大标题，媒体首屏到达较慢；
- 图标仍是字体符号，跨平台字形不稳定；
- 关键功能可用，但与参考站两列紧凑内容流的节奏差距很大。

## 5. 主要差距矩阵

| 维度 | GalleryEpic基线 | 当前CGM | 差距等级 | 建议优先级 |
| --- | --- | --- | --- | --- |
| 默认主题 | 白底中性灰 | 暖黑渐变 | 根本性 | P0 |
| 桌面导航 | 固定左侧栏 | Browse横向顶栏 | 根本性 | P0 |
| 移动导航 | 64px顶栏+抽屉 | 横向导航 | 根本性 | P0 |
| 字体 | 全Inter无衬线 | Inter+大量Georgia | 根本性 | P0 |
| 图标 | 统一Lucide线性图标 | Unicode字符混用 | 高 | P0 |
| 页面标题 | 紧凑28～32px | 最高6～7rem | 高 | P0 |
| 页面留白 | 32px、快速进入内容 | 2.5～6rem顶部留白 | 高 | P0 |
| Gallery卡片 | 图片主导、紧凑三层信息 | 装饰眉题+分散元信息 | 高 | P1 |
| Gallery详情 | 面包屑+紧凑标题+4列媒体+右栏 | 大Hero+8列媒体+底部相关 | 高 | P1 |
| Coser索引 | 头像文字行 | 边框Tile | 高 | P1 |
| 控件 | 轻边框、8～10px圆角 | 方形/圆形混杂、暖色边框 | 中高 | P0 |
| 状态反馈 | 中性Alert/Badge | 左边框消息、技术文案较多 | 中 | P2 |
| Manage | 无公开直接参考 | 深色管理台 | 需同风格扩展 | P2 |
| 响应式 | 两列移动、桌面侧栏 | 列数正确但壳层不同 | 高 | P0 |
| 无障碍 | 可见焦点、语义图标 | 已有ARIA基础但图标不统一 | 中 | 全阶段 |

## 6. 改造必须保持的产品边界

视觉高保真不能推翻已经确认的产品设计：

- 保留Gallery作为唯一Browse业务单元。
- 保留3:4卡片和6/5/4/3/2响应式列数。参考站当前桌面列数只能作为密度参考，不能覆盖备忘录的确认断点。
- 保留Poster Scrubber，但改用中性、弱化的预览轴。
- 保留R-18三角标、收藏、半星评分和P/S/G/V口径。
- 保留LIST/MAGIC/ALL、Scope、时间线、随机、收藏、历史和搜索。
- 保留Gallery详情每组首次24项、手工加载更多和完整轻量索引。
- 保留Coser头像/Banner、Work/Character/Tag无图片索引。
- 保留Manage五页签、Coser四页签、Manifest、扫描、任务、Operations和恢复流程。
- 不加入参考站的下载、热门、浏览次数、下载次数、广告、评论、用户体系或社交互动。
- 不复制参考站Logo、名称、媒体、广告、代码、远程字体文件或图标资源。
- 所有字体和图标必须随CGM本地打包，并更新许可证/SBOM。
- 不借视觉改造改变GraphQL DTO、数据库Schema、状态机或媒体文件安全边界。

## 7. 建议的目标设计系统

### 7.1 Token草案

建议先建立语义Token，不在页面组件中直接散落颜色：

```css
:root {
  --cgm-background: #ffffff;
  --cgm-foreground: #0a0a0a;
  --cgm-surface: #ffffff;
  --cgm-surface-subtle: #fafafa;
  --cgm-muted: #f5f5f5;
  --cgm-muted-foreground: #737373;
  --cgm-border: #e5e5e5;
  --cgm-primary: #171717;
  --cgm-primary-foreground: #fafafa;
  --cgm-danger: #e40014;
  --cgm-focus: #a1a1a1;
  --cgm-radius: 0.625rem;
}
```

要求：

- 默认只实现浅色基线；不要在本阶段同时保留旧深色主题，避免每个组件出现双套视觉分支。
- 危险色仅用于删除、恢复、清理和明确失败，不作为品牌强调色。
- R-18可继续使用独立成人色，但面积必须小。
- 成功、警告和信息状态使用低饱和背景+文字，不把整个页面染色。

### 7.2 字体

推荐：

- 本地打包Inter Variable，正文和标题统一使用；
- CJK回退使用系统无衬线字体栈；
- 移除Georgia/Times New Roman的页面级使用；
- 如果引入Inter字体文件，先确认可再分发许可证并更新SPDX/Third-Party Notices；
- 若暂不引入字体文件，先用`Inter, ui-sans-serif, system-ui`完成结构，但不能依赖远程Google Fonts或GalleryEpic字体URL。

建议层级：

| 用途 | 建议 |
| --- | --- |
| 页面主标题 | 30px / 1.2 / 650 |
| Section标题 | 20px / 1.3 / 600 |
| 卡片主标题 | 16px / 1.25 / 500 |
| 正文/导航 | 14px / 1.5 / 400～500 |
| 弱信息 | 12px / 1.4 / 400 |
| 技术表格 | 12～13px / 1.45 |

### 7.3 图标

建议引入一个本地、可Tree-shake的Lucide React图标实现，统一替换Unicode字符。

基础映射：

| CGM功能 | 建议图标 |
| --- | --- |
| Home | `House` |
| List | `Images`或`Camera` |
| Magic | `Sparkles` |
| Coser | `UserRound`或`SmilePlus` |
| Work | `Palette`或`BookOpen` |
| Character | `ContactRound` |
| Tag | `Tags` |
| Timeline | `CalendarDays` |
| Random | `Shuffle` |
| Search | `Search` |
| Favorites | `Heart` |
| History | `History` |
| Manage | `Settings` |
| Scan | `ScanSearch` |
| Manifest | `FileJson` |
| Backup | `Archive` |
| Warning | `TriangleAlert` |

全部图标必须：

- 使用`currentColor`；
- 常规尺寸16/18/20px；
- 导航可用20px；
- 有Tooltip和`aria-label`；
- 不单独依靠颜色表达状态。

### 7.4 共用组件

在逐页改造前，先建立：

- `AppLogo`：CGM原创本地图标+文字，占位尺寸贴近参考站，但不能复制GalleryEpic幽灵Logo。
- `Button`：primary、secondary、ghost、danger。
- `IconButton`。
- `Input`、`Select`、`Textarea`、`Checkbox`。
- `Badge`、`StatusBadge`、`IssueBadge`。
- `Alert`、`InlineNotice`。
- `Breadcrumbs`。
- `Pagination`。
- `Tabs`、`SegmentedControl`。
- `CardMedia`、`Avatar`、`Skeleton`。
- `Dialog`、`Drawer/Sheet`、`ConfirmDialog`。
- `DataTable`、`EmptyState`、`ErrorState`。

不要直接照搬GalleryEpic组件代码；只使用本项目自行实现的React/CSS组件。

## 8. 页面改造建议

### 8.1 BrowseShell

目标结构：

```text
┌──────────────┬──────────────────────────────────────┐
│              │ 64px 顶部品牌栏                     │
│ 固定侧栏     ├──────────────────────────────────────┤
│              │ 页面内容独立滚动                     │
│              │                                      │
└──────────────┴──────────────────────────────────────┘
```

建议侧栏分组：

- Home
- Galleries
  - List
  - Magic
- Library
  - Cosers
  - Works
  - Characters
  - Tags
- Explore
  - Timeline
  - Random
  - Favorites
  - History
- 底部
  - Search
  - Manage
  - Language
  - Legal/About

这样复用GalleryEpic纵向分组节奏，但保持CGM自己的路由和语义。

移动端：

- 使用菜单图标打开Drawer；
- 顶栏中间显示原创CGM Logo；
- 右侧保留一个高优先级操作，可优先使用Search或Manage；
- Drawer关闭后焦点返回菜单按钮；
- Escape和点击遮罩关闭；
- 不再展示横向滚动的完整导航。

### 8.2 首页与Gallery索引

- 移除超大“Recently added”Georgia标题和装饰性LIST眉题。
- 使用紧凑页标题或在首页直接进入卡片区。
- 筛选和排序放在标题下方的单行工具栏，移动端折叠为Drawer或Popover。
- 保持备忘录确认的6/5/4/3/2列，不照搬参考站的4/5列断点。
- 卡片改造为：
  1. 3:4封面；
  2. 封面右下角显示媒体数量，沿用`P / S / G / V`类型缩写，但数量为0的类型不得渲染；数量写在类型前，例如仅有100张Photo时显示`100P`，混合媒体可显示`96P 4S 2V`，四类全为0时隐藏整段；
  3. 封面右上角保留小型R-18三角；
  4. 第一文字行显示Character或Gallery标题；
  5. 第二行显示Work；Album没有Work时保留稳定空行或显示Gallery标题；
  6. 第三行显示Coser头像组和名称；
  7. 收藏图标占用参考站下载图标位置，但语义仍为收藏；
  8. 评分摘要弱化放在Coser行末尾或卡片Footer。
- 卡片Hover只在封面容器内缩放，不产生布局位移。
- Scrubber轴放在封面底部，保持2px中性半透明，Hover结束恢复封面。

需要先决定卡片主标题口径：

- 推荐Cosplay卡第一行显示Character，第二行显示Work，Coser第三行；Gallery正式标题放入Tooltip/无障碍名称，或在多角色/多Work时回退为Gallery标题。
- Album没有Character/Work时第一行显示Gallery标题，第二行保持空白或弱化`Album`。
- 多Coser使用头像组+去重名称。

这个映射最接近GalleryEpic，同时不丢失CGM多角色、多Coser模型。

### 8.3 Gallery详情

推荐桌面布局：

```text
主内容区
├── Breadcrumb
├── 标题 + 个人操作
├── 日期 / Coser / Work / Character / 计数
└── 4列媒体网格

右侧辅助栏
├── Related galleries
└── 无相关结果时 Recent additions
```

具体建议：

- 移除首屏独立大封面Hero；effective cover作为媒体网格第一项视觉来源，或在标题旁以小缩略图显示。
- 主标题改为Inter 28～32px粗体。
- 拍摄日期与收录日期继续遵守“只用图标、字号、颜色区分”的备忘录要求。
- Coser、Character和Work使用紧凑链接行或头像组。
- 媒体桌面改为4列；1024可3列，768及移动2列。
- 保留Photo/Selfie、GIF、Video分组和每组24项加载，但组标题缩小为紧凑行。
- 右栏不得使用“Hottest”：
  - 有强关系推荐时显示“Related”；
  - 无推荐时显示“Recently added”；
  - 每项使用横向小缩略图、两行标题和弱日期；
  - 不显示热度、浏览数或下载数。
- 移动端右栏移动到媒体组之后，不能插入第一屏。
- ExternalLink继续放在元数据末尾弱化显示。

### 8.4 Lightbox与媒体详情

- Lightbox可以保留深色遮罩，因为这是查看媒体的功能场景，不代表全站主题。
- 控件统一改为Lucide：
  - `X`、`ChevronLeft`、`ChevronRight`、`Heart`、`Star`、`Info`。
- 当前Lightbox首尾循环与备忘录“首尾不循环”不一致，视觉改造阶段应单独建功能核对任务，不能借样式修改静默改变。
- 媒体详情继续保持“媒体主体+Gallery侧栏”，但侧栏改为白色/浅灰表面和参考站紧凑标题。
- GIF/Video状态用小型Badge，不使用大色块。

### 8.5 Coser、Work、Character和Tag

Coser索引：

- 顶部Breadcrumb；
- 中央Search；
- 6/5/4/3/2列要求仍按备忘录执行，但每项改为头像+名称轻量行；
- 移除四边框Tile矩阵；
- 使用参考站紧凑数字分页。

Coser详情：

- Banner继续保留，这是CGM已确认扩展；桌面严格4:1，移动继续4:1；
- Banner下使用桌面128px、移动80px方形头像叠层，以及20/18px名称、Country和紧凑社交图标；
- 不使用超大Georgia标题；
- Profile与Biography使用轻边框可折叠正文，避免重新形成大Hero；
- Scope与Timeline保持现有功能，但采用36px高、8px圆角、1px浅灰边界的参考站控件几何；
- Gallery网格复用统一卡片。
- Gallery有多页时使用居中的36px数字分页：连续显示最多5个页码、当前页描边、左右Chevron；URL继续保留`scope/page`。

Work/Character/Tag：

- 使用相同的轻量文字行系统；
- 没有图片时使用首字母圆形占位或类型图标；
- Work详情Character列表保持高密度；
- Character/Tag详情Gallery继续使用统一卡片；
- 不为了“像参考站”而给Work/Character/Tag新增图片字段。

### 8.6 Search、Timeline、Random、Favorites和History

- 所有页面复用相同Breadcrumb、PageHeader、Toolbar、ScopeSwitcher和Pagination。
- Search输入框使用参考站居中、单行、轻边框形态。
- Timeline继续使用GalleryCard，不增加热门时间轴。
- Random媒体卡可使用GalleryEpic卡片外观，但仍显示Character行+Coser行且不显示Gallery标题。
- Favorites的Gallery/Media切换使用紧凑Tabs。
- History不显示浏览次数，只显示最后查看时间。
- Scope用中性SegmentedControl或Select，不使用暖色边框按钮。

### 8.7 ManageShell

Manage没有GalleryEpic公开对应页面，建议遵守“同设计系统、不同信息密度”：

- 保留现有固定左侧栏结构；
- 改为与Browse一致的白色/浅灰侧栏和64px顶部栏；
- 顶部Logo旁显示小型`Manage` Badge，明确当前上下文；
- 页面标题使用24～30px Inter；
- 高密度表格保持，不改为大卡片瀑布；
- 表格头使用12px弱文字，行高40～48px，Hover使用`#f5f5f5`；
- 表单Panel使用白底、1px边界、8～10px圆角；
- 编辑页Tab改为中性下划线或SegmentedControl；
- 状态使用浅色Badge，不使用整行高饱和背景；
- 主要操作使用黑底白字按钮，次要操作使用白底边框，危险操作使用红色；
- 技术ID、revision和路径用等宽字体，但不让整页变成开发工具风格。

#### Gallery编辑扩展功能的风格映射

| CGM功能 | GalleryEpic风格扩展方式 |
| --- | --- |
| 状态与ACTIVE | 顶部紧凑状态Badge+主按钮 |
| 激活阻断 | 标题下方浅色Alert，列出可理解原因和目标页签 |
| Cast匹配建议 | 浅灰Suggestion Panel，头像/实体名+小型`Use`按钮 |
| Media成员 | 参考详情媒体网格+右侧/底部编辑工具 |
| Source扫描 | 带状态Badge的Definition List+主扫描按钮 |
| Manifest | 状态Badge、Diff卡片、冲突选择Dialog |
| Recognition Rule | 紧凑表格或List Row+编辑Drawer |
| Task | DataTable+状态Badge+行尾操作 |
| Backup/Restore | DataTable；恢复继续使用高风险Dialog |
| Audit | 紧凑技术表格，不使用Gallery卡片 |

#### 已知交互问题的视觉修复机会

当前激活失败只显示`DISPLAYABLE_ITEM_REQUIRED`技术码。后续Manage UI改造时应：

- Activate前展示门禁摘要；
- `scan_revision=0`或`NEVER_SCANNED`时把`Scan source now`作为明确下一步；
- 对已知阻断可以禁用按钮并提供Tooltip/Alert；
- 后端仍是最终校验者，前端不能自行绕过状态机。

这属于用户引导改进，不改变ACTIVE业务规则。

### 8.8 Setup、Login、Legal和Maintenance

- Setup改为白底居中卡片，保留五步流程。
- 步骤进度使用浅灰Track和黑色当前步骤。
- Login保持单密码，只调整卡片、输入框、Logo和按钮。
- Legal使用正常文档宽度和无衬线标题；继续保留AGPL与Stash归属。
- Maintenance使用与Manage相同的Alert、Panel和高风险按钮。
- 这些页面不能加载GalleryEpic品牌或远程资源。

## 9. 开发计划

### UI-00 参考冻结与测量基线

#### 工作

- 确认本文件记录的2026-07-30 GalleryEpic浅色版本为目标参考。
- 建立1440×1000、1024×768、768×1024、390×844和360×800的CGM测试视口。
- 把关键几何写成设计规范：
  - 侧栏208/256px；
  - 顶栏64px；
  - 桌面内容边距32px；
  - 卡片图片3:4；
  - 8～10px圆角；
  - 16px基础Gap。
- 建立组件和页面视觉清单。
- 不提交GalleryEpic页面截图，只提交测量值和CGM自己的基准图。

#### 退出门禁

- 产品确认参考版本和浅色默认主题。
- 明确原创CGM Logo使用临时字标还是同步设计原创图标。
- 所有“参考站功能但CGM禁止”的项目进入禁止复制清单。

### UI-01 Token、字体、图标和基础组件

#### 工作

- 重构CSS变量为语义Token。
- 本地打包Inter或确认系统字体Fallback方案。
- 引入Lucide式本地图标并更新许可证/SBOM。
- 创建Button、IconButton、Input、Select、Badge、Alert、Breadcrumb、Pagination、Tabs、Dialog和Drawer。
- 为基础组件增加Vitest和Story/fixture测试页；项目当前没有Storybook，不建议仅为本轮引入。

#### 退出门禁

- 没有远程字体、CDN或运行时图标请求。
- 键盘焦点、disabled、hover、active、danger状态全部可见。
- 浅色Token通过WCAG AA对比度检查。

### UI-02 BrowseShell与响应式导航

#### 工作

- 横向顶部导航改为固定桌面侧栏+64px顶部栏。
- 建立移动Drawer。
- 重排现有路由为语义分组，不改变URL。
- 增加原创Logo占位组件。
- 保持Session、Browse/Manage切换、语言和Legal入口。

#### 测试

- 360、390、768、1024、1440、1920截图回归。
- 键盘Tab、Escape、Drawer焦点恢复。
- 200%缩放。

#### 退出门禁

- 桌面页面不再显示旧横向导航。
- 移动页面无横向导航溢出。
- 所有现有Browse路由仍可达。

### UI-03 Gallery卡片与所有Gallery索引

#### 工作

- 重构GalleryCard视觉结构。
- 增加Coser头像组所需的轻量DTO字段；若现有DTO不足，先评估是否可复用已返回资源，不能为纯视觉一次性加载实体详情。
- 将P/S/G/V叠加到封面右下。
- 收藏、评分、R-18和Scrubber按新结构重新排布。
- 首页、List、Magic、Timeline、Gallery收藏、Character/Tag/Coser相关Gallery统一复用。
- 保留6/5/4/3/2断点。

#### 测试

- 单/多Coser、单/多Character、Album、长标题、无封面、R-18、收藏、评分。
- P/S/G/V单类型、混合类型、部分为0和全部为0；必须验证零数量类型不占位、不输出`0P`/`0S`/`0G`/`0V`。
- Scrubber开启/关闭、Hover离开恢复、触摸不启用。
- 卡片布局无CLS。

#### 退出门禁

- 所有Gallery网格使用同一个Card实现。
- 卡片在目标视口与冻结几何基线对齐。
- 不增加首屏无界媒体请求。

### UI-04 Gallery详情、媒体网格和Lightbox

#### 工作

- Hero改为紧凑标题区域。
- 增加Breadcrumb和右侧Related/Recent辅助栏。
- 桌面媒体网格调整为4列主基线。
- 保留每组24项手工展开、筛选和完整Lightbox索引。
- Lightbox图标化并核对首尾不循环、URL深链和返回键行为。
- ExternalLink、日期、可用大小和关系信息按新视觉重排。

#### 测试

- 1、24、25、1000成员。
- Photo/Selfie/GIF/Video各组。
- 无Related时Recent回退。
- 移动端右栏下移。
- Lightbox键盘、Escape、reduced-motion。

#### 退出门禁

- 首屏媒体到达位置和参考站接近。
- 详情不引入热门、下载或评论。
- 1000成员性能门禁不回退。

### UI-05 实体、搜索和个人页面

#### 工作

- Coser索引改为头像文字行。
- Coser详情、Work/Character/Tag索引和详情统一视觉。
- Search、Timeline、Random、Favorites、History统一Header/Toolbar/Pagination。
- 社交账号使用本地图标；未知平台使用通用图标。

#### 退出门禁

- Coser 6/5/4/3/2列和实体60项分页保持。
- Work/Character/Tag不增加未确认图片字段。
- LIST/MAGIC/ALL范围和URL Query不变。

### UI-06 ManageShell与管理页面

#### 建议顺序

1. ManageShell和导航；
2. Gallery索引与问题摘要；
3. Gallery五页签；
4. Coser和核心实体；
5. Libraries/Recognition Rules；
6. Tasks；
7. Settings；
8. Operations/Maintenance。

#### 工作

- 使用同一浅色Token和基础组件。
- 保持高密度表格，不把管理功能改造成Browse卡片。
- 激活阻断、扫描状态和未完成表单提供可行动提示。
- 高风险操作继续保留明确确认词和重新认证。
- 保持路径、技术ID和审计信息只在Manage显示。

#### 退出门禁

- 全部Manage Mutation和revision逻辑不变。
- 768px可使用，1280px以上完成主验收。
- 没有因视觉改造隐藏关键诊断或危险确认。

### UI-07 Setup、Login、Legal、Maintenance和错误状态

#### 工作

- 统一系统页Logo、卡片、输入和按钮。
- 统一Loading Skeleton、EmptyState、ErrorState、403/404和Maintenance。
- 保留离线与认证边界。

#### 退出门禁

- Setup→Login→Browse完整流程通过。
- About/Legal许可证信息完整。
- 无JS外部依赖或远程资产。

### UI-08 视觉回归、无障碍和性能收敛

#### 工作

- 更新CGM自有Playwright截图基线。
- 对关键页面执行像素差异回归。
- 对Shell、Gallery卡片和详情做人工Overlay验收。
- 执行axe、键盘、焦点、屏幕阅读器名称、200%缩放和reduced-motion。
- 检查主JS、图标Tree-shaking、字体体积和首屏媒体请求。

#### 建议视觉验收公差

- 已冻结组件几何：目标视口位置/尺寸误差不超过2px。
- 颜色：必须来自确认Token，不允许页面自行近似。
- 字号和行高：误差不超过1px。
- 圆角：统一使用Token。
- Gallery网格：不允许列宽或Gap随页面各自漂移。
- 动画：只验证持续时间、位移范围和reduced-motion，不要求逐帧像素一致。

#### 最终门禁

- 所有Browse和Manage关键路径功能回归通过。
- 目标视口截图通过。
- 不出现远程字体、远程图标、GalleryEpic品牌或第三方媒体资产。
- 离线E2E通过。
- 百万Item后端性能和1000成员前端详情基线不因视觉改造明显回退。

## 10. 测试策略

### 10.1 视觉回归页面

至少覆盖：

- Setup
- Login
- Home
- List
- Magic
- Gallery详情
- Coser索引
- Coser详情
- Work/Character/Tag索引和详情
- Search
- Timeline
- Random
- Favorites
- History
- Media详情
- Manage Gallery索引
- Manage Gallery五页签
- Libraries
- Tasks
- Settings
- Operations
- Maintenance

### 10.2 视觉夹具

CGM自有夹具应包含：

- 无封面和有封面；
- Album与Cosplay；
- 单人、多人、单角色、多角色、多Work；
- 中英文和超长名称；
- P/S/G/V混合；
- R-18；
- Favorite和不同评分；
- READY/PENDING/ERROR/MISSING；
- 正常、警告、阻断和危险操作；
- 空列表和1000成员Gallery。

### 10.3 功能不变回归

视觉改造期间每一阶段都必须验证：

- GraphQL变量和Mutation不变；
- revision冲突仍能显示；
- Gallery状态机不被前端绕过；
- Scrubber不记录历史；
- 触摸不依赖Hover；
- Lightbox、媒体资源认证和路径隐私不变；
- Manifest、扫描、备份、恢复和危险确认不变；
- 应用仍不能删除或修改用户媒体来源。

## 11. 风险与控制

| 风险 | 表现 | 控制 |
| --- | --- | --- |
| 参考站继续改版 | 开发期间目标不断变化 | 冻结2026-07-30基线 |
| “像素级”变成复制品牌 | 使用相同Logo、图片或代码 | 只复刻几何/Token，使用CGM原创资产 |
| 全局CSS一次性替换 | Browse修复但Manage/Setup破坏 | Token和组件分阶段迁移 |
| 卡片DTO膨胀 | 为头像展示增加大量请求 | 使用轻量字段，禁止N+1实体查询 |
| 浅色主题对比度不足 | 弱灰文字不可读 | Token级axe/对比度门禁 |
| Manage过度追求画廊感 | 表格和诊断密度下降 | 同Token但保持管理信息架构 |
| 双主题成本失控 | 每个组件出现两套样式 | 第一阶段只交付浅色基线 |
| 图标/字体引入遗漏许可证 | SBOM和发行校验失败 | UI-01同步更新法律元数据 |
| 视觉改造夹带业务变化 | 状态机、路由或DTO意外变化 | 每阶段限制变更范围并运行功能回归 |

## 12. 建议确认的产品选择

以下选择不阻止本文作为开发指导，但在UI-00完成前建议确认：

1. **参考版本**：建议确认以2026-07-30观察到的GalleryEpic浅色、左侧栏版本为固定基线，而不是其历史旧版。

2. **主题范围**：建议1.5先只实现浅色主题，不同时维护旧深色主题。暗色模式如有需要，作为后续独立设计任务。

3. **品牌占位**：最终品牌和Logo在备忘录中仍为延期项。建议先使用原创CGM简洁字标和通用占位图形，尺寸与参考站品牌区一致，但不模仿其幽灵Logo。

4. **Manage一致性**：建议Manage也使用浅色侧栏和同一组件系统，通过`Manage` Badge和更高信息密度区分，而不是保留当前深色后台。

5. **详情右栏**：建议用强关系`Related`，无结果时用`Recently added`，明确不采用“Hottest”。

6. **卡片第一行语义**：建议Cosplay优先Character、第二行Work、第三行Coser；Album第一行Gallery标题。多角色/多Work时需要在UI-03用真实业务样本确认截断和回退规则。

## 13. 推荐的下一项开发任务

下一项不应直接批量修改全部页面，建议从`UI-00 + UI-01`开始：

1. 确认本文件第12节的六项选择；
2. 建立浅色Token、字体和图标基础；
3. 新建共用组件；
4. 用一个独立视觉夹具页面验证Token和控件；
5. 通过检查后再进入BrowseShell和GalleryCard。

最小纵向改造切片建议为：

```text
Token / Icon
  → BrowseShell桌面侧栏与移动Drawer
  → Home一个GalleryCard
  → 1440与390截图回归
  → 键盘与axe
```

这条切片通过后，再把同一组件扩展到全部Browse页面和Manage页面，可最大限度降低全局视觉重构导致功能回归的风险。

## 14. 本地实施进度

### 2026-07-30：UI-00、UI-01导航前置与UI-02完成

- 已建立本文第7节定义的浅色语义Token、字体Fallback、原创本地图标和第一批基础组件。
- 已完成固定桌面侧栏、64px顶栏和移动Drawer，现有Browse路由与业务查询不变。
- 移动Drawer已验证Escape、遮罩、焦点循环和关闭后焦点恢复。
- Gallery网格继续保持6/5/4/3/2列；没有复制GalleryEpic品牌、媒体、代码或远程资源。
- 卡片媒体计数已先行采用零值过滤规则，当前正文位置显示`2P 1G`；移至封面右下属于UI-03。
- Chromium离线主流程、axe A/AA、键盘焦点、1440桌面和390移动截图回归已通过。

### 2026-07-30：UI-03～UI-08完成

- UI-03：统一GalleryCard，媒体计数移至封面右下并过滤零值，正文采用Character/Work/Coser层级；CardMedia、Avatar和Skeleton已有真实消费者与测试。
- UI-04：Gallery详情改为Breadcrumb、小封面紧凑标题区、4/3/2媒体网格和Related右栏；Lightbox改用本地图标、键盘控制并严格禁止首尾循环。
- UI-05：实体索引、Coser详情、Search、Timeline、Random、Favorites、History和媒体详情统一浅色高密度视觉，既有范围、分页和URL语义保持不变。
- UI-06：ManageShell及全部管理模块切换到同一浅色Token；恢复、永久删除、实体生命周期和托管资源清理共用带焦点约束的Dialog，危险确认与revision逻辑不变。
- UI-07：Setup、Login、Legal、Maintenance、Loading和错误状态完成浅色系统页收敛，许可证、认证和恢复边界保持不变。
- UI-08：Chromium离线全流程、axe A/AA、键盘、Drawer焦点和1440/390截图通过；视觉差异容差由1%收紧为0.2%，并增加新版卡片DOM断言。
- 最终前端结果：TypeScript通过，16个Vitest文件/37项测试通过，669模块生产构建通过，主入口约472.07KiB且无大Chunk警告。

本计划的本地代码阶段已经完成。Firefox/WebKit、真实移动设备、读屏、200%缩放及低性能设备仍属于目标环境验收，不能根据Chromium自动化结果推定通过。

### 2026-07-31：指定Coser详情页高保真复核

- 对用户指定的`/zh/coser/298/1`重新抓取1440×1000桌面、390×844移动和页面底部分页画面，冻结了4:1 Banner、128/80px方形头像叠层、名称/社交行、36px控件及数字分页的精确几何。
- Coser详情新增Home/Cosers/当前Coser Breadcrumb；无Banner时仍保留4:1中性占位，避免首屏结构跳变。
- 社交账号由大块文字卡收敛为20～24px紧凑平台标记；链接、ACTIVE/INACTIVE状态和自定义platform key回退保持不变。
- LIST/MAGIC/ALL与Shoot timeline没有改成参考站并不存在于CGM的作品/角色筛选，只采用相同的轻边框输入控件外观。
- Coser详情Gallery隐藏重复的Coser行，继续使用统一3:4卡片；全局6/5/4/3/2列产品约束保持不变。
- 新增可操作的居中数字分页，最多连续显示5页，当前页使用圆角描边；点击后更新原有`scope/page`查询参数并重新读取本机GraphQL。
- 新增Coser详情桌面/移动视觉基线和页面组件测试；离线完整生命周期、axe A/AA、路由及生产构建作为本阶段门禁。

### 2026-08-01：Cosplay/Album双分区与实体层级补齐

经登录状态参考页复核并由用户确认，Browse信息架构固定为：

- Cosplay：Lists、Cosers、Parodies、Magic；
- Album：Lists、Models；
- Cosplay Lists只显示非成人COSPLAY，Magic只显示成人COSPLAY；
- Album Lists和Models显示全部分级ALBUM，不再另建Album Magic；
- Model不是新的核心实体或关系角色，只是现有Coser在ALBUM集合中的展示标题和路由上下文。

本阶段已经实现：

- 新增`/albums`、`/models`、`/model/:slug`，原`/list`、`/works`、`/work/:slug`和隐藏的Character索引路由继续兼容；
- 侧栏使用原创相机、人物、调色板和锁形本地SVG，几何与参考站对齐，不复制Logo、代码、远程字体或专有资产；
- Coser/Model采用同一四列头像文字索引组件，但通过派生CollectionType分别筛选，并支持名称/Alias搜索；
- Work/Parody索引为四列纯文字，Work详情为Character文字索引，Character详情才进入Gallery网格；
- Coser详情继续保留已确认的资料Hero、Scope和Timeline；Model详情按参考站使用Breadcrumb后直接显示Album网格；
- Album卡片中的人物入口改为`/model/:slug`，COSPLAY卡片继续进入`/coser/:slug`。

查询层只增加向后兼容的可选`collectionType`和实体索引`query`参数。类型仍由Cast实时推导，未新增数据库字段、Model表或Coser/Model角色字段；24/30/60分页以及Gallery网格`6/5/4/3/2`列约束保持不变。

离线E2E使用真实ALBUM夹具验证了该Gallery不会出现在Cosplay Lists或Magic，人物不会出现在Cosers但会出现在Models，并通过Model卡片进入Model详情。Home、Model详情桌面/移动、Coser详情桌面/移动视觉基线及axe A/AA均通过0.2%门禁。

后续实机复核发现，人物索引头像沿用了可容纳Alias的双行跨度，而无Alias时主名称停留在第一行，造成名称中心高于头像中心。2026-08-01按用户确认将Coser/Model列表收敛为只显示主名称的36px单行结构；Alias仍可参与搜索，但不在人物列表展示。离线E2E已增加头像与名称中心点误差小于1px的实际几何断言。

同日再次读取参考站公开页面源码，确认人物名称节点为`text-sm leading-none font-medium`，即14px字号、14px行高和500字重，头像后间距为8px。CGM原专属覆盖只把通用18px名称降至16px，仍保留550字重和1.35行高，因而视觉偏大、偏厚；现已按参考值完整覆盖并用浏览器计算样式锁定。
