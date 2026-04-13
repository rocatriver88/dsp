import type { components } from './api-types';

// Types from generated OpenAPI spec (Required<> ensures fields are non-optional, matching runtime behavior)
export type AdminAdvertiser = Required<components['schemas']['github_com_heartgryphon_dsp_internal_campaign.Advertiser']>;
export type InviteCode = Required<components['schemas']['github_com_heartgryphon_dsp_internal_registration.InviteCode']>;
export type AdminCreative = Required<components['schemas']['github_com_heartgryphon_dsp_internal_campaign.Creative']>;
export type AuditEntry = Required<components['schemas']['github_com_heartgryphon_dsp_internal_audit.Entry']>;
export type Registration = Required<components['schemas']['github_com_heartgryphon_dsp_internal_registration.Request']>;

const ADMIN_API_BASE = process.env.NEXT_PUBLIC_ADMIN_API_URL || "http://localhost:8182";

function getAdminToken(): string {
  if (typeof window !== "undefined") {
    return localStorage.getItem("dsp_admin_token") || "";
  }
  return "";
}

async function adminRequest<T>(path: string, options?: RequestInit): Promise<T> {
  const token = getAdminToken();
  const res = await fetch(`${ADMIN_API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { "X-Admin-Token": token } : {}),
      ...options?.headers,
    },
  });
  if (!res.ok) {
    // 401: clear invalid token and reload to show login screen
    if (res.status === 401 && typeof window !== "undefined") {
      localStorage.removeItem("dsp_admin_token");
      window.location.reload();
      throw new Error("Authentication failed");
    }
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `API error: ${res.status}`);
  }
  return res.json();
}

export interface CircuitStatus {
  circuit_breaker: string;
  reason: string;
  global_spend_today_cents: number;
}

export interface SystemHealth {
  status: string;
  active_campaigns: number;
  pending_registrations: number;
  redis: string;
  clickhouse: string;
}

export const adminApi = {
  // Advertisers
  listAdvertisers: () =>
    adminRequest<AdminAdvertiser[]>("/api/v1/admin/advertisers"),

  topUp: (advertiserId: number, amountCents: number, description?: string) =>
    adminRequest<{ id: number; balance_after: number }>("/api/v1/admin/topup", {
      method: "POST",
      body: JSON.stringify({ advertiser_id: advertiserId, amount_cents: amountCents, description: description || "" }),
    }),

  // Registrations
  listRegistrations: (status?: string) => {
    const params = status ? `?status=${status}` : "";
    return adminRequest<Registration[]>(`/api/v1/admin/registrations${params}`);
  },

  approveRegistration: (id: number) =>
    adminRequest<{ status: string }>(`/api/v1/admin/registrations/${id}/approve`, { method: "POST" }),

  rejectRegistration: (id: number, reason?: string) =>
    adminRequest<{ status: string }>(`/api/v1/admin/registrations/${id}/reject`, {
      method: "POST",
      body: JSON.stringify({ reason: reason || "" }),
    }),

  // Invite codes
  listInviteCodes: () =>
    adminRequest<InviteCode[]>("/api/v1/admin/invite-codes"),

  createInviteCode: (maxUses: number = 1) =>
    adminRequest<{ code: string }>("/api/v1/admin/invite-codes", {
      method: "POST",
      body: JSON.stringify({ max_uses: maxUses }),
    }),

  // Creative review
  listCreativesForReview: () =>
    adminRequest<AdminCreative[]>("/api/v1/admin/creatives"),

  approveCreative: (id: number) =>
    adminRequest<{ status: string }>(`/api/v1/admin/creatives/${id}/approve`, { method: "POST" }),

  rejectCreative: (id: number, reason?: string) =>
    adminRequest<{ status: string }>(`/api/v1/admin/creatives/${id}/reject`, {
      method: "POST",
      body: JSON.stringify({ reason: reason || "" }),
    }),

  // Circuit breaker
  getCircuitStatus: () =>
    adminRequest<CircuitStatus>("/api/v1/admin/circuit-status"),

  tripCircuitBreaker: (reason: string) =>
    adminRequest<{ status: string }>("/api/v1/admin/circuit-break", {
      method: "POST",
      body: JSON.stringify({ reason }),
    }),

  resetCircuitBreaker: () =>
    adminRequest<{ status: string }>("/api/v1/admin/circuit-reset", { method: "POST" }),

  // Health
  getHealth: () =>
    adminRequest<SystemHealth>("/api/v1/admin/health"),

  // Audit log
  getAuditLog: (limit = 50, offset = 0) =>
    adminRequest<AuditEntry[]>(`/api/v1/admin/audit-log?limit=${limit}&offset=${offset}`),
};
