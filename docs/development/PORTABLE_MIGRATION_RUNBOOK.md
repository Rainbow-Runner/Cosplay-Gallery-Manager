# 可移植元数据迁移操作手册

> 适用版本：CGM 1.5，portable package format v1，产品数据库 schema v10  
> 平台：当前正式支持 Linux/容器；Windows 首版延期

## 1. 选择迁移方式

- 同机完整恢复、需要保留本机路径和全部运行状态：使用完整备份/恢复。
- 新机器路径不同、希望核心实体与媒体解耦：使用本手册的可移植元数据包与 Gallery Manifest 重建。
- 目标业务库为空：使用 Empty Import。
- 目标已有 Coser/Work/Character/Tag/Gallery：使用 Merge，禁止用 Empty Import 绕过冲突审核。

可移植包不包含密码、Session、任务、缓存、日志、旧机器媒体绝对路径或原始 Gallery 媒体。迁移不会写入源媒体；Gallery 只从目标机已有媒体及其 `.cosplay.json`/相邻存档 Manifest 重建，并保持 DRAFT。

## 2. 源机导出

```bash
cgm -config /absolute/path/cgm.json -preflight-portable
cgm -config /absolute/path/cgm.json -export-portable /absolute/path/catalog.cgm-portable.zip
cgm -inspect-portable /absolute/path/catalog.cgm-portable.zip
```

若预检报告 Gallery Manifest 不完整，应先在源机核对并 Push。只有明确接受无法完整重建的 Gallery 时才使用 `-allow-incomplete-portable`。

## 3. 空业务库导入

```bash
cgm -config /absolute/path/cgm.json -import-portable /absolute/path/catalog.cgm-portable.zip
```

输入 `IMPORT` 后，CGM 先创建完整安全备份，再导入核心目录和 Coser 当前资源。中断且维护状态为 `PORTABLE_IMPORTING` 时运行：

```bash
cgm -config /absolute/path/cgm.json -recover-portable-import
```

## 4. 非空业务库 Merge

```bash
cgm -config /absolute/path/cgm.json -preflight-portable-merge /absolute/path/catalog.cgm-portable.zip
cgm -config /absolute/path/cgm.json -prepare-portable-merge /absolute/path/catalog.cgm-portable.zip
cgm -config /absolute/path/cgm.json -decide-portable-merge <merge-uuid>
cgm -config /absolute/path/cgm.json -apply-portable-merge <merge-uuid>
```

决策语义：

- `KEEP_LOCAL`：同 UUID 内容或关系冲突保留本地值。
- `USE_INCOMING`：同 UUID 内容或关系冲突采用包内值；Coser 当前资源会先保存本地回滚副本，再替换原图并重建派生图。
- `KEEP_SEPARATE`：同名或同 URL 候选仍作为不同身份新增。
- `MAP_TO_LOCAL`：明确认定为同一身份；传入 UUID 永久登记为指向本地 UUID 的 Alias，所有包内关系按目标 UUID 解析。

硬冲突不能通过决策绕过。包或目标目录在决定/应用前发生变化会使会话 `STALE`。应用前可输入以下命令关闭会话，保留包和冲突审计：

```bash
cgm -config /absolute/path/cgm.json -abort-portable-merge <merge-uuid>
```

应用会先创建完整安全备份并进入 `PORTABLE_MERGING`。若进程在 Coser 资源发布边界中断，使用：

```bash
cgm -config /absolute/path/cgm.json -recover-portable-merge <merge-uuid>
```

恢复器只处理带相同 Merge UUID 所有权标记的目录：数据库未提交则恢复旧资源/删除本轮新目录；数据库已提交则复验传入资源摘要并完成标记清理。无标记或标记不符的目录不会删除。

## 5. 映射新媒体根并重建 Gallery

Empty Import 使用 `<import-uuid>`；Merge 使用同一个 `<merge-uuid>`：

```bash
cgm -config /absolute/path/cgm.json -map-portable-libraries <workflow-uuid>
cgm -config /absolute/path/cgm.json -preflight-portable-rebuild <workflow-uuid>
cgm -config /absolute/path/cgm.json -rebuild-portable-galleries <workflow-uuid>
```

每个逻辑媒体库必须映射到本机已启用媒体库，或明确跳过。预检要求目标来源、Manifest 摘要/revision、相对成员路径和待接管 UUID 全部一致。正式重建要求 `REBUILD`，只创建 DRAFT Gallery；无法安全接管时保留稳定问题码和声明，不能生成替代 UUID 假装成功。

## 6. 验收

1. 确认维护状态已回到 `NORMAL`，`/healthz`与`/readyz`正常。
2. 检查数据库 `integrity_check=ok`，并核对核心实体、Gallery 和 Item 数量。
3. 抽样核对 Coser 头像/Banner/帐号、Character→Work、Tag层级以及 Gallery Cast/Credit/封面/排序。
4. 确认重建 Gallery 为 DRAFT，并逐项通过当前内容分级和可展示媒体激活门禁。
5. 保留导出包、应用前完整安全备份及其校验和，直到业务验收结束。

当前基础闭环提供 CLI；双语 Web 冲突工作台与可选 owner continuity 分区属于后续产品化增强，不影响上述核心目录及 Gallery UUID 迁移流程。
