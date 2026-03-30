"use client";

import { useEffect, useState } from "react";

export default function ApiKeyGate({ children, sidebar }: { children: React.ReactNode; sidebar?: React.ReactNode }) {
  const [apiKey, setApiKey] = useState<string | null>(null);
  const [input, setInput] = useState("");
  const [checking, setChecking] = useState(true);

  useEffect(() => {
    const key = localStorage.getItem("dsp_api_key");
    if (key) {
      setApiKey(key);
    }
    setChecking(false);
  }, []);

  if (checking) return null;

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
                localStorage.setItem("dsp_api_key", input.trim());
                setApiKey(input.trim());
              }
            }}
          />
          <button
            onClick={() => {
              if (input.startsWith("dsp_")) {
                localStorage.setItem("dsp_api_key", input.trim());
                setApiKey(input.trim());
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

  return <>{sidebar}{children}</>;
}
