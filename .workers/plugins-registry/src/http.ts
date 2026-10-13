/**
 * http.ts: CORS and JSON response helpers shared by the worker's route modules.
 */

export function corsHeaders(): Record<string, string> {
  return {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type, Authorization, If-None-Match",
    "Access-Control-Expose-Headers": "ETag",
  };
}

export function jsonResponse(
  body: unknown,
  status = 200,
  extra: Record<string, string> = {},
): Response {
  return new Response(JSON.stringify(body, null, 2), {
    status,
    headers: {
      "Content-Type": "application/json",
      ...corsHeaders(),
      ...extra,
    },
  });
}
