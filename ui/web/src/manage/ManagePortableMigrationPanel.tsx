import { useMutation, useQuery } from "@apollo/client/react";
import { useEffect, useMemo, useState } from "react";
import { useIntl } from "react-intl";

import { MANAGE_LIBRARIES, MANAGE_PORTABLE_MIGRATION, RUN_PORTABLE_MIGRATION } from "../api/manage";
import { Dialog } from "../ui/Patterns";
import type { ManageLibrary, ManageMaintenanceState, ManagePortableMigrationSnapshot, ManagePortablePreflight } from "./types";

type PortableAction = "PREFLIGHT_EXPORT" | "EXPORT" | "IMPORT" | "PREPARE_MERGE" | "DECIDE_MERGE" | "APPLY_MERGE" | "ABORT_MERGE" | "RECOVER_IMPORT" | "RECOVER_MERGE" | "MAP_LIBRARIES" | "PREFLIGHT_REBUILD" | "REBUILD" | "APPLY_CONTINUITY";
type ActionResult = { runPortableMigration: { code: string; importID?: string | null; mergeID?: string | null; exportID?: string | null; fileName: string; count: number; snapshot: ManagePortableMigrationSnapshot; preflight?: ManagePortablePreflight | null } };

const confirmations: Record<PortableAction, string> = {
  PREFLIGHT_EXPORT: "PREFLIGHT", EXPORT: "EXPORT", IMPORT: "IMPORT", PREPARE_MERGE: "PREPARE", DECIDE_MERGE: "DECIDE", APPLY_MERGE: "MERGE",
  ABORT_MERGE: "ABORT", RECOVER_IMPORT: "RECOVER", RECOVER_MERGE: "RECOVER", MAP_LIBRARIES: "MAP", PREFLIGHT_REBUILD: "PREFLIGHT", REBUILD: "REBUILD", APPLY_CONTINUITY: "CONTINUITY",
};

