import { useApolloClient, useMutation, useQuery } from "@apollo/client/react";
import { type FormEvent, useEffect, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { ADD_GALLERY_EXTERNAL_LINK, FORGET_GALLERY_ITEM, FORGET_MISSING_GALLERY_ITEMS, MANAGE_GALLERY, MANAGE_GALLERY_MANIFEST, PULL_GALLERY_MANIFEST, PUSH_GALLERY_MANIFEST, REORDER_GALLERY_ITEMS, REPLACE_GALLERY_RELATIONS, REPLACE_MISSING_GALLERY_ITEM, RESET_GALLERY_COVER, RESOLVE_GALLERY_MANIFEST, RETRY_GALLERY_ITEM_VIDEO, SCAN_GALLERY_SOURCE, SET_GALLERY_COVER_ITEM, SET_GALLERY_ITEM_EXCLUDED, SET_GALLERY_STATE, UPDATE_GALLERY_ITEM, UPDATE_GALLERY_METADATA } from "../api/manage";
import { buildGalleryMediaFolderTree, flattenGalleryMediaFolders, galleryMediaFileName, galleryMediaGroupKey, galleryMediaParentPath, groupGalleryMedia, moveGalleryMediaFolderTreeNode, naturalFileNameCompare, type GalleryMediaFolder, type GalleryMediaFolderNode, type GalleryMediaGroup } from "./galleryMediaFolders";
import { ManageEntitySelector } from "./ManageEntitySelector";
import { ManageGalleryDeletePanel } from "./ManageGalleryDeletePanel";
import type { ManageGalleryCredit, ManageGalleryDetail, ManageGalleryFolderMatch, ManageGalleryItem, ManageGalleryManifestState, ManageGalleryTag } from "./types";

const tabs = ["basic", "cast", "media", "source", "manifest"] as const;
function isAbsoluteHTTPURL(value: string) {
  try {
    const parsed = new URL(value);
    return (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      parsed.host !== "" && parsed.username === "" && parsed.password === "";
  } catch {
    return false;
  }
}
export function ManageGalleryEditorPage() {
  const client = useApolloClient();
  const navigate = useNavigate();
  const { setID = "" } = useParams(); const [parameters, setParameters] = useSearchParams(); const tab = tabs.includes(parameters.get("tab") as typeof tabs[number]) ? parameters.get("tab") as typeof tabs[number] : "basic";
  const query = useQuery<{ manageGallery: ManageGalleryDetail }>(MANAGE_GALLERY, { variables: { setID }, fetchPolicy: "network-only" }); const [save] = useMutation(UPDATE_GALLERY_METADATA); const [setState] = useMutation(SET_GALLERY_STATE);
  const [updateItem] = useMutation<{ updateGalleryItem: ManageGalleryDetail }>(UPDATE_GALLERY_ITEM);
  const [setExcluded] = useMutation<{ setGalleryItemExcluded: ManageGalleryDetail }>(SET_GALLERY_ITEM_EXCLUDED);
  const [forgetItem, forgetState] = useMutation<{ forgetGalleryItem: ManageGalleryDetail }>(FORGET_GALLERY_ITEM);
  const [forgetMissingItems, forgetMissingState] = useMutation<{ forgetMissingGalleryItems: ManageGalleryDetail }>(FORGET_MISSING_GALLERY_ITEMS);
  const [replaceMissingItem, replaceMissingState] = useMutation<{ replaceMissingGalleryItem: ManageGalleryDetail }>(REPLACE_MISSING_GALLERY_ITEM);
  const [reorderItems, reorderState] = useMutation<{ reorderGalleryItems: ManageGalleryDetail }>(REORDER_GALLERY_ITEMS);
  const [setCover] = useMutation<{ setGalleryCoverItem: ManageGalleryDetail }>(SET_GALLERY_COVER_ITEM);
  const [resetCover] = useMutation<{ resetGalleryCover: ManageGalleryDetail }>(RESET_GALLERY_COVER);
  const [scanSource, scanState] = useMutation<{ scanGallerySource: ManageGalleryDetail }>(SCAN_GALLERY_SOURCE);
	const [retryVideo, retryVideoState] = useMutation<{ retryGalleryItemVideo: boolean }>(RETRY_GALLERY_ITEM_VIDEO);
  const [replaceRelations, relationState] = useMutation<{ replaceGalleryRelations: ManageGalleryDetail }>(REPLACE_GALLERY_RELATIONS);
  const [addExternalLink, externalLinkState] = useMutation<{ addGalleryExternalLink: ManageGalleryDetail }>(ADD_GALLERY_EXTERNAL_LINK);
  const manifestQuery = useQuery<{ manageGalleryManifest: ManageGalleryManifestState }>(MANAGE_GALLERY_MANIFEST, { variables: { setID }, skip: tab !== "manifest", fetchPolicy: "network-only" });
  const [pushManifest, pushManifestState] = useMutation<{ pushGalleryManifest: ManageGalleryManifestState }>(PUSH_GALLERY_MANIFEST);
  const [pullManifest, pullManifestState] = useMutation<{ pullGalleryManifest: ManageGalleryManifestState }>(PULL_GALLERY_MANIFEST);
  const [resolveManifest, resolveManifestState] = useMutation<{ resolveGalleryManifest: ManageGalleryManifestState }>(RESOLVE_GALLERY_MANIFEST);
  const [draft, setDraft] = useState<ManageGalleryDetail | null>(null); const [message, setMessage] = useState("");
  const [excludeNewRootMedia, setExcludeNewRootMedia] = useState(true);
  const [missingOnly, setMissingOnly] = useState(false);
  const [collapsedFolders, setCollapsedFolders] = useState<Record<string, boolean>>({});
  const [replacementMissingUUID, setReplacementMissingUUID] = useState("");
  const [replacementItemUUID, setReplacementItemUUID] = useState("");
  const [linkDraft, setLinkDraft] = useState({ type: "SOURCE", label: "", url: "" });
  const [conflictChoices, setConflictChoices] = useState<Record<string, "DATABASE" | "FILE">>({});
  useEffect(() => { if (query.data) setDraft(query.data.manageGallery); }, [query.data]);
  if (!draft) return <main className="manage-page"><p>{query.error ? "Unable to load Gallery." : "Loading…"}</p></main>;
  const current = draft;
  const externalLinkValid = isAbsoluteHTTPURL(linkDraft.url);
  async function submit(event: FormEvent) { event.preventDefault(); setMessage(""); try { await save({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision, input: {
    title: current.row.title, aliases: current.aliases, description: current.description, shootDate: current.shootDate, shootDatePrecision: current.shootDate ? current.shootDatePrecision : "UNKNOWN",
    contentRating: current.row.contentRating || "NON_ADULT", photographerName: current.photographerName, studioName: current.studioName } } }); setMessage("Saved"); await query.refetch(); } catch { setMessage("Revision conflict or invalid metadata"); } }
  async function transition(state: "DRAFT" | "ACTIVE" | "ARCHIVED") { setMessage(""); try { await setState({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision, state } }); await query.refetch(); } catch (error) { setMessage(error instanceof Error ? error.message : "State transition blocked"); } }
  function editItem(itemUUID: string, patch: Partial<ManageGalleryItem>) { setDraft({ ...current, items: current.items.map((item) => item.uuid === itemUUID ? { ...item, ...patch } : item) }); }
  async function saveItem(item: ManageGalleryItem) { setMessage(""); try { const result = await updateItem({ variables: { setID, itemUUID: item.uuid, expectedMetadataRevision: current.row.metadataRevision, input: { imageCategory: item.mediaKind === "STATIC_IMAGE" ? item.imageCategory || "PHOTO" : null, caption: item.caption } } }); if (result.data) setDraft(result.data.updateGalleryItem); } catch (error) { setMessage(error instanceof Error ? error.message : "Item update failed"); } }
  async function toggleExcluded(item: ManageGalleryItem) { setMessage(""); try { const result = await setExcluded({ variables: { setID, itemUUID: item.uuid, excluded: !item.excluded, expectedMetadataRevision: current.row.metadataRevision } }); if (result.data) setDraft(result.data.setGalleryItemExcluded); } catch (error) { setMessage(error instanceof Error ? error.message : "Exclusion update failed"); } }
  async function forget(item: ManageGalleryItem) { if (!window.confirm(`Forget the CGM record for ${item.relativePath}? This permanently removes its UUID and item metadata, but never deletes the source file.`)) return; setMessage(""); try { const result = await forgetItem({ variables: { setID, itemUUID: item.uuid, expectedMetadataRevision: current.row.metadataRevision } }); if (result.data) setDraft(result.data.forgetGalleryItem); setMessage("Missing media record forgotten; Push Manifest explicitly when ready"); } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to forget media record"); } }
  async function forgetAllMissing() {
    const count = current.items.filter((item) => item.availability === "MISSING").length;
    if (!count || !window.confirm(`Forget all ${count} missing CGM media records? Their UUIDs and item metadata will be permanently retired. Source files are never deleted.`) || window.prompt(`Type FORGET ${count} to confirm`) !== `FORGET ${count}`) return;
    setMessage("");
    try {
      const result = await forgetMissingItems({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision } });
      if (result.data) setDraft(result.data.forgetMissingGalleryItems);
      setReplacementMissingUUID(""); setReplacementItemUUID("");
      client.cache.evict({ fieldName: "galleryMemberIndex" });
      setMessage(`${count} missing records forgotten; Push Manifest explicitly when ready`);
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to forget missing media records"); }
  }
  async function confirmReplacement() {
    const missing = current.items.find((item) => item.uuid === replacementMissingUUID);
    const replacement = current.items.find((item) => item.uuid === replacementItemUUID);
    if (!missing || !replacement) return;
    const preview = `Keep identity and business state from:\n${missing.relativePath}\n\nAdopt file content and path from:\n${replacement.relativePath}\n\nThe temporary replacement UUID will be permanently tombstoned. No source file will be changed or deleted.`;
    if (!window.confirm(preview) || window.prompt("Type REPLACE to confirm this identity migration") !== "REPLACE") return;
    setMessage("");
    try {
      const result = await replaceMissingItem({ variables: { setID, missingItemUUID: missing.uuid, replacementItemUUID: replacement.uuid, expectedMetadataRevision: current.row.metadataRevision, expectedScanRevision: current.row.scanRevision } });
      if (result.data) setDraft(result.data.replaceMissingGalleryItem);
      setReplacementMissingUUID(""); setReplacementItemUUID("");
      client.cache.evict({ fieldName: "galleryMemberIndex" });
      setMessage("Replacement confirmed; the original Item UUID and business state were retained. Push Manifest explicitly when ready.");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to confirm media replacement"); }
  }
  async function chooseCover(item: ManageGalleryItem) { setMessage(""); try { const result = await setCover({ variables: { setID, itemUUID: item.uuid, expectedMetadataRevision: current.row.metadataRevision } }); if (result.data) setDraft(result.data.setGalleryCoverItem); setMessage("Cover updated"); } catch (error) { setMessage(error instanceof Error ? error.message : "Cover update failed"); } }
  async function autoCover() { setMessage(""); try { const result = await resetCover({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision } }); if (result.data) setDraft(result.data.resetGalleryCover); setMessage("Cover reset to automatic selection"); } catch (error) { setMessage(error instanceof Error ? error.message : "Cover reset failed"); } }
  async function scan() { setMessage(""); try { const result = await scanSource({ variables: { setID, excludeNewRootMedia } }); if (result.data) setDraft(result.data.scanGallerySource); setMessage("Source scan completed"); } catch (error) { setMessage(error instanceof Error ? error.message : "Source scan failed"); } }
	async function retryVideoProcessing(item: ManageGalleryItem) { setMessage(""); try { await retryVideo({ variables:{ itemUUID:item.uuid } }); await query.refetch(); setMessage("Video processing queued"); } catch (error) { setMessage(error instanceof Error ? error.message : "Video retry failed"); } }
  function editCredit(index: number, patch: Partial<ManageGalleryCredit>) { setDraft({ ...current, credits: current.credits.map((credit, position) => position === index ? { ...credit, ...patch } : credit) }); }
  function editTag(index: number, patch: Partial<ManageGalleryTag>) { setDraft({ ...current, tags: current.tags.map((tag, position) => position === index ? { ...tag, ...patch } : tag) }); }
  const relationsValid = current.credits.every((credit) => credit.coserUUID && credit.cast.every((cast) => cast.characterUUID)) &&
    current.tags.every((tag) => tag.uuid);
  function folderMatchApplied(match: ManageGalleryFolderMatch) {
    if (match.kind === "COSER") return current.credits.some((credit) => credit.coserUUID === match.uuid);
    if (match.kind === "CHARACTER") return current.credits.some((credit) => credit.cast.some((cast) => cast.characterUUID === match.uuid));
    return current.credits.some((credit) => credit.cast.some((cast) => cast.workUUID === match.uuid));
  }
  function useFolderMatch(match: ManageGalleryFolderMatch) {
    setMessage("");
    if (match.kind === "COSER") {
      if (current.credits.some((credit) => credit.coserUUID === match.uuid)) {
        setMessage("This Coser is already in the relation draft");
        return;
      }
      setDraft({ ...current, credits: [...current.credits, { coserUUID: match.uuid, coserName: match.name, position: "", cast: [] }] });
      setMessage("Folder match added to the relation draft; save all relations to confirm");
      return;
    }
    if (match.kind === "CHARACTER") {
      if (current.credits.length !== 1) {
        setMessage("Add or retain exactly one Coser before applying a Character folder match");
        return;
      }
      if (current.credits[0].cast.some((cast) => cast.characterUUID === match.uuid)) {
        setMessage("This Character is already in the relation draft");
        return;
      }
      setDraft({
        ...current,
        credits: [{
          ...current.credits[0],
          cast: [...current.credits[0].cast, {
            characterUUID: match.uuid, characterName: match.name,
            workUUID: match.workUUID, workName: match.workName, position: "",
          }],
        }],
      });
      setMessage("Folder match added to the relation draft; save all relations to confirm");
    }
  }
  async function saveRelations() {
    if (!relationsValid) {
      setMessage("Select an existing Coser, Character or Tag for every unfinished relation row");
      return;
    }
    setMessage("");
    try {
      const result = await replaceRelations({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision, input: {
        credits: current.credits.map((credit, index) => ({ coserUUID: credit.coserUUID, position: String((index + 1) * 1024), cast: credit.cast.map((cast, castIndex) => ({ characterUUID: cast.characterUUID, position: String((castIndex + 1) * 1024) })) })),
        tags: current.tags.map((tag, index) => ({ tagUUID: tag.uuid, position: String((index + 1) * 1024) })),
      } } });
      if (!result.data) throw new Error("Server did not return the saved Gallery relations");
      setDraft(result.data.replaceGalleryRelations);
      const refreshed = await query.refetch();
      if (refreshed.data) setDraft(refreshed.data.manageGallery);
      setMessage("People, characters and tags saved");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Relation save failed"); }
  }
  async function createExternalLink(event: FormEvent) {
    event.preventDefault(); if (!externalLinkValid) return; setMessage("");
    try {
      const result = await addExternalLink({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision, input: { ...linkDraft, position: String((current.externalLinks.length + 1) * 1024) } } });
      if (result.data) setDraft(result.data.addGalleryExternalLink);
      setLinkDraft({ type: "SOURCE", label: "", url: "" }); setMessage("External link added");
    } catch (error) { setMessage(error instanceof Error ? error.message : "External link failed"); }
  }
  async function runManifestAction(action: "PUSH" | "PULL") {
    if (action === "PUSH") {
      const preview = manifestQuery.data?.manageGalleryManifest;
      if (!preview || !window.confirm(`Write database metadata to Manifest?\n\nMembers: +${preview.pushAdded} added / -${preview.pushRemoved} removed / ${preview.pushRetained} retained (${preview.pushUpdated} updated).\n\nThe source file hash will be checked again before writing.`)) return;
    }
    setMessage("");
    try {
      if (action === "PUSH") await pushManifest({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision } });
      else await pullManifest({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision } });
      await query.refetch(); await manifestQuery.refetch(); setMessage(`Manifest ${action === "PUSH" ? "Push" : "Pull"} completed`);
    } catch (error) { setMessage(error instanceof Error ? error.message : `Manifest ${action} failed`); }
  }
  async function resolveManifestConflicts() {
    const conflicts = manifestQuery.data?.manageGalleryManifest.conflicts || [];
    if (conflicts.some((conflict) => !conflictChoices[conflict.path])) { setMessage("Choose Database or File for every conflict"); return; }
    setMessage("");
    try {
      await resolveManifest({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision, choices: conflicts.map((conflict) => ({ path: conflict.path, choice: conflictChoices[conflict.path] })) } });
      setConflictChoices({}); await query.refetch(); await manifestQuery.refetch(); setMessage("Manifest conflicts resolved and pulled");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Manifest conflict resolution failed"); }
  }
  async function move(item: ManageGalleryItem, direction: -1 | 1) {
    const mediaGroup = groupGalleryMedia(current.items).find((value) => value.key === galleryMediaGroupKey(item));
    if (!mediaGroup) return;
    const folderIndex = mediaGroup.folders.findIndex((folder) => folder.path === galleryMediaParentPath(item.relativePath));
    const itemIndex = mediaGroup.folders[folderIndex]?.items.findIndex((value) => value.uuid === item.uuid) ?? -1;
    const target = itemIndex + direction;
    if (folderIndex < 0 || itemIndex < 0 || target < 0 || target >= mediaGroup.folders[folderIndex].items.length) return;
    const folders = mediaGroup.folders.map((folder) => ({ ...folder, items: [...folder.items] }));
    [folders[folderIndex].items[itemIndex], folders[folderIndex].items[target]] = [folders[folderIndex].items[target], folders[folderIndex].items[itemIndex]];
    await applyFolderOrder(folders, "Item order saved");
  }
  async function applyFolderOrder(folders: GalleryMediaFolder[], successMessage: string) {
    setMessage("");
    try {
      const result = await reorderItems({ variables: {
        setID,
        itemUUIDs: flattenGalleryMediaFolders(folders).map((item) => item.uuid),
        expectedMetadataRevision: current.row.metadataRevision,
      } });
      if (result.data) setDraft(result.data.reorderGalleryItems);
      client.cache.evict({ fieldName: "galleryMemberIndex" });
      setMessage(successMessage);
    } catch (error) { setMessage(error instanceof Error ? error.message : "Folder ordering failed"); }
  }
  async function moveFolder(folders: GalleryMediaFolder[], path: string, direction: -1 | 1) {
    const reordered = moveGalleryMediaFolderTreeNode(folders, path, direction);
    if (reordered) await applyFolderOrder(reordered, "Folder order saved");
  }
  async function naturalSortFolder(folders: GalleryMediaFolder[], folderIndex: number) {
    const reordered = folders.map((folder, index) => index === folderIndex ? { ...folder, items: [...folder.items].sort(naturalFileNameCompare) } : folder);
    await applyFolderOrder(reordered, "Folder sorted by filename");
  }
  const mediaGroups = groupGalleryMedia(missingOnly ? current.items.filter((item) => item.availability === "MISSING") : current.items);
  const duplicateFileNames = current.items.reduce<Record<string, number>>((counts, item) => {
    const name = galleryMediaFileName(item.relativePath).toLowerCase();
    counts[name] = (counts[name] || 0) + 1;
    return counts;
  }, {});
  const replacementMissing = current.items.find((item) => item.uuid === replacementMissingUUID);
  const replacementCandidates = replacementMissing ? current.items.filter((item) => item.uuid !== replacementMissing.uuid && item.availability === "AVAILABLE" && !item.excluded && !item.caption && galleryMediaGroupKey(item) === galleryMediaGroupKey(replacementMissing)) : [];
  function renderMediaFolder(group: GalleryMediaGroup, node: GalleryMediaFolderNode, siblingIndex: number, siblingCount: number) {
    const key = `${group.key}:${node.path}`;
    const collapsed = collapsedFolders[key] ?? (node.path !== "" && !missingOnly);
    const folderIndex = group.folders.findIndex((folder) => folder.path === node.path);
    return <section className="manage-media-folder" key={key}>
      <header><div className="manage-media-folder__identity"><button type="button" aria-label={`${collapsed ? "Expand" : "Collapse"} folder ${node.path || "Gallery root"}`} aria-expanded={!collapsed} onClick={() => setCollapsedFolders((previous) => ({ ...previous, [key]: !collapsed }))}>{collapsed ? "▸" : "▾"}</button><strong title={node.path || "Gallery root"}>{node.name}</strong><small>{node.totalCount} item{node.totalCount === 1 ? "" : "s"}{node.children.length && node.items.length ? ` · ${node.items.length} here` : ""}</small></div><div>
        {node.path ? <><button type="button" aria-label={`Move folder ${node.path} up`} disabled={missingOnly || reorderState.loading || siblingIndex === 0} onClick={() => moveFolder(group.folders, node.path, -1)}>↑ Folder</button><button type="button" aria-label={`Move folder ${node.path} down`} disabled={missingOnly || reorderState.loading || siblingIndex === siblingCount - 1} onClick={() => moveFolder(group.folders, node.path, 1)}>↓ Folder</button></> : <span className="manage-media-root-note">Always first</span>}
        {node.items.length > 1 ? <button type="button" disabled={missingOnly || reorderState.loading} onClick={() => naturalSortFolder(group.folders, folderIndex)}>Natural sort filenames</button> : null}
      </div></header>
      {!collapsed ? <div className="manage-media-folder__body">
        {node.items.length ? <div className="manage-table-wrap"><table className="manage-table manage-media-table"><thead><tr><th>Order</th><th>File / Caption</th><th>Type</th><th>Category</th><th>Availability</th><th>Processing</th><th>Actions</th></tr></thead><tbody>{node.items.map((item, itemIndex) => {
          const fileName = galleryMediaFileName(item.relativePath);
          return <tr key={item.uuid} className={`${item.excluded ? "is-excluded " : ""}${item.availability === "MISSING" ? "has-issue" : ""}`.trim()}><td><button type="button" aria-label={`Move ${item.relativePath} up`} disabled={missingOnly || reorderState.loading || itemIndex === 0} onClick={() => move(item, -1)}>↑</button><button type="button" aria-label={`Move ${item.relativePath} down`} disabled={missingOnly || reorderState.loading || itemIndex === node.items.length - 1} onClick={() => move(item, 1)}>↓</button></td><td><div className="manage-media-file-line"><strong title={item.relativePath}>{fileName}</strong><input aria-label={`Caption for ${item.relativePath}`} value={item.caption} maxLength={1000} placeholder="Caption" onChange={(event) => editItem(item.uuid, { caption: event.target.value })} /></div>{duplicateFileNames[fileName.toLowerCase()] > 1 ? <small className="manage-media-parent-path">{node.path || "Gallery root"} · duplicate filename</small> : null}</td><td>{item.mediaKind}<small>{item.contentFormat}</small>{item.mediaKind === "VIDEO" && item.videoProbeState ? <small>{item.videoContainer || "unknown"} · {item.videoCodec || "unknown"}{item.audioCodec ? ` / ${item.audioCodec}` : ""} · {item.videoWidth}×{item.videoHeight}</small> : null}</td><td>{item.mediaKind === "STATIC_IMAGE" ? <select value={item.imageCategory || "PHOTO"} onChange={(event) => editItem(item.uuid, { imageCategory: event.target.value as "PHOTO" | "SELFIE" })}><option value="PHOTO">PHOTO</option><option value="SELFIE">SELFIE</option></select> : "—"}</td><td>{item.availability}</td><td>{item.mediaKind === "VIDEO" ? item.videoProbeState || item.processingState : item.processingState}{item.videoErrorCode ? <small>{item.videoErrorCode}</small> : null}</td><td><button type="button" onClick={() => saveItem(item)}>Save</button><button type="button" onClick={() => toggleExcluded(item)}>{item.excluded ? "Restore" : "Exclude"}</button>{item.availability === "MISSING" ? <button type="button" onClick={() => { setReplacementMissingUUID(item.uuid); setReplacementItemUUID(""); }}>Replace file…</button> : null}{item.availability === "MISSING" || item.excluded ? <button type="button" disabled={forgetState.loading} onClick={() => forget(item)}>Forget record</button> : null}{item.mediaKind === "STATIC_IMAGE" ? <button type="button" onClick={() => chooseCover(item)}>Cover</button> : null}{item.mediaKind === "VIDEO" ? <button type="button" disabled={retryVideoState.loading} onClick={() => retryVideoProcessing(item)}>Retry video</button> : null}</td></tr>;
        })}</tbody></table></div> : null}
        {node.children.length ? <div className="manage-media-folder__children">{node.children.map((child, index) => renderMediaFolder(group, child, index, node.children.length))}</div> : null}
      </div> : null}
    </section>;
  }
  return <main className="manage-page"><header className="manage-heading"><div><Link to="/manage">← Gallery</Link><h2>{draft.row.title}</h2><p>{draft.row.state} · revision {draft.row.metadataRevision}</p></div>
    <div className="manage-actions">{draft.row.state !== "ACTIVE" ? <button onClick={() => transition("ACTIVE")}>Activate</button> : <button onClick={() => transition("DRAFT")}>Draft</button>}<button onClick={() => transition("ARCHIVED")}>Archive</button></div></header>
    <nav className="editor-tabs">{tabs.map((value) => <button key={value} className={tab === value ? "is-active" : ""} onClick={() => setParameters({ tab: value })}>{value}</button>)}</nav>
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    {tab === "basic" ? <><form className="metadata-form" onSubmit={submit}><label>Title<input value={draft.row.title} onChange={(event) => setDraft({ ...draft, row: { ...draft.row, title: event.target.value } })} /></label>
      <label>Aliases<input value={draft.aliases.join(" / ")} onChange={(event) => setDraft({ ...draft, aliases: event.target.value.split("/").map((value) => value.trim()).filter(Boolean) })} /></label>
      <label className="span-2">Description<textarea value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} /></label>
      <label>Shoot date<input value={draft.shootDate} placeholder="YYYY-MM or YYYY-MM-DD" onChange={(event) => setDraft({ ...draft, shootDate: event.target.value, shootDatePrecision: event.target.value.length === 7 ? "MONTH" : "DAY" })} /></label>
      <label>Rating<select value={draft.row.contentRating || "NON_ADULT"} onChange={(event) => setDraft({ ...draft, row: { ...draft.row, contentRating: event.target.value as "NON_ADULT" | "ADULT" } })}><option value="NON_ADULT">NON_ADULT</option><option value="ADULT">ADULT</option></select></label>
      <label>Photographer<input value={draft.photographerName} onChange={(event) => setDraft({ ...draft, photographerName: event.target.value })} /></label><label>Studio<input value={draft.studioName} onChange={(event) => setDraft({ ...draft, studioName: event.target.value })} /></label>
      <footer className="span-2"><button type="submit">Save metadata</button></footer></form>
      <section className="manage-panel manage-external-links"><h3>Original source links</h3><p>HTTP(S) links are stored as quiet references only; the application never fetches their content.</p>
        {draft.externalLinks.length ? <ul>{draft.externalLinks.map((link) => <li key={link.uuid}><span>{link.type}</span><a href={link.url} target="_blank" rel="noreferrer">{link.label || link.url}</a></li>)}</ul> : <p>No external links.</p>}
        <form className="manage-inline-form" onSubmit={createExternalLink}><select aria-label="External link type" value={linkDraft.type} onChange={(event) => setLinkDraft({ ...linkDraft, type: event.target.value })}><option value="SOURCE">SOURCE</option><option value="PROFILE">PROFILE</option><option value="REFERENCE">REFERENCE</option></select><input aria-label="External link label" placeholder="Label" maxLength={100} value={linkDraft.label} onChange={(event) => setLinkDraft({ ...linkDraft, label: event.target.value })} /><input aria-label="External link URL" type="url" required placeholder="https://…" value={linkDraft.url} onChange={(event) => setLinkDraft({ ...linkDraft, url: event.target.value })} /><button disabled={!externalLinkValid || externalLinkState.loading} type="submit">Add link</button></form>
      </section>
        <ManageGalleryDeletePanel key={`${draft.row.state}-${draft.row.metadataRevision}`} setID={setID} title={draft.row.title} onDeleted={() => navigate("/manage", { replace: true })} />
      </> : null}
    {tab === "cast" ? <section className="manage-panel manage-relations"><header><div><h3>人物、角色与标签</h3><p>一次显式保存整个关系集合。空 Cast 自动归类为 Album；任一 Cast 存在时归类为 Cosplay。</p></div><strong>{draft.credits.some((credit) => credit.cast.length > 0) ? "COSPLAY" : "ALBUM"}</strong></header>
      {(draft.folderMatches || []).length ? <div className="manage-folder-matches"><h4>Folder-name matches</h4><p>These are review-only hints matched against existing names and aliases. Nothing becomes a formal relation until you apply it and save all relations.</p>
        <ul>{draft.folderMatches.map((match) => <li key={`${match.kind}-${match.uuid}`}><div><strong>{match.kind}</strong><span>{match.kind === "CHARACTER" ? `${match.workName} · ${match.name}` : match.name}</span><small>matched “{match.matchedName}”</small></div>{folderMatchApplied(match) ? <em>Already saved</em> : match.kind === "WORK" ? <em>Context only</em> : <button type="button" onClick={() => useFolderMatch(match)}>Use</button>}</li>)}</ul>
      </div> : null}
      <div className="manage-relation-block"><div className="manage-inline-toolbar"><h4>Coser credits</h4><button type="button" onClick={() => setDraft({ ...draft, credits: [...draft.credits, { coserUUID: "", coserName: "", position: "", cast: [] }] })}>Add Coser</button></div>
        {draft.credits.length ? draft.credits.map((credit, creditIndex) => <article className="manage-credit" key={`${credit.coserUUID}-${creditIndex}`}><div className="manage-credit__head"><ManageEntitySelector kind="COSER" label="Coser" uuid={credit.coserUUID} name={credit.coserName} onSelect={(entity) => editCredit(creditIndex, { coserUUID: entity.uuid, coserName: entity.name })} /><button type="button" onClick={() => setDraft({ ...draft, credits: draft.credits.filter((_, index) => index !== creditIndex) })}>Remove</button></div>
          <div className="manage-cast-list"><h5>Characters played by this Coser</h5>{credit.cast.map((cast, castIndex) => <div className="manage-cast-row" key={`${cast.characterUUID}-${castIndex}`}><ManageEntitySelector kind="CHARACTER" label="Character" uuid={cast.characterUUID} name={cast.workName ? `${cast.workName} · ${cast.characterName}` : cast.characterName} onSelect={(entity) => editCredit(creditIndex, { cast: credit.cast.map((value, index) => index === castIndex ? { ...value, characterUUID: entity.uuid, characterName: entity.name, workUUID: entity.workUUID || "", workName: entity.workName || "" } : value) })} /><button type="button" onClick={() => editCredit(creditIndex, { cast: credit.cast.filter((_, index) => index !== castIndex) })}>Remove</button></div>)}<button type="button" onClick={() => editCredit(creditIndex, { cast: [...credit.cast, { characterUUID: "", characterName: "", workUUID: "", workName: "", position: "" }] })}>Add Character</button></div>
        </article>) : <p>No Coser credits. This Gallery is currently an Album.</p>}
      </div>
      <div className="manage-relation-block"><div className="manage-inline-toolbar"><h4>Direct tags</h4><button type="button" onClick={() => setDraft({ ...draft, tags: [...draft.tags, { uuid: "", name: "", position: "" }] })}>Add Tag</button></div>{draft.tags.map((tag, tagIndex) => <div className="manage-tag-row" key={`${tag.uuid}-${tagIndex}`}><ManageEntitySelector kind="TAG" label="Tag" uuid={tag.uuid} name={tag.name} assignableOnly onSelect={(entity) => editTag(tagIndex, { uuid: entity.uuid, name: entity.name })} /><button type="button" onClick={() => setDraft({ ...draft, tags: draft.tags.filter((_, index) => index !== tagIndex) })}>Remove</button></div>)}</div>
      {!relationsValid ? <p className="manage-error">Finish or remove every empty Coser, Character and Tag row before saving.</p> : null}
      <div className="manage-panel-actions"><button type="button" disabled={!relationsValid || relationState.loading} onClick={saveRelations}>{relationState.loading ? "Saving…" : "Save all relations"}</button></div>
    </section> : null}
    {tab === "media" ? <section><div className="manage-inline-toolbar"><span>{draft.items.length} members · {draft.row.missingCount} missing · folder paths are relative to the Gallery root</span><label><input type="checkbox" checked={missingOnly} onChange={(event) => setMissingOnly(event.target.checked)} /> 仅显示缺失媒体 / Missing only</label>{draft.row.missingCount > 0 ? <button type="button" disabled={forgetMissingState.loading} onClick={forgetAllMissing}>{forgetMissingState.loading ? "Forgetting…" : "批量忘记缺失记录 / Forget all missing"}</button> : null}<button type="button" onClick={autoCover}>Reset cover to auto</button></div>
      {replacementMissing ? <section className="manage-panel"><h3>确认媒体替换 / Confirm media replacement</h3><p>旧记录保留UUID、Caption、排序、Exclude、收藏、评分及封面意图；仅采用新记录的路径和文件内容。此操作不会改动或删除源文件。</p><dl><dt>保留身份 / Keep</dt><dd><code>{replacementMissing.relativePath}</code></dd><dt>采用文件 / Adopt</dt><dd><select aria-label="Replacement media file" value={replacementItemUUID} onChange={(event) => setReplacementItemUUID(event.target.value)}><option value="">选择同类型、未编辑的可用媒体…</option>{replacementCandidates.map((item) => <option key={item.uuid} value={item.uuid}>{item.relativePath} · {item.byteSize.toLocaleString()} bytes</option>)}</select></dd></dl>{replacementCandidates.length ? <p>系统不会自动判断哪一项是高清替换；请依据文件路径人工确认。新记录如已有Caption、Exclude、收藏、评分或封面引用，后端会拒绝迁移。</p> : <p className="manage-error">没有同一媒体分组内可安全采用的未编辑项目。</p>}<div className="manage-panel-actions"><button type="button" onClick={() => { setReplacementMissingUUID(""); setReplacementItemUUID(""); }}>取消 / Cancel</button><button type="button" disabled={!replacementItemUUID || replaceMissingState.loading} onClick={confirmReplacement}>{replaceMissingState.loading ? "Replacing…" : "预览并确认 / Preview & confirm"}</button></div></section> : null}
      <div className="manage-media-groups">{mediaGroups.map((mediaGroup) => <section className="manage-media-group" key={mediaGroup.key}><header><h3>{mediaGroup.label}</h3><span>{flattenGalleryMediaFolders(mediaGroup.folders).length}</span></header>
        {renderMediaFolder(mediaGroup, buildGalleryMediaFolderTree(mediaGroup.folders), 0, 1)}
      </section>)}</div></section> : null}
    {tab === "source" ? <section className="manage-panel"><h3>来源与扫描</h3><dl><dt>Path</dt><dd>{draft.row.sourcePath}</dd><dt>Availability</dt><dd>{draft.row.sourceAvailability}</dd><dt>Reconcile</dt><dd>{draft.row.reconcileState}</dd><dt>Last scan</dt><dd>{draft.row.lastScanCompleted || "Never completed"}</dd><dt>Last scan error</dt><dd>{draft.row.lastScanErrorCode || "None"}</dd><dt>Scan revision</dt><dd>{draft.row.scanRevision}</dd></dl><label className="manage-scan-option"><input type="checkbox" checked={excludeNewRootMedia} onChange={(event) => setExcludeNewRootMedia(event.target.checked)} /><span><strong>Auto-exclude newly discovered media in Gallery root</strong><small>Nested media stays included. Existing Restore/Exclude choices are never overwritten.</small></span></label><div className="manage-panel-actions"><button type="button" disabled={scanState.loading} onClick={scan}>{scanState.loading ? "Scanning…" : "Scan source now"}</button><button type="button" onClick={() => navigator.clipboard?.writeText(draft.row.sourcePath)}>Copy path</button></div><h4>最近扫描记录 / Recent scan history</h4>{(draft.scanRuns || []).length ? <div className="manage-table-wrap"><table className="manage-table"><thead><tr><th>Status</th><th>Started</th><th>Completed</th><th>Error</th></tr></thead><tbody>{draft.scanRuns.map((run) => <tr key={run.id} className={run.errorCode ? "has-issue" : ""}><td>{run.status}</td><td>{run.startedAt}</td><td>{run.completedAt || "—"}</td><td>{run.errorCode || "—"}</td></tr>)}</tbody></table></div> : <p>尚无扫描记录 / No scan history yet.</p>}</section> : null}
    {tab === "manifest" ? <section className="manage-panel manage-manifest"><h3>Manifest</h3><p>Manifest is optional. Scans only detect its state; database and file changes are synchronized only by these explicit actions.</p>
      {manifestQuery.loading ? <p>Checking Manifest…</p> : manifestQuery.error ? <p className="manage-error">Manifest inspection failed: {manifestQuery.error.message}</p> : manifestQuery.data ? <><dl><dt>Status</dt><dd><strong className={`manifest-status is-${manifestQuery.data.manageGalleryManifest.status.toLowerCase()}`}>{manifestQuery.data.manageGalleryManifest.status}</strong></dd><dt>Path</dt><dd>{manifestQuery.data.manageGalleryManifest.path || "No source path"}</dd><dt>Manifest revision</dt><dd>{manifestQuery.data.manageGalleryManifest.manifestRevision}</dd><dt>Database revision</dt><dd>{manifestQuery.data.manageGalleryManifest.metadataRevision}</dd><dt>Push member preview</dt><dd>+{manifestQuery.data.manageGalleryManifest.pushAdded} added · −{manifestQuery.data.manageGalleryManifest.pushRemoved} removed · {manifestQuery.data.manageGalleryManifest.pushRetained} retained · {manifestQuery.data.manageGalleryManifest.pushUpdated} updated</dd></dl>
        {manifestQuery.data.manageGalleryManifest.conflicts.length ? <div className="manifest-conflicts"><h4>Conflicts require an explicit decision for every field</h4>{manifestQuery.data.manageGalleryManifest.conflicts.map((conflict) => <article key={conflict.path}><code>{conflict.path}</code><div><label>Database<pre>{conflict.databaseJSON}</pre></label><label>Manifest file<pre>{conflict.fileJSON}</pre></label></div><select aria-label={`Resolution for ${conflict.path}`} value={conflictChoices[conflict.path] || ""} onChange={(event) => setConflictChoices({ ...conflictChoices, [conflict.path]: event.target.value as "DATABASE" | "FILE" })}><option value="">Choose…</option><option value="DATABASE">Keep database</option><option value="FILE">Use Manifest file</option></select></article>)}<button type="button" disabled={resolveManifestState.loading} onClick={resolveManifestConflicts}>{resolveManifestState.loading ? "Resolving…" : "Resolve all and Pull"}</button></div> : null}
      </> : null}
      <div className="manage-panel-actions"><button type="button" onClick={() => manifestQuery.refetch()}>Check again</button><button type="button" disabled={pushManifestState.loading} onClick={() => runManifestAction("PUSH")}>{pushManifestState.loading ? "Pushing…" : "Push database → Manifest"}</button><button type="button" disabled={pullManifestState.loading} onClick={() => runManifestAction("PULL")}>{pullManifestState.loading ? "Pulling…" : "Pull Manifest → database"}</button></div>
    </section> : null}</main>;
}
