"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";

const ADVERTISER_ID = 1;

export default function NewCampaignPage() {
  const router = useRouter();
  const [step, setStep] = useState(1);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Step 1: Basic info
  const [name, setName] = useState("");
  const [budgetTotal, setBudgetTotal] = useState("");
  const [budgetDaily, setBudgetDaily] = useState("");
  const [bidCPM, setBidCPM] = useState("");

  // Step 2: Targeting
  const [geoTargets, setGeoTargets] = useState<string[]>([]);
  const [osTargets, setOsTargets] = useState<string[]>([]);

  // Step 3: Creative
  const [creativeName, setCreativeName] = useState("");
  const [creativeMarkup, setCreativeMarkup] = useState("");
  const [creativeURL, setCreativeURL] = useState("");

  const geoOptions = ["CN", "US", "JP", "KR", "GB", "DE", "FR", "BR"];
  const osOptions = ["iOS", "Android", "Windows", "macOS", "Linux"];

  const toggleGeo = (g: string) =>
    setGeoTargets((prev) => prev.includes(g) ? prev.filter((x) => x !== g) : [...prev, g]);
  const toggleOS = (o: string) =>
    setOsTargets((prev) => prev.includes(o) ? prev.filter((x) => x !== o) : [...prev, o]);

  const handleSubmit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const result = await api.createCampaign({
        advertiser_id: ADVERTISER_ID,
        name,
        budget_total_cents: Math.round(parseFloat(budgetTotal) * 100),
        budget_daily_cents: Math.round(parseFloat(budgetDaily) * 100),
        bid_cpm_cents: Math.round(parseFloat(bidCPM) * 100),
        targeting: {
          geo: geoTargets.length > 0 ? geoTargets : undefined,
          os: osTargets.length > 0 ? osTargets : undefined,
        },
      });

      // Create creative if provided
      if (creativeName && creativeMarkup) {
        await api.createCreative({
          campaign_id: result.id,
          name: creativeName,
          format: "banner",
          size: "300x250",
          ad_markup: creativeMarkup,
          destination_url: creativeURL || "https://example.com",
        });
      }

      router.push("/campaigns");
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "创建失败");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="max-w-2xl">
      <h2 className="text-xl font-semibold mb-6">创建 Campaign</h2>

      {/* Step indicator */}
      <div className="flex items-center gap-2 mb-8">
        {[1, 2, 3].map((s) => (
          <div key={s} className="flex items-center gap-2">
            <div className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium
              ${step >= s ? "bg-blue-600 text-white" : "bg-gray-200 text-gray-500"}`}>
              {s}
            </div>
            <span className={`text-sm ${step >= s ? "text-gray-900" : "text-gray-400"}`}>
              {s === 1 ? "基本信息" : s === 2 ? "定向" : "素材"}
            </span>
            {s < 3 && <div className={`w-12 h-0.5 ${step > s ? "bg-blue-600" : "bg-gray-200"}`} />}
          </div>
        ))}
      </div>

      {error && (
        <div className="mb-4 p-3 rounded bg-red-50 text-red-700 text-sm">{error}</div>
      )}

      {/* Step 1 */}
      {step === 1 && (
        <div className="space-y-4">
          <Field label="Campaign 名称" required>
            <input type="text" value={name} onChange={(e) => setName(e.target.value)}
              placeholder="例: 云服务器春季促销"
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" />
          </Field>
          <div className="grid grid-cols-2 gap-4">
            <Field label="总预算 (¥)" required>
              <input type="number" value={budgetTotal} onChange={(e) => setBudgetTotal(e.target.value)}
                placeholder="10000"
                className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500" />
            </Field>
            <Field label="日预算 (¥)" required>
              <input type="number" value={budgetDaily} onChange={(e) => setBudgetDaily(e.target.value)}
                placeholder="1000"
                className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500" />
            </Field>
          </div>
          <Field label="CPM 出价 (¥)" required>
            <input type="number" value={bidCPM} onChange={(e) => setBidCPM(e.target.value)}
              placeholder="5.00"
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500" />
            <p className="text-xs text-gray-500 mt-1">每千次曝光的出价金额</p>
          </Field>
          <div className="flex justify-end pt-4">
            <button onClick={() => setStep(2)}
              disabled={!name || !budgetTotal || !budgetDaily || !bidCPM}
              className="px-6 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed">
              下一步: 定向
            </button>
          </div>
        </div>
      )}

      {/* Step 2 */}
      {step === 2 && (
        <div className="space-y-6">
          <Field label="地区定向">
            <div className="flex flex-wrap gap-2">
              {geoOptions.map((g) => (
                <button key={g} onClick={() => toggleGeo(g)}
                  className={`px-3 py-1.5 text-sm rounded-md border ${
                    geoTargets.includes(g)
                      ? "bg-blue-50 border-blue-300 text-blue-700"
                      : "bg-white border-gray-300 text-gray-600 hover:bg-gray-50"
                  }`}>
                  {g}
                </button>
              ))}
            </div>
            <p className="text-xs text-gray-500 mt-1">不选则投放所有地区</p>
          </Field>
          <Field label="操作系统定向">
            <div className="flex flex-wrap gap-2">
              {osOptions.map((o) => (
                <button key={o} onClick={() => toggleOS(o)}
                  className={`px-3 py-1.5 text-sm rounded-md border ${
                    osTargets.includes(o)
                      ? "bg-blue-50 border-blue-300 text-blue-700"
                      : "bg-white border-gray-300 text-gray-600 hover:bg-gray-50"
                  }`}>
                  {o}
                </button>
              ))}
            </div>
          </Field>
          <div className="flex justify-between pt-4">
            <button onClick={() => setStep(1)}
              className="px-4 py-2 text-sm text-gray-600 hover:text-gray-900">
              上一步
            </button>
            <button onClick={() => setStep(3)}
              className="px-6 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700">
              下一步: 素材
            </button>
          </div>
        </div>
      )}

      {/* Step 3 */}
      {step === 3 && (
        <div className="space-y-4">
          <Field label="素材名称">
            <input type="text" value={creativeName} onChange={(e) => setCreativeName(e.target.value)}
              placeholder="例: 首页 Banner 300x250"
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500" />
          </Field>
          <Field label="广告代码 (HTML)">
            <textarea value={creativeMarkup} onChange={(e) => setCreativeMarkup(e.target.value)}
              rows={4} placeholder='<div style="width:300px;height:250px;background:#0066ff;color:#fff">Your Ad</div>'
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm font-mono focus:ring-2 focus:ring-blue-500" />
          </Field>
          <Field label="落地页 URL">
            <input type="url" value={creativeURL} onChange={(e) => setCreativeURL(e.target.value)}
              placeholder="https://example.com/landing"
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500" />
          </Field>

          {/* Summary */}
          <div className="mt-6 p-4 rounded-lg bg-gray-50 border border-gray-200">
            <h4 className="text-sm font-medium mb-3">确认信息</h4>
            <div className="grid grid-cols-2 gap-y-2 text-sm">
              <span className="text-gray-500">名称:</span><span>{name}</span>
              <span className="text-gray-500">总预算:</span><span>¥{budgetTotal}</span>
              <span className="text-gray-500">日预算:</span><span>¥{budgetDaily}</span>
              <span className="text-gray-500">CPM 出价:</span><span>¥{bidCPM}</span>
              <span className="text-gray-500">地区:</span><span>{geoTargets.length > 0 ? geoTargets.join(", ") : "全部"}</span>
              <span className="text-gray-500">系统:</span><span>{osTargets.length > 0 ? osTargets.join(", ") : "全部"}</span>
            </div>
          </div>

          <div className="flex justify-between pt-4">
            <button onClick={() => setStep(2)}
              className="px-4 py-2 text-sm text-gray-600 hover:text-gray-900">
              上一步
            </button>
            <button onClick={handleSubmit} disabled={submitting}
              className="px-6 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700 disabled:opacity-50">
              {submitting ? "创建中..." : "创建 Campaign"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function Field({ label, required, children }: { label: string; required?: boolean; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-sm font-medium text-gray-700 mb-1">
        {label} {required && <span className="text-red-500">*</span>}
      </label>
      {children}
    </div>
  );
}
