/**
 * api-version.ts — X-API-Version header middleware for the Marketplace Worker (Cloudflare Worker)
 *
 * Reads the X-API-Version request header and resolves it to a supported version.
 * Missing header defaults to '1' (locked forever per v1.0.9 LTS).
 * Unknown version values return 400 Bad Request.
 *
 * LTS commitment: 'X-API-Version: 1' and missing header BOTH return v1 behavior
 * through 2027-04-17 (12-month backward-compatibility window).
 *
 * Cloudflare Worker pattern: this module exports a pure function (not Express middleware).
 * Call resolveApiVersion(request) at the start of the fetch handler.
 */

/** Currently supported API versions */
const SUPPORTED_VERSIONS = new Set<string>(['1']);

/** Default version when header is absent (LTS: locked to '1' forever) */
const DEFAULT_VERSION = '1';

/** Maximum header value length to accept */
const MAX_VERSION_LENGTH = 16;

export interface ApiVersionResult {
  version: string;
  errorResponse: Response | null;
}

/**
 * Sanitize an X-API-Version header value.
 * Returns the sanitized version string, or null if invalid.
 */
function sanitizeVersion(raw: string): string | null {
  if (!raw || raw.length > MAX_VERSION_LENGTH) {
    return null;
  }
  // Only allow digits and dots (e.g. "1", "1.0", "2")
  if (!/^[0-9.]+$/.test(raw)) {
    return null;
  }
  return raw.trim();
}

/**
 * Standard CORS headers for all responses.
 */
function corsHeaders(): Record<string, string> {
  return {
    'Access-Control-Allow-Origin': '*',
    'Access-Control-Allow-Methods': 'GET, POST, OPTIONS',
    'Access-Control-Allow-Headers': 'Content-Type, Authorization, X-API-Version',
  };
}

/**
 * Resolve the X-API-Version header from a Cloudflare Worker request.
 *
 * Returns { version, errorResponse } where:
 * - errorResponse is null when the version was resolved successfully
 * - errorResponse is a 400 Response when the header is invalid or unsupported
 *
 * If errorResponse is non-null, return it immediately from the fetch handler.
 */
export function resolveApiVersion(request: Request): ApiVersionResult {
  const raw = request.headers.get('x-api-version');

  // No header: default to v1 (LTS commitment)
  if (!raw) {
    return { version: DEFAULT_VERSION, errorResponse: null };
  }

  const version = sanitizeVersion(raw);

  if (version === null) {
    return {
      version: DEFAULT_VERSION,
      errorResponse: new Response(
        JSON.stringify({
          error: 'Invalid X-API-Version header value',
          detail: 'Version must contain only digits and dots (e.g. "1" or "1.0")',
        }),
        {
          status: 400,
          headers: { 'Content-Type': 'application/json', ...corsHeaders() },
        }
      ),
    };
  }

  if (!SUPPORTED_VERSIONS.has(version)) {
    return {
      version: DEFAULT_VERSION,
      errorResponse: new Response(
        JSON.stringify({
          error: `Unsupported API version: ${version}`,
          detail: `Supported versions: ${[...SUPPORTED_VERSIONS].join(', ')}`,
          docs: 'https://docs.nself.org/api/v1/marketplace-worker',
        }),
        {
          status: 400,
          headers: { 'Content-Type': 'application/json', ...corsHeaders() },
        }
      ),
    };
  }

  return { version, errorResponse: null };
}

/**
 * Add X-API-Version response header to an existing Response headers object.
 * Call this when building the response headers to reflect the resolved version.
 */
export function addApiVersionHeader(
  headers: Record<string, string>,
  version: string
): Record<string, string> {
  return { ...headers, 'X-API-Version': version };
}
