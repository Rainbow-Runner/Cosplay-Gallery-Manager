# 可管理媒体自动排除规则实施计划

日期：2026-08-28  
目标版本：1.5  
数据库目标：schema v5  
状态：源码与自动化测试已完成，待清洁提交、正式备份及部署

## 1. 目标

为已绑定的 DIRECTORY GallerySource 及 ZIP/CBZ Archive 成员增加数据库驱动的媒体自动排除规则。所有支持媒体仍由安全扫描器发现并形成GalleryItem；规则只决定新Item的初始`excluded`状态，不删除、不移动、不改写来源文件，也不把“排除”变成文件系统级忽略。

后台必须支持全局和单媒体库规则、文件夹与文件匹配、Exact/Glob/Go RE2校验、单路径测试、存量预览及人工审核。已有Item的人工Exclude/Restore、Manifest结果和唯一指纹重绑定结果不得被普通重扫覆盖。

## 2. 冻结边界

- 自定义规则对`DIRECTORY`新Item和`ARCHIVE`新成员统一生效；ZIP/CBZ发现与成员安全校验保持不变。
- 当前“自动排除本次新发现的Gallery根目录媒体”扫描开关继续保留，并高于自定义规则；关闭它不关闭自定义规则。
- 自定义规则只自动决定扫描中新建Item；按路径或唯一完整指纹识别出的既有Item保持持久状态。
- 规则编辑、禁用或删除不自动恢复过去已排除的Item。
- 存量评估只为当前未排除Item生成`EXCLUDE`待审核建议；`INCLUDE`只作为新Item规则链中的例外，不自动恢复存量Item。
- 排除Item保留UUID、Position、Caption、分类、收藏与评分，不进入Browse、计数、封面、随机、1000上限或媒体处理任务。
- 规则及决策只存产品SQLite，不写入Gallery `.cosplay.json`；最终Item排除状态仍可沿用Manifest `excluded_items`同步。

## 3. 匹配模型

规则字段：

- scope：全局或指定媒体库；
- name、enabled、order、revision；
- subject：`PARENT_FOLDER`、`PARENT_PATH`、`FILE_NAME`、`FILE_STEM`、`RELATIVE_PATH`；
- operator：`EXACT`、`GLOB`、`RE2`；
- pattern：Exact/Glob每行一个OR模式，RE2单表达式；
- case_sensitive；
- media_kind：`ALL | STATIC_IMAGE | ANIMATED_IMAGE | VIDEO`；
- decision：`EXCLUDE | INCLUDE`。

`PARENT_FOLDER`匹配任意父目录段。`PARENT_PATH`依次提供从完整直接父路径到各级祖先路径，例如`bonus/screenshots/raw/a.jpg`提供`bonus/screenshots/raw`、`bonus/screenshots`、`bonus`，所以Exact可安全表达整个子树。

路径在匹配前统一为NFC、正斜杠和Gallery相对路径。匹配器无文件系统访问、无Shell展开；RE2由后端Go引擎保存前编译，Glob由后端语法校验。单规则最多100个Exact/Glob模式、4,000字节，规则总数最多500。

有效规则按`order ASC`；同order时媒体库规则先于全局规则，再按ID。首个同时命中路径和媒体类型的规则获胜；无命中默认纳入。

## 4. schema v5

新增`media_exclusion_rules`：保存规则定义、作用域、revision、默认标识和时间戳，使用数据库CHECK、外键和有效规则索引。

新增`media_exclusion_decisions`：保存Item、规则及revision、规则名称快照、决策、命中字段和值和`PENDING | APPLIED | REJECTED | SUPERSEDED | REVERSED`状态。它同时承担存量审核队列和自动排除来源记录；删除规则使用`ON DELETE SET NULL`保留历史说明。

现有`gallery_items.excluded`继续是当前状态唯一事实来源。人工Restore把该Item当前APPLIED规则决策标为REVERSED；人工Exclude不需要伪造规则来源。迁移不扫描来源、不改变任何既有Gallery/Item、不创建规则默认值。

数据库从v4升级前必须创建并验证来源版本准确的SQLite在线快照；事务内建表并把产品身份更新为v5，失败保留v4数据库。正式部署另需按现有维护流程创建并校验额外完整回滚包。

## 5. 扫描集成

扫描仍完整暂存Observation。提交事务在1000项有效成员检查和新Item插入前加载来源所属媒体库的有效规则并编译一次：

1. 同路径既有Item保持原Exclude状态；
2. 唯一指纹重绑定既有Item保持原Exclude状态；
3. 新根级Item且本次根目录开关开启时直接Exclude；
4. 其他新DIRECTORY Item执行规则链；首个`EXCLUDE`命中则排除，首个`INCLUDE`命中则纳入；
5. ARCHIVE成员使用经过安全校验的内部相对路径执行自定义规则；
6. 排除结果在有效成员上限判断、Item插入和派生任务排队中使用同一决策，避免口径分裂。

