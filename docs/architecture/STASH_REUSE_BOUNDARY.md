# Stash复用与隔离边界

> 状态：阶段0执行约束
> 依据：[开发备忘录](../COSPLAY_DEVELOPMENT_MEMO.md)与[第一版开发计划](../COSPLAY_V1_DEVELOPMENT_PLAN.md)
> 工作名：Cosplay Gallery Manager

## 1. 目的

本项目以Stash代码为工程起点，但建立独立产品、数据库和业务模型。本文件约束新代码的依赖方向，防止Gallery优先领域再次被原Stash的Scene、Image和软Gallery语义绑住。

基本原则：复用已经验证的底层机制，重建领域身份、数据模型、API和前端。复用不等于保留旧业务入口。

## 2. 依赖方向

```text
React 19 BrowseShell / ManageShell
                 ↓
       Browse API / Manage API
                 ↓
       Gallery优先应用服务
                 ↓
  新领域模型 / 新仓储接口 / 新任务契约
                 ↓
SQLite、文件、图片、FFmpeg等底层适配器
```

新领域层不得导入原Stash的Scene、Image、Performer、Studio或旧Gallery服务。底层适配器可以包装既有通用能力，但必须向上暴露GalleryItem和新实体语义。

## 3. 可直接复用的底层能力

以下模块可以在测试确认后复用其机制或实现：

- `pkg/sqlite`的连接、事务、WAL、备份和迁移执行机制；不复用旧业务schema作为新产品schema。
- `pkg/image`的libvips/FFmpeg图片解码、方向处理、缩略图和流式输出能力。
- `pkg/ffmpeg`的探测、Range、Remux、转码和流管理基础封装。
- `pkg/file`、`pkg/fsutil`的只读文件访问、归档读取、路径规范化和文件锁基础能力。
- `internal/api`中的HTTP服务器、中间件和响应基础设施；资源授权和DTO必须重建。
- GraphQL生成、i18n、Apollo、Vite及现有构建链的工具能力。

复用前必须补充路径穿越、只读来源、资源上限、认证和跨平台测试。旧代码中能够删除或改写用户媒体的能力不得进入新适配器接口。

## 4. 必须通过适配层复用的能力

以下能力有价值，但旧实现带有Stash业务假设，只能被新接口包装：

- 图片派生：缓存键改为`item_uuid + content_revision + variant + processing_profile`。
- 视频处理：只暴露GalleryItem级Poster、直接播放、Remux和基础代理操作。
- 扫描：只复用遍历/探测原语；候选、来源所有权、staging和原子提交全部重建。
- 任务并发：第一版最终使用数据库持久化租约队列，不能把内存Job状态当成业务事实。
- Session和HTTP认证：重建为无User表的单密码所有者模型，并区分Browse/Manage可见性。
- GraphQL：保留传输和生成工具，重建Browse DTO、Manage Mutation及权限边界。

## 5. 必须替换或退出发行包的旧业务

- 原Scene/Image独立业务页面、resolver、Mutation和单文件删除入口。
- 原Gallery软组合、多来源/外部媒体关联及按Image索引的Preview API。
- Performer、Studio、Group/Movie等不符合新最小实体模型的业务关系。
- 网络Scraper、StashBox识别、在线更新和其他主动外联路径。
- 插件市场、运行时插件、脚本Hook和旧主题兼容。
- Clean任务及任何用户媒体物理删除/改写能力。
- DLNA、外部播放器、打开文件管理器和执行外部命令的产品入口。
- 旧React 17、Bootstrap和`ui/v2.5`业务页面。

这些代码在迁移期间可以继续维持旧基线编译，但新功能不得继续依赖；完成替代后从正式发行构建中移除。

## 6. 新代码落点

- `internal/product`：稳定机器身份和四套版本语义。
- `internal/domain/...`：不依赖存储/API的Gallery优先领域模型。
- `internal/application/...`：导入、扫描、激活、编辑、Manifest和任务用例。
- `internal/persistence/...`：新SQLite schema与仓储实现。
- `internal/api/browse`、`internal/api/manage`：分层API和白名单DTO。
- `ui/web`：React 19 BrowseShell、ManageShell和SetupShell。

目录可以在实现中进一步细分，但不得反转上述依赖方向。

## 7. 评审检查清单

新工作包合并前必须确认：

1. 是否把Gallery作为聚合根和最小事务范围？
2. 是否意外导入了原Scene/Image/旧Gallery业务服务？
3. 是否允许路径、数字ID或UUID绕过资源认证？
4. 是否存在删除、移动或改写用户来源媒体的能力？
5. 是否把扫描技术状态错误地写入metadata_revision？
6. 是否把全部成员URL或物理路径放入Browse DTO？
7. 是否引入主动外联、插件执行或上游兼容负担？
8. 是否为异常、恢复和跨平台行为增加了测试？

违反核心不变量的复用必须停止，并通过ADR说明替代方案；不能以减少改动量为理由保留旧业务语义。
