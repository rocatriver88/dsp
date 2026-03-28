const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8181";

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `API error: ${res.status}`);
  }
  return res.json();
}

export interface Advertiser {
  id: number;
  company_name: string;
  contact_email: string;
  api_key: string;
  balance_cents: number;
  billing_type: string;
}

export interface Campaign {
  id: number;
  advertiser_id: number;
  name: string;
  status: "draft" | "active" | "paused" | "completed";
  budget_total_cents: number;
  budget_daily_cents: number;
  spent_cents: number;
  billing_model: string;
  bid_cpm_cents: number;
  bid_cpc_cents: number;
  ocpm_target_cpa_cents: number;
  start_date: string | null;
  end_date: string | null;
  targeting: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface Creative {
  id: number;
  campaign_id: number;
  name: string;
  format: string;
  size: string;
  ad_markup: string;
  destination_url: string;
  status: string;
}

export interface CampaignStats {
  campaign_id: number;
  impressions: number;
  clicks: number;
  wins: number;
  bids: number;
  spend_cents: number;
  ctr: number;
  win_rate: number;
}

export interface HourlyStats {
  hour: number;
  impressions: number;
  clicks: number;
  spend_cents: number;
}

export interface GeoStats {
  country: string;
  impressions: number;
  clicks: number;
  spend_cents: number;
}

export interface BidDetail {
  time: string;
  request_id: string;
  event_type: string;
  bid_price_cents: number;
  clear_price_cents: number;
  geo_country: string;
  device_os: string;
  loss_reason: string;
}

export const api = {
  // Advertisers
  createAdvertiser: (data: { company_name: string; contact_email: string; balance_cents: number }) =>
    request<{ id: number; api_key: string }>("/api/v1/advertisers", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  getAdvertiser: (id: number) =>
    request<Advertiser>(`/api/v1/advertisers/${id}`),

  // Overview
  getOverviewStats: (advertiserId: number) =>
    request<{ today_spend_cents: number; today_impressions: number; today_clicks: number }>(
      `/api/v1/reports/overview?advertiser_id=${advertiserId}`
    ),

  // Campaigns
  listCampaigns: (advertiserId: number) =>
    request<Campaign[]>(`/api/v1/campaigns?advertiser_id=${advertiserId}`),

  getCampaign: (id: number) =>
    request<Campaign>(`/api/v1/campaigns/${id}`),

  createCampaign: (data: {
    advertiser_id: number;
    name: string;
    budget_total_cents: number;
    budget_daily_cents: number;
    bid_cpm_cents: number;
    targeting?: Record<string, unknown>;
  }) =>
    request<{ id: number }>("/api/v1/campaigns", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateCampaign: (id: number, data: {
    name: string;
    bid_cpm_cents: number;
    budget_daily_cents: number;
    targeting: Record<string, unknown>;
  }) =>
    request<{ status: string }>(`/api/v1/campaigns/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  startCampaign: (id: number) =>
    request<{ status: string }>(`/api/v1/campaigns/${id}/start`, { method: "POST" }),

  pauseCampaign: (id: number) =>
    request<{ status: string }>(`/api/v1/campaigns/${id}/pause`, { method: "POST" }),

  // Reports (Phase 2)
  getCampaignStats: (id: number, from?: string, to?: string) => {
    const params = new URLSearchParams();
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    return request<CampaignStats>(`/api/v1/reports/campaign/${id}/stats?${params}`);
  },

  getHourlyStats: (id: number, date?: string) => {
    const params = date ? `?date=${date}` : "";
    return request<HourlyStats[]>(`/api/v1/reports/campaign/${id}/hourly${params}`);
  },

  getGeoBreakdown: (id: number, from?: string, to?: string) => {
    const params = new URLSearchParams();
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    return request<GeoStats[]>(`/api/v1/reports/campaign/${id}/geo?${params}`);
  },

  getBidTransparency: (id: number, from?: string, to?: string, limit = 50, offset = 0) => {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    return request<BidDetail[]>(`/api/v1/reports/campaign/${id}/bids?${params}`);
  },

  // Creatives
  createCreative: (data: {
    campaign_id: number;
    name: string;
    format: string;
    size: string;
    ad_markup: string;
    destination_url: string;
  }) =>
    request<{ id: number }>("/api/v1/creatives", {
      method: "POST",
      body: JSON.stringify(data),
    }),
};