export function ManagePortableMigrationPanel() {
  const intl = useIntl();
  const zh = intl.locale.toLowerCase().startsWith("zh");
  const t = (cn: string, en: string) => zh ? cn : en;
  const [path, setPath] = useState("");
  const [selectedImportID, setSelectedImportID] = useState("");
  const [selectedMergeID, setSelectedMergeID] = useState("");
  const [includeLifecycle, setIncludeLifecycle] = useState(false);
  const [includeFlags, setIncludeFlags] = useState(false);
  const [allowIncomplete, setAllowIncomplete] = useState(false);
  const [mergeDecisions, setMergeDecisions] = useState<Record<string, string>>({});
  const [libraryDecisions, setLibraryDecisions] = useState<Record<string, string>>({});
  const [pendingAction, setPendingAction] = useState<PortableAction | null>(null);
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [message, setMessage] = useState("");
  const [preflight, setPreflight] = useState<ManagePortablePreflight | null>(null);

  const snapshotQuery = useQuery<{ managePortableMigration: ManagePortableMigrationSnapshot; manageMaintenance: ManageMaintenanceState }>(MANAGE_PORTABLE_MIGRATION, {
    variables: { importID: selectedImportID || null, mergeID: selectedMergeID || null },
  });
  const librariesQuery = useQuery<{ manageLibraries: ManageLibrary[] }>(MANAGE_LIBRARIES);
  const [runAction, actionState] = useMutation<ActionResult>(RUN_PORTABLE_MIGRATION);
  const snapshot = snapshotQuery.data?.managePortableMigration;
  const maintenance = snapshotQuery.data?.manageMaintenance;

  useEffect(() => {
    const next: Record<string, string> = {};
    for (const conflict of snapshot?.conflicts ?? []) next[conflict.issueKey] = conflict.decision === "UNRESOLVED" ? "" : conflict.decision;
    setMergeDecisions(next);
  }, [snapshot?.conflicts]);

  useEffect(() => {
    const next: Record<string, string> = {};
    for (const mapping of snapshot?.mappings ?? []) next[mapping.libraryKey] = mapping.decision === "SKIP" ? "__SKIP__" : mapping.targetLibraryID ? String(mapping.targetLibraryID) : "";
    setLibraryDecisions(next);
  }, [snapshot?.mappings]);

  const reviewConflicts = useMemo(() => (snapshot?.conflicts ?? []).filter((item) => item.severity === "REVIEW"), [snapshot?.conflicts]);
  const selectedImport = (snapshot?.imports ?? []).find((item) => item.importID === selectedImportID);
  const selectedMerge = (snapshot?.merges ?? []).find((item) => item.mergeID === selectedMergeID);

  function openAction(action: PortableAction) {
    setMessage("");
    setPassword("");
    setConfirmation("");
    setPendingAction(action);
  }

  async function executeAction() {
    if (!pendingAction || confirmation !== confirmations[pendingAction] || !password) return;
    const input = {
      action: pendingAction, path: path.trim(), importID: selectedImportID,
      mergeID: pendingAction === "RECOVER_MERGE" ? maintenance?.restoreBackupID ?? selectedMergeID : selectedMergeID,
      password, confirmation, allowIncompleteGallery: allowIncomplete,
      includeGalleryLifecycle: includeLifecycle, includePersonalFlags: includeFlags,
      mergeDecisions: reviewConflicts.map((item) => ({ issueKey: item.issueKey, decision: mergeDecisions[item.issueKey] ?? "" })),
      libraryDecisions: (snapshot?.mappings ?? []).map((item) => ({ libraryKey: item.libraryKey, targetLibraryID: libraryDecisions[item.libraryKey] === "__SKIP__" ? null : Number(libraryDecisions[item.libraryKey]) })),
    };
    try {
      const result = await runAction({ variables: { input } });
      const value = result.data?.runPortableMigration;
      if (!value) throw new Error(t("迁移操作未返回结果。", "Migration operation returned no result."));
      const importID = value.importID ?? selectedImportID;
      const mergeID = value.mergeID ?? selectedMergeID;
      if (value.importID) setSelectedImportID(value.importID);
      if (value.mergeID) setSelectedMergeID(value.mergeID);
      setMessage(t(`操作完成：${value.code}（${value.count} 项）`, `Completed: ${value.code} (${value.count} items)`));
      setPreflight(value.preflight ?? null);
      setPendingAction(null);
      setPassword("");
      setConfirmation("");
      await snapshotQuery.refetch({ importID: importID || null, mergeID: mergeID || null });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : t("迁移操作失败。", "Migration operation failed."));
    }
  }

  const decisionsReady = reviewConflicts.length > 0 && reviewConflicts.every((item) => Boolean(mergeDecisions[item.issueKey]));
  const mappingsReady = Boolean(snapshot) && (snapshot?.mappings ?? []).every((item) => Boolean(libraryDecisions[item.libraryKey]));
  const needsPath = pendingAction === "EXPORT" || pendingAction === "IMPORT" || pendingAction === "PREPARE_MERGE";

  return <section className="operation-panel portable-workbench">
    <header><div><h3>{t("可移植迁移工作台", "Portable migration workbench")}</h3><p>{t("在新机器路径不同的情况下迁移核心实体并从 Manifest 重建 Gallery。所有路径都是服务器本机绝对路径，操作不会复制原始媒体。", "Move core entities between installations and rebuild Galleries from Manifests when media roots differ. Paths are absolute paths on this server; original media is never copied.")}</p></div></header>
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    {snapshotQuery.error ? <p className="manage-message" role="alert">{t("无法载入迁移会话。", "Unable to load migration sessions.")}</p> : null}

    <div className="portable-workbench__source">
      <label>{t("服务器绝对路径", "Absolute server path")}<input value={path} onChange={(event) => setPath(event.target.value)} placeholder="/srv/cgm-transfer/catalog.cgm-portable.zip" /></label>
      <div className="portable-workbench__options">
        <label><input type="checkbox" checked={allowIncomplete} onChange={(event) => setAllowIncomplete(event.target.checked)} />{t("允许不完整 Gallery 声明", "Allow incomplete Gallery claims")}</label>
        <label><input type="checkbox" checked={includeLifecycle} onChange={(event) => setIncludeLifecycle(event.target.checked)} />{t("导出生命周期状态与首次收录时间", "Export lifecycle state and added-at time")}</label>
        <label><input type="checkbox" checked={includeFlags} onChange={(event) => setIncludeFlags(event.target.checked)} />{t("导出收藏与隐藏标记", "Export favourite and hidden flags")}</label>
      </div>
      <p className="portable-workbench__boundary">{t("不会迁移：Gallery 地址、评分、最后浏览时间、最后浏览项目及其他浏览历史。评分继续由 .cosplay.json Manifest 负责。", "Never migrated: Gallery addresses, ratings, last-viewed time/item, or other browsing history. Ratings remain owned by the .cosplay.json Manifest.")}</p>
      <div className="portable-workbench__actions">
        <button type="button" onClick={() => openAction("PREFLIGHT_EXPORT")}>{t("导出就绪检查…", "Check export readiness…")}</button>
        <button type="button" disabled={!path.trim()} onClick={() => openAction("EXPORT")}>{t("导出包…", "Export package…")}</button>
        <button type="button" disabled={!path.trim()} onClick={() => openAction("IMPORT")}>{t("导入空业务库…", "Import into empty catalog…")}</button>
        <button type="button" disabled={!path.trim()} onClick={() => openAction("PREPARE_MERGE")}>{t("准备合并…", "Prepare merge…")}</button>
      </div>
    </div>

    {preflight ? <div className="portable-workbench__preflight" role="status"><strong>{t("最近一次导出检查", "Latest export readiness check")}</strong><span>{preflight.identityCount} IDs · {preflight.coreEntityCount} Core · {preflight.galleryCount} Gallery · {preflight.assetCount} Assets</span><span>{t("阻断", "Blocking")} {preflight.blockingCount} · {t("警告", "Warnings")} {preflight.warningCount} · {t("Manifest不完整", "Incomplete Manifests")} {preflight.incompleteGalleryCount}</span>{preflight.issues.map((issue) => <small key={`${issue.severity}-${issue.code}`}>{issue.severity} · {issue.code} × {issue.count}</small>)}</div> : null}

    {maintenance?.state === "PORTABLE_IMPORTING" || maintenance?.state === "PORTABLE_MERGING" ? <div className="portable-workbench__recovery" role="alert"><div><strong>{t("检测到中断的迁移维护状态", "Interrupted migration maintenance state detected")}</strong><small>{maintenance.state} · {maintenance.restoreBackupID}</small></div><button type="button" onClick={() => openAction(maintenance.state === "PORTABLE_IMPORTING" ? "RECOVER_IMPORT" : "RECOVER_MERGE")}>{t("执行安全恢复…", "Run safe recovery…")}</button></div> : null}

    <div className="portable-workbench__columns">
      <SessionList title={t("导入 / 重建会话", "Import / rebuild sessions")} empty={t("暂无导入会话", "No import sessions")} items={(snapshot?.imports ?? []).map((item) => ({ id: item.importID, state: item.state, detail: `${item.galleryClaimCount} Gallery · ${item.itemClaimCount} Media`, error: item.errorCode }))} selected={selectedImportID} onSelect={(id) => setSelectedImportID(id)} />
      <SessionList title={t("合并会话", "Merge sessions")} empty={t("暂无合并会话", "No merge sessions")} items={(snapshot?.merges ?? []).map((item) => ({ id: item.mergeID, state: item.state, detail: `${item.reviewCount} Review · ${item.hardBlockingCount} Blocked`, error: item.errorCode }))} selected={selectedMergeID} onSelect={(id) => setSelectedMergeID(id)} />
    </div>

    {selectedMerge ? <div className="portable-workbench__phase"><h4>{t("合并审核", "Merge review")} · {selectedMerge.state}</h4>
      {snapshot?.conflicts.length ? <div className="manage-table-wrap"><table className="manage-table"><thead><tr><th>{t("级别", "Severity")}</th><th>{t("问题", "Issue")}</th><th>{t("传入 / 本地身份", "Incoming / local identity")}</th><th>{t("决定", "Decision")}</th></tr></thead><tbody>{snapshot.conflicts.map((item) => <tr key={item.issueKey}><td><span className={`state state--${item.severity.toLowerCase()}`}>{item.severity}</span></td><td>{item.issueCode}<small>{item.entityKind}{item.fieldKey ? ` · ${item.fieldKey}` : ""}</small></td><td><small>{item.incomingUUID}</small><small>{item.localUUID || "—"}</small></td><td>{item.severity === "REVIEW" ? <select aria-label={`${item.issueCode} decision`} value={mergeDecisions[item.issueKey] ?? ""} onChange={(event) => setMergeDecisions((current) => ({ ...current, [item.issueKey]: event.target.value }))}><option value="">{t("请选择", "Select")}</option>{decisionOptions(item.issueCode).map((value) => <option key={value} value={value}>{value}</option>)}</select> : <span>{t("硬阻断", "Hard blocker")}</span>}</td></tr>)}</tbody></table></div> : <p className="state-message">{t("此合并没有冲突。", "This merge has no conflicts.")}</p>}
      <div className="portable-workbench__actions"><button type="button" disabled={!decisionsReady || selectedMerge.state !== "DECISIONS_PENDING"} onClick={() => openAction("DECIDE_MERGE")}>{t("保存全部决定…", "Save all decisions…")}</button><button type="button" disabled={selectedMerge.state !== "READY"} onClick={() => openAction("APPLY_MERGE")}>{t("应用合并…", "Apply merge…")}</button><button className="danger" type="button" disabled={!["BLOCKED", "DECISIONS_PENDING", "READY", "STALE", "FAILED"].includes(selectedMerge.state)} onClick={() => openAction("ABORT_MERGE")}>{t("终止会话…", "Abort session…")}</button></div>
    </div> : null}

    {selectedImport ? <div className="portable-workbench__phase"><h4>{t("媒体库映射与 Gallery 重建", "Library mapping and Gallery rebuild")} · {selectedImport.state}</h4>
      {snapshot?.owner ? <p className="portable-workbench__boundary">{snapshot.owner.available ? t(`此包包含可选连续性：${snapshot.owner.galleryLifecycle ? "生命周期" : ""}${snapshot.owner.galleryLifecycle && snapshot.owner.personalFlags ? "、" : ""}${snapshot.owner.personalFlags ? "收藏/隐藏" : ""}；${snapshot.owner.galleryCount} 个 Gallery，${snapshot.owner.itemCount} 个收藏媒体。`, `Optional continuity included: ${snapshot.owner.galleryLifecycle ? "lifecycle" : ""}${snapshot.owner.galleryLifecycle && snapshot.owner.personalFlags ? " and " : ""}${snapshot.owner.personalFlags ? "favourite/hidden flags" : ""}; ${snapshot.owner.galleryCount} Galleries and ${snapshot.owner.itemCount} favourite items.`) : t("此包不包含所有者连续性；无需执行连续性应用。", "This package has no owner continuity partition; no continuity apply is needed.")}</p> : null}
      {snapshot?.mappings.length ? <div className="manage-table-wrap"><table className="manage-table"><thead><tr><th>{t("源逻辑库", "Source library")}</th><th>{t("目标媒体库", "Target library")}</th><th>{t("当前状态", "Current state")}</th></tr></thead><tbody>{snapshot.mappings.map((item) => <tr key={item.libraryKey}><td>{item.libraryName}<small>{item.libraryKey}</small></td><td><select aria-label={`${item.libraryName} target library`} value={libraryDecisions[item.libraryKey] ?? ""} onChange={(event) => setLibraryDecisions((current) => ({ ...current, [item.libraryKey]: event.target.value }))}><option value="">{t("请选择", "Select")}</option><option value="__SKIP__">{t("明确跳过", "Explicitly skip")}</option>{(librariesQuery.data?.manageLibraries ?? []).filter((library) => library.enabled).map((library) => <option key={library.id} value={library.id}>{library.name} · {library.rootPath}</option>)}</select></td><td>{item.decision || "PENDING"}<small>{item.targetRoot}</small></td></tr>)}</tbody></table></div> : <p className="state-message">{t("此包没有待映射的逻辑媒体库。", "This package has no logical media libraries to map.")}</p>}
      <div className="portable-workbench__actions"><button type="button" disabled={!mappingsReady || selectedImport.state !== "CORE_IMPORTED"} onClick={() => openAction("MAP_LIBRARIES")}>{t("保存媒体库映射…", "Save library mappings…")}</button><button type="button" disabled={!(["LIBRARIES_MAPPED", "GALLERIES_REBUILT"].includes(selectedImport.state))} onClick={() => openAction("PREFLIGHT_REBUILD")}>{t("重建预检…", "Preflight rebuild…")}</button><button type="button" disabled={selectedImport.state !== "LIBRARIES_MAPPED"} onClick={() => openAction("REBUILD")}>{t("重建 Gallery…", "Rebuild Galleries…")}</button><button type="button" disabled={selectedImport.state !== "GALLERIES_REBUILT" || !snapshot?.owner?.available} onClick={() => openAction("APPLY_CONTINUITY")}>{t("应用可选所有者连续性…", "Apply optional owner continuity…")}</button></div>
      {snapshot?.rebuilds.length ? <div className="manage-table-wrap"><table className="manage-table"><thead><tr><th>Gallery</th><th>{t("来源", "Source")}</th><th>{t("定位 / Manifest", "Locator / Manifest")}</th><th>{t("状态", "State")}</th></tr></thead><tbody>{snapshot.rebuilds.map((item) => <tr key={item.setID}><td><small>{item.setID}</small></td><td>{item.sourceType}<small>{item.relativeSource}</small></td><td>{item.locatorStatus} · {item.manifestStatus}</td><td>{item.state}<small>{item.issueCode || "—"}</small></td></tr>)}</tbody></table></div> : null}
    </div> : null}

    {pendingAction ? <Dialog titleID="portable-action-title" dismissible={!actionState.loading} onClose={() => setPendingAction(null)}><p>{t("高影响迁移操作", "HIGH-IMPACT MIGRATION OPERATION")}</p><h3 id="portable-action-title">{pendingAction}</h3><ul><li>{t("将重新校验保留包、当前目标状态和阶段前置条件。", "The retained package, current target state, and phase prerequisites will be revalidated.")}</li><li>{t("导入和合并会在变更前创建安全备份。", "Import and merge create a safety backup before business data changes.")}</li><li>{t("原始媒体文件不会被复制或删除。", "Original media files are never copied or deleted.")}</li></ul>{needsPath ? <p><strong>{path}</strong></p> : null}<label>{t("所有者密码", "Owner password")}<input type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} /></label><label>{t(`输入 ${confirmations[pendingAction]} 继续`, `Type ${confirmations[pendingAction]} to continue`)}<input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} /></label><footer><button type="button" onClick={() => setPendingAction(null)}>{t("取消", "Cancel")}</button><button type="button" disabled={!password || confirmation !== confirmations[pendingAction] || actionState.loading} onClick={executeAction}>{actionState.loading ? t("正在执行…", "Running…") : t("确认执行", "Confirm")}</button></footer></Dialog> : null}
  </section>;
}

function decisionOptions(issueCode: string) {
  return issueCode === "PORTABLE_CORE_NAME_MATCH_REVIEW" || issueCode === "PORTABLE_SOCIAL_ACCOUNT_URL_REVIEW"
    ? ["KEEP_SEPARATE", "MAP_TO_LOCAL"] : ["KEEP_LOCAL", "USE_INCOMING"];
}

function SessionList({ title, empty, items, selected, onSelect }: { title: string; empty: string; items: { id: string; state: string; detail: string; error: string }[]; selected: string; onSelect: (id: string) => void }) {
  return <section><h4>{title}</h4>{items.length ? <div className="portable-workbench__sessions">{items.map((item) => <button type="button" className={selected === item.id ? "is-selected" : ""} key={item.id} onClick={() => onSelect(item.id)}><strong>{item.state}</strong><span>{item.detail}</span><small>{item.id}</small>{item.error ? <small>{item.error}</small> : null}</button>)}</div> : <p className="state-message">{empty}</p>}</section>;
}
