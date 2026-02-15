"""
Podcast discovery and metadata enrichment.

Integrates with iTunes Search API and Podcast Index API.
"""

import httpx
from typing import List, Dict, Any


ITUNES_SEARCH_URL = "https://itunes.apple.com/search"
PODCAST_INDEX_SEARCH_URL = "https://api.podcastindex.org/api/1.0/search/byterm"


async def search_itunes(query: str, limit: int = 10) -> List[Dict[str, Any]]:
    """
    Search for podcasts using iTunes Search API.

    Args:
        query: Search query string
        limit: Maximum number of results

    Returns:
        List of podcast metadata dicts

    Raises:
        httpx.HTTPError: If API request fails
    """
    params = {"term": query, "entity": "podcast", "limit": limit}

    async with httpx.AsyncClient() as client:
        resp = await client.get(ITUNES_SEARCH_URL, params=params)
        resp.raise_for_status()
        data = resp.json()

        results = []
        for item in data.get("results", []):
            results.append(
                {
                    "feed_url": item.get("feedUrl", ""),
                    "title": item.get("collectionName", ""),
                    "author": item.get("artistName", ""),
                    "artwork_url": item.get("artworkUrl600", ""),
                    "description": item.get("description", ""),
                }
            )

        return results


async def search_podcast_index(query: str, api_key: str, limit: int = 10) -> List[Dict[str, Any]]:
    """
    Search for podcasts using Podcast Index API.

    Requires API key from https://podcastindex.org/

    Args:
        query: Search query string
        api_key: Podcast Index API key
        limit: Maximum number of results

    Returns:
        List of podcast metadata dicts

    Raises:
        httpx.HTTPError: If API request fails
    """
    params = {"q": query, "max": limit}
    headers = {"X-Auth-Key": api_key}

    async with httpx.AsyncClient() as client:
        resp = await client.get(PODCAST_INDEX_SEARCH_URL, params=params, headers=headers)
        resp.raise_for_status()
        data = resp.json()

        results = []
        for item in data.get("feeds", []):
            results.append(
                {
                    "feed_url": item.get("url", ""),
                    "title": item.get("title", ""),
                    "author": item.get("author", ""),
                    "artwork_url": item.get("artwork", ""),
                    "description": item.get("description", ""),
                }
            )

        return results
