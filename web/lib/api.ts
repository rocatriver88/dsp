import type { components } from './api-types';

// Types from generated OpenAPI spec (Required<> ensures fields are non-optional, matching runtime behavior)
export type Advertiser = Required<components['schemas']['github_com_heartgryphon_dsp_internal_campaign.Advertiser']>;
export type Campaign = Required<components['schemas']['github_com_heartgryphon_dsp_internal_campaign.Campaign']>;
export type Creative = Required<components['schemas']['github_com_heartgryphon_dsp_internal_campaign.Creative']>;

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8181";

// API Key: read from env or localStorage (set on first visit via key input UI)
function getAPIKey(): string {
  if (typeof window !== "undefined") {
    return localStorage.getItem("dsp_api_key") || "";
  }
  return process.env.NEXT_PUBLIC_API_KEY || "";
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const apiKey = getAPIKey();
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(apiKey ? { "X-API-Key": apiKey } : {}),
      ...options?.headers,
    },
  });
  if (!res.ok) {
    // 401: clear invalid API key and reload to show login screen
    if (res.status === 401 && typeof window !== "undefined") {
      localStorage.removeItem("dsp_api_key");
      window.location.reload();
      throw new Error("Authentication failed");
    }
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `API error: ${res.status}`);
  }
  return res.json();
}

export interface CampaignStats {
  campaign_id: number;
  impressions: number;
  clicks: number;
  conversions: number;
  wins: number;
  bids: number;
  spend_cents: number;
  adx_cost_cents: number;
  profit_cents: number;
  ctr: number;
  win_rate: number;
  cvr: number;
  cpa: number;
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

  // Overview (backend scopes by API key)
  getOverviewStats: () =>
    request<{
      today_spend_cents: number;
      today_impressions: number;
      today_clicks: number;
      ctr: number;
      balance_cents: number;
    }>(
      `/api/v1/reports/overview`
    ),

  // Campaigns (backend scopes by API key)
  listCampaigns: () =>
    request<Campaign[]>(`/api/v1/campaigns`),

  getCampaign: (id: number) =>
    request<Campaign>(`/api/v1/campaigns/${id}`),

  createCampaign: (data: {
    advertiser_id: number;
    name: string;
    billing_model?: string;
    budget_total_cents: number;
    budget_daily_cents: number;
    bid_cpm_cents?: number;
    bid_cpc_cents?: number;
    ocpm_target_cpa_cents?: number;
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

  // Upload
  uploadFile: async (file: File): Promise<{ url: string; filename: string }> => {
    const apiKey = getAPIKey();
    const formData = new FormData();
    formData.append("file", file);
    const res = await fetch(`${API_BASE}/api/v1/upload`, {
      method: "POST",
      headers: { ...(apiKey ? { "X-API-Key": apiKey } : {}) },
      body: formData,
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.error || `Upload failed: ${res.status}`);
    }
    return res.json();
  },

  // Creatives
  listCreatives: (campaignId: number) =>
    request<Creative[]>(`/api/v1/campaigns/${campaignId}/creatives`),

  createCreative: (data: {
    campaign_id: number;
    name: string;
    ad_type?: string;
    format?: string;
    size?: string;
    ad_markup?: string;
    destination_url: string;
    native_title?: string;
    native_desc?: string;
    native_icon_url?: string;
    native_image_url?: string;
    native_cta?: string;
  }) =>
    request<{ id: number }>("/api/v1/creatives", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateCreative: (id: number, data: {
    name?: string; ad_type?: string; format?: string; size?: string;
    ad_markup?: string; destination_url?: string;
  }) =>
    request<{ status: string }>(`/api/v1/creatives/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteCreative: (id: number) =>
    request<{ status: string }>(`/api/v1/creatives/${id}`, { method: "DELETE" }),

  // Bid Simulator
  simulateBid: (campaignId: number, bidCPMCents: number) =>
    request<{
      current_bid_cpm_cents: number;
      simulated_bid_cpm_cents: number;
      total_bids: number;
      actual_wins: number;
      current_win_rate: number;
      simulated_wins: number;
      simulated_win_rate: number;
      simulated_spend_cents: number;
      median_clear_price_cents: number;
      max_clear_price_cents: number;
      data_days: number;
    }>(`/api/v1/reports/campaign/${campaignId}/simulate?bid_cpm_cents=${bidCPMCents}`),

  // Billing — self-service routes always act on the authenticated advertiser.
  // The backend resolves the advertiser from the API key (auth context); the
  // client must not pass an advertiser id, and passing a foreign one on the
  // write path now fails with 400 instead of silently redirecting.
  getBalance: () =>
    request<{ advertiser_id: number; balance_cents: number; billing_type: string }>(
      `/api/v1/billing/balance`
    ),

  getTransactions: (limit = 50, offset = 0) =>
    request<Array<{
      id: number; type: string; amount_cents: number;
      balance_after: number; description: string; created_at: string;
    }>>(`/api/v1/billing/transactions?limit=${limit}&offset=${offset}`),

  topUp: (amountCents: number, description?: string) =>
    request<{ id: number; balance_after: number }>("/api/v1/billing/topup", {
      method: "POST",
      body: JSON.stringify({ amount_cents: amountCents, description: description || "" }),
    }),
};
