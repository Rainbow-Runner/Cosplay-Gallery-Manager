# Gallery聚合根与阶段1数据模型

> 状态：P01-03～P01-05初始实现
> 更新日期：2026-07-22

## 1. 聚合边界

Gallery是唯一业务聚合根。GallerySource通过`gallery_id UNIQUE`保证一个Gallery最多一个物理来源；GalleryItem使用复合外键`(source_id, gallery_id)`保证成员只能属于该Gallery的唯一Source。

GalleryItem的`item_uuid`、Caption、分类、Position、排除、文件可用性、处理状态和个人状态都属于当前Gallery上下文。不同Source中路径或内容相同的文件仍创建不同item_uuid，数据库不提供跨Gallery成员合并关系。

## 2. 状态和首次激活

- DRAFT允许缺来源、成员和业务元数据，不进入Browse。
- ACTIVE只能由GalleryStore在完整激活校验通过后写入。
- ARCHIVED保留数据和首次激活时间但退出Browse。
- ARCHIVED恢复失败时原子回到DRAFT。
- ACTIVE的业务元数据或Credit/Cast修改若使校验失效，同一事务内回到DRAFT，不删除任何关系。

`added_at_utc`只在第一次成功转为ACTIVE时用`COALESCE`写入；归档恢复和后续激活不会刷新。普通仓储API不提供修改接口，未来仅允许独立的审计管理操作修正。

## 3. ACTIVE校验

当前校验框架已经阻止：

- 缺Source、Source不可用或超过1,000成员；
- 未解决的BLOCKING SourceIssue；
- 没有AVAILABLE且READY的非排除Item；
- 缺标题或内容分级；
- 没有GalleryCredit；
- 有GalleryCast时任一Credit没有Character；
- 存在未处理Coser/Work/Character建议。

无Cast且至少一个Credit派生为Album；有Cast时派生为Cosplay，不保存`collection_type`字段。

## 4. 三层来源健康与Item双状态

GallerySource分别保存：

- availability：AVAILABLE/MISSING/UNREADABLE；
- reconcile_state：NEVER_SCANNED/SCANNING/IN_SYNC/NEEDS_RESCAN/ERROR；
- 可并存、可解决的GallerySourceIssue。

GalleryItem的availability与processing_state独立。来源整体不可访问只更新Source和Gallery `scan_revision`，不批量改写成员状态。`browsable`由后端根据ACTIVE、来源可用、无阻断Issue和至少一个可展示成员派生。

## 5. Revision边界

- Gallery业务元数据、状态、Credit/Cast和Gallery/Item评分增加`metadata_revision`并要求调用方提供expected revision。
- 来源健康、Issue和成员发现属于技术变化，只增加`scan_revision`。
- Favorite、Hidden和History不修改任一Gallery revision。
- 过期expected revision使整个事务回滚，不能泄漏部分个人评分或关系写入。

## 6. 个人状态

GalleryPersonalState和GalleryItemPersonalState都没有user_id：

- Gallery：favorite/favorited_at、1～10半星评分、hidden、last_viewed_at、last_item_id；
- Item：favorite/favorited_at、1～10半星评分；
- `last_item_id`由Trigger保证属于同一Gallery；
- 清理历史只清last_viewed_at和last_item_id；
- 没有view_count、视频播放进度或观看状态。

## 7. 数据库硬约束

- Gallery set_id和item_uuid必须在全局Registry中以正确Kind注册。
- set_id、Item聚合身份和Source所属Gallery不可隐式修改。
- Gallery内Position唯一，Source内相对路径唯一。
- 静态图必须为PHOTO/SELFIE，动画和视频不得保存image_category。
- 非排除成员最多1,000；排除成员可保留，但恢复为第1,001项被Trigger拒绝。
- 所有成员相对路径使用正斜杠并拒绝绝对路径和`..`穿越。

## 8. 已覆盖的纵向测试

- 从DRAFT补齐Source、Item和Credit后激活；归档恢复不刷新added_at。
- ACTIVE业务编辑失效自动回DRAFT；旧metadata_revision写入回滚。
- Source不可访问暂停Browse但不修改Item availability。
- 两个Source中的同路径媒体拥有不同item_uuid，跨Source绑定被复合外键拒绝。
- 多Credit Cosplay要求每个Credit都有Character。
- 第1,001个非排除成员及排除成员超限恢复被拒绝。
- Gallery/Item收藏评分独立，评分原子增加Gallery metadata_revision。
- last_item_id不能跨Gallery。
