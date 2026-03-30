"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";

const billingModels = [
  { value: "cpm", label: "CPM", desc: "按千次曝光计费，适合品牌曝光", field: "bid_cpm_cents", placeholder: "8.00" },
  { value: "cpc", label: "CPC", desc: "按点击计费，适合效果导向", field: "bid_cpc_cents", placeholder: "2.00" },
  { value: "ocpm", label: "oCPM", desc: "按目标转化成本智能出价，按曝光计费", field: "ocpm_target_cpa_cents", placeholder: "50.00" },
];

const adTypes = [
  { value: "banner", label: "横幅", desc: "固定尺寸，页面嵌入展示", sizes: ["300x250", "728x90", "320x50", "300x600"], needsMarkup: true },
  { value: "native", label: "原生", desc: "结构化数据，融入内容流", sizes: [], needsMarkup: false },
  { value: "splash", label: "开屏", desc: "全屏，App启动时展示 3-5秒", sizes: ["1080x1920", "1242x2208"], needsMarkup: true },
  { value: "interstitial", label: "插屏", desc: "全屏，页面切换时展示", sizes: ["320x480", "768x1024"], needsMarkup: true },
];

const geoOptions = ["CN", "US", "JP", "KR", "GB", "DE", "FR", "BR"];
const osOptions = ["iOS", "Android", "Windows", "macOS", "Linux"];

