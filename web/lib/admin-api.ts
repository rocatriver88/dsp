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

export interface AdminAdvertiser {
  id: number;
  company_name: string;
  contact_email: string;
  api_key: string;
  balance_cents: number;
  billing_type: string;
  status: string;
  created_at: string;
}

export interface InviteCode {
  id: number;
  code: string;
  used: boolean;
  used_by?: number;
  created_at: string;
  expires_at?: string;
}

export interface AdminCreative {
  id: number;
  campaign_id: number;
  advertiser_id: number;
  name: string;
  ad_type: string;
  format: string;
  size: string;
  ad_markup: string;
  destination_url: string;
  status: string;
  created_at: string;
}

export interface CircuitStatus {
  name: string;
  state: "closed" | "open" | "half-open";
  failures: number;
  last_failure?: string;
}

export interface AuditEntry {
  id: number;
  admin_user: string;
  action: string;
  target_type: string;
  target_id: number;
  details: Record<string, unknown>;
  created_at: string;
}

export interface Registration {
  id: number;
  company_name: string;
  contact_email: string;
  invite_code: string;
  status: "pending" | "approved" | "rejected";
  created_at: string;
}

export const adminApi = {
  // Advertisers
  listAdvertisers: () =>
    adminRequest<AdminAdvertiser[]>("/api/admin/advertisers"),

  topUp: (advertiserId: number, amountCents: number, description?: string) =>
    adminRequest<{ id: number; balance_after: number }>("/api/admin/billing/topup", {
      method: "POST",
      body: JSON.stringify({ advertiser_id: advertiserId, amount_cents: amountCents, description: description || "" }),
    }),

  // Registrations
  listRegistrations: (status?: string) => {
    const params = status ? `?status=${status}` : "";
    return adminRequest<Registration[]>(`/api/admin/registrations${params}`);
  },

  approveRegistration: (id: number) =>
    adminRequest<{ status: string }>(`/api/admin/registrations/${id}/approve`, { method: "POST" }),

  rejectRegistration: (id: number, reason?: string) =>
    adminRequest<{ status: string }>(`/api/admin/registrations/${id}/reject`, {
      method: "POST",
      body: JSON.stringify({ reason: reason || "" }),
    }),

  // Invite codes
  listInviteCodes: () =>
    adminRequest<InviteCode[]>("/api/admin/invite-codes"),

  createInviteCode: (expiresAt?: string) =>
    adminRequest<InviteCode>("/api/admin/invite-codes", {
      method: "POST",
      body: JSON.stringify({ expires_at: expiresAt }),
    }),

  // Creative review
  listCreativesForReview: () =>
    adminRequest<AdminCreative[]>("/api/admin/creatives/pending"),

  approveCreative: (id: number) =>
    adminRequest<{ status: string }>(`/api/admin/creatives/${id}/approve`, { method: "POST" }),

  rejectCreative: (id: number, reason?: string) =>
    adminRequest<{ status: string }>(`/api/admin/creatives/${id}/reject`, {
      method: "POST",
      body: JSON.stringify({ reason: reason || "" }),
    }),

  // Circuit breaker
  getCircuitStatus: () =>
    adminRequest<CircuitStatus[]>("/api/admin/circuit-breakers"),

  tripCircuitBreaker: (name: string) =>
    adminRequest<{ status: string }>(`/api/admin/circuit-breakers/${name}/trip`, { method: "POST" }),

  resetCircuitBreaker: (name: string) =>
    adminRequest<{ status: string }>(`/api/admin/circuit-breakers/${name}/reset`, { method: "POST" }),

  // Health
  getHealth: () =>
    adminRequest<{ status: string; checks: Record<string, string> }>("/api/admin/health"),

  // Audit log
  getAuditLog: (limit = 50, offset = 0) =>
    adminRequest<AuditEntry[]>(`/api/admin/audit?limit=${limit}&offset=${offset}`),
};
