# Archive 内部规则与 Gallery 标题扩展规划

日期：2026-08-29  
目标版本：1.5 后续增量阶段  
状态：阶段 A（成员排除、标题保底、外部路径/文件名候选）已部署；TAR/TAR.GZ/TGZ/7Z读取支持已在本地完成，待schema v7一同部署；阶段 B/C 待后续

## 1. 当前行为与问题

ZIP/CBZ 在媒体库发现时作为独立 `ARCHIVE` GallerySource，来源路径是存档文件本身。发现阶段只读取中央目录并统计受支持媒体成员；正式扫描阶段再读取成员内容、计算指纹和建立 GalleryItem。

当前规则边界如下：

- `.cosplay-root` 只适用于 `DIRECTORY`，不适用于 Archive。
- 自动媒体排除规则只应用于 `DIRECTORY` 新 Item，不应用于 ZIP/CBZ 内部成员。
- PHOTO/SELFIE 媒体分类规则在扫描后针对可用静态图片 Item 生成建议，因此已经可以匹配 Archive 成员的 `relative_path`。
- Archive 有效相邻 Manifest（例如 `set.cbz.cosplay.json`）可以提供 Gallery 标题和实体关系。
- 没有 Manifest 时，Archive 导入不会稳定地从存档文件名生成标题，可能产生空标题 DRAFT。
- `PATH_TEMPLATE` 可以匹配存档在媒体库中的外部路径并生成待审核建议，但当前不会匹配 Archive 内部成员路径。

主要问题是：存档内部常包含 `thumbs`、`preview`、`sample`、章节目录和其他辅助文件；同时无 Manifest 的存档缺少可靠标题，用户需要大量手工处理。

## 2. 冻结目标

本扩展分为两个相互独立的能力：

1. Archive 成员级排除：按存档内部规范化相对路径对新成员应用可管理 `EXCLUDE/INCLUDE` 规则。
2. Archive Gallery 标题与实体候选：为无 Manifest 存档提供确定性标题保底，并基于外部路径、文件名和可控的内部目录信息生成待审核候选。

两项能力不得把“某个成员的目录名”直接当作 Gallery 的确定元数据，也不得在没有用户确认时自动绑定 Coser、Work 或 Character。

## 3. Archive 成员路径模型

规则匹配使用通过 Archive 安全校验后的成员路径，统一为 NFC、正斜杠和相对于 Archive 内部根的路径。例如：

```text
cosplay/preview/selfie/001.jpg
```

提供以下匹配值：

- `PARENT_FOLDER`：`selfie`、`preview`、`cosplay` 等任意父目录段；
- `PARENT_PATH`：`cosplay/preview/selfie`、`cosplay/preview`、`cosplay` 等祖先路径；
- `FILE_NAME`：`001.jpg`；
- `FILE_STEM`：`001`；
- `RELATIVE_PATH`：完整成员路径。

不得使用压缩包外部路径代替成员路径；Gallery 级识别和成员级排除必须分别保留外部路径与内部路径。

## 4. 自动排除规则扩展

### 4.1 作用范围

在现有规则增加来源范围字段，建议取值：

- `DIRECTORY`；
- `ARCHIVE`；
- `BOTH`。

为保持兼容，现有规则迁移后默认 `DIRECTORY`，不改变已经部署的排除行为。新建规则默认仍为 `DIRECTORY`，用户必须显式选择 Archive 或 Both。

### 4.2 执行边界

Archive 扫描在安全校验完成后，对新发现成员执行规则链：

1. 已存在且按路径或唯一指纹重绑定的 Item 保留当前排除状态；
2. 新成员按 `order`、同序媒体库优先、规则 ID 排序；
3. 首个命中的 `EXCLUDE` 或 `INCLUDE` 决定初始状态；
4. 自动排除写入规则决策快照并跳过处理任务、Browse、计数和封面候选；
5. 无命中默认纳入；
6. Archive 成员路径发生变化时，不得仅因规则重算覆盖人工或 Manifest 决定。

现有根目录媒体排除开关只适用于 DIRECTORY，不适用于 Archive 成员。Archive 整体仍可通过 Gallery/Manifest 级操作排除，但不能替代成员级规则。

### 4.3 存量审核

存量 Archive 评估默认只生成 `PENDING EXCLUDE` 建议，不直接改变成员状态。`INCLUDE` 只作为新成员规则链的例外，不自动恢复已排除存量成员。接受、拒绝、Restore、任务取消与恢复语义沿用 DIRECTORY 规则。

### 4.4 安全要求

- 先完成现有 Archive 安全校验，再进入规则匹配；危险归档不得产生可执行排除结果；
- 规则匹配器不访问文件系统、不执行 Shell、不展开用户路径；
- 审计只记录规则 ID、范围、决策数量和 revision，不记录成员完整路径或 pattern；
- 归档成员的路径必须经过 NFC、路径穿越、大小写冲突和特殊 Entry 校验。

