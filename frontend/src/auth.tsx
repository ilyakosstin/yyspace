import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { call, sessionExpired } from "./api";
import { App } from "./App";
import { DataProvider } from "./data";
import "./auth.css";

type Session = { username: string | null };

function SignIn({ onSignedIn }: { onSignedIn: (username: string) => void }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [visible, setVisible] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const session = await call<{ username: string }>("POST", "/session", { username, password });
      onSignedIn(session.username);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not sign in");
      setPending(false);
    }
  }

  return (
    <main className="sign-in">
      <section className="sign-in-card" aria-labelledby="sign-in-title">
        <header>
          <div className="sign-in-brand">
            <span className="focus-mark" aria-hidden="true" />
            Focus Helper
          </div>
          <h1 id="sign-in-title">Sign in</h1>
          <p>Open your tasks, timers, and documents.</p>
        </header>
        <form onSubmit={submit} className="sign-in-form">
          <label>
            Username
            <input autoComplete="username" autoCapitalize="none" spellCheck={false} required
              value={username} onChange={(event) => setUsername(event.target.value)} />
          </label>
          <label htmlFor="sign-in-password">Password</label>
          <div className="password-field">
            <input id="sign-in-password" type={visible ? "text" : "password"} required
              autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} />
            <button type="button" aria-pressed={visible} onClick={() => setVisible(!visible)}>
              {visible ? "Hide" : "Show"}
            </button>
          </div>
          {error && <p className="bad-text" role="alert">{error}</p>}
          <button className="primary" type="submit" disabled={pending}>
            {pending ? "Signing in…" : "Sign in"}
          </button>
        </form>
      </section>
    </main>
  );
}

export function AuthGate() {
  const [user, setUser] = useState<string | null>();
  const [problem, setProblem] = useState("");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let active = true;
    const expire = () => setUser(null);
    window.addEventListener(sessionExpired, expire);
    call<Session>("GET", "/session").then(
      (session) => { if (active) setUser(session.username); },
      (error: Error) => { if (active) setProblem(error.message); },
    );
    return () => {
      active = false;
      window.removeEventListener(sessionExpired, expire);
    };
  }, [attempt]);

  if (user === null) return <SignIn onSignedIn={setUser} />;
  if (user === undefined) return (
    <main className="sign-in">
      <div className="sign-in-card">
        {problem ? (
          <>
            <p role="alert">{problem}</p>
            <button onClick={() => { setProblem(""); setAttempt((current) => current + 1); }}>Retry</button>
          </>
        ) : <p role="status">Loading Focus Helper…</p>}
      </div>
    </main>
  );

  const signOut = async () => {
    await call<void>("DELETE", "/session");
    setUser(null);
  };

  return (
    <DataProvider>
      <App username={user} signOut={signOut} />
    </DataProvider>
  );
}
