# Portable UUID全局注册表

> 状态：P01-02初始实现
> 更新日期：2026-07-22

## 1. 目标

所有可进入Manifest或跨数据库恢复的实体共享一个UUID命名空间。Gallery set_id、GalleryItem、Coser、Work、Character、Tag、ExternalLink和SocialAccount不能跨类型复用同一UUID。

Slug、数据库行ID、路径和文件指纹都不是永久业务身份，也不进入注册表。

## 2. 接受格式

- 系统新建实体生成规范小写UUIDv4。
- 导入接受规范小写UUIDv4或UUIDv7。
- 大写、无连字符、非v4/v7以及其他可被宽松解析但不规范的写法均拒绝。
- UUID种类使用封闭枚举；已移除的Studio实体不能占用实体种类。

## 3. 只增不回收模型

`portable_uuid_registry`是永久分配账本。记录一旦创建，UUID、实体种类和首次创建时间均不可修改，记录不可删除。

生命周期由两张只增不改的表派生：

- `portable_uuid_aliases`：实体合并后记录旧UUID指向目标UUID；允许目标以后继续合并，解析时沿链找到当前实体。
- `portable_uuid_tombstones`：实体删除后永久占用UUID，保留删除时间和原因。

Alias和Tombstone记录通过SQLite Trigger禁止更新或删除。注册表主键阻止同UUID跨类型注册，也使Alias或Tombstone UUID永远无法重新创建实体。

## 4. 关系约束

- Alias源和目标在建立时必须均为ACTIVE且属于同一实体种类。
- Alias不能指向当时已经是Alias或Tombstone的UUID；调用方应先解析目标。
- 已是Alias的源不能再成为Tombstone。
- ACTIVE目标以后可以合并到新目标或被删除；旧Alias解析会相应到达新目标或Tombstone。
- 发现类型冲突时返回明确错误，Manifest层后续映射为`PORTABLE_UUID_KIND_CONFLICT`，不按扫描顺序改写。

## 5. 当前边界

本切片建立注册、生成、查询、Alias解析和Tombstone语义。Gallery等实体表尚未建立，因此当前Alias/Tombstone是独立事务；P01-03及后续合并/删除服务必须使用同一检查逻辑的事务内版本，将实体关系变更和UUID生命周期记录原子提交。

## 6. 必须保持的测试

- v4/v7格式和实体种类校验。
- 同UUID同类型重复注册以及跨类型复用均拒绝。
- 多级Alias解析到最终ACTIVE目标。
- Tombstone不能解析或重新注册。
- Registry、Alias和Tombstone历史记录不能通过SQL删除或改写。
