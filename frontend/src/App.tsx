import { NavLink, Route, Routes } from "react-router-dom";
import { useData } from "./data";
import { Home } from "./pages/Home";
import { Settings } from "./pages/Settings";
import { Tasks } from "./pages/Tasks";
import { Timers } from "./pages/Timers";
import { Vault } from "./pages/Vault";

export function App({ username, signOut }: { username: string; signOut: () => Promise<void> }) {
  const { error } = useData();
  return (
    <>
      <nav>
        <NavLink to="/" end>
          Focus
        </NavLink>
        <NavLink to="/tasks">Tasks</NavLink>
        <NavLink to="/timers">Timers</NavLink>
        <NavLink to="/vault">Vault</NavLink>
        <NavLink to="/settings" className="gear" aria-label="Settings" title="Settings">
          ⚙
        </NavLink>
      </nav>
      {error && <div className="error" role="alert">{error}</div>}
      <main>
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/tasks" element={<Tasks />} />
          <Route path="/timers" element={<Timers />} />
          <Route path="/vault" element={<Vault />} />
          <Route path="/settings" element={<Settings username={username} signOut={signOut} />} />
        </Routes>
      </main>
    </>
  );
}
