import { useState } from "react";
import { api } from "../api/client";
import type { ShortenedURL } from "../types";

interface Props {
  onCreated: () => void;
}

export function ShortenForm({ onCreated }: Props) {
  const [longURL, setLongURL] = useState("");
  const [customSlug, setCustomSlug] = useState("");
  const [title, setTitle] = useState("");
  const [result, setResult] = useState<ShortenedURL | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      const res = await api.shorten({
        long_url: longURL,
        custom_slug: customSlug || undefined,
        title: title || undefined,
      });
      setResult(res);
      onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="panel">
      <h2>Shorten a URL</h2>
      <form onSubmit={handleSubmit}>
        <label>
          Long URL
          <input
            type="url"
            required
            placeholder="https://example.com/very/long/path"
            value={longURL}
            onChange={(e) => setLongURL(e.target.value)}
          />
        </label>
        <label>
          Custom slug (optional)
          <input value={customSlug} onChange={(e) => setCustomSlug(e.target.value)} />
        </label>
        <label>
          Title (optional)
          <input value={title} onChange={(e) => setTitle(e.target.value)} />
        </label>
        <button type="submit" disabled={submitting}>
          {submitting ? "Shortening..." : "Shorten"}
        </button>
        {error && <p className="error">{error}</p>}
      </form>

      {result && (
        <div className="result">
          <p>
            <a href={result.short_url} target="_blank" rel="noreferrer">
              {result.short_url}
            </a>
          </p>
          <img src={result.qr_code} alt="QR code" width={128} height={128} />
        </div>
      )}
    </div>
  );
}
