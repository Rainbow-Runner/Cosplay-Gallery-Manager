import { useEffect, useMemo, useState } from "react";
import { useIntl } from "react-intl";

import type { ManageCoreEntity } from "./types";

interface Provider { key: string; label: string }
interface Candidate { ref: string; display_name: string; source_url: string; match_quality: number }
interface SuggestedAccount { platform_key: string; label: string; handle: string; url: string }
interface Preview {
  token: string; provider_key: string; display_name: string; source_url: string; has_avatar: boolean; has_banner: boolean;
  accounts: SuggestedAccount[]; expires_at: string;
}

async function metadataRequest<T>(path: string, body?: unknown): Promise<T> {
  const response = await fetch(`/manage/coser-metadata/${path}`, body === undefined ? { credentials: "same-origin" } : {
    method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
  });
  if (!response.ok) throw new Error((await response.text()).trim() || "Coser metadata request failed");
  return response.json() as Promise<T>;
}

export function ManageCoserMetadataImport({ coser, onUpdated }: { coser: ManageCoreEntity; onUpdated: () => Promise<void> }) {
  const intl = useIntl(); const t = (id: string) => intl.formatMessage({ id });
  const [providers, setProviders] = useState<Provider[]>([]); const [loadingProviders, setLoadingProviders] = useState(true);
  const [providerKey, setProviderKey] = useState(""); const [query, setQuery] = useState(coser.name);
  const [candidates, setCandidates] = useState<Candidate[]>([]); const [preview, setPreview] = useState<Preview | null>(null);
  const [selectedAccounts, setSelectedAccounts] = useState<string[]>([]); const [importAvatar, setImportAvatar] = useState(false);
  const [replaceAvatar, setReplaceAvatar] = useState(false); const [importBanner, setImportBanner] = useState(false);
  const [replaceBanner, setReplaceBanner] = useState(false); const [busy, setBusy] = useState<"search" | "prepare" | "apply" | null>(null);
  const [message, setMessage] = useState("");

  useEffect(() => {
    let active = true;
    metadataRequest<{ providers: Provider[] }>("providers").then((value) => {
      if (!active) return; setProviders(value.providers); setProviderKey(value.providers[0]?.key || "");
    }).catch(() => { if (active) setProviders([]); }).finally(() => { if (active) setLoadingProviders(false); });
    return () => { active = false; };
  }, []);
  useEffect(() => { setQuery(coser.name); setCandidates([]); setPreview(null); }, [coser.uuid, coser.name]);
  const existingURLs = useMemo(() => new Set(coser.socialAccounts.map((account) => account.url)), [coser.socialAccounts]);

  async function search() {
    if (!providerKey || !query.trim()) return; setBusy("search"); setMessage(""); setPreview(null);
    try { const result = await metadataRequest<{ candidates: Candidate[] }>("search", { provider_key: providerKey, query }); setCandidates(result.candidates); }
    catch (error) { setMessage(error instanceof Error ? error.message : t("manage.coserMetadata.failed")); }
    finally { setBusy(null); }
  }
  async function prepare(candidate: Candidate) {
    setBusy("prepare"); setMessage("");
    try {
      const value = await metadataRequest<Preview>("prepare", { provider_key: providerKey, coser_uuid: coser.uuid, candidate_ref: candidate.ref });
      setPreview(value); setImportAvatar(value.has_avatar && !coser.avatarURL); setReplaceAvatar(false);
      setImportBanner(value.has_banner && !coser.bannerURL); setReplaceBanner(false);
      setSelectedAccounts(value.accounts.filter((account) => !existingURLs.has(account.url)).map((account) => account.url));
    } catch (error) { setMessage(error instanceof Error ? error.message : t("manage.coserMetadata.failed")); }
    finally { setBusy(null); }
  }
  async function apply() {
    if (!preview) return; setBusy("apply"); setMessage("");
    try {
      await metadataRequest("apply", { token: preview.token, expected_metadata_revision: coser.metadataRevision,
        import_avatar: importAvatar, replace_avatar: replaceAvatar, import_banner: importBanner, replace_banner: replaceBanner,
        account_urls: selectedAccounts });
      await onUpdated(); setPreview(null); setCandidates([]); setMessage(t("manage.coserMetadata.saved"));
    } catch (error) { setMessage(error instanceof Error ? error.message : t("manage.coserMetadata.failed")); }
    finally { setBusy(null); }
  }

  if (!loadingProviders && providers.length === 0) return null;
  return <section className="manage-panel coser-metadata-import">
    <h3>{t("manage.coserMetadata.heading")}</h3><p>{t("manage.coserMetadata.summary")}</p>
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    <div className="manage-inline-toolbar">
      <label>{t("manage.coserMetadata.provider")}<select value={providerKey} disabled={loadingProviders || busy !== null} onChange={(event) => setProviderKey(event.target.value)}>{providers.map((provider) => <option key={provider.key} value={provider.key}>{provider.label}</option>)}</select></label>
      <label>{t("manage.coserMetadata.name")}<input maxLength={300} value={query} onChange={(event) => setQuery(event.target.value)} /></label>
      <button type="button" disabled={!providerKey || !query.trim() || busy !== null} onClick={search}>{busy === "search" ? t("manage.coserMetadata.searching") : t("manage.coserMetadata.search")}</button>
    </div>
    {candidates.length ? <div className="coser-metadata-candidates">{candidates.map((candidate) => <button type="button" key={candidate.ref} disabled={busy !== null} onClick={() => prepare(candidate)}><strong>{candidate.display_name}</strong><span>{t("manage.coserMetadata.match")} {candidate.match_quality}%</span></button>)}</div> : null}
    {preview ? <div className="coser-metadata-preview"><header><div><strong>{preview.display_name}</strong><a href={preview.source_url} target="_blank" rel="noreferrer">{t("manage.coserMetadata.source")}</a></div><small>{t("manage.coserMetadata.review")}</small></header>
      <div className="coser-metadata-assets">
        {preview.has_avatar ? <label><img className="coser-metadata-avatar" src={`/manage/coser-metadata/preview/${preview.token}/avatar`} alt="" /><span><input type="checkbox" checked={importAvatar} onChange={(event) => setImportAvatar(event.target.checked)} /> {t("manage.coserMetadata.avatar")}</span>{coser.avatarURL && importAvatar ? <span><input type="checkbox" checked={replaceAvatar} onChange={(event) => setReplaceAvatar(event.target.checked)} /> {t("manage.coserMetadata.replace")}</span> : null}</label> : null}
        {preview.has_banner ? <label><img className="coser-metadata-banner" src={`/manage/coser-metadata/preview/${preview.token}/banner`} alt="" /><span><input type="checkbox" checked={importBanner} onChange={(event) => setImportBanner(event.target.checked)} /> {t("manage.coserMetadata.banner")}</span>{coser.bannerURL && importBanner ? <span><input type="checkbox" checked={replaceBanner} onChange={(event) => setReplaceBanner(event.target.checked)} /> {t("manage.coserMetadata.replace")}</span> : null}</label> : null}
      </div>
      <div className="coser-metadata-accounts">{preview.accounts.map((account) => { const exists = existingURLs.has(account.url); return <label key={account.url}><input type="checkbox" disabled={exists} checked={!exists && selectedAccounts.includes(account.url)} onChange={(event) => setSelectedAccounts(event.target.checked ? [...selectedAccounts, account.url] : selectedAccounts.filter((value) => value !== account.url))} /><span><strong>{account.label || account.platform_key}</strong>{account.handle ? ` · ${account.handle}` : ""}{exists ? ` · ${t("manage.coserMetadata.exists")}` : ""}</span></label>; })}</div>
      <button type="button" disabled={busy !== null || (!importAvatar && !importBanner && selectedAccounts.length === 0) || (Boolean(coser.avatarURL) && importAvatar && !replaceAvatar) || (Boolean(coser.bannerURL) && importBanner && !replaceBanner)} onClick={apply}>{busy === "apply" ? t("manage.coserMetadata.applying") : t("manage.coserMetadata.apply")}</button>
    </div> : null}
  </section>;
}
