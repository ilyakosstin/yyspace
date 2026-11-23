import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import Markdown from "react-markdown";
import { useBeforeUnload, useBlocker } from "react-router-dom";
import { vault } from "../vault";
import type { DocumentInput, DocumentSummary, Folder, VaultDocument } from "../vault";
import "./Vault.css";

const leaveMessage = "Discard unsaved changes to this document?";

function draftFrom(document: VaultDocument): DocumentInput {
  return { title: document.title, folder_id: document.folder_id, markdown: document.markdown };
}

function message(error: unknown) {
  return error instanceof Error ? error.message : "Could not reach the vault";
}

export function Vault() {
  const [folder, setFolder] = useState("");
  const [folders, setFolders] = useState<Folder[]>([]);
  const [documents, setDocuments] = useState<DocumentSummary[]>([]);
  const [newFolder, setNewFolder] = useState<string | null>(null);
  const [loaded, setLoaded] = useState<VaultDocument | null>(null);
  const [draft, setDraft] = useState<DocumentInput | null>(null);
  const [preview, setPreview] = useState(false);
  const [listing, setListing] = useState(true);
  const [busy, setBusy] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const [listError, setListError] = useState("");
  const [editorError, setEditorError] = useState("");
  const [notice, setNotice] = useState("");

  const dirty = draft !== null && (loaded
    ? draft.title !== loaded.title || draft.folder_id !== loaded.folder_id || draft.markdown !== loaded.markdown
    : Boolean(draft.title || draft.markdown));

  const blocker = useBlocker(dirty || busy);
  useEffect(() => {
    if (blocker.state !== "blocked") return;
    if (busy || !window.confirm(leaveMessage)) blocker.reset();
    else blocker.proceed();
  }, [blocker, busy]);
  useBeforeUnload((event) => {
    if (dirty || busy) {
      event.preventDefault();
      event.returnValue = "";
    }
  });

  useEffect(() => {
    let active = true;
    vault.list(folder).then(
      (result) => {
        if (!active) return;
        setFolders(result.folders);
        setDocuments(result.documents);
        setListing(false);
      },
      (error: unknown) => {
        if (!active) return;
        setListError(message(error));
        setListing(false);
      },
    );
    return () => { active = false; };
  }, [folder, refresh]);

  function selectFolder(id: string) {
    if (id === folder) return;
    setFolder(id);
    setDocuments([]);
    setListing(true);
    setListError("");
  }

  function edit(document: VaultDocument | null) {
    setLoaded(document);
    setDraft(document ? draftFrom(document) : { title: "", folder_id: folder || null, markdown: "" });
    setPreview(false);
    setEditorError("");
    setNotice("");
  }

  function create() {
    if (!dirty || window.confirm(leaveMessage)) edit(null);
  }

  async function open(id: string) {
    if (loaded?.id === id || (dirty && !window.confirm(leaveMessage))) return;
    setBusy(true);
    setEditorError("");
    try {
      edit(await vault.read(id));
    } catch (error) {
      setEditorError(message(error));
    } finally {
      setBusy(false);
    }
  }

  async function addFolder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!newFolder?.trim()) return;
    setBusy(true);
    setListError("");
    try {
      const created = await vault.createFolder(newFolder);
      setFolders((previous) => [...previous, created]);
      setNewFolder(null);
      selectFolder(created.id);
    } catch (error) {
      setListError(message(error));
    } finally {
      setBusy(false);
    }
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!draft?.title.trim()) return;
    setBusy(true);
    setEditorError("");
    setNotice("");
    try {
      const saved = loaded ? await vault.save(loaded.id, loaded.revision, draft) : await vault.create(draft);
      setLoaded(saved);
      setDraft(draftFrom(saved));
      setNotice(saved.merged ? "Saved with concurrent changes merged" : "Saved");
      selectFolder(saved.folder_id ?? "");
      setRefresh((value) => value + 1);
    } catch (error) {
      setEditorError(message(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="vault" aria-labelledby="vault-title">
      <header className="vault-header">
        <h1 id="vault-title">Document vault</h1>
        <button type="button" className="primary" disabled={busy} onClick={create}>New document</button>
      </header>
      <div className="vault-layout">
        <aside className="vault-browser" aria-label="Browse the vault">
          <div className="vault-section-title">
            <h2>Folders</h2>
            {newFolder === null && (
              <button type="button" disabled={busy} onClick={() => setNewFolder("")}>New folder</button>
            )}
          </div>
          {newFolder !== null && (
            <form className="vault-folder-form" onSubmit={addFolder}>
              <input autoFocus aria-label="Folder name" required maxLength={160} disabled={busy}
                placeholder="Folder name" value={newFolder} onChange={(event) => setNewFolder(event.target.value)} />
              <div>
                <button className="primary" disabled={busy || !newFolder.trim()}>Create folder</button>
                <button type="button" disabled={busy} onClick={() => { setNewFolder(null); setListError(""); }}>Cancel</button>
              </div>
            </form>
          )}
          <ul className="vault-folders">
            <li><button type="button" disabled={busy} aria-pressed={folder === ""} onClick={() => selectFolder("")}>Root</button></li>
            {folders.map((entry) => (
              <li key={entry.id}>
                <button type="button" disabled={busy} aria-pressed={folder === entry.id} onClick={() => selectFolder(entry.id)}>
                  {entry.name}
                </button>
              </li>
            ))}
          </ul>
          <h2>Documents</h2>
          {listError && <p role="alert" className="bad-text">{listError}</p>}
          {listing ? <p role="status">Loading documents…</p>
            : documents.length ? (
              <ul className="vault-files">
                {documents.map((entry) => (
                  <li key={entry.id}>
                    <button type="button" disabled={busy} aria-current={loaded?.id === entry.id}
                      onClick={() => { void open(entry.id); }}>
                      {entry.title}
                    </button>
                  </li>
                ))}
              </ul>
            ) : !listError && <p className="muted">This folder is empty.</p>}
        </aside>

        <div className="vault-workspace">
          {editorError && <p className="bad-text" role="alert">{editorError}</p>}
          {!draft ? <p className="vault-empty">Open a document or create one to start writing.</p> : (
            <form className="vault-form" onSubmit={save}>
              <fieldset disabled={busy}>
                <div className="vault-meta">
                  <label>
                    Title
                    <input key={loaded?.id ?? "new"} required maxLength={160} value={draft.title}
                      onChange={(event) => setDraft({ ...draft, title: event.target.value })} />
                  </label>
                  <label>
                    Folder
                    <select value={draft.folder_id ?? ""} onChange={(event) => setDraft({ ...draft, folder_id: event.target.value || null })}>
                      <option value="">Root</option>
                      {folders.map((entry) => <option key={entry.id} value={entry.id}>{entry.name}</option>)}
                    </select>
                  </label>
                </div>
                <div className="vault-section-title">
                  <h2>Markdown</h2>
                  <div className="seg vault-modes" aria-label="Markdown view">
                    <button type="button" aria-pressed={!preview} onClick={() => setPreview(false)}>Edit</button>
                    <button type="button" aria-pressed={preview} onClick={() => setPreview(true)}>Preview</button>
                  </div>
                </div>
                {preview ? (
                  <div className="vault-preview desc" aria-label="Markdown preview">
                    {draft.markdown ? <Markdown>{draft.markdown}</Markdown> : <p className="muted">Nothing to preview yet.</p>}
                  </div>
                ) : (
                  <textarea aria-label="Markdown" value={draft.markdown} spellCheck={false}
                    onChange={(event) => { setDraft({ ...draft, markdown: event.target.value }); setNotice(""); }} />
                )}
                <div className="vault-save">
                  <button className="primary" disabled={!draft.title.trim() || (loaded !== null && !dirty)}>
                    Save document
                  </button>
                  <span role="status">{dirty ? "Unsaved changes" : notice}</span>
                </div>
              </fieldset>
            </form>
          )}
        </div>
      </div>
    </section>
  );
}
