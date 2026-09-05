export interface ShortenedURL {
  short_code: string;
  short_url: string;
  long_url: string;
  title?: string;
  created_at: string;
  expires_at?: string;
  clicks_count: number;
  qr_code: string;
}

export interface URLListItem {
  short_code: string;
  short_url: string;
  title?: string;
  clicks: number;
  created_at: string;
}

export interface ListURLsResponse {
  urls: URLListItem[];
  pagination: { page: number; limit: number; total: number; pages: number };
}

export interface TimelinePoint {
  date: string;
  clicks: number;
  unique_visitors: number;
}

export interface BreakdownItem {
  key: string;
  count: number;
  percentage: number;
}

export interface AnalyticsResponse {
  short_code: string;
  total_clicks: number;
  unique_visitors: number;
  timeline: TimelinePoint[];
  by_country: BreakdownItem[];
  by_device: BreakdownItem[];
  by_referrer: BreakdownItem[];
  browsers: BreakdownItem[];
}
