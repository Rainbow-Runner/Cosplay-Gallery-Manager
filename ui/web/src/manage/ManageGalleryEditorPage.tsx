import { useMutation, useQuery } from "@apollo/client/react";
import { type FormEvent, useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { ADD_GALLERY_EXTERNAL_LINK, MANAGE_GALLERY, MANAGE_GALLERY_MANIFEST, MOVE_GALLERY_ITEM, PULL_GALLERY_MANIFEST, PUSH_GALLERY_MANIFEST, REPLACE_GALLERY_RELATIONS, RESET_GALLERY_COVER, RESOLVE_GALLERY_MANIFEST, SCAN_GALLERY_SOURCE, SET_GALLERY_COVER_ITEM, SET_GALLERY_ITEM_EXCLUDED, SET_GALLERY_STATE, UPDATE_GALLERY_ITEM, UPDATE_GALLERY_METADATA } from "../api/manage";
import { ManageEntitySelector } from "./ManageEntitySelector";
import type { ManageGalleryCredit, ManageGalleryDetail, ManageGalleryItem, ManageGalleryManifestState, ManageGalleryTag } from "./types";

const tabs = ["basic", "cast", "media", "source", "manifest"] as const;
export function ManageGalleryEditorPage() {
  const { setID = "" } = useParams(); const [parameters, setParameters] = useSearchParams(); const tab = tabs.includes(parameters.get("tab") as typeof tabs[number]) ? parameters.get("tab") as typeof tabs[number] : "basic";
  const query = useQuery<{ manageGallery: ManageGalleryDetail }>(MANAGE_GALLERY, { variables: { setID } }); const [save] = useMutation(UPDATE_GALLERY_METADATA); const [setState] = useMutation(SET_GALLERY_STATE);
  const [updateItem] = useMutation<{ updateGalleryItem: ManageGalleryDetail }>(UPDATE_GALLERY_ITEM);
  const [setExcluded] = useMutation<{ setGalleryItemExcluded: ManageGalleryDetail }>(SET_GALLERY_ITEM_EXCLUDED);
  const [moveItem] = useMutation<{ moveGalleryItem: ManageGalleryDetail }>(MOVE_GALLERY_ITEM);
  const [setCover] = useMutation<{ setGalleryCoverItem: ManageGalleryDetail }>(SET_GALLERY_COVER_ITEM);
  const [resetCover] = useMutation<{ resetGalleryCover: ManageGalleryDetail }>(RESET_GALLERY_COVER);
  const [scanSource, scanState] = useMutation<{ scanGallerySource: ManageGalleryDetail }>(SCAN_GALLERY_SOURCE);
  const [replaceRelations, relationState] = useMutation<{ replaceGalleryRelations: ManageGalleryDetail }>(REPLACE_GALLERY_RELATIONS);
  const [addExternalLink, externalLinkState] = useMutation<{ addGalleryExternalLink: ManageGalleryDetail }>(ADD_GALLERY_EXTERNAL_LINK);
  const manifestQuery = useQuery<{ manageGalleryManifest: ManageGalleryManifestState }>(MANAGE_GALLERY_MANIFEST, { variables: { setID }, skip: tab !== "manifest", fetchPolicy: "network-only" });
  const [pushManifest, pushManifestState] = useMutation<{ pushGalleryManifest: ManageGalleryManifestState }>(PUSH_GALLERY_MANIFEST);
  const [pullManifest, pullManifestState] = useMutation<{ pullGalleryManifest: ManageGalleryManifestState }>(PULL_GALLERY_MANIFEST);
  const [resolveManifest, resolveManifestState] = useMutation<{ resolveGalleryManifest: ManageGalleryManifestState }>(RESOLVE_GALLERY_MANIFEST);
  const [draft, setDraft] = useState<ManageGalleryDetail | null>(null); const [message, setMessage] = useState("");
  const [linkDraft, setLinkDraft] = useState({ type: "SOURCE", label: "", url: "" });
  const [conflictChoices, setConflictChoices] = useState<Record<string, "DATABASE" | "FILE">>({});
  useEffect(() => { if (query.data) setDraft(query.data.manageGallery); }, [query.data]);
  if (!draft) return <main className="manage-page"><p>{query.error ? "Unable to load Gallery." : "Loading…"}</p></main>;
  const current = draft;
  async function submit(event: FormEvent) { event.preventDefault(); setMessage(""); try { await save({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision, input: {
    title: current.row.title, aliases: current.aliases, description: current.description, shootDate: current.shootDate, shootDatePrecision: current.shootDate ? current.shootDatePrecision : "UNKNOWN",
    contentRating: current.row.contentRating || "NON_ADULT", photographerName: current.photographerName, studioName: current.studioName } } }); setMessage("Saved"); await query.refetch(); } catch { setMessage("Revision conflict or invalid metadata"); } }
  async function transition(state: "DRAFT" | "ACTIVE" | "ARCHIVED") { setMessage(""); try { await setState({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision, state } }); await query.refetch(); } catch (error) { setMessage(error instanceof Error ? error.message : "State transition blocked"); } }
  function editItem(itemUUID: string, patch: Partial<ManageGalleryItem>) { setDraft({ ...current, items: current.items.map((item) => item.uuid === itemUUID ? { ...item, ...patch } : item) }); }
  async function saveItem(item: ManageGalleryItem) { setMessage(""); try { const result = await updateItem({ variables: { setID, itemUUID: item.uuid, expectedMetadataRevision: current.row.metadataRevision, input: { imageCategory: item.mediaKind === "STATIC_IMAGE" ? item.imageCategory || "PHOTO" : null, caption: item.caption } } }); if (result.data) setDraft(result.data.updateGalleryItem); } catch (error) { setMessage(error instanceof Error ? error.message : "Item update failed"); } }
  async function toggleExcluded(item: ManageGalleryItem) { setMessage(""); try { const result = await setExcluded({ variables: { setID, itemUUID: item.uuid, excluded: !item.excluded, expectedMetadataRevision: current.row.metadataRevision } }); if (result.data) setDraft(result.data.setGalleryItemExcluded); } catch (error) { setMessage(error instanceof Error ? error.message : "Exclusion update failed"); } }
  async function chooseCover(item: ManageGalleryItem) { setMessage(""); try { const result = await setCover({ variables: { setID, itemUUID: item.uuid, expectedMetadataRevision: current.row.metadataRevision } }); if (result.data) setDraft(result.data.setGalleryCoverItem); setMessage("Cover updated"); } catch (error) { setMessage(error instanceof Error ? error.message : "Cover update failed"); } }
  async function autoCover() { setMessage(""); try { const result = await resetCover({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision } }); if (result.data) setDraft(result.data.resetGalleryCover); setMessage("Cover reset to automatic selection"); } catch (error) { setMessage(error instanceof Error ? error.message : "Cover reset failed"); } }
  async function scan() { setMessage(""); try { const result = await scanSource({ variables: { setID } }); if (result.data) setDraft(result.data.scanGallerySource); setMessage("Source scan completed"); } catch (error) { setMessage(error instanceof Error ? error.message : "Source scan failed"); } }
  function editCredit(index: number, patch: Partial<ManageGalleryCredit>) { setDraft({ ...current, credits: current.credits.map((credit, position) => position === index ? { ...credit, ...patch } : credit) }); }
  function editTag(index: number, patch: Partial<ManageGalleryTag>) { setDraft({ ...current, tags: current.tags.map((tag, position) => position === index ? { ...tag, ...patch } : tag) }); }
  async function saveRelations() {
    setMessage("");
    try {
      const result = await replaceRelations({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision, input: {
        credits: current.credits.map((credit, index) => ({ coserUUID: credit.coserUUID, position: String((index + 1) * 1024), cast: credit.cast.map((cast, castIndex) => ({ characterUUID: cast.characterUUID, position: String((castIndex + 1) * 1024) })) })),
        tags: current.tags.map((tag, index) => ({ tagUUID: tag.uuid, position: String((index + 1) * 1024) })),
      } } });
      if (result.data) setDraft(result.data.replaceGalleryRelations);
      setMessage("People, characters and tags saved");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Relation save failed"); }
  }
  async function createExternalLink(event: FormEvent) {
    event.preventDefault(); setMessage("");
    try {
      const result = await addExternalLink({ variables: { setID, expectedMetadataRevision: current.row.metadataRevision, input: { ...linkDraft, position: String((current.externalLinks.length + 1) * 1024) } } });
      if (result.data) setDraft(result.data.addGalleryExternalLink);
      setLinkDraft({ type: "SOURCE", label: "", url: "" }); setMessage("External link added");
    } catch (error) { setMessage(error instanceof Error ? error.message : "External link failed"); }
  }
  async function runManifestAction(action: "PUSH" | "PULL") {
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
  function group(item: ManageGalleryItem) { return item.mediaKind === "STATIC_IMAGE" ? `STATIC_IMAGE:${item.imageCategory || "PHOTO"}` : item.mediaKind; }
  async function move(item: ManageGalleryItem, direction: -1 | 1) { const peers = current.items.filter((value) => group(value) === group(item)); const index = peers.findIndex((value) => value.uuid === item.uuid); if (index < 0 || index + direction < 0 || index + direction >= peers.length) return; const before = direction < 0 ? peers[index - 1].uuid : peers[index + 2]?.uuid ?? null; setMessage(""); try { const result = await moveItem({ variables: { setID, itemUUID: item.uuid, beforeItemUUID: before, expectedMetadataRevision: current.row.metadataRevision } }); if (result.data) setDraft(result.data.moveGalleryItem); } catch (error) { setMessage(error instanceof Error ? error.message : "Item ordering failed"); } }
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
        <form className="manage-inline-form" onSubmit={createExternalLink}><select aria-label="External link type" value={linkDraft.type} onChange={(event) => setLinkDraft({ ...linkDraft, type: event.target.value })}><option value="SOURCE">SOURCE</option><option value="PROFILE">PROFILE</option><option value="REFERENCE">REFERENCE</option></select><input aria-label="External link label" placeholder="Label" maxLength={100} value={linkDraft.label} onChange={(event) => setLinkDraft({ ...linkDraft, label: event.target.value })} /><input aria-label="External link URL" type="url" required placeholder="https://…" value={linkDraft.url} onChange={(event) => setLinkDraft({ ...linkDraft, url: event.target.value })} /><button disabled={externalLinkState.loading} type="submit">Add link</button></form>
      </section></> : null}
    {tab === "cast" ? <section className="manage-panel manage-relations"><header><div><h3>人物、角色与标签</h3><p>一次显式保存整个关系集合。空 Cast 自动归类为 Album；任一 Cast 存在时归类为 Cosplay。</p></div><strong>{draft.credits.some((credit) => credit.cast.length > 0) ? "COSPLAY" : "ALBUM"}</strong></header>
      <div className="manage-relation-block"><div className="manage-inline-toolbar"><h4>Coser credits</h4><button type="button" onClick={() => setDraft({ ...draft, credits: [...draft.credits, { coserUUID: "", coserName: "", position: "", cast: [] }] })}>Add Coser</button></div>
        {draft.credits.length ? draft.credits.map((credit, creditIndex) => <article className="manage-credit" key={`${credit.coserUUID}-${creditIndex}`}><div className="manage-credit__head"><ManageEntitySelector kind="COSER" label="Coser" uuid={credit.coserUUID} name={credit.coserName} onSelect={(entity) => editCredit(creditIndex, { coserUUID: entity.uuid, coserName: entity.name })} /><button type="button" onClick={() => setDraft({ ...draft, credits: draft.credits.filter((_, index) => index !== creditIndex) })}>Remove</button></div>
          <div className="manage-cast-list"><h5>Characters played by this Coser</h5>{credit.cast.map((cast, castIndex) => <div className="manage-cast-row" key={`${cast.characterUUID}-${castIndex}`}><ManageEntitySelector kind="CHARACTER" label="Character" uuid={cast.characterUUID} name={cast.workName ? `${cast.workName} · ${cast.characterName}` : cast.characterName} onSelect={(entity) => editCredit(creditIndex, { cast: credit.cast.map((value, index) => index === castIndex ? { ...value, characterUUID: entity.uuid, characterName: entity.name, workUUID: entity.workUUID || "", workName: "" } : value) })} /><button type="button" onClick={() => editCredit(creditIndex, { cast: credit.cast.filter((_, index) => index !== castIndex) })}>Remove</button></div>)}<button type="button" onClick={() => editCredit(creditIndex, { cast: [...credit.cast, { characterUUID: "", characterName: "", workUUID: "", workName: "", position: "" }] })}>Add Character</button></div>
        </article>) : <p>No Coser credits. This Gallery is currently an Album.</p>}
      </div>
      <div className="manage-relation-block"><div className="manage-inline-toolbar"><h4>Direct tags</h4><button type="button" onClick={() => setDraft({ ...draft, tags: [...draft.tags, { uuid: "", name: "", position: "" }] })}>Add Tag</button></div>{draft.tags.map((tag, tagIndex) => <div className="manage-tag-row" key={`${tag.uuid}-${tagIndex}`}><ManageEntitySelector kind="TAG" label="Tag" uuid={tag.uuid} name={tag.name} onSelect={(entity) => editTag(tagIndex, { uuid: entity.uuid, name: entity.name })} /><button type="button" onClick={() => setDraft({ ...draft, tags: draft.tags.filter((_, index) => index !== tagIndex) })}>Remove</button></div>)}</div>
      <div className="manage-panel-actions"><button type="button" disabled={relationState.loading} onClick={saveRelations}>{relationState.loading ? "Saving…" : "Save all relations"}</button></div>
    </section> : null}
    {tab === "media" ? <section><div className="manage-inline-toolbar"><span>{draft.items.length} members</span><button type="button" onClick={autoCover}>Reset cover to auto</button></div><div className="manage-table-wrap"><table className="manage-table manage-media-table"><thead><tr><th>Order</th><th>Path / Caption</th><th>Type</th><th>Category</th><th>Availability</th><th>Processing</th><th>Actions</th></tr></thead><tbody>{draft.items.map((item) => <tr key={item.uuid} className={item.excluded ? "is-excluded" : ""}><td><button type="button" aria-label="Move up" onClick={() => move(item, -1)}>↑</button><button type="button" aria-label="Move down" onClick={() => move(item, 1)}>↓</button></td><td><strong>{item.relativePath}</strong><input aria-label={`Caption for ${item.relativePath}`} value={item.caption} maxLength={1000} placeholder="Caption" onChange={(event) => editItem(item.uuid, { caption: event.target.value })} /></td><td>{item.mediaKind}<small>{item.contentFormat}</small></td><td>{item.mediaKind === "STATIC_IMAGE" ? <select value={item.imageCategory || "PHOTO"} onChange={(event) => editItem(item.uuid, { imageCategory: event.target.value as "PHOTO" | "SELFIE" })}><option value="PHOTO">PHOTO</option><option value="SELFIE">SELFIE</option></select> : "—"}</td><td>{item.availability}</td><td>{item.processingState}</td><td><button type="button" onClick={() => saveItem(item)}>Save</button><button type="button" onClick={() => toggleExcluded(item)}>{item.excluded ? "Restore" : "Exclude"}</button>{item.mediaKind === "STATIC_IMAGE" ? <button type="button" onClick={() => chooseCover(item)}>Cover</button> : null}</td></tr>)}</tbody></table></div></section> : null}
    {tab === "source" ? <section className="manage-panel"><h3>来源与扫描</h3><dl><dt>Path</dt><dd>{draft.row.sourcePath}</dd><dt>Availability</dt><dd>{draft.row.sourceAvailability}</dd><dt>Reconcile</dt><dd>{draft.row.reconcileState}</dd><dt>Scan revision</dt><dd>{draft.row.scanRevision}</dd></dl><div className="manage-panel-actions"><button type="button" disabled={scanState.loading} onClick={scan}>{scanState.loading ? "Scanning…" : "Scan source now"}</button><button type="button" onClick={() => navigator.clipboard?.writeText(draft.row.sourcePath)}>Copy path</button></div></section> : null}
    {tab === "manifest" ? <section className="manage-panel manage-manifest"><h3>Manifest</h3><p>Manifest is optional. Scans only detect its state; database and file changes are synchronized only by these explicit actions.</p>
      {manifestQuery.loading ? <p>Checking Manifest…</p> : manifestQuery.error ? <p className="manage-error">Manifest inspection failed: {manifestQuery.error.message}</p> : manifestQuery.data ? <><dl><dt>Status</dt><dd><strong className={`manifest-status is-${manifestQuery.data.manageGalleryManifest.status.toLowerCase()}`}>{manifestQuery.data.manageGalleryManifest.status}</strong></dd><dt>Path</dt><dd>{manifestQuery.data.manageGalleryManifest.path || "No source path"}</dd><dt>Manifest revision</dt><dd>{manifestQuery.data.manageGalleryManifest.manifestRevision}</dd><dt>Database revision</dt><dd>{manifestQuery.data.manageGalleryManifest.metadataRevision}</dd></dl>
        {manifestQuery.data.manageGalleryManifest.conflicts.length ? <div className="manifest-conflicts"><h4>Conflicts require an explicit decision for every field</h4>{manifestQuery.data.manageGalleryManifest.conflicts.map((conflict) => <article key={conflict.path}><code>{conflict.path}</code><div><label>Database<pre>{conflict.databaseJSON}</pre></label><label>Manifest file<pre>{conflict.fileJSON}</pre></label></div><select aria-label={`Resolution for ${conflict.path}`} value={conflictChoices[conflict.path] || ""} onChange={(event) => setConflictChoices({ ...conflictChoices, [conflict.path]: event.target.value as "DATABASE" | "FILE" })}><option value="">Choose…</option><option value="DATABASE">Keep database</option><option value="FILE">Use Manifest file</option></select></article>)}<button type="button" disabled={resolveManifestState.loading} onClick={resolveManifestConflicts}>{resolveManifestState.loading ? "Resolving…" : "Resolve all and Pull"}</button></div> : null}
      </> : null}
      <div className="manage-panel-actions"><button type="button" onClick={() => manifestQuery.refetch()}>Check again</button><button type="button" disabled={pushManifestState.loading} onClick={() => runManifestAction("PUSH")}>{pushManifestState.loading ? "Pushing…" : "Push database → Manifest"}</button><button type="button" disabled={pullManifestState.loading} onClick={() => runManifestAction("PULL")}>{pullManifestState.loading ? "Pulling…" : "Pull Manifest → database"}</button></div>
    </section> : null}</main>;
}