export default function NewCampaignPage() {
  const router = useRouter();
  const [step, setStep] = useState(1);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Step 1: Basic info + billing model
  const [name, setName] = useState("");
  const [billingModel, setBillingModel] = useState("cpm");
  const [budgetTotal, setBudgetTotal] = useState("");
  const [budgetDaily, setBudgetDaily] = useState("");
  const [bidAmount, setBidAmount] = useState("");

  // Step 2: Targeting
  const [geoTargets, setGeoTargets] = useState<string[]>([]);
  const [osTargets, setOsTargets] = useState<string[]>([]);

  // Step 3: Creative
  const [adType, setAdType] = useState("banner");
  const [adSize, setAdSize] = useState("300x250");
  const [creativeName, setCreativeName] = useState("");
  const [creativeMarkup, setCreativeMarkup] = useState("");
  const [creativeURL, setCreativeURL] = useState("");
  // Native fields
  const [nativeTitle, setNativeTitle] = useState("");
  const [nativeDesc, setNativeDesc] = useState("");
  const [nativeIconURL, setNativeIconURL] = useState("");
  const [nativeImageURL, setNativeImageURL] = useState("");
  const [nativeCTA, setNativeCTA] = useState("了解更多");

  const selectedBilling = billingModels.find((b) => b.value === billingModel)!;
  const selectedAdType = adTypes.find((a) => a.value === adType)!;

  const toggleGeo = (g: string) =>
    setGeoTargets((prev) => prev.includes(g) ? prev.filter((x) => x !== g) : [...prev, g]);
  const toggleOS = (o: string) =>
    setOsTargets((prev) => prev.includes(o) ? prev.filter((x) => x !== o) : [...prev, o]);

  const handleSubmit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const bidCents = Math.round(parseFloat(bidAmount) * 100);
      const result = await api.createCampaign({
        advertiser_id: 0,
        name,
        billing_model: billingModel,
        budget_total_cents: Math.round(parseFloat(budgetTotal) * 100),
        budget_daily_cents: Math.round(parseFloat(budgetDaily) * 100),
        bid_cpm_cents: billingModel === "cpm" ? bidCents : 0,
        bid_cpc_cents: billingModel === "cpc" ? bidCents : 0,
        ocpm_target_cpa_cents: billingModel === "ocpm" ? bidCents : 0,
        targeting: {
          geo: geoTargets.length > 0 ? geoTargets : undefined,
          os: osTargets.length > 0 ? osTargets : undefined,
        },
      });

      // Create creative
      if (adType === "native") {
        await api.createCreative({
          campaign_id: result.id,
          name: creativeName || `${name}-原生素材`,
          ad_type: "native",
          format: "native",
          destination_url: creativeURL || "https://example.com",
          native_title: nativeTitle,
          native_desc: nativeDesc,
          native_icon_url: nativeIconURL,
          native_image_url: nativeImageURL,
          native_cta: nativeCTA,
        });
      } else if (creativeName || creativeMarkup) {
        await api.createCreative({
          campaign_id: result.id,
          name: creativeName || `${name}-素材`,
          ad_type: adType,
          format: "banner",
          size: adSize,
          ad_markup: creativeMarkup || `<div style="width:100%;height:100%;background:#1a1a2e;color:#fff;display:flex;align-items:center;justify-content:center">${name}</div>`,
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
      <h2 className="text-2xl font-semibold mb-6">创建 Campaign</h2>

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

      {/* Step 1: Basic info + billing model */}
      {step === 1 && (
        <div className="space-y-4">
          <Field label="Campaign 名称" required>
            <input type="text" value={name} onChange={(e) => setName(e.target.value)}
              placeholder="例: 云服务器春季促销"
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" />
          </Field>

          <Field label="计费方式" required>
            <div className="grid grid-cols-3 gap-2">
              {billingModels.map((b) => (
                <button key={b.value} onClick={() => { setBillingModel(b.value); setBidAmount(""); }}
                  className={`p-3 rounded-lg border text-left transition-colors ${
                    billingModel === b.value
                      ? "border-blue-500 bg-blue-50 ring-1 ring-blue-500"
                      : "border-gray-200 hover:border-gray-300"
                  }`}>
                  <span className="text-sm font-semibold">{b.label}</span>
                  <p className="text-xs text-gray-500 mt-0.5">{b.desc}</p>
                </button>
              ))}
            </div>
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

          <Field label={`${selectedBilling.label} 出价 (¥)`} required>
            <input type="number" value={bidAmount} onChange={(e) => setBidAmount(e.target.value)}
              placeholder={selectedBilling.placeholder}
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500" />
            <p className="text-xs text-gray-500 mt-1">
              {billingModel === "cpm" && "每千次曝光的出价金额"}
              {billingModel === "cpc" && "每次点击的出价金额"}
              {billingModel === "ocpm" && "目标转化成本，系统自动优化出价"}
            </p>
          </Field>

          <div className="flex justify-end pt-4">
            <button onClick={() => setStep(2)}
              disabled={!name || !budgetTotal || !budgetDaily || !bidAmount}
              className="px-6 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed">
              下一步: 定向
            </button>
          </div>
        </div>
      )}

      {/* Step 2: Targeting */}
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
            <button onClick={() => setStep(1)} className="px-4 py-2 text-sm text-gray-600 hover:text-gray-900">上一步</button>
            <button onClick={() => setStep(3)} className="px-6 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700">
              下一步: 素材
            </button>
          </div>
        </div>
      )}

      {/* Step 3: Creative with ad type selection */}
      {step === 3 && (
        <div className="space-y-4">
          <Field label="广告类型" required>
            <div className="grid grid-cols-2 gap-2">
              {adTypes.map((a) => (
                <button key={a.value} onClick={() => { setAdType(a.value); if (a.sizes.length > 0) setAdSize(a.sizes[0]); }}
                  className={`p-3 rounded-lg border text-left transition-colors ${
                    adType === a.value
                      ? "border-blue-500 bg-blue-50 ring-1 ring-blue-500"
                      : "border-gray-200 hover:border-gray-300"
                  }`}>
                  <span className="text-sm font-semibold">{a.label}</span>
                  <p className="text-xs text-gray-500 mt-0.5">{a.desc}</p>
                </button>
              ))}
            </div>
          </Field>

          {/* Size selector for non-native types */}
          {selectedAdType.sizes.length > 0 && (
            <Field label="尺寸">
              <div className="flex gap-2">
                {selectedAdType.sizes.map((s) => (
                  <button key={s} onClick={() => setAdSize(s)}
                    className={`px-3 py-1.5 text-sm rounded-md border font-mono ${
                      adSize === s ? "bg-blue-50 border-blue-300 text-blue-700" : "border-gray-300 text-gray-600 hover:bg-gray-50"
                    }`}>
                    {s}
                  </button>
                ))}
              </div>
            </Field>
          )}

          <Field label="素材名称">
            <input type="text" value={creativeName} onChange={(e) => setCreativeName(e.target.value)}
              placeholder={`例: ${selectedAdType.label}素材-01`}
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500" />
          </Field>

          {/* Native ad fields */}
          {adType === "native" && (
            <div className="space-y-3 p-4 rounded-lg bg-gray-50">
              <p className="text-xs font-medium text-gray-500 mb-2">原生广告素材</p>
              <input type="text" value={nativeTitle} onChange={(e) => setNativeTitle(e.target.value)}
                placeholder="标题 (必填)"
                className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500" />
              <textarea value={nativeDesc} onChange={(e) => setNativeDesc(e.target.value)}
                rows={2} placeholder="描述文案"
                className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500" />
              <div className="grid grid-cols-2 gap-3">
                <input type="url" value={nativeIconURL} onChange={(e) => setNativeIconURL(e.target.value)}
                  placeholder="图标 URL" className="px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500" />
                <input type="url" value={nativeImageURL} onChange={(e) => setNativeImageURL(e.target.value)}
                  placeholder="大图 URL" className="px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500" />
              </div>
              <input type="text" value={nativeCTA} onChange={(e) => setNativeCTA(e.target.value)}
                placeholder="CTA 按钮文字 (默认: 了解更多)"
                className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500" />
            </div>
          )}

          {/* Markup ad fields */}
          {selectedAdType.needsMarkup && (
            <>
              <Field label="广告代码 (HTML)">
                <textarea value={creativeMarkup} onChange={(e) => setCreativeMarkup(e.target.value)}
                  rows={3} placeholder={`<div style="width:${adSize.split('x')[0]}px;height:${adSize.split('x')[1]}px;background:#1a1a2e;color:#fff;display:flex;align-items:center;justify-content:center">广告内容</div>`}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm font-mono focus:ring-2 focus:ring-blue-500" />
                <p className="text-xs text-gray-400 mt-1">留空将自动生成占位素材</p>
              </Field>
            </>
          )}

          <Field label="落地页 URL">
            <input type="url" value={creativeURL} onChange={(e) => setCreativeURL(e.target.value)}
              placeholder="https://example.com/landing"
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:ring-2 focus:ring-blue-500" />
          </Field>

          {/* Summary */}
          <div className="mt-6 p-4 rounded-lg bg-gray-50">
            <h4 className="text-sm font-medium mb-3">确认信息</h4>
            <div className="grid grid-cols-2 gap-y-2 text-sm">
              <span className="text-gray-500">名称:</span><span>{name}</span>
              <span className="text-gray-500">计费方式:</span><span>{selectedBilling.label}</span>
              <span className="text-gray-500">出价:</span><span>¥{bidAmount} {selectedBilling.label}</span>
              <span className="text-gray-500">总预算:</span><span>¥{budgetTotal}</span>
              <span className="text-gray-500">日预算:</span><span>¥{budgetDaily}</span>
              <span className="text-gray-500">广告类型:</span><span>{selectedAdType.label}{adSize && adType !== "native" ? ` (${adSize})` : ""}</span>
              <span className="text-gray-500">地区:</span><span>{geoTargets.length > 0 ? geoTargets.join(", ") : "全部"}</span>
              <span className="text-gray-500">系统:</span><span>{osTargets.length > 0 ? osTargets.join(", ") : "全部"}</span>
            </div>
          </div>

          <div className="flex justify-between pt-4">
            <button onClick={() => setStep(2)} className="px-4 py-2 text-sm text-gray-600 hover:text-gray-900">上一步</button>
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
