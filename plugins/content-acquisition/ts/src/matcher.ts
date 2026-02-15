/**
 * Content Matcher
 * Fuzzy matching for RSS items against content acquisition rules
 */

export interface MatchCriteria {
  title: string;
  year?: number;
  quality?: string[];
  category?: string;
}

export interface RSSItem {
  title: string;
  link: string;
  pubDate: string;
}

export class ContentMatcher {
  /**
   * Normalize title for matching
   */
  normalizeTitle(title: string): string {
    return title
      .toLowerCase()
      .replace(/[^a-z0-9\s]/g, '') // Remove special chars
      .replace(/\s+/g, ' ') // Collapse whitespace
      .trim();
  }

  /**
   * Extract year from title
   */
  extractYear(title: string): number | null {
    const match = title.match(/\b(19|20)\d{2}\b/);
    return match ? parseInt(match[0], 10) : null;
  }

  /**
   * Extract quality from title
   */
  extractQuality(title: string): string[] {
    const qualities: string[] = [];
    const lower = title.toLowerCase();

    if (lower.includes('2160p') || lower.includes('4k')) qualities.push('4k');
    if (lower.includes('1080p')) qualities.push('1080p');
    if (lower.includes('720p')) qualities.push('720p');
    if (lower.includes('hdr')) qualities.push('hdr');
    if (lower.includes('dolby vision')) qualities.push('dolby-vision');

    return qualities;
  }

  /**
   * Fuzzy match titles
   */
  fuzzyMatch(a: string, b: string, threshold = 0.8): boolean {
    const longer = a.length > b.length ? a : b;
    const shorter = a.length > b.length ? b : a;

    if (longer.length === 0) return true;

    const distance = this.levenshteinDistance(longer, shorter);
    const similarity = (longer.length - distance) / longer.length;

    return similarity >= threshold;
  }

  /**
   * Levenshtein distance algorithm
   */
  private levenshteinDistance(a: string, b: string): number {
    const matrix: number[][] = [];

    for (let i = 0; i <= b.length; i++) {
      matrix[i] = [i];
    }

    for (let j = 0; j <= a.length; j++) {
      matrix[0][j] = j;
    }

    for (let i = 1; i <= b.length; i++) {
      for (let j = 1; j <= a.length; j++) {
        if (b.charAt(i - 1) === a.charAt(j - 1)) {
          matrix[i][j] = matrix[i - 1][j - 1];
        } else {
          matrix[i][j] = Math.min(
            matrix[i - 1][j - 1] + 1,
            matrix[i][j - 1] + 1,
            matrix[i - 1][j] + 1
          );
        }
      }
    }

    return matrix[b.length][a.length];
  }

  /**
   * Match RSS item against criteria
   */
  match(item: RSSItem, criteria: MatchCriteria): boolean {
    // Normalize titles
    const itemTitle = this.normalizeTitle(item.title);
    const criteriaTitle = this.normalizeTitle(criteria.title);

    // Title match (fuzzy)
    if (!this.fuzzyMatch(itemTitle, criteriaTitle)) {
      return false;
    }

    // Year match (if specified)
    if (criteria.year) {
      const itemYear = this.extractYear(item.title);
      if (itemYear !== criteria.year) {
        return false;
      }
    }

    // Quality match (if specified)
    if (criteria.quality && criteria.quality.length > 0) {
      const itemQualities = this.extractQuality(item.title);
      const hasMatch = criteria.quality.some(q =>
        itemQualities.includes(q)
      );
      if (!hasMatch) {
        return false;
      }
    }

    return true;
  }
}
