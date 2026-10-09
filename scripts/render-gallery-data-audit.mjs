// Render read-only production matcher results into reviewable Markdown patches.
// Usage: node scripts/render-gallery-data-audit.mjs results.json GalleryData.md chunk
// chunk=-1 updates the header; chunks>=0 update 32 original rows each.
import fs from 'node:fs';
import crypto from 'node:crypto';
const [resultPath,inputPath,chunkArg]=process.argv.slice(2);
const input=fs.readFileSync(inputPath,'utf8');
const rows=JSON.parse(fs.readFileSync(resultPath,'utf8'));
const original=input.split('\n').filter(line=>/^\| \d+ \| /.test(line));
if(rows.length!==1170||original.length!==rows.length) throw Error('Business input count mismatch');
const counts=new Map();for(const row of rows) counts.set(row.Name,(counts.get(row.Name)||0)+1);
const stats={确认:0,存疑:0,冲突:0,scan:0,admin:0,none:0,adminOnly:0};
const scanCounts={COSER:0,WORK:0,CHARACTER:0},adminCounts={...scanCounts};
const unique=items=>[...new Map((items||[]).map(x=>[`${x.Kind}:${x.UUID}`,x])).values()];
const keyset=items=>items.map(x=>`${x.Kind}:${x.UUID}`).sort().join(',');
const rendered=rows.map((row,i)=>{
 if(original[i].split(' | ')[1]!==row.Name)throw Error(`Original name changed at ${i+1}`);
 const scan=unique(row.Scan),admin=unique(row.Admin),problems=[...(row.Problems||[])];
 if(counts.get(row.Name)>1)problems.push('原始输入完全重复');
 for(const kind of ['COSER','WORK','CHARACTER']){
  const names=new Map();for(const m of admin.filter(x=>x.Kind===kind)){const ids=names.get(m.Name)||new Set();ids.add(m.UUID);names.set(m.Name,ids)}
  for(const [name,ids]of names)if(ids.size>1)problems.push(`${kind}同名候选对应多个UUID：${name}`);
 }
 const same=keyset(scan)===keyset(admin);
 const state=problems.length?'冲突':scan.length&&same?'确认':'存疑';stats[state]++;
 if(scan.length)stats.scan++;if(admin.length)stats.admin++;
 if(!scan.length&&!admin.length)stats.none++;if(!scan.length&&admin.length)stats.adminOnly++;
 for(const m of scan)scanCounts[m.Kind]++;for(const m of admin)adminCounts[m.Kind]++;
 const notes=[...problems];
 if(problems.some(x=>x==='CHARACTER与明确Work上下文冲突：D'))notes.push('疑似扫描单字符名称D命中D.VA造成的规则误判，需复核，不视为已证实业务冲突');
 if(!scan.length&&!admin.length)notes.push('当前实体库与两层规则均无匹配');
 for(const kind of ['COSER','WORK','CHARACTER']){
  const s=scan.filter(x=>x.Kind===kind),a=admin.filter(x=>x.Kind===kind);
  const missing=a.filter(x=>!s.some(y=>y.UUID===x.UUID)).length;
  if(missing)notes.push(`${kind}仅管理页弱候选／扫描未接受${missing}项`);
  const extra=s.filter(x=>!a.some(y=>y.UUID===x.UUID)).length;
  if(extra)notes.push(`${kind}扫描建议与管理候选差异${extra}项`);
 }
 if(same&&scan.length)notes.push('扫描强建议与管理候选UUID集合一致');
 const cosers=scan.filter(x=>x.Kind==='COSER').length,characters=scan.filter(x=>x.Kind==='CHARACTER').length;
 if(characters&&!scan.some(x=>x.Kind==='WORK'))notes.push('未提供Work强上下文，作品由唯一Character所属关系推导');
 if(cosers>1&&characters)notes.push('多Coser角色归属未确定，Cast建议需人工确认');
 if(!cosers&&(scan.length||admin.length))notes.push('无Coser强建议，不能仅凭角色候选自动激活');
 const cell=kind=>{
  const values=unique([...scan,...admin,...(row.Rejected||[])]).filter(x=>x.Kind===kind);
  if(!values.length)return '—';
  return values.map(x=>{const strong=scan.some(y=>y.Kind===kind&&y.UUID===x.UUID),rejected=(row.Rejected||[]).some(y=>y.UUID===x.UUID);return `${strong?'✓':'?'} ${kind==='CHARACTER'?x.WorkName+' / ':''}${x.Name}（${strong?'扫描完整名称／明确或标签软边界':rejected?'扫描命中但Work上下文冲突，不接受':'仅管理页候选'}；UUID…${x.UUID.slice(-8)}）`}).join('；');
 };
 const escape=s=>s.replaceAll('|','\\|').replaceAll('\n',' ');
 return `| ${i+1} | ${row.Name} | ${escape(cell('COSER'))} | ${escape(cell('WORK'))} | ${escape(cell('CHARACTER'))} | ${state} | ${row.Norm} | ${escape(notes.join('；'))} |`;
});
const chunk=Number(chunkArg);
if(chunk===-2){console.log(JSON.stringify({stats,scanCounts,adminCounts},null,2));process.exit()}
let oldLines,newLines;
if(chunk===-1){
 const split=input.indexOf('| 1 | ');oldLines=input.slice(0,split).trimEnd().split('\n');
 const stamp=new Date().toLocaleString('sv-SE',{timeZone:'Asia/Shanghai'}).replace(' ','T')+'+08:00';
 const logic=crypto.createHash('sha256');for(const f of ['entity_inference_match.go','discovery_store.go','manage_gallery_store.go','automation_runner.go'])logic.update(fs.readFileSync('internal/persistence/productdb/'+f));
 const digest=crypto.createHash('sha256').update(fs.readFileSync(resultPath.replace(/results\.json$/,'product.sqlite'))).digest('hex');
 newLines=`# Gallery 外部命名实体推测业务审计

> 下表完整保留1170条原始外部命名及命名规范备注；本轮仅在正式库在线一致副本上执行只读匹配，不创建Gallery、不写正式库、不读取存档内容。

## 测试基线

- 生成时间：\`${stamp}\`
- 推测逻辑：当前工作区（#结构解析、完整名称词界、已有Tag软边界、原文重叠优先级）；四个生产匹配／自动化源文件联合SHA-256 \`${logic.digest('hex')}\`。
- 原始清单：1170条；初始逐行清单SHA-256 \`a3eef42dd57701715f84768324bdb731bed25e4587c0d18bdcc0db0037db408d\`。本轮逐行复核“外部命名”及顺序不变。
- 隔离数据库：正式schema v22在线一致副本，完整性ok；SHA-256 \`${digest}\`。
- 实体基线：136 Coser／19 Alias、108 Work／15 Alias、697 Character／94 Alias；软边界词库23 Tag／0 Tag Alias。
- 执行方式：\`TestGalleryDataBusinessAudit\`直接调用生产\`archiveEntitySuggestions\`及\`sourceEntityMatches\`，扫描建议按实际UUID唯一性和明确Work上下文复核。未运行正式库自动化或激活。

## 汇总结论

| 指标 | 本轮结果 | 上轮结果 |
|---|---:|---:|
| 确认 | ${stats.确认} | 471 |
| 存疑 | ${stats.存疑} | 688 |
| 冲突 | ${stats.冲突} | 11 |
| 扫描产生至少一项可解析身份建议 | ${stats.scan} / 1170 | 738 / 1170 |
| 管理页产生至少一项候选 | ${stats.admin} / 1170 | 917 / 1170 |
| 两层规则均无匹配 | ${stats.none} / 1170 | 253 / 1170 |
| 仅管理页有候选 | ${stats.adminOnly} / 1170 | 179 / 1170 |
| 扫描建议明细（按UUID去重／上下文复核） | Coser ${scanCounts.COSER}／Work ${scanCounts.WORK}／Character ${scanCounts.CHARACTER} | Coser 724／Work 2／Character 47 |
| 管理候选明细 | Coser ${adminCounts.COSER}／Work ${adminCounts.WORK}／Character ${adminCounts.CHARACTER} | Coser 803／Work 204／Character 398 |
| 命名规范（原备注保留） | 1040规范／130需整理 | 1040规范／130需整理 |

### 本轮复核发现

- 原“蜜汁猫裘 - 黑天鹅 #黑丝 #巨乳 #崩坏：星穹铁道”记录已由存疑变为确认，Coser／Work／Character三项均为扫描强建议；原文件名中的不确定标记仍单独提示整理。
- 9条冲突包含7条完全重复输入及2条扫描Character与Work证据冲突。多角色／多作品不再仅因数量判冲突。
- 其中\`Mercurius-i (露) - NO.007 守望先锋 D.VA\`疑似单字符Character\`D\`被扫描匹配到\`D.VA\`，管理页则排除短名称：属于潜在匹配规则缺口，不是已证实业务身份冲突，本轮仅记录，不更改生产逻辑。
- \`Umeko J - Eve 2B Dress - Stellar Blade #尼尔：机械纪元\`命中明确Work“尼尔”，但扫描Character“Eve”所属Work不同，仍需人工复核作品混搭／数据库Alias完整性，不能强行自动归属。

### 判定与自动化边界

- **确认**：至少一项扫描强建议可解析到唯一身份，且扫描与管理候选UUID集合一致。仅表示规则一致，不替代人工业务真值，也不等于已验证自动激活。
- **存疑**：无匹配、仅管理页有弱候选，或两层UUID集合不一致；不是整个Gallery的运行时激活禁令。
- **冲突**：完全重复输入、同一名称对应多个身份，或Character与明确Work上下文冲突。多个独立Character或Work本身不再直接判为冲突。
- 原文完整名称及Alias匹配，保留含空格名称；更长名称仅抑制同一位置被覆盖的短名称，不拆数据库名称反向扩展。
- \`#\`／\`＃\`和普通空格可形成明确词界；连续中文只有相邻剩余内容全部能由已有Tag／Alias解释时才形成软边界，否则仅为弱候选。编号、数量和容量先行排除。
- 每个独立名称须唯一UUID。没有Work描述不等于冲突：唯一Character可导出Work；明确矛盾的Work阻止接受。
- 自动关联需启用对应策略；自动激活还需TRUSTED及自动激活开关、至少一位Coser、标题／分级／可展示媒体和无阻断问题。多Coser角色归属不明确时保留Cast建议，不能默认分配第一位。
- 命名规范独立评价：\`??\`／\`？\`标记只提示整理，不直接决定实体匹配。既有草稿关系仍受防覆盖保护，不会因重跑本审计改写。

### 建议命名

推荐\`<Coser> - <Work> - <Character> - <标题> [NO.xxx] [数量-容量].7z\`，也支持\`<Coser> - <Character> #描述标签 #Work\`。多Coser支持\`&\`／\`x\`／\`×\`／\`+\`；身份名称使用完整主名称或已保存Alias。

## 逐项人工复核表

说明：\`✓\`为扫描强建议经UUID／Work复核后的身份；\`?\`为仅管理页候选或因Work冲突未接受的扫描身份（单独注明）。UUID显示末8位。角色所属作品不等于文件名提供了Work证据。

| # | 外部命名 | Coser推测 | Work推测 | Character推测 | 结论 | 命名规范 | 复核说明 |
|---:|---|---|---|---|---|---|---|`.split('\n');
}else{oldLines=original.slice(chunk*32,(chunk+1)*32);newLines=rendered.slice(chunk*32,(chunk+1)*32)}
if(!oldLines.length)throw Error('Empty patch');
console.log('*** Begin Patch\n*** Update File: '+inputPath+'\n@@\n'+oldLines.map(x=>'-'+x).join('\n')+'\n'+newLines.map(x=>'+'+x).join('\n')+'\n*** End Patch');