自动EXCLUDE的新Item同时写入APPLIED决策快照。扫描结束仍执行PHOTO/SELFIE分类建议，不让两套规则相互调用。

## 6. API与后台

GraphQL Manage契约提供规则查询、校验、创建、更新、删除、单路径测试、单媒体库预览、显式存量评估、待审核查询和接受/拒绝。全部入口沿用所有者认证、同源/CSRF、revision及不含路径的管理审计；Browse不新增字段。

Manage → Libraries & import在现有独立“媒体分类规则”旁新增独立“自动排除规则”。页面按全局规则和当前媒体库覆盖规则分组；媒体库选择只控制覆盖规则和预览目标，不暗示每个媒体库必须重复配置全局规则。

界面提供中英文：

- 规则CRUD、启停和删除二次确认；
- subject/operator/media kind/decision/优先级；
- RE2显式后端校验，未通过时禁止保存；
- 单条相对路径测试，并要求选择用于测试的媒体类型；
- 最多200条存量预览样本，显示最终胜出规则；
- 显式评估当前媒体库，逐项或批量接受/拒绝EXCLUDE建议；
- 明确提示保存/删除规则不会改变既有Item，普通重扫不会覆盖人工决定。

## 7. 测试与交付门禁

- 通用匹配器：Unicode/大小写、五类subject、三类operator、无效Glob/RE2、父路径祖先语义。
- 数据库：新库v5、v4→v5在线快照迁移、约束、CRUD/revision/优先级、预览、审核状态及删除保留历史。
- 扫描：根目录开关优先、全局/媒体库规则、INCLUDE例外、三种媒体类型、既有路径与指纹状态保持、1000上限、处理任务、DIRECTORY与ARCHIVE成员路径均生效。
- API：认证Manage契约、校验错误码、DTO无物理路径、接受建议revision冲突和无部分写入。
- React：双语规则管理、范围分组、RE2保存门禁、测试/预览、审核及删除确认。
- 回归：既有媒体分类规则、Gallery手动Exclude/Restore、Manifest、Archive、安全扫描、产品API/Server、正式嵌入标签和生产构建。

本阶段完成源码与测试后先更新实施状态，不自动迁移正式数据库或部署。正式部署必须由所有者明确授权，并完成清洁提交、额外完整备份、自动v4快照、迁移后完整性、Health/Ready/About与journal核验。

## 8. 2026-08-28 实施结果

- 已抽取无数据库、无文件系统访问的`internal/mediarules`共享匹配器；原媒体分类规则改为复用它且保持原有四类subject边界，自动排除使用包含`PARENT_PATH`的五类subject。
- 产品数据库目标升级为schema v5；新库直接建表，v1～v4均沿用来源版本校验、SQLite Online Backup和单事务迁移。v4迁移测试证明原Item排除状态不变、新规则与决策表为空、自动快照仍是可独立打开的有效v4。
- DIRECTORY与ARCHIVE扫描在硬上限判断前加载有效规则一次；根目录本次开关、既有路径、唯一指纹重绑定、单库优先、INCLUDE例外、新Item APPLIED记录、1000有效成员和Archive成员路径匹配均由SQLite集成测试锁定。
- 存量评估不会直接修改Item；接受EXCLUDE后才递增Gallery metadata revision、排除Item并取消其未完成处理任务，人工Restore会把APPLIED历史改为REVERSED并恢复当前内容revision的任务。规则删除只把历史`rule_id`置空，保留规则名称、revision和命中快照。
- Manage GraphQL与Libraries & import双语页面已完成规则CRUD/二次删除确认、范围分组、后端RE2校验、路径与媒体类型测试、最多200项预览、显式评估及逐项/批量审核。Browse schema和来源文件没有变化，管理审计不记录pattern或媒体路径。
- 自动验证通过：目标Go包、带`cgm_web_embed cgm_galleryepic`标签的Product API/Server/cmd组合、TypeScript检查、29个Vitest文件80项测试及680模块Vite生产构建。仓库级`go test ./...`中的CGM产品包均通过，但总命令仍受既有旧Stash `ui/v2.5/build`缺失及沙箱禁止`httptest`监听IPv6端口影响，不能记为全仓通过。
- Archive扩展在现有schema v5上复用`relative_path`，不新增作用范围字段或数据库迁移；本轮源码尚未重新提交、构建正式候选或重新部署。进入正式部署前必须先形成清洁提交，再按本文件门禁执行额外完整回滚包和schema v5完整性核验。
