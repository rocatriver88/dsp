"use client";

import { useState, useSyncExternalStore } from "react";
import { usePathname } from "next/navigation";

const STORAGE_KEY = "dsp_api_key";
const listeners = new Set<() => void>();

function subscribeApiKey(cb: () => void) {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

function getApiKeySnapshot(): string {
  return localStorage.getItem(STORAGE_KEY) ?? "";
}

// Returning null on the server distinguishes "not hydrated yet" from "no key
// saved" (""), which lets the component render nothing during SSR and avoid a
// hydration mismatch while still showing the login screen once mounted.
function getApiKeyServerSnapshot(): string | null {
  return null;
}

function saveApiKey(value: string) {
  localStorage.setItem(STORAGE_KEY, value);
  listeners.forEach((cb) => cb());
}

export default function ApiKeyGate({ children, sidebar }: { children: React.ReactNode; sidebar?: React.ReactNode }) {
  const apiKey = useSyncExternalStore(subscribeApiKey, getApiKeySnapshot, getApiKeyServerSnapshot);
  const [input, setInput] = useState("");
  const pathname = usePathname();
  const isAdmin = pathname.startsWith("/admin");

  if (apiKey === null) return null;

  // Admin routes bypass the API key gate entirely — AdminTokenGate in
  // /admin/layout.tsx handles admin auth independently.  This check MUST
  // come before the !apiKey login screen, otherwise admin-only users
  // (who have an admin token but no tenant api_key) get stuck on the
  // tenant login page and can never reach /admin/*.
  if (isAdmin) {
    return <>{children}</>;
  }

  if (!apiKey) {
    return (
      <div className="min-h-screen w-full flex items-center justify-center"
        style={{ background: "var(--bg-page)" }}>
        <div className="p-8 w-full max-w-md"
          style={{ background: "var(--bg-card)", border: "1px solid var(--border)", borderRadius: 14 }}>
          <div className="flex items-center gap-3 mb-2">
            <div className="w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0"
              style={{ background: "linear-gradient(135deg, #8B5CF6, #6D28D9)" }}>
              <span className="text-white text-xs font-bold">D</span>
            </div>
            <h2 className="text-xl font-semibold" style={{ color: "var(--text-primary)" }}>DSP Platform</h2>
          </div>
          <p className="text-sm mb-6" style={{ color: "var(--text-secondary)" }}>
            输入你的 API Key 登录广告管理后台
          </p>
          <input
            type="text"
            placeholder="dsp_..."
            value={input}
            onChange={(e) => setInput(e.target.value)}
            className="w-full px-3 py-2 rounded-md text-sm mb-4 focus:outline-none"
            style={{ background: "var(--bg-input)", border: "1px solid var(--border)", color: "var(--text-primary)" }}
            onFocus={(e) => e.currentTarget.style.borderColor = "var(--primary)"}
            onBlur={(e) => e.currentTarget.style.borderColor = "var(--border)"}
            autoFocus
            onKeyDown={(e) => {
              if (e.key === "Enter" && input.startsWith("dsp_")) {
                saveApiKey(input.trim());
              }
            }}
          />
          <button
            onClick={() => {
              if (input.startsWith("dsp_")) {
                saveApiKey(input.trim());
              }
            }}
            disabled={!input.startsWith("dsp_")}
            className="w-full px-4 py-2 text-sm font-medium text-white rounded-md disabled:cursor-not-allowed"
            style={{ background: "var(--primary)", opacity: !input.startsWith("dsp_") ? 0.5 : 1 }}
          >
            登录
          </button>
          <p className="text-xs mt-3" style={{ color: "var(--text-muted)" }}>
            API Key 由管理员分配，格式为 dsp_ 开头的字符串
          </p>
        </div>
      </div>
    );
  }

  return (
    <>
      {sidebar}
      <main id="main-content" className="flex-1 overflow-auto" role="main">
        <div className="max-w-6xl mx-auto px-4 py-4 md:px-8 md:py-6">
          {children}
        </div>
      </main>
    </>
  );
}
