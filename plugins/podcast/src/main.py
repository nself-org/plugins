"""
FastAPI application for podcast feed ingestion and management.
"""

import os
from contextlib import asynccontextmanager
from typing import List, Optional

from fastapi import FastAPI, HTTPException, Query
from fastapi.responses import Response
from pydantic import BaseModel, HttpUrl

from .parser import parse_feed, Feed
from .fetcher import fetch_feed
from .scheduler import schedule_feed_refresh, FetchStatus
from .enricher import search_itunes, search_podcast_index
from .importer import import_opml, export_opml


# Configuration
DATABASE_URL = os.getenv("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/nself_tv")
PODCAST_REFRESH_INTERVAL = int(os.getenv("PODCAST_REFRESH_INTERVAL", "3600"))
PODCAST_INDEX_API_KEY = os.getenv("PODCAST_INDEX_API_KEY", "")
ITUNES_API_ENABLED = os.getenv("ITUNES_API_ENABLED", "true").lower() == "true"


# Request/Response models
class SubscribeRequest(BaseModel):
    feed_url: HttpUrl
    family_id: str


class SearchResult(BaseModel):
    feed_url: str
    title: str
    author: str
    artwork_url: Optional[str] = None
    description: Optional[str] = None


class Episode(BaseModel):
    id: str
    title: str
    pub_date: str
    duration: Optional[int] = None
    enclosure_url: str


class ImportOPMLRequest(BaseModel):
    opml_data: str
    family_id: str


class RefreshRequest(BaseModel):
    show_id: str


# Lifespan for startup/shutdown
@asynccontextmanager
async def lifespan(app: FastAPI):
    """Initialize scheduler on startup, clean up on shutdown."""
    # Startup: Initialize feed refresh scheduler
    # TODO: Start APScheduler background tasks
    yield
    # Shutdown: Clean up resources
    pass


# Create FastAPI app
app = FastAPI(
    title="nself-tv Podcast Plugin",
    version="1.0.0",
    description="RSS/Atom feed ingestion and podcast management",
    lifespan=lifespan,
)


@app.get("/health")
async def health_check():
    """Health check endpoint."""
    return {"status": "healthy", "service": "podcast-plugin"}


@app.get("/api/v1/podcasts/search")
async def search_podcasts(
    q: str = Query(..., min_length=1, description="Search query"),
    source: str = Query("itunes", description="Search source: itunes or podcastindex"),
) -> List[SearchResult]:
    """
    Search for podcasts using iTunes or Podcast Index API.
    """
    if source == "itunes" and ITUNES_API_ENABLED:
        results = await search_itunes(q)
    elif source == "podcastindex" and PODCAST_INDEX_API_KEY:
        results = await search_podcast_index(q, PODCAST_INDEX_API_KEY)
    else:
        raise HTTPException(status_code=400, detail=f"Search source '{source}' not available")

    return results


@app.post("/api/v1/podcasts/subscribe")
async def subscribe_to_podcast(request: SubscribeRequest):
    """
    Subscribe to a podcast feed by URL.

    1. Fetch feed from URL
    2. Parse feed metadata and episodes
    3. Insert into podcast_shows and podcast_episodes tables
    4. Schedule feed refresh
    """
    try:
        # Fetch feed
        xml_data, headers = await fetch_feed(str(request.feed_url))

        if not xml_data:
            raise HTTPException(status_code=304, detail="Feed not modified")

        # Parse feed
        feed = parse_feed(xml_data)

        # TODO: Insert into database
        # 1. Insert podcast_shows record
        # 2. Insert podcast_episodes records
        # 3. Schedule feed refresh based on latest episode date

        # For now, return parsed data
        return {
            "success": True,
            "show": {
                "title": feed.title,
                "author": feed.author,
                "episodes_count": len(feed.episodes),
            },
        }

    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to subscribe: {str(e)}")


@app.get("/api/v1/podcasts/{show_id}/episodes")
async def get_show_episodes(show_id: str) -> List[Episode]:
    """
    Get all episodes for a podcast show.
    """
    # TODO: Query podcast_episodes table
    # For now, return empty list
    return []


@app.post("/api/v1/podcasts/import-opml")
async def import_opml_file(request: ImportOPMLRequest):
    """
    Import podcast subscriptions from OPML file.

    Parses OPML XML and subscribes to all feeds.
    """
    try:
        feed_urls = import_opml(request.opml_data)

        # TODO: Subscribe to each feed
        # For now, return count
        return {
            "success": True,
            "imported": len(feed_urls),
            "feed_urls": feed_urls,
        }

    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to import OPML: {str(e)}")


@app.get("/api/v1/podcasts/export-opml")
async def export_opml_file(user_id: str = Query(...)):
    """
    Export user's podcast subscriptions as OPML file.
    """
    try:
        # TODO: Query user's subscriptions from podcast_subscriptions
        # For now, return empty OPML
        subscriptions = []

        opml_xml = export_opml(subscriptions)

        return Response(
            content=opml_xml,
            media_type="text/xml",
            headers={"Content-Disposition": "attachment; filename=subscriptions.opml"},
        )

    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to export OPML: {str(e)}")


@app.post("/api/v1/podcasts/refresh/{show_id}")
async def refresh_feed(show_id: str):
    """
    Manually trigger feed refresh for a show.
    """
    try:
        # TODO: Fetch feed, parse, update episodes
        return {"success": True, "message": f"Feed refresh triggered for show {show_id}"}

    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to refresh feed: {str(e)}")
