# Coser网络元数据Provider可拔除边界

> 状态：CGM 1.5已确认架构；2026-08-12

## 目标

Coser网络资料导入是可选增强能力，不是Gallery扫描、Coser领域模型、Browse、Manifest或托管资源系统的组成部分。GalleryEpic停止运营、页面不再可用或项目决定停止支持时，移除该适配器不得需要数据库迁移，也不得影响核心产品编译、启动和日常业务。

## 依赖方向

```text
CGM Coser/资产/SocialAccount业务
              ↑
产品端导入编排（只认通用数据）
              ↑
internal/cosermetadata Provider契约与短期预览
              ↑
组合根按构建标签注册可选Provider
              ↑
internal/cosermetadata/galleryepic 站点适配器
```

依赖只能自下而上实现接口。核心业务、数据库和产品Server不得导入GalleryEpic包，不得包含GalleryEpic域名、DOM选择器、页面路径或远端ID语义。

## 模块职责

- `internal/cosermetadata`：Provider接口、规范化候选/Profile/SocialAccount数据、15分钟短期预览和内存上限。不保存业务数据，也不知道任何网站。
- `internal/cosermetadata/galleryepic`：GalleryEpic域名、HTTP安全策略、搜索/详情HTML解析、图片引用和社交平台映射。不得访问CGM数据库。
- `cmd/cgm/metadata_providers_*.go`：唯一组合根。`cgm_galleryepic`标签决定是否实例化GalleryEpic Provider。
- `internal/productserver/coser_metadata.go`：已认证、同源、配置开关、人工预览/选择、审计与通用导入编排。只依赖Provider契约。
- `coserasset`与`productdb`：校验并托管图片、原子发布头像/Banner/SocialAccount、revision和Manifest dirty；只处理通用输入。

## 运行边界

- 启动配置`metadata_scraping_enabled`默认`false`。
- 只有已登录单所有者的显式搜索、预览和应用操作可以发起外联。
- 启动、Browse、Gallery扫描、媒体处理、自动计划和普通Coser编辑不触发Provider。
- 远端图片只进入短期同源预览；选中后通过现有托管资产管线保存，Browse不热链接第三方。
- 不导入浏览器Cookie，不访问发现的社交链接，不记录搜索词、人物名或完整URL。

## 数据与失败语义

- Candidate Ref、RemoteAsset Ref和来源人物ID只存在于15分钟预览内，不进入数据库或Manifest。
- 应用前先校验并暂存全部图片，再在一个数据库事务中发布头像、Banner和账号，Coser metadata revision只递增一次。
- 数据库提交失败最多留下应用生成的未引用托管文件，可由既有资源审阅清理；不会产生半套数据库元数据。
- 相同规范化账号URL跳过；相同平台的不同URL不自动覆盖。现有头像/Banner必须显式确认替换。

## 拆除验收

完全移除GalleryEpic支持只需要：

1. 删除`internal/cosermetadata/galleryepic`；
2. 删除带`cgm_galleryepic`标签的组合根文件；
3. 从正式构建标签中移除`cgm_galleryepic`。

不修改数据库schema、Coser表、SocialAccount表、Manifest、Browse API、资产目录或普通管理页面。无Provider构建的`providers`集合为空，前端自动隐藏网络导入面板。

`make verify-cgm-metadata-provider-boundary`同时验证默认核心依赖图不含GalleryEpic包，以及显式标签构建才包含该包。
