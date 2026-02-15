"""
HTTP feed fetcher with conditional GET support.

Implements ETag and If-Modified-Since headers for efficient feed updates.
"""

import httpx
from typing import Dict, Tuple, Optional


USER_AGENT = "nself-tv/0.8 +https://nself.tv"
TIMEOUT = 30.0
MAX_REDIRECTS = 5


async def fetch_feed(
    url: str, etag: Optional[str] = None, last_modified: Optional[str] = None
) -> Tuple[bytes, Dict[str, str]]:
    """
    Fetch RSS/Atom feed with conditional GET support.

    Implements HTTP caching via ETag and If-Modified-Since headers.

    Args:
        url: Feed URL to fetch
        etag: ETag from previous fetch (for conditional GET)
        last_modified: Last-Modified timestamp from previous fetch

    Returns:
        Tuple of (xml_data, metadata)
        - xml_data: Raw XML bytes (empty if 304 Not Modified)
        - metadata: Dict with 'etag', 'last_modified', 'not_modified' keys

    Raises:
        httpx.HTTPError: If request fails
    """
    headers = {"User-Agent": USER_AGENT}

    # Add conditional GET headers
    if etag:
        headers["If-None-Match"] = etag
    if last_modified:
        headers["If-Modified-Since"] = last_modified

    async with httpx.AsyncClient(
        follow_redirects=True, max_redirects=MAX_REDIRECTS, timeout=TIMEOUT
    ) as client:
        resp = await client.get(url, headers=headers)

        # Handle 304 Not Modified
        if resp.status_code == 304:
            return b"", {"not_modified": True}

        # Raise for other error status codes
        resp.raise_for_status()

        # Return XML data and caching metadata
        metadata = {
            "etag": resp.headers.get("ETag", ""),
            "last_modified": resp.headers.get("Last-Modified", ""),
            "not_modified": False,
        }

        return resp.content, metadata
