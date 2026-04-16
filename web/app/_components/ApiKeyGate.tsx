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

  if (!apiKey) {
    return (
      <div className="min-h-screen w-full flex items-center justify-center bg-gray-50">
        <div className="bg-white rounded-lg p-8 w-full max-w-md">
          <h2 className="text-xl font-semibold mb-2">DSP Platform</h2>
          <p className="text-sm text-gray-500 mb-6">
            输入你的 API Key 登录广告管理后台
          </p>
          <input
            type="text"
            placeholder="dsp_..."
            value={input}
            onChange={(e) => setInput(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm mb-4 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
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
            className="w-full px-4 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700 disabled:bg-gray-300 disabled:cursor-not-allowed"
          >
            登录
          </button>
          <p className="text-xs text-gray-400 mt-3">
            API Key 由管理员分配，格式为 dsp_ 开头的字符串
          </p>
        </div>
      </div>
    );
  }

  if (isAdmin) {
    return <>{children}</>;
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
