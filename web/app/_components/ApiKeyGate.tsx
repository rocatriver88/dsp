"use client";

import { useState, useEffect } from "react";
import { usePathname } from "next/navigation";
import { login, isAuthenticated } from "@/lib/api";
import TopBar from "./TopBar";

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

    // ===== Split login page =====
    return (
      <div className="min-h-screen w-full flex" style={{ background: "var(--bg-page)" }}>
        {/* Left brand side */}
        <div className="hidden md:flex flex-1 flex-col justify-center px-12 lg:px-16 relative overflow-hidden"
          style={{ background: "#0F0A1A" }}>
          {/* Animated gradient orbs */}
          <div className="absolute top-[-20%] left-[-20%] w-[70%] h-[70%] rounded-full pointer-events-none"
            style={{
              background: "radial-gradient(circle, rgba(139,92,246,0.15) 0%, transparent 60%)",
              animation: "loginFloat1 8s ease-in-out infinite",
            }} />
          <div className="absolute bottom-[-10%] right-[-10%] w-[60%] h-[60%] rounded-full pointer-events-none"
            style={{
              background: "radial-gradient(circle, rgba(59,130,246,0.1) 0%, transparent 60%)",
              animation: "loginFloat2 10s ease-in-out infinite",
            }} />

          <style jsx>{`
            @keyframes loginFloat1 {
              0%, 100% { transform: translate(0, 0); }
              50% { transform: translate(30px, 20px); }
            }
            @keyframes loginFloat2 {
              0%, 100% { transform: translate(0, 0); }
              50% { transform: translate(-20px, -30px); }
            }
          `}</style>

          <div className="relative z-10 max-w-lg">
            {/* Logo */}
            <div className="flex items-center gap-3 mb-10">
              <div className="w-10 h-10 rounded-[10px] flex items-center justify-center"
                style={{
                  background: "linear-gradient(135deg, #8B5CF6, #6D28D9)",
                  boxShadow: "0 8px 24px rgba(139,92,246,0.3)",
                }}>
                <span className="text-white text-sm font-bold">D</span>
              </div>
              <span className="text-lg font-bold" style={{ color: "var(--text-primary)" }}>DSP Platform</span>
            </div>

            {/* Headline */}
            <h2 className="text-[28px] font-extrabold leading-tight mb-4"
              style={{
                background: "linear-gradient(135deg, #fff 30%, #c4b5fd 100%)",
                WebkitBackgroundClip: "text",
                WebkitTextFillColor: "transparent",
                backgroundClip: "text",
              }}>
              智能投放<br />精准触达
            </h2>
            <p className="text-[15px] leading-relaxed max-w-sm" style={{ color: "var(--text-secondary)" }}>
              面向中国中小广告代理商的程序化广告投放管理平台。实时竞价、智能优化、全链路归因。
            </p>

            {/* Stats strip */}
            <div className="flex gap-8 mt-10 pt-6" style={{ borderTop: "1px solid rgba(139,92,246,0.15)" }}>
              <div>
                <p className="text-xl font-bold tabular-nums" style={{ color: "var(--primary)" }}>50M+</p>
                <p className="text-[11px] uppercase tracking-wider mt-0.5" style={{ color: "var(--text-muted)" }}>日均竞价请求</p>
              </div>
              <div>
                <p className="text-xl font-bold tabular-nums" style={{ color: "var(--primary)" }}>200+</p>
                <p className="text-[11px] uppercase tracking-wider mt-0.5" style={{ color: "var(--text-muted)" }}>活跃广告主</p>
              </div>
              <div>
                <p className="text-xl font-bold tabular-nums" style={{ color: "var(--primary)" }}>98.5%</p>
                <p className="text-[11px] uppercase tracking-wider mt-0.5" style={{ color: "var(--text-muted)" }}>服务可用性</p>
              </div>
            </div>
          </div>
        </div>

        {/* Right form side */}
        <div className="w-full md:w-[440px] flex-shrink-0 flex items-center justify-center px-8"
          style={{ background: "var(--bg-card)", borderLeft: "1px solid var(--border)" }}>
          <div className="w-full max-w-xs">
            {/* Mobile logo (only shown on small screens) */}
            <div className="flex items-center gap-3 mb-2 md:hidden">
              <div className="w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0"
                style={{ background: "linear-gradient(135deg, #8B5CF6, #6D28D9)" }}>
                <span className="text-white text-xs font-bold">D</span>
              </div>
              <span className="text-lg font-semibold" style={{ color: "var(--text-primary)" }}>DSP Platform</span>
            </div>

            <h3 className="text-xl font-semibold mb-1" style={{ color: "var(--text-primary)" }}>登录</h3>
            <p className="text-[13px] mb-7" style={{ color: "var(--text-secondary)" }}>登录广告管理后台</p>

            {/* Email */}
            <div className="mb-3">
              <label className="block text-xs font-medium mb-1.5" style={{ color: "var(--text-secondary)" }}>邮箱</label>
              <input
                type="email"
                placeholder="your@email.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="w-full px-3.5 py-2.5 rounded-lg text-[13px] focus:outline-none transition-colors"
                style={{ background: "#0F0A1A", border: "1px solid var(--border)", color: "var(--text-primary)" }}
                onFocus={(e) => e.currentTarget.style.borderColor = "var(--primary)"}
                onBlur={(e) => e.currentTarget.style.borderColor = "var(--border)"}
                autoFocus
                onKeyDown={(e) => { if (e.key === "Enter") handleLogin(); }}
              />
            </div>

            {/* Password */}
            <div className="mb-5">
              <label className="block text-xs font-medium mb-1.5" style={{ color: "var(--text-secondary)" }}>密码</label>
              <input
                type="password"
                placeholder="输入密码"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full px-3.5 py-2.5 rounded-lg text-[13px] focus:outline-none transition-colors"
                style={{ background: "#0F0A1A", border: "1px solid var(--border)", color: "var(--text-primary)" }}
                onFocus={(e) => e.currentTarget.style.borderColor = "var(--primary)"}
                onBlur={(e) => e.currentTarget.style.borderColor = "var(--border)"}
                onKeyDown={(e) => { if (e.key === "Enter") handleLogin(); }}
              />
            </div>

            {error && <p className="text-xs mb-3" style={{ color: "var(--error)" }}>{error}</p>}

            <button
              onClick={handleLogin}
              disabled={loading || !email || !password}
              className="btn-primary w-full px-4 py-2.5 text-sm"
            >
              {loading ? "登录中..." : "登录"}
            </button>

            {/* API Key fallback */}
            <div className="mt-5 pt-4" style={{ borderTop: "1px solid var(--border)" }}>
              <button
                onClick={() => setShowApiKey(!showApiKey)}
                className="text-xs transition-colors"
                style={{ color: "var(--text-muted)", minHeight: "auto", minWidth: "auto" }}
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
                    className="w-full px-3.5 py-2.5 rounded-lg text-[13px] mb-3 focus:outline-none transition-colors"
                    style={{ background: "#0F0A1A", border: "1px solid var(--border)", color: "var(--text-primary)" }}
                    onFocus={(e) => e.currentTarget.style.borderColor = "var(--primary)"}
                    onBlur={(e) => e.currentTarget.style.borderColor = "var(--border)"}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && apiKeyInput.startsWith("dsp_")) handleApiKeyLogin();
                    }}
                  />
                  <button
                    onClick={handleApiKeyLogin}
                    disabled={!apiKeyInput.startsWith("dsp_")}
                    className="btn-primary w-full px-4 py-2.5 text-sm"
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
      </div>
    );
  }

  // ===== Authenticated shell with TopBar =====
  return (
    <>
      {sidebar}
      <div className="flex-1 flex flex-col overflow-hidden">
        <TopBar />
        <main id="main-content" className="flex-1 overflow-auto ambient-glow" role="main">
          <div className="relative z-10 max-w-6xl mx-auto px-4 py-4 md:px-8 md:py-7">
            {children}
          </div>
        </main>
      </div>
    </>
  );
}
