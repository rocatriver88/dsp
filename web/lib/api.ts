import type { components } from './api-types';

// Types from generated OpenAPI spec (Required<> ensures fields are non-optional, matching runtime behavior)
export type Advertiser = Required<components['schemas']['internal_handler.AdvertiserResponse']>;
export type Campaign = Required<components['schemas']['github_com_heartgryphon_dsp_internal_campaign.Campaign']>;
export type Creative = Required<components['schemas']['github_com_heartgryphon_dsp_internal_campaign.Creative']>;
export type CampaignStats = Required<components['schemas']['github_com_heartgryphon_dsp_internal_reporting.CampaignStats']>;
export type HourlyStats = Required<components['schemas']['github_com_heartgryphon_dsp_internal_reporting.HourlyStats']>;
export type GeoStats = Required<components['schemas']['github_com_heartgryphon_dsp_internal_reporting.GeoStats']>;
export type BidDetail = Required<components['schemas']['github_com_heartgryphon_dsp_internal_reporting.BidDetail']>;
export type BidSimulation = Required<components['schemas']['github_com_heartgryphon_dsp_internal_reporting.BidSimulation']>;
export type Transaction = Required<components['schemas']['github_com_heartgryphon_dsp_internal_billing.Transaction']>;

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8181";

// --- Token management ---

// Access + refresh tokens stored in localStorage for persistence across tabs/reloads.
// Exported so admin-api.ts can share the same token store.
let _accessToken: string | null = null;
let _refreshToken: string | null = null;

if (typeof window !== "undefined") {
  _accessToken = localStorage.getItem("dsp_access_token");
  _refreshToken = localStorage.getItem("dsp_refresh_token");
}

export function getAccessToken(): string | null {
  return _accessToken;
}

export function getRefreshToken(): string | null {
  return _refreshToken;
}

function setTokens(access: string, refresh: string) {
  _accessToken = access;
  _refreshToken = refresh;
  if (typeof window !== "undefined") {
    localStorage.setItem("dsp_access_token", access);
    localStorage.setItem("dsp_refresh_token", refresh);
  }
}

function clearTokens() {
  _accessToken = null;
  _refreshToken = null;
  if (typeof window !== "undefined") {
    localStorage.removeItem("dsp_access_token");
    localStorage.removeItem("dsp_refresh_token");
  }
}

/** Login with email + password. Stores access + refresh tokens on success. */
export async function login(email: string, password: string): Promise<{
  access_token: string;
  refresh_token: string;
  user: { id: number; email: string; name: string; role: string; advertiser_id: number | null };
}> {
  const res = await fetch(`${API_BASE}/api/v1/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `Login failed: ${res.status}`);
  }
  const data = await res.json();
  setTokens(data.access_token, data.refresh_token);
  return data;
}

/** Clear tokens and redirect to the login page. */
export function logout() {
  clearTokens();
  // Also clear legacy API key if present
  if (typeof window !== "undefined") {
    localStorage.removeItem("dsp_api_key");
    window.location.href = "/";
  }
}

/** Attempt to refresh the access token using the stored refresh token. Returns true on success. */
export async function refreshAccessToken(): Promise<boolean> {
  if (!_refreshToken) return false;
  try {
    const res = await fetch(`${API_BASE}/api/v1/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: _refreshToken }),
    });
    if (!res.ok) return false;
    const data = await res.json();
    _accessToken = data.access_token;
    if (typeof window !== "undefined") {
      localStorage.setItem("dsp_access_token", data.access_token);
    }
    return true;
  } catch {
    return false;
  }
}

/** Returns auth headers: Bearer token if available, API Key fallback for backward compat. */
export function getAuthHeaders(): Record<string, string> {
  if (_accessToken) {
    return { Authorization: `Bearer ${_accessToken}` };
  }
  // Legacy API Key fallback for programmatic/migration compatibility
  const apiKey = getAPIKey();
  if (apiKey) {
    return { "X-API-Key": apiKey };
  }
  return {};
}

// API Key: read from env or localStorage (legacy support)
function getAPIKey(): string {
  if (typeof window !== "undefined") {
    return localStorage.getItem("dsp_api_key") || "";
  }
  return process.env.NEXT_PUBLIC_API_KEY || "";
}

// Flag to prevent concurrent refresh attempts
let _refreshing: Promise<boolean> | null = null;

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const authHeaders = getAuthHeaders();
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...authHeaders,
      ...options?.headers,
    },
  });
  if (!res.ok) {
    // 401: try token refresh if we have a refresh token
    if (res.status === 401 && typeof window !== "undefined") {
      if (_refreshToken) {
        // Deduplicate concurrent refresh attempts
        if (!_refreshing) {
          _refreshing = refreshAccessToken().finally(() => { _refreshing = null; });
        }
        const refreshed = await _refreshing;
        if (refreshed) {
          // Retry the original request with the new access token
          const retryHeaders = getAuthHeaders();
          const retryRes = await fetch(`${API_BASE}${path}`, {
            ...options,
            headers: {
              "Content-Type": "application/json",
              ...retryHeaders,
              ...options?.headers,
            },
          });
          if (retryRes.ok) {
            return retryRes.json();
          }
          // Retry also failed — fall through to logout
        }
        // Refresh failed — logout
        logout();
        throw new Error("Authentication failed");
      }
      // No refresh token — clear legacy API key and reload
      localStorage.removeItem("dsp_api_key");
      window.location.reload();
      throw new Error("Authentication failed");
    }
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `API error: ${res.status}`);
  }
  return res.json();
}

/** Check if user is currently authenticated (has access token or API key). */
export function isAuthenticated(): boolean {
  return !!_accessToken || !!getAPIKey();
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
    const authHeaders = getAuthHeaders();
    const formData = new FormData();
    formData.append("file", file);
    const res = await fetch(`${API_BASE}/api/v1/upload`, {
      method: "POST",
      headers: { ...authHeaders },
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
    request<BidSimulation>(`/api/v1/reports/campaign/${campaignId}/simulate?bid_cpm_cents=${bidCPMCents}`),

  // Billing — self-service routes always act on the authenticated advertiser.
  // The backend resolves the advertiser from the API key (auth context); the
  // client must not pass an advertiser id, and passing a foreign one on the
  // write path now fails with 400 instead of silently redirecting.
  getBalance: () =>
    request<{ advertiser_id: number; balance_cents: number; billing_type: string }>(
      `/api/v1/billing/balance`
    ),

  getTransactions: (limit = 50, offset = 0) =>
    request<Transaction[]>(`/api/v1/billing/transactions?limit=${limit}&offset=${offset}`),

  topUp: (amountCents: number, description?: string) =>
    request<{ id: number; balance_after: number }>("/api/v1/billing/topup", {
      method: "POST",
      body: JSON.stringify({ amount_cents: amountCents, description: description || "" }),
    }),

  // Auth
  getMe: () =>
    request<{
      id: number;
      email: string;
      name: string;
      role: string;
      advertiser_id: number | null;
      status: string;
      last_login_at: string | null;
      created_at: string;
    }>("/api/v1/auth/me"),
};