## 5. Gallery 标题获取规则

为避免空标题，建议在发现快照中确定标题候选，并在导入时使用同一快照值。优先级如下：

1. 有效相邻 Manifest 的标题；
2. 用户确认的 `PATH_TEMPLATE` 标题建议；
3. 去除 `.zip/.cbz` 扩展名后的存档文件名；
4. 存档所在目录名；
5. 若以上均不可用，进入阻断问题，不创建空标题 Gallery。

示例：

```text
/library/Alice/Blue Archive Vol.1.cbz
```

默认标题为 `Blue Archive Vol.1`，而不是 `Alice`。

标题候选必须执行现有 NFC、空值和 300 字符边界校验。已导入 Gallery 不自动回写标题，避免历史数据在重扫时静默改名。

## 6. Coser/Work/Character 候选建议

建议分阶段实现：

### 阶段 A：外部路径和存档文件名

- 使用媒体库相对路径、外部父目录名、存档文件名 stem；
- 通过已存在实体名称和 Alias 做大小写不敏感、NFC 规范化匹配；
- 只生成 `PENDING` identity suggestion；
- 多个候选、冲突或低置信度时不自动选择。

### 阶段 B：Archive 内部顶层目录

- 仅使用经过安全校验的内部目录；
- 只把稳定、重复出现的顶层目录作为候选信号；
- 忽略 `thumbs`、`preview`、`sample`、`cover`、章节号等可配置噪声目录；
- 不把单个媒体文件夹直接认定为 Gallery 的 Coser 或 Work。

### 阶段 C：显式 Archive 模板

增加针对 Archive 外部路径和内部路径的模板规则，允许命名捕获 `title`、`coser`、`work`、`character`、`year`、`month`。模板结果继续进入待审核队列，不绕过实体唯一性、内容分级和激活门禁。

## 7. 数据库与 API 影响

- 自动排除规则需要新增来源范围字段；若采用独立字段并保持默认值，目标为 schema v6，迁移必须保留现有规则、决策和 Item 状态。
- Archive 成员无需新增物理路径字段，现有 `gallery_items.relative_path` 已能保存内部成员路径。
- 标题候选可复用现有 discovery snapshot/candidate suggestion，不新增 Gallery 元数据表。
- Entity suggestion 可复用现有 `gallery_identity_suggestions`；若需要置信度和来源，应增加受限枚举字段，不记录未经审核的外部原始路径。
- Manage GraphQL 必须明确显示规则作用范围和预览目标是“DIRECTORY 媒体”还是“存档内部成员”。
- Browse DTO、资源 URL 和普通 Gallery 列表不增加内部成员路径。

## 8. 实施顺序

1. 补充当前行为文档和回归测试，锁定媒体分类规则已支持 Archive、自动排除规则当前不支持 Archive。
2. 实现 Archive 标题文件名保底，保持 Manifest 和已导入 Gallery 优先级不变。
3. 增加规则来源范围模型、schema 迁移、GraphQL 和双语管理 UI。
4. 将规则链接入 Archive 扫描提交事务，完成新成员、存量审核、任务队列和 1000 项上限回归。
5. 增加 Archive 内部路径测试、规则预览和 Archive 成员排除/恢复测试。
6. 实现外部路径和文件名实体候选建议。
7. 在独立阶段评估内部顶层目录和显式模板，不与成员排除首个版本合并发布。

## 9. 测试与部署门禁

- 安全：危险归档、路径穿越、符号链接、加密 Entry、嵌套归档不会进入规则匹配；
- 匹配：五类 subject、Exact/Glob/RE2、Unicode、大小写、目录祖先和文件名边界；
- 状态：新成员自动排除、既有成员状态保持、人工/Manifest 优先、恢复和任务取消/恢复；
- 标题：Manifest、模板、文件名、父目录回退及空值/长度校验；
- 兼容：现有 DIRECTORY 规则结果不变，未选择 Archive 的旧规则不会影响 Archive；
- API/UI：来源范围、单路径测试、单 Archive 预览、双语文案、删除确认和审计脱敏；
- 性能：发现阶段只读取中央目录，规则编译每次扫描一次，避免为每条成员重复读取 Archive；
- 部署：若引入 schema v6，必须先提交、创建额外完整回滚包、验证 v5 快照和迁移后完整性，再执行正式迁移和服务核验。

阶段 A 已按本规划实现并于 2026-08-29 增量部署。2026-08-30后续本地阶段把相同的安全校验、发现、正式扫描、按需物化、标题扩展名去除和相邻Manifest规则扩展到TAR、TAR.GZ/TGZ与7Z；7Z由纯Go适配层读取，不依赖宿主机7z命令。加密ZIP/7Z不会保存或尝试密码，而是进入媒体库覆盖诊断。阶段 B/C 仍只定义后续开发边界。
