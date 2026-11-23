import { useState } from "react";
import * as keys from "../crypto";
import { useData } from "../data";
import { currentTheme, setTheme, THEMES } from "../settings";

function Appearance() {
  const [theme, set] = useState(currentTheme);
  return (
    <section>
      <h2>Theme</h2>
      <div className="themes">
        {THEMES.map((t) => (
          <button
            key={t.id}
            className={`theme ${theme === t.id ? "on" : ""}`}
            aria-pressed={theme === t.id}
            onClick={() => {
              setTheme(t.id);
              set(t.id);
            }}
          >
            <span className="swatch" style={{ background: t.swatch[0], borderColor: t.swatch[1] }}>
              <span style={{ background: t.swatch[1] }} />
            </span>
            {t.label}
          </button>
        ))}
      </div>
    </section>
  );
}

function Security() {
  const { refresh } = useData();
  const [, rerender] = useState(0);
  const [reveal, setReveal] = useState(false);
  const [paste, setPaste] = useState("");
  const [problem, setProblem] = useState<string | null>(null);
  const update = async () => {
    rerender((n) => n + 1);
    await refresh();
  };

  if (!keys.available())
    return (
      <section>
        <h2>Encryption</h2>
        <p className="bad-text">Encryption needs a secure context (https or localhost), which this page isn’t.</p>
      </section>
    );

  const use = async (text: string) => {
    try {
      await keys.useKeyText(text);
      setPaste("");
      setProblem(null);
      await update();
    } catch (e) {
      setProblem(e instanceof Error && e.message.startsWith("A key") ? e.message : "That isn’t a valid key");
    }
  };
  const keyText = keys.getKeyText();

  return (
    <section>
      <h2>Encryption</h2>
      <p className="muted">
        Task titles, descriptions and timer names are encrypted in this browser before they are sent, so the
        server only stores unreadable text. Older entries stay as they were and are still shown normally.
      </p>
      {keyText ? (
        <div className="stack">
          <label className="check">
            <input
              type="checkbox"
              checked={keys.isEnabled()}
              onChange={(e) => {
                keys.setEnabled(e.target.checked);
                update();
              }}
            />
            Encrypt new and edited text
          </label>
          <div>
            <button onClick={() => setReveal(!reveal)}>{reveal ? "Hide key" : "Show key"}</button>{" "}
            <button onClick={() => navigator.clipboard?.writeText(keyText)}>Copy key</button>{" "}
            <button
              className="danger"
              onClick={() => {
                if (confirm("Remove the key from this browser? Encrypted entries will stay unreadable until you add it again.")) {
                  keys.removeKey();
                  update();
                }
              }}
            >
              Remove key
            </button>
          </div>
          {reveal && <code className="key">{keyText}</code>}
          <p className="muted">
            The key lives only in this browser. Save a copy somewhere safe and paste it on your other devices:
            if it is lost, encrypted entries cannot be recovered.
          </p>
        </div>
      ) : (
        <div className="stack">
          <div>
            <button className="primary" onClick={() => use(keys.generateKeyText())}>
              Generate a key
            </button>
          </div>
          <label>
            <span className="muted">…or use a key from another device</span>
            <span className="row">
              <input value={paste} onChange={(e) => setPaste(e.target.value)} placeholder="Paste key" />
              <button disabled={!paste.trim()} onClick={() => use(paste)}>
                Use key
              </button>
            </span>
          </label>
        </div>
      )}
      {problem && <p className="bad-text">{problem}</p>}
    </section>
  );
}

export function Settings({ username, signOut }: { username: string; signOut: () => Promise<void> }) {
  const [signingOut, setSigningOut] = useState(false);
  const [problem, setProblem] = useState("");
  const leave = async () => {
    setSigningOut(true);
    setProblem("");
    try {
      await signOut();
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "Could not sign out");
      setSigningOut(false);
    }
  };
  return (
    <div className="settings">
      <h1>Settings</h1>
      <Appearance />
      <Security />
      <section>
        <h2>Account</h2>
        <div className="row account-row">
          <span>Signed in as <strong>{username}</strong></span>
          <button disabled={signingOut} onClick={leave}>Sign out</button>
        </div>
        {problem && <p className="bad-text" role="alert">{problem}</p>}
      </section>
    </div>
  );
}
