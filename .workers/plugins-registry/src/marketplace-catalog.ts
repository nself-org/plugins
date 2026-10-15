import type { PluginEntry } from "./registry.ts";
import type { MarketplacePlugin, RatingAggregate, BundleInfo } from "./marketplace-types.ts";

interface BundleDefinition {
  name: string;
  price: string;
  plugins: string[];
  description: string;
}

export const BUNDLES: Record<string, BundleDefinition> = {
  task: {
    name: "ɳTask Bundle", price: "$0", description: "Free task-app backend: storage, jobs, search, webhooks, tokens",
    plugins: ["storage", "notifications", "jobs", "search", "feature-flags", "audit-log", "webhooks", "tokens"],
  },
  chat: {
    name: "ɳChat Bundle", price: "$0.99/mo", description: "Messaging with video calls, bots, SMS and support",
    plugins: ["bots", "livekit", "nself-sms", "support"],
  },
  claw: {
    name: "ɳClaw Bundle", price: "$0.99/mo", description: "Full AI assistant: email, calendar, news, budget, tools",
    plugins: ["ai", "browser", "claw", "claw-budget", "claw-news", "claw-web", "cron", "google", "knowledge-base", "mcp", "mux", "notify", "voice"],
  },
  family: {
    name: "ɳFamily Bundle", price: "$0.99/mo", description: "Private family social, photo sharing, activity feeds",
    plugins: ["activity-feed", "calendar", "chat", "family", "family-geni", "geolocation", "moderation", "nself-csam", "photos", "social"],
  },
  sentry: {
    name: "ɳSentry Bundle", price: "$0.99/mo", description: "Error tracking, uptime, status pages and on-call",
    plugins: ["nself-alert-router", "nself-anomaly", "nself-audit", "nself-crash", "nself-cron-monitor", "nself-errors", "nself-incident-mgmt", "nself-oncall", "nself-rum", "nself-saas-gateway", "nself-slo-tracker", "nself-status-page", "nself-stripe", "nself-synthetic-monitor", "nself-uptime-monitor"],
  },
  clawde: {
    name: "ClawDE Bundle", price: "$0.99/mo", description: "Cloud sync, premium models, team features",
    plugins: ["auth", "cms", "realtime"],
  },
};

const CURATED_PLUGINS: ReadonlySet<string> = new Set(["ai", "claw", "mux", "voice", "browser"]);

const CATEGORY_ICONS: Record<string, string> = {
  authentication: "🔐",
  automation: "⚙️",
  commerce: "🛒",
  communication: "💬",
  content: "📝",
  data: "🗄️",
  development: "🔧",
  infrastructure: "🏗️",
  integrations: "🔌",
  media: "🎬",
  streaming: "📡",
  ai: "🤖",
  compliance: "✅",
};

function bundleForPlugin(pluginName: string): { slug: string; def: BundleDefinition } | null {
  for (const [slug, def] of Object.entries(BUNDLES)) {
    if (def.plugins.includes(pluginName)) {
      return { slug, def };
    }
  }
  return null;
}

function toDisplayName(plugin: PluginEntry): string {
  if (plugin.displayName) return plugin.displayName;
  return (plugin.name ?? "")
    .split(/[-_]/)
    .map((w: string) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}

function categoryIcon(category: string): string {
  return CATEGORY_ICONS[category.toLowerCase()] ?? "🔌";
}

function normaliseTier(tier: string): "free" | "pro" {
  // Fail closed: only the literal "free" is free; pro, max and anything else is licensed.
  return tier === "free" ? "free" : "pro";
}

export function buildCard(plugin: PluginEntry, aggregate: RatingAggregate): MarketplacePlugin {
  const bundleMatch = bundleForPlugin(plugin.name);
  const tier = normaliseTier(plugin.tier);

  let price: string | null = null;
  if (bundleMatch) {
    price = bundleMatch.def.price;
  } else if (tier === "pro") {
    price = "$0.99/mo";
  }

  const rawCategory = (plugin.category ?? "other").toLowerCase();
  const icon = categoryIcon(rawCategory);

  return {
    name: plugin.name,
    displayName: toDisplayName(plugin),
    version: plugin.version ?? "0.0.0",
    description: plugin.description ?? "",
    category: rawCategory,
    tier,
    bundle: bundleMatch ? bundleMatch.slug : null,
    author: plugin.author ?? "nself",
    icon,
    tags: Array.isArray(plugin.tags) ? plugin.tags : [],
    downloads: plugin.downloads ?? 0,
    rating: aggregate.rating,
    reviewCount: aggregate.reviewCount,
    licenseRequired: tier === "pro",
    price,
    related: bundleMatch
      ? bundleMatch.def.plugins.filter((n) => n !== plugin.name)
      : [],
    homepage: plugin.homepage ?? null,
    repository: plugin.repository ?? null,
    curated: CURATED_PLUGINS.has(plugin.name),
  };
}

// ---------------------------------------------------------------------------
// Build bundle list
// ---------------------------------------------------------------------------

export function buildBundles(): BundleInfo[] {
  return Object.entries(BUNDLES).map(([slug, def]) => ({
    slug,
    name: def.name,
    price: def.price,
    plugins: def.plugins,
    description: def.description,
  }));
}
