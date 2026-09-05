import { useState } from "react";
import { Register } from "./components/Register";
import { ShortenForm } from "./components/ShortenForm";
import { URLList } from "./components/URLList";
import { AnalyticsView } from "./components/AnalyticsView";
import { getAPIKey, clearAPIKey } from "./api/client";
import "./App.css";

export default function App() {
  const [registered, setRegistered] = useState(!!getAPIKey());
  const [refreshKey, setRefreshKey] = useState(0);
  const [selected, setSelected] = useState<string | null>(null);

  if (!registered) {
    return (
      <div className="app">
        <header>
          <h1>URL Shortener Dashboard</h1>
        </header>
        <main className="grid-single">
          <Register onRegistered={() => setRegistered(true)} />
        </main>
      </div>
    );
  }

  return (
    <div className="app">
      <header>
        <h1>URL Shortener Dashboard</h1>
        <button
          className="logout"
          onClick={() => {
            clearAPIKey();
            setRegistered(false);
          }}
        >
          Log out
        </button>
      </header>
      <main className="grid">
        <ShortenForm onCreated={() => setRefreshKey((k) => k + 1)} />
        <URLList refreshKey={refreshKey} onSelect={setSelected} selected={selected} />
        {selected ? (
          <AnalyticsView shortCode={selected} />
        ) : (
          <div className="panel">
            <h2>Analytics</h2>
            <p>Select a URL from the list to view its analytics.</p>
          </div>
        )}
      </main>
      <footer>
        Geolocation by{" "}
        <a href="https://db-ip.com" target="_blank" rel="noreferrer">
          DB-IP
        </a>
      </footer>
    </div>
  );
}
