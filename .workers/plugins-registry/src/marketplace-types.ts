import type { Env } from "./registry.ts";

export interface MarketplacePlugin {
  name: string;
  displayName: string;
  version: string;
  description: string;
  category: string;
  tier: "free" | "pro";
  bundle: string | null;
  author: string;
  icon: string;
  tags: string[];
  downloads: number;
  rating: number;
  reviewCount: number;
  licenseRequired: boolean;
  price: string | null;
  related: string[];
  homepage: string | null;
  repository: string | null;
  /** T03: curated plugins are pinned in search results via curated_boost */
  curated?: boolean;
}

export interface BundleInfo {
  slug: string;
  name: string;
  price: string;
  plugins: string[];
  description: string;
}

export interface MarketplaceResponse {
  plugins: MarketplacePlugin[];
  bundles: BundleInfo[];
  categories: string[];
  stats: { total: number; free: number; pro: number; updatedAt: string };
}

export interface RatingAggregate {
  rating: number;
  reviewCount: number;
  reviews: RatingReview[];
}

export interface RatingReview {
  user: string;
  rating: number;
  comment: string;
  createdAt: string;
}

export interface RatingResponse {
  name: string;
  rating: number;
  reviewCount: number;
  reviews: RatingReview[];
}

export type MarketplaceEnv = Env;
