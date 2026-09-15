import { useLazyQuery, useMutation } from "@apollo/client/react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { Link } from "react-router-dom";

import { BROWSE_TAG_OPTIONS, REPLACE_GALLERY_TAGS } from "../api/browse";
import type { EntitySummary } from "./types";

interface TagOption extends EntitySummary {
  aliases: string[];
}

interface GalleryTagEditResult {
  metadataRevision: number;
  tags: EntitySummary[];
}

export function GalleryTagEditor({ setID, metadataRevision, tags, onSaved }: {
  setID: string;
  metadataRevision: number;
  tags: EntitySummary[];
  onSaved: (tags: EntitySummary[], metadataRevision: number) => void;
}) {
  const intl = useIntl();
  const editorRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [editing, setEditing] = useState(false);
  const [selected, setSelected] = useState<EntitySummary[]>(tags);
  const [search, setSearch] = useState("");
  const [optionsOpen, setOptionsOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const [message, setMessage] = useState("");
  const [loadOptions, optionState] = useLazyQuery<{ manageCoreEntityOptions: TagOption[] }>(BROWSE_TAG_OPTIONS, { fetchPolicy: "network-only" });
  const [replaceTags, replaceState] = useMutation<{ replaceGalleryTags: GalleryTagEditResult }>(REPLACE_GALLERY_TAGS);

  useEffect(() => {
    if (!editing) setSelected(tags);
  }, [editing, tags]);

  useEffect(() => {
    if (!editing || !optionsOpen) return;
    const query = search.trim();
    const timer = window.setTimeout(() => void loadOptions({ variables: { query, limit: 20 } }), query ? 180 : 0);
    return () => window.clearTimeout(timer);
  }, [editing, loadOptions, optionsOpen, search]);

  useEffect(() => {
    if (!optionsOpen) return;
    const closeOutside = (event: PointerEvent) => {
      if (!editorRef.current?.contains(event.target as Node)) setOptionsOpen(false);
    };
    document.addEventListener("pointerdown", closeOutside);
    return () => document.removeEventListener("pointerdown", closeOutside);
  }, [optionsOpen]);

  const selectedIDs = useMemo(() => new Set(selected.map((tag) => tag.uuid)), [selected]);
  const options = (optionState.data?.manageCoreEntityOptions || []).filter((tag) => !selectedIDs.has(tag.uuid));
  const dirty = selected.length !== tags.length || selected.some((tag, index) => tag.uuid !== tags[index]?.uuid);

  useEffect(() => setActiveIndex(0), [search, options.length]);

  function beginEdit() {
    setSelected(tags);
    setSearch("");
    setMessage("");
    setEditing(true);
    setOptionsOpen(true);
    window.setTimeout(() => inputRef.current?.focus(), 0);
  }

  function choose(tag: TagOption) {
    if (selectedIDs.has(tag.uuid) || selected.length >= 200) return;
    setSelected([...selected, { uuid: tag.uuid, name: tag.name }]);
    setSearch("");
    setOptionsOpen(true);
    inputRef.current?.focus();
  }

  function cancel() {
    setSelected(tags);
    setSearch("");
    setMessage("");
    setOptionsOpen(false);
    setEditing(false);
  }

  async function save() {
    if (!dirty || replaceState.loading) return;
    setMessage("");
    try {
      const result = await replaceTags({ variables: {
        setID,
        expectedMetadataRevision: metadataRevision,
        tags: selected.map((tag, index) => ({ tagUUID: tag.uuid, position: String((index + 1) * 1024) })),
      } });
      if (!result.data) throw new Error("No Gallery Tag result");
      onSaved(result.data.replaceGalleryTags.tags, result.data.replaceGalleryTags.metadataRevision);
      setSelected(result.data.replaceGalleryTags.tags);
      setEditing(false);
      setOptionsOpen(false);
      setMessage(intl.formatMessage({ id: "gallery.tagsSaved" }));
    } catch (error) {
      const conflict = error instanceof Error && error.message.toLowerCase().includes("metadata revision conflict");
      setMessage(intl.formatMessage({ id: conflict ? "gallery.tagsConflict" : "gallery.tagsFailed" }));
    }
  }

  if (!editing) return <div className="gallery-tag-editor">
    <div className="tag-row">
      {tags.map((tag) => <Link key={tag.uuid} to={`/tag/${encodeURIComponent(tag.uuid)}`}>#{tag.name}</Link>)}
      <button type="button" className="gallery-tag-editor__trigger" onClick={beginEdit}>{intl.formatMessage({ id: "gallery.editTags" })}</button>
    </div>
    {message ? <p className="gallery-tag-editor__message" role="status">{message}</p> : null}
  </div>;

  return <div className="gallery-tag-editor is-editing" ref={editorRef}>
    <div className="gallery-tag-editor__control" onClick={() => inputRef.current?.focus()}>
      {selected.map((tag) => <span className="gallery-tag-editor__chip" key={tag.uuid}>
        <span>{tag.name}</span>
        <button type="button" aria-label={intl.formatMessage({ id: "gallery.removeTag" }, { name: tag.name })} onClick={(event) => { event.stopPropagation(); setSelected(selected.filter((value) => value.uuid !== tag.uuid)); }}>×</button>
      </span>)}
      <input
        ref={inputRef}
        role="combobox"
        aria-label={intl.formatMessage({ id: "gallery.searchTags" })}
        aria-expanded={optionsOpen}
        aria-controls="gallery-tag-options"
        aria-autocomplete="list"
        autoComplete="off"
        value={search}
        placeholder={selected.length ? "" : intl.formatMessage({ id: "gallery.searchTags" })}
        onFocus={() => setOptionsOpen(true)}
        onChange={(event) => { setSearch(event.target.value); setOptionsOpen(true); }}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" && options.length) { event.preventDefault(); setOptionsOpen(true); setActiveIndex((value) => Math.min(value + 1, options.length - 1)); }
          if (event.key === "ArrowUp" && options.length) { event.preventDefault(); setActiveIndex((value) => Math.max(value - 1, 0)); }
          if (event.key === "Enter" && optionsOpen && options[activeIndex]) { event.preventDefault(); choose(options[activeIndex]); }
          if (event.key === "Escape") { event.preventDefault(); setOptionsOpen(false); }
          if (event.key === "Backspace" && !search && selected.length) setSelected(selected.slice(0, -1));
        }}
      />
    </div>
    {optionsOpen ? <div className="gallery-tag-editor__options" id="gallery-tag-options" role="listbox">
      {optionState.loading ? <p>{intl.formatMessage({ id: "gallery.searchingTags" })}</p> : options.length ? options.map((tag, index) => <button
        type="button"
        role="option"
        aria-selected={index === activeIndex}
        className={index === activeIndex ? "is-active" : ""}
        key={tag.uuid}
        onPointerMove={() => setActiveIndex(index)}
        onMouseDown={(event) => event.preventDefault()}
        onClick={() => choose(tag)}
      ><strong>{tag.name}</strong>{search && tag.aliases.length ? <small>{tag.aliases.join(" / ")}</small> : null}</button>) : <p>{intl.formatMessage({ id: "gallery.noTagMatches" })}</p>}
    </div> : null}
    <div className="gallery-tag-editor__actions">
      <span>{selected.length}/200</span>
      <button type="button" onClick={cancel}>{intl.formatMessage({ id: "gallery.cancelTags" })}</button>
      <button type="button" disabled={!dirty || replaceState.loading} onClick={() => void save()}>{intl.formatMessage({ id: replaceState.loading ? "gallery.savingTags" : "gallery.saveTags" })}</button>
    </div>
    {message ? <p className="gallery-tag-editor__message" role="alert">{message}</p> : null}
  </div>;
}
