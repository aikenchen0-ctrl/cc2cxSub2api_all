import type { Competitor, PricingPlan, SiteAnalysis, UseCase } from "./types";

function asString(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value.trim() : fallback;
}

function asStringList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => asString(item)).filter(Boolean);
}

export function cleanDomain(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/^https?:\/\//, "")
    .replace(/^www\./, "")
    .replace(/\/.*$/, "")
    .replace(/[^\w.-]/g, "");
}

function nameKey(name: string): string {
  return name.toLowerCase().replace(/[^a-z0-9]/g, "");
}

export function normalizeCompetitor(raw: unknown): Competitor | null {
  if (typeof raw === "string") {
    const name = raw.trim();
    return name ? { name, domain: "" } : null;
  }
  if (!raw || typeof raw !== "object") return null;
  const row = raw as Record<string, unknown>;
  const name = asString(row.name);
  if (!name) return null;
  const domain = cleanDomain(asString(row.domain || row.website || row.url));
  return { name, domain };
}

export function normalizePricingPlan(raw: unknown): PricingPlan | null {
  if (typeof raw === "string") {
    const text = raw.trim();
    if (!text) return null;
    return { name: "Plan", price: text, note: "", highlights: [], trial: "" };
  }
  if (!raw || typeof raw !== "object") return null;
  const row = raw as Record<string, unknown>;
  const name = asString(row.name) || "Plan";
  const price = asString(row.price || row.cost || row.amount);
  const note = asString(row.note || row.detail || row.description);
  const trial = asString(row.trial || row.trialInfo || row.freeTrial);
  const highlights = asStringList(row.highlights || row.includes || row.features);
  if (!price && !note && highlights.length === 0 && !trial) return null;
  return {
    name,
    price: price || (trial.toLowerCase().includes("free") ? "Free" : "Contact for pricing"),
    note,
    highlights,
    trial,
  };
}

export function normalizeUseCase(raw: unknown): UseCase | null {
  if (typeof raw === "string") {
    const title = raw.trim();
    return title ? { title, description: "", audienceHint: "" } : null;
  }
  if (!raw || typeof raw !== "object") return null;
  const row = raw as Record<string, unknown>;
  const title = asString(row.title || row.name);
  const description = asString(row.description || row.summary || row.detail);
  const audienceHint = asString(row.audienceHint || row.audience || row.who);
  if (!title && !description) return null;
  return {
    title: title || "Use case",
    description,
    audienceHint,
  };
}

/** Split legacy freeform pricing like "Pro $25/mo, Max $50/mo" into plan cards. */
export function plansFromPricingString(pricing: string): PricingPlan[] {
  const text = pricing.trim();
  if (!text || /^not stated$/i.test(text)) return [];

  const parts = text
    .split(/[;|]|(?:,\s*)(?=[A-Z][\w\s/+-]*\$)|(?:\s+\/\s+)/)
    .map((part) => part.trim())
    .filter(Boolean);

  if (parts.length <= 1) {
    const match = text.match(/^(.+?)\s+(\$?\d[\d.,]*(?:\s*\/\s*\w+)?)\s*(.*)$/);
    if (match) {
      return [
        {
          name: match[1]!.trim() || "Plan",
          price: match[2]!.trim(),
          note: match[3]!.trim(),
          highlights: [],
          trial: "",
        },
      ];
    }
    return [{ name: "Pricing", price: text, note: "", highlights: [], trial: "" }];
  }

  return parts
    .map((part) => {
      const match = part.match(/^(.+?)\s+(\$?\d[\d.,]*(?:\s*\/\s*\w+)?)\s*(.*)$/);
      if (match) {
        return {
          name: match[1]!.trim() || "Plan",
          price: match[2]!.trim(),
          note: match[3]!.trim(),
          highlights: [],
          trial: "",
        };
      }
      return { name: part, price: "", note: "", highlights: [], trial: "" };
    })
    .filter((plan) => plan.name || plan.price);
}

export function summarizePricingPlans(plans: PricingPlan[]): string {
  if (plans.length === 0) return "not stated";
  return plans
    .map((plan) => {
      const bits = [plan.name, plan.price].filter(Boolean);
      if (plan.trial) bits.push(`(${plan.trial})`);
      return bits.join(" ");
    })
    .join(", ");
}

/**
 * Merge competitor lists by normalized name (and domain), so a site-named
 * "Cluely" without a domain collapses into the online "Cluely / cluely.com".
 */
export function mergeCompetitors(
  base: Competitor[],
  extra: Competitor[],
  ownDomain: string,
): Competitor[] {
  const own = cleanDomain(ownDomain);
  const byName = new Map<string, Competitor>();
  const byDomain = new Map<string, string>();

  for (const item of [...base, ...extra]) {
    const domain = cleanDomain(item.domain);
    if (domain && domain === own) continue;
    const key = nameKey(item.name);
    if (!key) continue;

    const existing = byName.get(key);
    const merged: Competitor = {
      name: existing && existing.name.length >= item.name.length ? existing.name : item.name,
      domain: (existing?.domain || domain) as string,
    };
    byName.set(key, merged);
    if (merged.domain) byDomain.set(merged.domain, key);
  }

  // Collapse two names that resolved to the same domain.
  const seenDomains = new Set<string>();
  const result: Competitor[] = [];
  for (const competitor of byName.values()) {
    if (competitor.domain) {
      if (seenDomains.has(competitor.domain)) continue;
      seenDomains.add(competitor.domain);
    }
    result.push(competitor);
  }
  return result.slice(0, 12);
}

/**
 * Coerce LLM / legacy / PATCH payloads into a stable SiteAnalysis shape.
 * Always safe to call before persisting or returning to the UI.
 */
export function normalizeSiteAnalysis(raw: unknown): SiteAnalysis {
  const row = raw && typeof raw === "object" ? (raw as Record<string, unknown>) : {};

  const competitors = mergeCompetitors(
    (Array.isArray(row.competitors) ? row.competitors : [])
      .map(normalizeCompetitor)
      .filter((item): item is Competitor => Boolean(item)),
    [],
    "",
  );

  let pricingPlans = (Array.isArray(row.pricingPlans) ? row.pricingPlans : [])
    .map(normalizePricingPlan)
    .filter((item): item is PricingPlan => Boolean(item));

  const pricing = asString(row.pricing);
  if (pricingPlans.length === 0 && pricing) {
    pricingPlans = plansFromPricingString(pricing);
  }

  const useCases = (Array.isArray(row.useCases) ? row.useCases : [])
    .map(normalizeUseCase)
    .filter((item): item is UseCase => Boolean(item));

  return {
    productName: asString(row.productName) || "Product",
    oneLiner: asString(row.oneLiner),
    features: asStringList(row.features),
    painPointsSolved: asStringList(row.painPointsSolved),
    platforms: asStringList(row.platforms),
    competitors,
    pricingPlans,
    pricing: pricing || summarizePricingPlans(pricingPlans),
    useCases,
  };
}

export function competitorWebsiteUrl(domain: string): string | null {
  const clean = cleanDomain(domain);
  if (!clean || !/^[a-z0-9-]+(\.[a-z0-9-]+)+$/.test(clean)) return null;
  return `https://${clean}`;
}
