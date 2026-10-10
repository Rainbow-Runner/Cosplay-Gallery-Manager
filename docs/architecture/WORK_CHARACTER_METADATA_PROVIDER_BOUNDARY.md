# Work / Character 名称资料 Provider 边界

日期：2026-09-03  
状态：1.5 已确认并实现

## 目标

在不改变CGM离线核心和现有实体模型的前提下，允许所有者从可选资料源搜索Work或Character页面，预览其他名称，并把逐项勾选的名称追加为现有实体Alias。首个适配器为萌娘百科，但站点名称、URL、HTML结构和选择器只存在于`internal/entitymetadata/moegirl`。

## 不变量

- `entitymetadata.Provider`只返回候选与名称建议，不持有数据库、Manifest或产品Server依赖。
- 主程序只有在`cgm_moegirl`编译标签存在时注册萌娘百科适配器；默认核心构建不依赖站点包，删除适配器不需要数据库迁移。
- 启动配置`entity_metadata_scraping_enabled`默认`false`。未启用时Manage路由不存在；启用但未编译Provider时返回明确空列表。
- 只有已认证所有者在Work/Character管理页的显式搜索、候选预览和应用操作可发起外联。启动、Browse、扫描、自动化、计划任务和普通实体读取不得调用Provider。
- 搜索结果不是事实写入。短期预览令牌绑定Provider、实体类型、实体UUID和候选引用；应用时必须再次校验实体revision以及所选Alias确实属于该预览。
- 只追加人工勾选的Alias；不改主名称、Sort name、Character所属Work或任何Gallery关系。名称仍遵守NFC、300字符、100个Alias和规范化重复校验。
- Work/Character前端以真实字符串数组维护Alias气泡，名称内部的`/`不是分隔符；这保证外部建议与后端Alias数组语义一致。
- 写入继续走`UpdateWork`/`UpdateCharacter`路径，保留revision递增、关联Gallery Manifest dirty传播和管理审计。
- 审计只记录Provider键、实体类型/UUID、建议或导入数量、来源page ID/revision ID；默认日志不记录查询、名称、Alias、URL或页面正文。
- HTTP仅允许固定HTTPS主机、公共解析地址、无环境代理、有限重定向/响应大小/超时。页面正文不持久化，测试只保存人工合成最小结构；候选UI显示来源链接供所有者复核。

## 分层

1. `internal/entitymetadata`：站点无关契约、Provider注册表和15分钟预览令牌。
2. `internal/entitymetadata/moegirl`：公开搜索页与文章HTML适配、安全HTTP和语义字段提取。
3. `internal/productserver/entity_metadata.go`：认证、配置门禁、候选预览、选择复核、审计和错误边界。
4. `internal/persistence/productdb/entity_metadata_store.go`：把选中名称合并进现有Alias规则。
5. `ManageEntityMetadataImport.tsx`：Work/Character候选与字段级人工审核；本地表单有未保存修改时禁止应用。

## 萌娘百科首版提取约束

- Work允许字段：原名、官方译名、外文名、译名、常用译名、简称、其他名称。
- Character允许字段：本名、外文名、官方译名、译名、别名、别号、昵称。
- 按字段语义扫描flex信息框和表格，不绑定单一CSS模板；兼容当前`MOE_SKIN_TEMPLATE_BODYCONTENT`正文容器。
- 删除线、隐藏/黑幕、脚注、ruby注音`rt`和脚本样式不形成Alias；ruby正文`rb`保留。
- 官方/原名类建议可默认勾选，别号、昵称、常用简称默认不勾选；最终决定始终属于所有者。

## 删除与失效行为

禁用配置、移除`cgm_moegirl`标签或删除适配器后，已导入Alias仍是普通本地实体数据，Work/Character编辑、扫描、Browse、Manifest和备份恢复不受影响。站点结构变化只应造成局部候选/预览失败，不得使实体管理页面整体黑屏。
