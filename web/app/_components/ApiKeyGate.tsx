"use client";

import { useState, useEffect } from "react";
import { usePathname } from "next/navigation";
import { login, isAuthenticated } from "@/lib/api";

const STORAGE_KEY = "dsp_api_key";

export default function ApiKeyGate({ children, sidebar }: { children: React.ReactNode; sidebar?: React.ReactNode }) {
  const pathname = usePathname();
  const isAdmin = pathname.startsWith("/admin");
  const [mounted, setMounted] = useState(false);
  const [authed, setAuthed] = useState(false);

  // Login form state
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  // API Key fallback state
  const [showApiKey, setShowApiKey] = useState(false);
  const [apiKeyInput, setApiKeyInput] = useState("");

  useEffect(() => {
    setMounted(true);
    setAuthed(isAuthenticated());
  }, []);

  if (!mounted) return null;

  // Admin routes bypass the API key gate entirely — admin/layout.tsx handles
  // admin auth independently. This check MUST come before the !authed login
  // screen, otherwise admin-only users get stuck on the tenant login page.
  if (isAdmin) {
    return <>{children}</>;
  }

  if (!authed) {
    async function handleLogin() {
      if (!email || !password) return;
      setError(null);
      setLoading(true);
      try {
        const result = await login(email, password);
        if (result.user?.role === "platform_admin") {
          window.location.href = "/admin";
          return;
        }
        window.location.reload();
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : "邮箱或密码错误");
      } finally {
        setLoading(false);
      }
    }

    function handleApiKeyLogin() {
      if (apiKeyInput.startsWith("dsp_")) {
        localStorage.setItem(STORAGE_KEY, apiKeyInput.trim());
        setAuthed(true);
      }
    }

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
            登录广告管理后台
          </p>

          {/* Email + Password login form */}
          <div className="mb-3">
            <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-secondary)" }}>邮箱</label>
            <input
              type="email"
              placeholder="your@email.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full px-3 py-2 rounded-md text-sm focus:outline-none"
              style={{ background: "var(--bg-input)", border: "1px solid var(--border)", color: "var(--text-primary)" }}
              onFocus={(e) => e.currentTarget.style.borderColor = "var(--primary)"}
              onBlur={(e) => e.currentTarget.style.borderColor = "var(--border)"}
              autoFocus
              onKeyDown={(e) => {
                if (e.key === "Enter") handleLogin();
              }}
            />
          </div>
          <div className="mb-4">
            <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-secondary)" }}>密码</label>
            <input
              type="password"
              placeholder="输入密码"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full px-3 py-2 rounded-md text-sm focus:outline-none"
              style={{ background: "var(--bg-input)", border: "1px solid var(--border)", color: "var(--text-primary)" }}
              onFocus={(e) => e.currentTarget.style.borderColor = "var(--primary)"}
              onBlur={(e) => e.currentTarget.style.borderColor = "var(--border)"}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleLogin();
              }}
            />
          </div>

          {error && <p className="text-xs mb-3" style={{ color: "var(--error)" }}>{error}</p>}

          <button
            onClick={handleLogin}
            disabled={loading || !email || !password}
            className="w-full px-4 py-2 text-sm font-medium text-white rounded-md disabled:cursor-not-allowed"
            style={{ background: "var(--primary)", opacity: (loading || !email || !password) ? 0.5 : 1 }}
            onMouseEnter={(e) => { if (!loading && email && password) e.currentTarget.style.background = "var(--primary-hover)"; }}
            onMouseLeave={(e) => { e.currentTarget.style.background = "var(--primary)"; }}
          >
            {loading ? "登录中..." : "登录"}
          </button>

          {/* API Key fallback section */}
          <div className="mt-4 pt-3" style={{ borderTop: "1px solid var(--border)" }}>
            <button
              onClick={() => setShowApiKey(!showApiKey)}
              className="text-xs transition-colors"
              style={{ color: "var(--text-muted)" }}
            >
              {showApiKey ? "收起" : "使用 API Key 登录"}
            </button>

            {showApiKey && (
              <div className="mt-3">
                <input
                  type="text"
                  placeholder="dsp_..."
                  value={apiKeyInput}
                  onChange={(e) => setApiKeyInput(e.target.value)}
                  className="w-full px-3 py-2 rounded-md text-sm mb-3 focus:outline-none"
                  style={{ background: "var(--bg-input)", border: "1px solid var(--border)", color: "var(--text-primary)" }}
                  onFocus={(e) => e.currentTarget.style.borderColor = "var(--primary)"}
                  onBlur={(e) => e.currentTarget.style.borderColor = "var(--border)"}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && apiKeyInput.startsWith("dsp_")) {
                      handleApiKeyLogin();
                    }
                  }}
                />
                <button
                  onClick={handleApiKeyLogin}
                  disabled={!apiKeyInput.startsWith("dsp_")}
                  className="w-full px-4 py-2 text-sm font-medium text-white rounded-md disabled:cursor-not-allowed"
                  style={{ background: "var(--primary)", opacity: !apiKeyInput.startsWith("dsp_") ? 0.5 : 1 }}
                >
                  使用 API Key 登录
                </button>
                <p className="text-xs mt-2" style={{ color: "var(--text-muted)" }}>
                  API Key 由管理员分配，格式为 dsp_ 开头的字符串
                </p>
              </div>
            )}
          </div>
        </div>
      </div>
    );
  }

  return (
    <>
      {sidebar}
      <main id="main-content" className="flex-1 overflow-auto" role="main" style={{ background: "transparent" }}>
        <div className="max-w-6xl mx-auto px-4 py-4 md:px-8 md:py-6">
          {children}
        </div>
      </main>
    </>
  );
}
