import { useLazyQuery, useMutation, useQuery } from "@apollo/client/react";
import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { MANAGE_GALLERIES, PREVIEW_GALLERY_MANIFEST_BATCH_PUSH, PUSH_GALLERY_MANIFESTS, RETRY_GALLERY_DRAFTS } from "../api/manage";
import type { ManageGalleryDraftRetryQueueResult, ManageGalleryManifestBatchPreview, ManageGalleryManifestBatchResult, ManageGalleryPage } from "./types";

const filters = [
  { issue: "ALL", label: "全部 / All", count: "all" },
  { issue: "DRAFT", label: "草稿 / Draft", count: "draft" },
  { issue: "OVER_LIMIT", label: "超过上限 / Over limit", count: "overLimit" },
  { issue: "UNAVAILABLE", label: "来源不可用 / Unavailable", count: "unavailable" },
  { issue: "BLOCKING", label: "阻断问题 / Blocking", count: "blocking" },
  { issue: "MISSING", label: "缺失媒体 / Missing", count: "missingGallery" },
  { issue: "PROCESSING_ERROR", label: "处理失败 / Processing error", count: "processingError" },
  { issue: "MANIFEST", label: "Manifest 待处理", count: "manifestAttention" },
  { issue: "CAPTURE_DATE", label: "拍摄时间待复核 / Date review", count: "captureDateAttention" },
] as const;

const manifestReasons: Record<string, string> = {
  METADATA_WRITEBACK_DISABLED: "元数据写回已关闭 / Metadata writeback disabled",
  MANIFEST_PARENT_NOT_WRITABLE: "Manifest 目录不可写 / Manifest directory not writable",
  MANIFEST_PARENT_INVALID: "Manifest 目录无效 / Invalid Manifest directory",
  SOURCE_UNAVAILABLE: "来源不可用 / Source unavailable",
  MANIFEST_CHECK_FAILED: "Manifest 校验失败 / Manifest check failed",
  INVALID_OR_OTHER_MANIFEST: "Manifest 无效或属于其他图集 / Invalid or different Manifest",
  MANIFEST_READ_FAILED: "Manifest 读取失败 / Manifest read failed",
  NO_SOURCE: "没有来源 / No source",
};
function reasonLabel(reason: string) { return manifestReasons[reason] || reason; }
const draftRetryReasons: Record<string, string> = {
  NOT_DRAFT: "已不是草稿 / No longer a draft",
  GALLERY_NOT_FOUND: "图集已不存在 / Gallery not found",
  LIBRARY_REQUIRED: "尚未归属媒体库 / Library required",
  LIBRARY_DISABLED: "媒体库已关闭 / Library disabled",
  SAVED_AUTOMATION_POLICY_REQUIRED: "请先在对应媒体库保存辅助或可信自动化策略 / Save an assisted or trusted policy first",
  AUTOMATION_ACTIVE: "该媒体库已有自动化任务，完成后可重试 / Library automation is already active",
  SELECTION_CHANGED: "入队前所选草稿状态或所属库变化，请刷新后重试 / Selection changed; refresh and retry",
  POLICY_READ_FAILED: "无法读取媒体库策略 / Unable to read library policy",
  QUEUE_FAILED: "入队失败，请刷新后重试 / Unable to queue; refresh and retry",
};

