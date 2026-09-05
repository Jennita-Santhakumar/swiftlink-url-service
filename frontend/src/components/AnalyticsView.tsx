import { useEffect, useState } from "react";
import {
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { api } from "../api/client";
import type { AnalyticsResponse, BreakdownItem } from "../types";

interface Props {
  shortCode: string;
}

function BreakdownList({ title, items }: { title: string; items: BreakdownItem[] }) {
  return (
    <div className="breakdown">
      <h4>{title}</h4>
      {items.length === 0 ? (
        <p className="muted">No data yet</p>
      ) : (
        <ul>
          {items.map((item) => (
            <li key={item.key}>
              <span>{item.key || "unknown"}</span>
              <span>
                {item.count} ({item.percentage.toFixed(1)}%)
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function AnalyticsView({ shortCode }: Props) {
  const [data, setData] = useState<AnalyticsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setData(null);
    api
      .getAnalytics(shortCode)
      .then((res) => {
        if (!cancelled) setData(res);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [shortCode]);

  if (error) return <div className="panel error">Failed to load analytics: {error}</div>;
  if (!data) return <div className="panel">Loading analytics...</div>;

  return (
    <div className="panel">
      <h2>Analytics: {shortCode}</h2>
      <div className="stat-tiles">
        <div className="stat-tile">
          <div className="stat-value">{data.total_clicks}</div>
          <div className="stat-label">Total Clicks</div>
        </div>
        <div className="stat-tile">
          <div className="stat-value">{data.unique_visitors}</div>
          <div className="stat-label">Unique Visitors</div>
        </div>
      </div>

      {data.timeline.length > 0 && (
        <div className="chart">
          <ResponsiveContainer width="100%" height={200}>
            <LineChart data={data.timeline}>
              <XAxis dataKey="date" tick={{ fontSize: 11 }} />
              <YAxis tick={{ fontSize: 11 }} />
              <Tooltip />
              <Line type="monotone" dataKey="clicks" stroke="#4c7cf3" strokeWidth={2} />
              <Line type="monotone" dataKey="unique_visitors" stroke="#4ade80" strokeWidth={2} />
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}

      <div className="breakdown-grid">
        <BreakdownList title="By Country" items={data.by_country} />
        <BreakdownList title="By Device" items={data.by_device} />
        <BreakdownList title="By Referrer" items={data.by_referrer} />
        <BreakdownList title="Browsers" items={data.browsers} />
      </div>
    </div>
  );
}
