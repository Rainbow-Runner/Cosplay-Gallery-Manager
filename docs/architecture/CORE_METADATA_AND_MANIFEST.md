# 核心元数据、实体生命周期与 Manifest

> 对应开发计划：P03-01～P03-09，门禁 G3
> 状态：第一版后端领域闭环已实现

## 1. 聚合与身份

- Gallery 是作品集业务聚合根；类型不落库，`GalleryCast` 为空推导为 Album，非空推导为 Cosplay。
- Coser、Work、Character、Tag 使用全局可移植 UUID；普通改名不改变 Slug。
- 显式改变 Slug 时保存旧 Slug 到轻量重定向表；UUID 仍是唯一身份。
- 核心实体删除只允许在完全无引用时执行，并在同一事务中永久 Tombstone UUID。
- 核心实体合并必须先生成预览；任何资料差异、重复关系或 Tag DAG 风险均阻断提交。无冲突合并原子迁移关系、保留源名称为 Alias，并把源 UUID 永久指向目标 UUID。

## 2. 关系规则

- GalleryCredit 直接引用 Coser，不区分 Coser/Model。
- GalleryCast 引用具体 Credit 与 Character；Character 的 Work 是唯一作品来源。
- ACTIVE Cosplay 的每个 Credit 至少有一个 Cast；ACTIVE Album 至少有一个 Credit且 Cast 为空。
- Character 在同一 Work 内规范化名称唯一。
- Tag 是允许多父但禁止环的 DAG；名称和别名在全局保持无歧义。
- 合并或实体元数据变化会把受影响的 Gallery Manifest 标记为 `DB_DIRTY`，但不改变 Gallery 的收录时间和个人状态。

## 3. Manifest 边界

- Gallery DIRECTORY 使用 `.cosplay.json`；ZIP/CBZ 使用相邻的 `<完整归档名>.cosplay.json`，写入 Manifest 永不重打包归档。
- Coser 使用单一元数据根下的 `<uuid>/coser.json`；合并后的源目录可写入独立纯文本 `redirect_to_uuid`。
- Gallery 与 Coser Manifest 都是可选项。未建立基线为 `NONE`，同步后文件丢失为 `MISSING`。
- 首次导入允许部分字段；系统 Push 输出完整、确定性排序的快照。
- Gallery Item 首导可用精确相对路径省略 UUID；建立基线后已有成员必须用 UUID 更新。
- 新 ExternalLink 与 SocialAccount 可由系统生成 UUID；无 UUID 的既有 URL 不允许被当作更新。

## 4. 显式三方同步

- 基线保存业务快照、数据库 `metadata_revision`、文件 BLAKE3 哈希和系统 Push revision。
- Pull 按“基线—数据库—文件”递归到集合成员字段比较；互不重叠的修改自动合并。
- 同字段双方修改及删除—修改产生结构化 JSON Pointer 冲突，普通 Pull 不写数据库。
- 冲突解决必须为每条当前冲突显式选择 `DATABASE` 或 `FILE`；缺项、未知项和过期选择均拒绝。
- Push 遇到外部修改或无基线既有文件时拒绝覆盖；只读媒体库拒绝 Push。
- 原子写保留一份 `.bak`，使用同目录临时文件、fsync 和原子重命名。

## 5. 数据安全与限制

- JSON 顶层未知字段被拒绝，扩展只能位于 `extensions`。
- 路径必须为 NFC、正斜杠相对路径，不允许穿越；Manifest 和托管目录不跟随符号链接。
- Description/Biography 使用禁止 HTML、图片和危险链接协议的受限 Markdown。
- 名称、Alias、Caption、长文本、链接及各集合数量在解析和数据库写入两层执行硬限制。
- ExternalLink 与 SocialAccount 只接受无用户信息的绝对 HTTP(S) URL，不访问外部内容。

## 6. 门禁覆盖

- Album/Cosplay 推导与 ACTIVE 关系规则由 SQLite 集成测试覆盖。
- Tag 多父、环阻断、合并预览、UUID Alias、Tombstone 和 Slug 历史有回归测试。
- Gallery/Coser Manifest 严格解析、首导、Pull、Push、丢失、外部修改、只读来源、成员级合并和显式冲突解决有回归测试。
- 相同 UUID 的名称提示不一致会形成确定性错误，不按扫描顺序选择名称。