export function ManageGalleryIndexPage() {
  const [parameters, setParameters] = useSearchParams();
  const page = Math.max(1, Number(parameters.get("page")) || 1);
  const issue = parameters.get("issue") || "ALL";
  const search = parameters.get("search") || "";
  const [searchDraft, setSearchDraft] = useState(search);
  const query = useQuery<{ manageGalleries: ManageGalleryPage }>(MANAGE_GALLERIES, { variables: { page, issue, search } });
  const [loadPreview, previewState] = useLazyQuery<{ previewGalleryManifestPush: ManageGalleryManifestBatchPreview[] }>(PREVIEW_GALLERY_MANIFEST_BATCH_PUSH, { fetchPolicy: "network-only" });
  const [pushBatch, pushState] = useMutation<{ pushGalleryManifests: ManageGalleryManifestBatchResult[] }>(PUSH_GALLERY_MANIFESTS);
  const [retryDrafts, retryState] = useMutation<{ retryGalleryDrafts: ManageGalleryDraftRetryQueueResult[] }>(RETRY_GALLERY_DRAFTS);
  const [retryResults, setRetryResults] = useState<ManageGalleryDraftRetryQueueResult[] | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [preview, setPreview] = useState<ManageGalleryManifestBatchPreview[] | null>(null);
  const [results, setResults] = useState<ManageGalleryManifestBatchResult[] | null>(null);
  const [message, setMessage] = useState("");
  // Variable changes temporarily clear data. Keep the last result mounted so
  // the filter cards and table do not collapse while the next request runs.
  const data = query.data?.manageGalleries ?? query.previousData?.manageGalleries;
  const updating = query.loading || searchDraft.trim() !== search;
  const listActionsDisabled = updating || !!query.error || !query.data?.manageGalleries || retryState.loading;
  const selectedOnPage = data?.items.filter((row) => selected.includes(row.setID)).length || 0;

  useEffect(() => { setSearchDraft(search); }, [search]);
  useEffect(() => {
    const value = searchDraft.trim();
    if (value === search) return;
    const timeout = window.setTimeout(() => setParameters((current) => {
      const next = new URLSearchParams(current);
      next.delete("page");
      if (value) next.set("search", value); else next.delete("search");
      return next;
    }), 350);
    return () => window.clearTimeout(timeout);
  }, [searchDraft, search, setParameters]);

  function changeIssue(nextIssue: string) {
    setParameters((current) => {
      const next = new URLSearchParams(current);
      next.delete("page");
      if (nextIssue === "ALL") next.delete("issue"); else next.set("issue", nextIssue);
      return next;
    });
  }

  function changePage(nextPage: number) {
    setParameters((current) => {
      const next = new URLSearchParams(current);
      if (nextPage <= 1) next.delete("page"); else next.set("page", String(nextPage));
      return next;
    });
  }

  function toggle(setID: string) {
    if (listActionsDisabled) return;
    setSelected((current) => current.includes(setID) ? current.filter((value) => value !== setID) : current.length < 100 ? [...current, setID] : current);
  }
  async function previewBatch() {
    if (!selected.length || listActionsDisabled) return;
    setMessage(""); setResults(null);
    try {
      const response = await loadPreview({ variables: { setIDs: selected } });
      if (!response.data) throw new Error("No preview returned");
      setPreview(response.data.previewGalleryManifestPush);
    } catch (error) { setMessage(error instanceof Error ? error.message : "Preview failed"); }
  }
  async function retrySelectedDrafts() {
    if (!selected.length || listActionsDisabled || retryState.loading) return;
    if (!window.confirm(`重新识别并处理所选 ${selected.length} 个图集中的草稿？仅处理勾选项，不扫描媒体；非草稿会跳过。跨媒体库时分别使用各库已保存策略，仅在开启可信自动激活时尝试激活。\nRetry selected drafts only (${selected.length} selected)? No media scan. Non-drafts are skipped; each library's saved policy applies.`)) return;
    setMessage("");
    try {
      const response = await retryDrafts({ variables: { setIDs: [...selected] } });
      if (!response.data) throw new Error("No draft retry result returned");
      const items = response.data.retryGalleryDrafts;
      setRetryResults(items);
      const queued = new Set(items.filter((item) => item.outcome === "QUEUED").map((item) => item.setID));
      setSelected((current) => current.filter((id) => !queued.has(id)));
    } catch (error) { setMessage(error instanceof Error ? error.message : "草稿重试入队失败 / Unable to queue draft retry"); }
    try { await query.refetch(); } catch { /* Preserve queue results if the list refresh fails. */ }
  }
  async function execute(overwriteLocal: boolean) {
    if (!preview || pushState.loading || listActionsDisabled) return;
    const overwriteCount = preview.filter((item) => item.localFileChanged && !item.blockReason).length;
    if (overwriteLocal && overwriteCount > 0 && !window.confirm(`Overwrite ${overwriteCount} locally changed Manifest file(s)? This cannot be undone without a separate backup.`)) return;
    setMessage("");
    try {
      const response = await pushBatch({ variables: { items: preview.map((item) => ({ setID: item.setID, expectedMetadataRevision: item.metadataRevision, expectedPath: item.path, expectedFileHash: item.fileHash })), overwriteLocal } });
      if (!response.data) throw new Error("No batch result returned");
      setResults(response.data.pushGalleryManifests);
      setPreview(null); setSelected([]);
    } catch (error) { setMessage(error instanceof Error ? error.message : "Batch Push failed"); }
    try { await query.refetch(); } catch { /* Keep the confirmed write result visible if refreshing the list fails. */ }
  }

  return <main className="manage-page"><header className="manage-heading"><div><p>GALLERY ROOT</p><h2>Gallery</h2></div><button type="button">＋ Import</button></header>
    {data ? <nav className="issue-summary" aria-label="作品集筛选 / Gallery filters">
      {filters.map((filter) => <button key={filter.issue} type="button" className={issue === filter.issue ? "is-active" : ""}
        aria-pressed={issue === filter.issue} onClick={() => changeIssue(filter.issue)}>
        <span>{filter.label}</span><strong>{data.summary[filter.count]}</strong>
      </button>)}
    </nav> : null}
    <div className="manage-gallery-search" role="search">
      <label htmlFor="manage-gallery-search">搜索作品集 / Search Galleries</label>
      <input id="manage-gallery-search" type="search" maxLength={300} value={searchDraft} onChange={(event) => setSearchDraft(event.target.value)} placeholder="标题、别名、来源路径或 ID / Title, alias, source path or ID" />
      {searchDraft ? <button type="button" onClick={() => setSearchDraft("")}>清除 / Clear</button> : null}
      <span role="status" aria-live="polite" title={updating && data ? "正在更新，暂时显示上次结果；选择和批量操作已暂停。 / Updating; previous results remain visible and list actions are paused." : undefined}>{updating ? "搜索中… / Searching…" : query.error ? (data ? "保留上次结果 / Previous results" : "加载失败 / Load failed") : data ? `${data.totalItems} 个匹配作品集 / matches` : ""}</span>
    </div>
    <div className="manage-panel-actions"><button type="button" disabled={!selected.length || previewState.loading || listActionsDisabled} onClick={previewBatch}>批量 Push Manifest / Batch Push ({selected.length}/100)</button><span>先预览，写入前重新校验文件和数据库。</span></div>
    <div className="manage-panel-actions"><button type="button" disabled={!selected.length || listActionsDisabled || pushState.loading || previewState.loading} onClick={retrySelectedDrafts}>重新识别并处理所选草稿 / Retry selected drafts ({selected.length}/100)</button><span>仅勾选的 DRAFT，不扫描媒体；按各媒体库已保存策略处理，其他状态跳过。可跨页选择。 / Selected drafts only, no media scan.</span></div>
    <button type="button" disabled={!selected.length || retryState.loading || pushState.loading} onClick={() => setSelected([])}>清除选择 / Clear selection</button>
    {message ? <p role="alert" className="manage-error">{message}</p> : null}
    {query.error ? <p role="alert">搜索失败，请重试；保留上次可用结果。 / Unable to load Manage data; previous results are retained. <button type="button" disabled={updating} onClick={() => { void query.refetch().catch(() => { /* The query error remains visible for explicit retry. */ }); }}>重试 / Retry</button></p> : null}
    {data ? <><div className="manage-table-wrap" aria-busy={updating}><table className="manage-table"><thead><tr><th><input type="checkbox" aria-label="Select all Galleries on this page" disabled={listActionsDisabled} checked={data.items.length > 0 && selectedOnPage === data.items.length} onChange={(event) => { if (listActionsDisabled) return; setSelected((current) => event.target.checked ? Array.from(new Set([...current, ...data.items.map((row) => row.setID)])).slice(0, 100) : current.filter((id) => !data.items.some((row) => row.setID === id))); }} /></th><th>State</th><th>类型 / Type</th><th>Title</th><th>Source</th><th>Items</th><th>Missing</th><th>Process</th><th>Issues</th><th>Manifest</th></tr></thead><tbody>
      {data.items.map((row) => <tr key={row.setID} className={row.blockingIssues || row.overLimit || row.missingCount || row.errorCount || row.sourceAvailability !== "AVAILABLE" || ["FILE_DIRTY", "CONFLICT", "ERROR", "MISSING"].includes(row.manifestStatus) ? "has-issue" : ""}><td><input type="checkbox" aria-label={`Select ${row.title}`} disabled={listActionsDisabled} checked={selected.includes(row.setID)} onChange={() => toggle(row.setID)} /></td><td><span className={`state state--${row.state.toLowerCase()}`}>{row.state}</span></td><td><span className="cgm-badge" title={row.collectionType === "COSPLAY" ? "已关联角色 / Character linked" : row.collectionType === "ALBUM" ? "当前未关联角色；草稿类型会随关系更新 / No linked Character; draft type follows saved relations" : undefined}>{row.collectionType ?? "—"}</span></td>
        <td><Link to={`/manage/gallery/${row.setID}`}>{row.title}</Link><small>{row.contentRating || "UNRATED"}</small>{row.captureDateReviewStatus === "PENDING" ? <small><Link to={`/manage/gallery/${row.setID}?tab=basic`}>拍摄时间待复核 / Date review</Link></small> : null}</td><td title={row.sourcePath}>{row.sourceAvailability} · {row.reconcileState}{row.lastScanErrorCode ? <small>{row.lastScanErrorCode}</small> : null}{row.lastScanCompleted ? <small><Link to={`/manage/gallery/${row.setID}?tab=source`}>{row.lastScanChangeCount > 0 ? `扫描发现变化 ${row.lastScanChangeCount} 项 · 查看详情` : "查看扫描记录 / Scan history"}</Link></small> : null}</td>
        <td>{row.itemCount}</td><td>{row.missingCount > 0 ? <Link to={`/manage/gallery/${row.setID}?tab=source`} title="查看最近扫描变化摘要 / View scan changes">{row.missingCount} · 查看变化</Link> : row.missingCount}</td><td>{row.pendingCount} / {row.errorCount}</td><td>{row.blockingIssues}</td><td><Link to={`/manage/gallery/${row.setID}?tab=manifest`}>{row.manifestStatus || "UNCHECKED"}</Link>{row.manifestCheckedAt ? <small>Checked {row.manifestCheckedAt}</small> : <small>Not checked yet</small>}</td></tr>)}</tbody></table></div>
      {data.totalItems === 0 && !listActionsDisabled ? <p className="manage-gallery-empty">没有匹配的作品集 / No matching Galleries.</p> : null}
      {data.totalPages > 1 ? <nav className="manage-pagination" aria-label="Pagination"><button disabled={listActionsDisabled || page <= 1} onClick={() => changePage(page - 1)}>Previous</button><span>{data.page} / {data.totalPages}</span><button disabled={listActionsDisabled || page >= data.totalPages} onClick={() => changePage(page + 1)}>Next</button></nav> : null}</> : null}
    {preview ? <div className="manage-modal-backdrop"><section role="dialog" aria-modal="true" aria-label="Batch Manifest Push preview" className="manage-panel manage-batch-manifest-dialog"><h3>批量 Push 预览 / Batch Push preview</h3><p>数据库更新可正常写回；只有本地文件改动的项目需要选择覆盖或跳过。元数据写回关闭、目录不可写、错误和离线项目始终跳过。</p>{preview.every((item) => !!item.blockReason) ? <p role="alert" className="manage-error">所有选中图集都被阻断，无法执行 Push。/ All selected Galleries are blocked.</p> : null}<div className="manage-table-wrap" aria-busy={updating}><table className="manage-table"><thead><tr><th>Gallery</th><th>Manifest</th><th>DB content</th><th>Local file</th><th>Decision</th></tr></thead><tbody>{preview.map((item) => <tr key={item.setID}><td>{item.title}</td><td>{item.status}</td><td>{item.databaseContentChanged ? "Changed" : "Unchanged"}</td><td>{item.localFileChanged ? "Changed" : "Unchanged"}</td><td>{item.blockReason ? reasonLabel(item.blockReason) : item.localFileChanged ? "Overwrite or skip" : item.databaseContentChanged || item.status === "MISSING" ? "Push" : "No rewrite"}</td></tr>)}</tbody></table></div><div className="manage-panel-actions"><button type="button" onClick={() => setPreview(null)}>Cancel</button><button type="button" disabled={listActionsDisabled || pushState.loading || preview.every((item) => !!item.blockReason)} onClick={() => execute(false)}>跳过本地改动 / Skip changed files</button><button type="button" disabled={listActionsDisabled || pushState.loading || preview.every((item) => !!item.blockReason)} onClick={() => execute(true)}>覆盖本地改动 / Overwrite changed files</button></div></section></div> : null}
    {results ? <div className="manage-modal-backdrop"><section role="dialog" aria-modal="true" aria-label="Batch Manifest Push result" className="manage-panel manage-batch-manifest-dialog"><h3>批量 Push 结果 / Batch Push result</h3><p>{results.filter((item) => item.outcome === "PUSHED").length} pushed · {results.filter((item) => item.outcome === "SKIPPED").length} skipped · {results.filter((item) => item.outcome === "FAILED").length} failed</p><ul>{results.map((item) => <li key={item.setID}>{item.setID}: {item.outcome}{item.reason ? ` · ${reasonLabel(item.reason)}` : ""}</li>)}</ul><button type="button" onClick={() => setResults(null)}>Close</button></section></div> : null}
    {retryResults ? <div className="manage-modal-backdrop"><section role="dialog" aria-modal="true" aria-label="Selected draft retry result" className="manage-panel manage-batch-manifest-dialog"><h3>所选草稿重试 / Selected draft retry</h3><p>已入队 / Queued: {retryResults.filter((item) => item.outcome === "QUEUED").length} · 已跳过 / Skipped: {retryResults.filter((item) => item.outcome !== "QUEUED").length}</p><p>入队不等于已激活。进度、取消及最终逐项结果请查看对应媒体库的自动化面板。 / Queued does not mean activated.</p><Link to="/manage/libraries">查看媒体库任务 / Open library automation</Link><ul>{retryResults.map((item) => <li key={item.setID}><Link to={`/manage/gallery/${item.setID}`}>{item.title || item.setID}</Link>: {item.outcome === "QUEUED" ? `已入队 / Queued · Library ${item.libraryID} · Run #${item.runID}` : `已跳过 / Skipped · ${draftRetryReasons[item.reason] || item.reason}`}</li>)}</ul><button type="button" onClick={() => setRetryResults(null)}>关闭 / Close</button></section></div> : null}
  </main>;
}
