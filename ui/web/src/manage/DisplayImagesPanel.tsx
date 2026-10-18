import { useMutation, useQuery } from "@apollo/client/react";
import { useState } from "react";
import { MANAGE_GALLERY_DISPLAY_IMAGE_STATUS, PREPARE_GALLERY_DISPLAY_IMAGES } from "../api/manage";

export function DisplayImagesPanel({ setID }: { setID: string }) {
  const [message, setMessage] = useState("");
  const { data, error, refetch } = useQuery<{ manageGalleryDisplayImageStatus: { total: number; ready: number; pending: number; failed: number } }>(MANAGE_GALLERY_DISPLAY_IMAGE_STATUS, { variables: { setID }, pollInterval: 5000, fetchPolicy: "network-only" });
  const [prepare, state] = useMutation<{ prepareGalleryDisplayImages: number }>(PREPARE_GALLERY_DISPLAY_IMAGES);
  const status = data?.manageGalleryDisplayImageStatus;
  return <section className="manage-panel"><h3>基础展示图</h3><p>补齐缺失的 960 WebP 展示图；不修改原图或旧 CARD_960 增强缓存。CARD_DISPLAY_960 属于 BASE，封面、详情卡片及快速预览共用一份；旧图库补齐失败仍可沿用已有 480，成功后缓存维护可在安全条件满足时回收冗余 480。仅补齐可用、未排除的静态 IMAGE，不处理 RAW、动画和视频；启动时不自动补齐旧图集。失败任务可在任务管理中查看 CARD_DISPLAY_960 并显式重试。</p>
    {status ? <p>共 {status.total} 张 · 就绪 {status.ready} · 处理中 {status.pending} · 失败 {status.failed}</p> : null}
    <p>本面板打开期间每5秒刷新生成状态，仅为浏览器查询，不代表服务器每5秒扫描或生成。Status refreshes every 5 seconds while this panel is mounted; it does not schedule generation.</p>
    <p>1600 高清封面另由“设置 → 高清封面”显式开启（默认关闭），不属于本补齐操作。</p>
    <button type="button" disabled={state.loading} onClick={async () => { try { const result = await prepare({ variables: { setID } }); setMessage(`已排队 ${result.data?.prepareGalleryDisplayImages ?? 0} 张展示图`); await refetch(); } catch (error) { setMessage(error instanceof Error ? error.message : "展示图排队失败"); } }}>{state.loading ? "排队中…" : "补齐展示图"}</button>
    {message || error ? <p role="status">{message || error?.message}</p> : null}
  </section>;
}
