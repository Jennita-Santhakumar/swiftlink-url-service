import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { URLListItem } from "../types";

interface Props {
  refreshKey: number;
  onSelect: (shortCode: string) => void;
  selected: string | null;
}

export function URLList({ refreshKey, onSelect, selected }: Props) {
  const [urls, setURLs] = useState<URLListItem[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    api
      .listURLs()
      .then((res) => {
        if (!cancelled) setURLs(res.urls);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [refreshKey]);

  if (error) return <div className="panel error">Failed to load URLs: {error}</div>;

  return (
    <div className="panel">
      <h2>Your URLs</h2>
      {urls.length === 0 ? (
        <p>No URLs yet — shorten one above.</p>
      ) : (
        <ul className="url-list">
          {urls.map((u) => (
            <li
              key={u.short_code}
              className={selected === u.short_code ? "selected" : ""}
              onClick={() => onSelect(u.short_code)}
            >
              <span className="short-code">{u.short_code}</span>
              <span className="clicks">{u.clicks} clicks</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
