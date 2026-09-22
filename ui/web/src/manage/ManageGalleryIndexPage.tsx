import { useLazyQuery, useMutation, useQuery } from "@apollo/client/react";
import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { MANAGE_GALLERIES, PREVIEW_GALLERY_MANIFEST_BATCH_PUSH, PUSH_GALLERY_MANIFESTS } from "../api/manage";
import type { ManageGalleryManifestBatchPreview, ManageGalleryManifestBatchResult, ManageGalleryPage } from "./types";

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

export function ManageGalleryIndexPage() {
  const [parameters, setParameters] = useSearchParams();
  const page = Math.max(1, Number(parameters.get("page")) || 1);
  const issue = parameters.get("issue") || "ALL";
  const search = parameters.get("search") || "";
  const [searchDraft, setSearchDraft] = useState(search);
  const query = useQuery<{ manageGalleries: ManageGalleryPage }>(MANAGE_GALLERIES, { variables: { page, issue, search } });
  const [loadPreview, previewState] = useLazyQuery<{ previewGalleryManifestPush: ManageGalleryManifestBatchPreview[] }>(PREVIEW_GALLERY_MANIFEST_BATCH_PUSH, { fetchPolicy: "network-only" });
  const [pushBatch, pushState] = useMutation<{ pushGalleryManifests: ManageGalleryManifestBatchResult[] }>(PUSH_GALLERY_MANIFESTS);
  const [selected, setSelected] = useState<string[]>([]);
  const [preview, setPreview] = useState<ManageGalleryManifestBatchPreview[] | null>(null);
  const [results, setResults] = useState<ManageGalleryManifestBatchResult[] | null>(null);
  const [message, setMessage] = useState("");
  const data = query.data?.manageGalleries;
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
    setSelected((current) => current.includes(setID) ? current.filter((value) => value !== setID) : current.length < 100 ? [...current, setID] : current);
  }
  async function previewBatch() {
    if (!selected.length) return;
    setMessage(""); setResults(null);
    try {
      const response = await loadPreview({ variables: { setIDs: selected } });
      if (!response.data) throw new Error("No preview returned");
      setPreview(response.data.previewGalleryManifestPush);
    } catch (error) { setMessage(error instanceof Error ? error.message : "Preview failed"); }
  }
  async function execute(overwriteLocal: boolean) {
    if (!preview || pushState.loading) return;
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
      {data ? <span aria-live="polite">{data.totalItems} 个匹配作品集 / matches</span> : null}
    </div>
    <div className="manage-panel-actions"><button type="button" disabled={!selected.length || previewState.loading} onClick={previewBatch}>批量 Push Manifest / Batch Push ({selected.length}/100)</button><span>先预览，写入前重新校验文件和数据库。</span></div>
    {message ? <p role="alert" className="manage-error">{message}</p> : null}
    {query.error ? <p role="alert">Unable to load Manage data.</p> : null}
    {data ? <><div className="manage-table-wrap"><table className="manage-table"><thead><tr><th><input type="checkbox" aria-label="Select all Galleries on this page" checked={data.items.length > 0 && selectedOnPage === data.items.length} onChange={(event) => setSelected((current) => event.target.checked ? Array.from(new Set([...current, ...data.items.map((row) => row.setID)])).slice(0, 100) : current.filter((id) => !data.items.some((row) => row.setID === id)))} /></th><th>State</th><th>Title</th><th>Source</th><th>Items</th><th>Missing</th><th>Process</th><th>Issues</th><th>Manifest</th></tr></thead><tbody>
      {data.items.map((row) => <tr key={row.setID} className={row.blockingIssues || row.overLimit || row.missingCount || row.errorCount || row.sourceAvailability !== "AVAILABLE" || ["FILE_DIRTY", "CONFLICT", "ERROR", "MISSING"].includes(row.manifestStatus) ? "has-issue" : ""}><td><input type="checkbox" aria-label={`Select ${row.title}`} checked={selected.includes(row.setID)} onChange={() => toggle(row.setID)} /></td><td><span className={`state state--${row.state.toLowerCase()}`}>{row.state}</span></td>
        <td><Link to={`/manage/gallery/${row.setID}`}>{row.title}</Link><small>{row.contentRating || "UNRATED"}</small>{row.captureDateReviewStatus === "PENDING" ? <small><Link to={`/manage/gallery/${row.setID}?tab=basic`}>拍摄时间待复核 / Date review</Link></small> : null}</td><td title={row.sourcePath}>{row.sourceAvailability} · {row.reconcileState}{row.lastScanErrorCode ? <small>{row.lastScanErrorCode}</small> : null}</td>
        <td>{row.itemCount}</td><td>{row.missingCount}</td><td>{row.pendingCount} / {row.errorCount}</td><td>{row.blockingIssues}</td><td><Link to={`/manage/gallery/${row.setID}?tab=manifest`}>{row.manifestStatus || "UNCHECKED"}</Link>{row.manifestCheckedAt ? <small>Checked {row.manifestCheckedAt}</small> : <small>Not checked yet</small>}</td></tr>)}</tbody></table></div>
      {data.totalItems === 0 ? <p className="manage-gallery-empty">没有匹配的作品集 / No matching Galleries.</p> : null}
      {data.totalPages > 1 ? <nav className="manage-pagination" aria-label="Pagination"><button disabled={page <= 1} onClick={() => changePage(page - 1)}>Previous</button><span>{page} / {data.totalPages}</span><button disabled={page >= data.totalPages} onClick={() => changePage(page + 1)}>Next</button></nav> : null}</> : null}
    {preview ? <div className="manage-modal-backdrop"><section role="dialog" aria-modal="true" aria-label="Batch Manifest Push preview" className="manage-panel manage-batch-manifest-dialog"><h3>批量 Push 预览 / Batch Push preview</h3><p>数据库更新可正常写回；只有本地文件改动的项目需要选择覆盖或跳过。元数据写回关闭、目录不可写、错误和离线项目始终跳过。</p>{preview.every((item) => !!item.blockReason) ? <p role="alert" className="manage-error">所有选中图集都被阻断，无法执行 Push。/ All selected Galleries are blocked.</p> : null}<div className="manage-table-wrap"><table className="manage-table"><thead><tr><th>Gallery</th><th>Manifest</th><th>DB content</th><th>Local file</th><th>Decision</th></tr></thead><tbody>{preview.map((item) => <tr key={item.setID}><td>{item.title}</td><td>{item.status}</td><td>{item.databaseContentChanged ? "Changed" : "Unchanged"}</td><td>{item.localFileChanged ? "Changed" : "Unchanged"}</td><td>{item.blockReason ? reasonLabel(item.blockReason) : item.localFileChanged ? "Overwrite or skip" : item.databaseContentChanged || item.status === "MISSING" ? "Push" : "No rewrite"}</td></tr>)}</tbody></table></div><div className="manage-panel-actions"><button type="button" onClick={() => setPreview(null)}>Cancel</button><button type="button" disabled={pushState.loading || preview.every((item) => !!item.blockReason)} onClick={() => execute(false)}>跳过本地改动 / Skip changed files</button><button type="button" disabled={pushState.loading || preview.every((item) => !!item.blockReason)} onClick={() => execute(true)}>覆盖本地改动 / Overwrite changed files</button></div></section></div> : null}
    {results ? <div className="manage-modal-backdrop"><section role="dialog" aria-modal="true" aria-label="Batch Manifest Push result" className="manage-panel manage-batch-manifest-dialog"><h3>批量 Push 结果 / Batch Push result</h3><p>{results.filter((item) => item.outcome === "PUSHED").length} pushed · {results.filter((item) => item.outcome === "SKIPPED").length} skipped · {results.filter((item) => item.outcome === "FAILED").length} failed</p><ul>{results.map((item) => <li key={item.setID}>{item.setID}: {item.outcome}{item.reason ? ` · ${reasonLabel(item.reason)}` : ""}</li>)}</ul><button type="button" onClick={() => setResults(null)}>Close</button></section></div> : null}
  </main>;
}
