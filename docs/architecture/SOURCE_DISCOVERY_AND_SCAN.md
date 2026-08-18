# 来源发现、扫描与安全边界

> 状态：P02-01～P02-09初始实现
> 更新日期：2026-08-08

## 1. 媒体库与来源所有权

媒体库只定义允许发现来源的物理边界。路径始终由最具体的媒体库根拥有；被禁用子根仍然阻断父库扫描。删除或改根只提供影响预览，已有GallerySource必须显式转移或进入未分配状态。

GallerySource仍是Gallery唯一的物理来源。DIRECTORY成员必须位于根内，扫描使用`WalkDir`且拒绝根符号链接、永不跟随内部符号链接；ARCHIVE只允许ZIP/CBZ，成员不能附加任何外部媒体。

## 2. 确定性发现

发现顺序固定为：Ignore/set_id → 已绑定来源 → 有效Manifest → `.cosplay-root` → PATH_TEMPLATE → FIXED_DEPTH/DIRECT_CHILD。自动规则默认全部关闭；DIRECT_CHILD只是深度1预设，不生成元数据。

PATH_TEMPLATE使用Go `regexp`的RE2语义匹配完整NFC相对目录路径。命名捕获只允许title、coser、work、character、year和month，命中结果只保存为建议。没有确定性命中的媒体按实际父目录聚合为未归属诊断，不存在启发式候选模块。

`.cosplay-root`只标记其所在父目录为GallerySource根，不会把唯一子目录提升为来源根。新MARKER候选的标题保底规则为：根内恰好一个直属、真实且非符号链接的子目录时取该子目录名；没有或存在多个直属子目录时取根目录名。标题在发现快照中确定，候选预览、手动导入和AUTO_CREATE_DRAFT使用同一值；不回写已有Gallery。根级媒体默认排除仍属于后续扫描选项，与标题判断无关；ARCHIVE候选不使用此规则。

候选导入只确认来源并创建DRAFT。除上述MARKER确定性标题保底外，即使规则开启AUTO_CREATE_DRAFT，日期和实体关系也不会被自动写入。

## 3. set_id重绑定

新位置出现已注册`set_id`时只创建SOURCE_REBIND_CANDIDATE，不创建第二个Gallery，也不自动修改来源。旧来源仍可访问时候选带冲突标记，确认API要求调用方传入额外的重复来源确认。

确认后保留Gallery和全部Item身份，只替换GallerySource路径并进入NEEDS_RESCAN；旧路径写入路径级Ignore，避免父媒体库静默重新导入。`set_id`级Ignore高于所有显式和自动识别规则。

## 4. 扫描事务

一次扫描按以下边界执行：

1. 创建STAGING运行并把来源置为SCANNING；
2. 只读枚举完整来源，计算内容分类和指纹；
3. 逐项写入source-scoped observation；
4. 只有完整成功后，在一个SQLite事务内对账全部Item并提交来源快照；
5. 失败、取消或不安全来源删除暂存结果，上一版Item状态保持不变。

来源整体失联只把Source改为UNREADABLE/ERROR，既有Item不会被批量标成MISSING。恢复必须完成一次全量成功扫描。

## 5. 身份、指纹与内容变化

- 同一来源同路径优先保留item_uuid。
- 完整内容使用`blake3-v1`，首尾采样和长度使用`blake3-sample-v1`缩小候选。
- 原路径消失且新路径的完整BLAKE3在本次观察和旧Item中都唯一时，自动重绑定同一Item。
- 指纹歧义时不选择：旧Item变为MISSING，新路径形成独立Item。
- 同路径完整指纹变化增加content_revision并把媒体处理状态重置为PENDING。
- 跨来源永不使用指纹合并。

## 6. 排除、忘记与排序

来源内子目录的受支持媒体默认纳入。用户扫描默认把本次新发现、相对路径不含目录段的Gallery根目录媒体写为`excluded`，扫描前可关闭该选项；按路径命中或唯一指纹重绑定的既有Item永远保留此前人工Exclude/Restore决定。自动排除的Item不进入1,000硬上限和媒体处理队列，人工Restore后立即排队所需基础派生任务。

显式忘记仅允许MISSING或已排除Item，只删除应用数据库中的成员记录，并把item_uuid写入永久Tombstone。该操作没有任何磁盘删除能力。

Position在Gallery内全局唯一。首次按PHOTO、SELFIE、动画、视频优先级和完整相对路径自然排序，以1,024间隔分配；后续新增只写全局高水位。Manage以完整父目录分组，根目录分组固定排在所属媒体组最前，文件夹只在同媒体组内排序且移动时保留内部顺序；具体文件夹可由用户显式按文件名自然排序。文件夹和文件夹内排序提交该媒体组的完整Item UUID序列，后端在单事务中验证完整集合、过期revision和跨组输入，再复用该组原Position槽位完成一次metadata revision更新。

## 7. 内容分类与归档安全

扩展名只负责发现，文件签名负责静态图片、动画和视频分类。静态图片默认PHOTO；扩展名与实际内容错配保留观察但产生BLOCKING Issue。RAW先校验格式入口，正式LibRaw解码和代理图属于后续媒体处理阶段。

ZIP/CBZ先检查中央目录，再决定是否读取Entry。不可关闭的结构检查包括路径穿越、绝对/Windows路径、非NFC路径、大小写折叠重复、符号链接、特殊Entry、加密Entry和嵌套归档。可配置资源阈值默认是20,000 Entry、单Entry 2 GiB、总解压估算100 GiB、压缩比1,000和单图200 MP。

归档内Video、RAW和AVIF形成阻断Issue；归档成员总大小使用压缩后大小。危险归档不会用空观察覆盖上一版成功快照。

## 8. 无用户媒体删除能力

阶段2新增代码只调用只读打开、目录遍历、ZIP读取和文件信息查询。用户来源的移动、删除和重命名均不在应用能力范围内。备份失败清理、缓存、临时文件和明确托管元数据资源属于独立的应用生成数据能力，不得复用为媒体来源删除入口。
