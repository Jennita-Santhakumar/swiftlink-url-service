import { useState } from "react";
import { api, setAPIKey } from "../api/client";

interface Props {
  onRegistered: () => void;
}

export function Register({ onRegistered }: Props) {
  const [email, setEmail] = useState("");
  const [issuedKey, setIssuedKey] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      const res = await api.register(email);
      setAPIKey(res.api_key);
      setIssuedKey(res.api_key);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  if (issuedKey) {
    return (
      <div className="panel">
        <h2>Your API Key</h2>
        <p className="warning">Store this now — it cannot be shown again.</p>
        <pre className="key-box">{issuedKey}</pre>
        <button onClick={onRegistered}>Continue to dashboard</button>
      </div>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="panel">
      <h2>Register</h2>
      <label>
        Email
        <input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
      </label>
      <button type="submit" disabled={submitting}>
        {submitting ? "Registering..." : "Register & Get API Key"}
      </button>
      {error && <p className="error">{error}</p>}
    </form>
  );
}
