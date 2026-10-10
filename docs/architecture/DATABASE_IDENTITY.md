# 产品数据库身份守卫

> 状态：P01-01初始实现
> 更新日期：2026-07-22

## 1. 目标

Cosplay Gallery Manager只支持全新产品数据库，不转换原Stash数据库，也不接管未知非空SQLite数据库。数据库身份检查必须发生在任何产品建表、迁移或WAL配置之前。

## 2. 身份表

每个产品数据库包含单行表`cgm_product_identity`：

| 字段 | 约束 | 用途 |
| --- | --- | --- |
| singleton_id | 固定为1的主键 | 保证只有一个身份 |
| product_id | 必须等于`cosplay-gallery-manager` | 稳定机器身份 |
| database_schema_version | 正整数 | 独立产品Schema版本 |
| created_at_utc | RFC3339Nano UTC | 首次初始化审计时间 |

产品显示名称和最终品牌不写入身份判断。未来品牌变化不得修改product_id。

## 3. 分类顺序

1. 不含用户表：`EMPTY`，允许初始化身份表。
2. 含有效身份表：`PRODUCT`，校验product_id和数据库Schema版本。
3. 无身份表但存在`schema_migrations`及至少两个Stash核心表：`LEGACY_STASH`。
4. 其他非空数据库：`UNKNOWN`。

`LEGACY_STASH`和`UNKNOWN`都拒绝启动。识别为UNKNOWN并不意味着可以由用户强制接管；它仍需使用新的空数据库路径。

## 4. 无修改拒绝流程

已有数据库首先使用只读连接检查。只有EMPTY或有效PRODUCT数据库才重新打开写连接。写连接建立后再次检查，以防只读检查与写入之间文件被替换；通过后才初始化身份或配置WAL。

因此被拒绝的原Stash/未知数据库不会收到身份表，不运行旧迁移，也不会被原地转换。

## 5. 与旧SQLite模块的边界

旧`pkg/sqlite.Database.Open`把Schema版本0视为新Stash数据库并运行全部旧迁移，不能用于新产品启动。

新入口位于`internal/persistence/productdb`，当前只负责：

- 只读分类；
- 身份初始化和验证；
- 启动时SQLite integrity_check；
- 本地SQLite可写连接；
- Foreign Keys、5秒Busy Timeout、单写连接、WAL和synchronous=NORMAL。

Schema v1同时创建[Portable UUID全局注册表](PORTABLE_UUID_REGISTRY.md)和[Gallery聚合根](GALLERY_AGGREGATE.md)阶段1表。后续产品迁移继续加入独立模块；在新启动服务和Browse API准备完成前，不把此连接注入旧Manager或旧Repository。

## 6. 必须保持的测试

- 不存在/空数据库可初始化并重开，身份和created_at保持稳定。
- 原Stash特征数据库返回明确错误且不新增身份表。
- 未知非空数据库返回明确错误且不新增身份表。
- 外部product_id和不兼容Schema版本均拒绝。
- 成功打开后journal_mode为WAL且Foreign Keys已启用。

实现已经在写连接上执行二次检查，以拒绝检查期间被替换的数据库。待数据库打开器具备注入测试接缝后，再增加可控竞态回归测试；不得通过依赖真实文件系统竞态的脆弱测试模拟该场景。

## 7. 一致性快照

`Database.Backup`使用SQLite原生Online Backup API逐页生成一致性快照，不直接复制主库、WAL和SHM文件，也不使用动态拼接路径的`VACUUM INTO`。

- 目标路径必须与主库不同且事先不存在；使用独占创建防止检查与写入之间被替换。
- 永不覆盖现有备份。
- 失败或取消时只清理由本次调用创建的目标及其SQLite sidecar。
- 完成后用只读身份、Schema和integrity_check重新验证快照，验证失败不保留。
