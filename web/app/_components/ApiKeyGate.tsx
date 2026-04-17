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
      <div className="min-h-screen w-full flex items-center justify-center bg-gray-50">
        <div className="bg-white rounded-lg p-8 w-full max-w-md">
          <h2 className="text-xl font-semibold mb-2">DSP Platform</h2>
          <p className="text-sm text-gray-500 mb-6">
            登录广告管理后台
          </p>

          {/* Email + Password login form */}
          <div className="mb-3">
            <label className="block text-xs font-medium text-gray-500 mb-1">邮箱</label>
            <input
              type="email"
              placeholder="your@email.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              autoFocus
              onKeyDown={(e) => {
                if (e.key === "Enter") handleLogin();
              }}
            />
          </div>
          <div className="mb-4">
            <label className="block text-xs font-medium text-gray-500 mb-1">密码</label>
            <input
              type="password"
              placeholder="输入密码"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              onKeyDown={(e) => {
                if (e.key === "Enter") handleLogin();
              }}
            />
          </div>

          {error && <p className="text-xs text-red-500 mb-3">{error}</p>}

          <button
            onClick={handleLogin}
            disabled={loading || !email || !password}
            className="w-full px-4 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700 disabled:bg-gray-300 disabled:cursor-not-allowed"
          >
            {loading ? "登录中..." : "登录"}
          </button>

          {/* API Key fallback section */}
          <div className="mt-4 border-t border-gray-100 pt-3">
            <button
              onClick={() => setShowApiKey(!showApiKey)}
              className="text-xs text-gray-400 hover:text-gray-600 transition-colors"
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
                  className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm mb-3 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && apiKeyInput.startsWith("dsp_")) {
                      handleApiKeyLogin();
                    }
                  }}
                />
                <button
                  onClick={handleApiKeyLogin}
                  disabled={!apiKeyInput.startsWith("dsp_")}
                  className="w-full px-4 py-2 text-sm font-medium text-white rounded-md bg-gray-600 hover:bg-gray-700 disabled:bg-gray-300 disabled:cursor-not-allowed"
                >
                  使用 API Key 登录
                </button>
                <p className="text-xs text-gray-400 mt-2">
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
      <main id="main-content" className="flex-1 overflow-auto" role="main">
        <div className="max-w-6xl mx-auto px-4 py-4 md:px-8 md:py-6">
          {children}
        </div>
      </main>
    </>
  );
}
