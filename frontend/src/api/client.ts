import type { AnalyticsResponse, ListURLsResponse, ShortenedURL } from "../types";

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8050";

let apiKey: string | null = localStorage.getItem("urlshort_api_key");

export function setAPIKey(key: string) {
  apiKey = key;
  localStorage.setItem("urlshort_api_key", key);
}

export function getAPIKey(): string | null {
  return apiKey;
}

export function clearAPIKey() {
  apiKey = null;
  localStorage.removeItem("urlshort_api_key");
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (apiKey) headers["Authorization"] = `Bearer ${apiKey}`;

  const res = await fetch(`${BASE_URL}${path}`, { headers, ...init });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status} ${res.statusText}: ${body}`);
  }
  return res.json() as Promise<T>;
}

export const api = {
  register: (email: string) =>
    request<{ user_id: string; email: string; api_key: string; warning: string }>(
      "/api/v1/register",
      { method: "POST", body: JSON.stringify({ email }) },
    ),
  shorten: (payload: {
    long_url: string;
    custom_slug?: string;
    title?: string;
    expires_in_days?: number;
  }) => request<ShortenedURL>("/api/v1/shorten", { method: "POST", body: JSON.stringify(payload) }),
  listURLs: (page = 1, limit = 20) =>
    request<ListURLsResponse>(`/api/v1/urls?page=${page}&limit=${limit}`),
  getAnalytics: (shortCode: string) =>
    request<AnalyticsResponse>(`/api/v1/urls/${shortCode}/analytics`),
};
