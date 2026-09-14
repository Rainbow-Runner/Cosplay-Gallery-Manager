import { useQuery } from "@apollo/client/react";
import { Link, useSearchParams } from "react-router-dom";
import { MANAGE_GALLERIES } from "../api/manage";
import type { ManageGalleryPage } from "./types";

export function ManageGalleryIndexPage() {
  const [parameters, setParameters] = useSearchParams(); const page = Math.max(1, Number(parameters.get("page")) || 1); const issue = parameters.get("issue") || "ALL";
  const query = useQuery<{ manageGalleries: ManageGalleryPage }>(MANAGE_GALLERIES, { variables: { page, issue } }); const data = query.data?.manageGalleries;
  return <main className="manage-page"><header className="manage-heading"><div><p>GALLERY ROOT</p><h2>Gallery</h2></div><button type="button">＋ Import</button></header>
    {data ? <section className="issue-summary"><span>DRAFT <strong>{data.summary.draft}</strong></span><span>OVER LIMIT <strong>{data.summary.overLimit}</strong></span>
      <span>UNAVAILABLE <strong>{data.summary.unavailable}</strong></span><span>BLOCKING <strong>{data.summary.blocking}</strong></span><span>MISSING <strong>{data.summary.missingItem}</strong></span></section> : null}
    <label className="manage-gallery-filter">异常筛选 / Issue filter<select value={issue} onChange={(event) => setParameters(event.target.value === "ALL" ? {} : { issue: event.target.value })}><option value="ALL">全部 / All</option><option value="UNAVAILABLE">来源不可用 / Unavailable source</option><option value="MISSING">存在缺失媒体 / Missing media</option><option value="PROCESSING_ERROR">媒体处理失败 / Processing error</option><option value="BLOCKING">阻断问题 / Blocking issue</option><option value="OVER_LIMIT">超过上限 / Over limit</option><option value="DRAFT">草稿 / Draft</option></select></label>
    {query.error ? <p role="alert">Unable to load Manage data.</p> : null}
    {data ? <><div className="manage-table-wrap"><table className="manage-table"><thead><tr><th>State</th><th>Title</th><th>Source</th><th>Items</th><th>Missing</th><th>Process</th><th>Issues</th></tr></thead><tbody>
      {data.items.map((row) => <tr key={row.setID} className={row.blockingIssues || row.overLimit || row.missingCount || row.errorCount || row.sourceAvailability !== "AVAILABLE" ? "has-issue" : ""}><td><span className={`state state--${row.state.toLowerCase()}`}>{row.state}</span></td>
        <td><Link to={`/manage/gallery/${row.setID}`}>{row.title}</Link><small>{row.contentRating || "UNRATED"}</small></td><td title={row.sourcePath}>{row.sourceAvailability} · {row.reconcileState}{row.lastScanErrorCode ? <small>{row.lastScanErrorCode}</small> : null}</td>
        <td>{row.itemCount}</td><td>{row.missingCount}</td><td>{row.pendingCount} / {row.errorCount}</td><td>{row.blockingIssues}</td></tr>)}</tbody></table></div>
      {data.totalPages > 1 ? <nav className="manage-pagination" aria-label="Pagination"><button disabled={page <= 1} onClick={() => setParameters({ page: String(page - 1), ...(issue === "ALL" ? {} : { issue }) })}>Previous</button><span>{page} / {data.totalPages}</span><button disabled={page >= data.totalPages} onClick={() => setParameters({ page: String(page + 1), ...(issue === "ALL" ? {} : { issue }) })}>Next</button></nav> : null}</> : null}</main>;
}
