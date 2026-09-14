import { useQuery } from "@apollo/client/react";
import { Link, useSearchParams } from "react-router-dom";
import { MANAGE_GALLERIES } from "../api/manage";
import type { ManageGalleryPage } from "./types";

const filters = [
  { issue: "ALL", label: "全部 / All", count: "all" },
  { issue: "DRAFT", label: "草稿 / Draft", count: "draft" },
  { issue: "OVER_LIMIT", label: "超过上限 / Over limit", count: "overLimit" },
  { issue: "UNAVAILABLE", label: "来源不可用 / Unavailable", count: "unavailable" },
  { issue: "BLOCKING", label: "阻断问题 / Blocking", count: "blocking" },
  { issue: "MISSING", label: "缺失媒体 / Missing", count: "missingGallery" },
  { issue: "PROCESSING_ERROR", label: "处理失败 / Processing error", count: "processingError" },
] as const;

export function ManageGalleryIndexPage() {
  const [parameters, setParameters] = useSearchParams();
  const page = Math.max(1, Number(parameters.get("page")) || 1);
  const issue = parameters.get("issue") || "ALL";
  const query = useQuery<{ manageGalleries: ManageGalleryPage }>(MANAGE_GALLERIES, { variables: { page, issue } });
  const data = query.data?.manageGalleries;

  return <main className="manage-page"><header className="manage-heading"><div><p>GALLERY ROOT</p><h2>Gallery</h2></div><button type="button">＋ Import</button></header>
    {data ? <nav className="issue-summary" aria-label="作品集筛选 / Gallery filters">
      {filters.map((filter) => <button key={filter.issue} type="button" className={issue === filter.issue ? "is-active" : ""}
        aria-pressed={issue === filter.issue} onClick={() => setParameters(filter.issue === "ALL" ? {} : { issue: filter.issue })}>
        <span>{filter.label}</span><strong>{data.summary[filter.count]}</strong>
      </button>)}
    </nav> : null}
    {query.error ? <p role="alert">Unable to load Manage data.</p> : null}
    {data ? <><div className="manage-table-wrap"><table className="manage-table"><thead><tr><th>State</th><th>Title</th><th>Source</th><th>Items</th><th>Missing</th><th>Process</th><th>Issues</th></tr></thead><tbody>
      {data.items.map((row) => <tr key={row.setID} className={row.blockingIssues || row.overLimit || row.missingCount || row.errorCount || row.sourceAvailability !== "AVAILABLE" ? "has-issue" : ""}><td><span className={`state state--${row.state.toLowerCase()}`}>{row.state}</span></td>
        <td><Link to={`/manage/gallery/${row.setID}`}>{row.title}</Link><small>{row.contentRating || "UNRATED"}</small></td><td title={row.sourcePath}>{row.sourceAvailability} · {row.reconcileState}{row.lastScanErrorCode ? <small>{row.lastScanErrorCode}</small> : null}</td>
        <td>{row.itemCount}</td><td>{row.missingCount}</td><td>{row.pendingCount} / {row.errorCount}</td><td>{row.blockingIssues}</td></tr>)}</tbody></table></div>
      {data.totalPages > 1 ? <nav className="manage-pagination" aria-label="Pagination"><button disabled={page <= 1} onClick={() => setParameters({ page: String(page - 1), ...(issue === "ALL" ? {} : { issue }) })}>Previous</button><span>{page} / {data.totalPages}</span><button disabled={page >= data.totalPages} onClick={() => setParameters({ page: String(page + 1), ...(issue === "ALL" ? {} : { issue }) })}>Next</button></nav> : null}</> : null}</main>;
}
