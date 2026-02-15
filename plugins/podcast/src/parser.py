"""
RSS/Atom feed parser for podcast feeds.

Handles parsing of RSS 2.0, Atom, and podcast namespace extensions.
"""

import feedparser
from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional, List
import time


@dataclass
class Chapter:
    """Podcast chapter from podcast namespace."""

    start_time: int  # seconds
    title: str
    url: str = ""
    image: str = ""


@dataclass
class Episode:
    """Podcast episode metadata."""

    guid: str
    title: str
    description: str
    pub_date: datetime
    duration: int  # seconds
    enclosure_url: str
    enclosure_type: str
    enclosure_length: int
    artwork_url: str = ""
    season: Optional[int] = None
    episode: Optional[int] = None
    explicit: bool = False
    chapters: List[Chapter] = field(default_factory=list)
    transcript_url: str = ""
    funding_url: str = ""


@dataclass
class Feed:
    """Podcast feed metadata."""

    title: str
    author: str
    description: str
    artwork_url: str
    language: str
    explicit: bool
    categories: List[str]
    episodes: List[Episode]


def parse_feed(xml_data: bytes) -> Feed:
    """
    Parse RSS/Atom XML feed data into a Feed object.

    Handles malformed feeds gracefully using feedparser library.
    Extracts podcast namespace extensions (chapters, transcripts, funding).

    Args:
        xml_data: Raw XML bytes from feed

    Returns:
        Feed object with all metadata and episodes

    Raises:
        ValueError: If feed cannot be parsed or is missing required fields
    """
    # Parse with feedparser (handles both RSS and Atom)
    d = feedparser.parse(xml_data)

    if d.bozo and not d.entries:
        # Malformed feed with no entries
        raise ValueError(f"Failed to parse feed: {d.bozo_exception}")

    # Extract feed-level metadata
    feed_info = d.feed

    title = feed_info.get("title", "Unknown Podcast")
    author = feed_info.get("author", "") or feed_info.get("itunes_author", "")
    description = feed_info.get("description", "") or feed_info.get("subtitle", "")
    language = feed_info.get("language", "en")

    # iTunes artwork
    artwork_url = ""
    if hasattr(feed_info, "image"):
        artwork_url = feed_info.image.get("href", "")
    elif hasattr(feed_info, "itunes_image"):
        artwork_url = feed_info.itunes_image

    # Explicit content flag
    explicit = False
    if hasattr(feed_info, "itunes_explicit"):
        explicit = feed_info.itunes_explicit.lower() in ("yes", "true")

    # Categories
    categories = []
    if hasattr(feed_info, "tags"):
        categories = [tag.term for tag in feed_info.tags]
    elif hasattr(feed_info, "itunes_category"):
        categories = [feed_info.itunes_category]

    # Parse episodes
    episodes = []
    for entry in d.entries:
        episode = _parse_episode(entry)
        if episode:
            episodes.append(episode)

    return Feed(
        title=title,
        author=author,
        description=description,
        artwork_url=artwork_url,
        language=language,
        explicit=explicit,
        categories=categories,
        episodes=episodes,
    )


def _parse_episode(entry) -> Optional[Episode]:
    """
    Parse a single episode entry from feedparser.

    Args:
        entry: feedparser entry object

    Returns:
        Episode object or None if entry is invalid
    """
    # Required fields
    guid = entry.get("id") or entry.get("link", "")
    title = entry.get("title", "")

    # Enclosure (audio file)
    enclosure_url = ""
    enclosure_type = ""
    enclosure_length = 0

    if hasattr(entry, "enclosures") and entry.enclosures:
        enc = entry.enclosures[0]
        enclosure_url = enc.get("href", "")
        enclosure_type = enc.get("type", "audio/mpeg")
        enclosure_length = int(enc.get("length", 0))

    if not guid or not title or not enclosure_url:
        # Invalid episode, skip
        return None

    # Publication date
    pub_date = datetime.now()
    if hasattr(entry, "published_parsed") and entry.published_parsed:
        pub_date = datetime.fromtimestamp(time.mktime(entry.published_parsed))
    elif hasattr(entry, "updated_parsed") and entry.updated_parsed:
        pub_date = datetime.fromtimestamp(time.mktime(entry.updated_parsed))

    # Description
    description = entry.get("description", "") or entry.get("summary", "")

    # Duration
    duration = 0
    if hasattr(entry, "itunes_duration"):
        duration = _parse_duration(entry.itunes_duration)

    # Artwork
    artwork_url = ""
    if hasattr(entry, "itunes_image"):
        artwork_url = entry.itunes_image
    elif hasattr(entry, "image"):
        artwork_url = entry.image.get("href", "")

    # Season/Episode numbers
    season = None
    episode_num = None
    if hasattr(entry, "itunes_season"):
        try:
            season = int(entry.itunes_season)
        except (ValueError, TypeError):
            pass
    if hasattr(entry, "itunes_episode"):
        try:
            episode_num = int(entry.itunes_episode)
        except (ValueError, TypeError):
            pass

    # Explicit
    explicit = False
    if hasattr(entry, "itunes_explicit"):
        explicit = entry.itunes_explicit.lower() in ("yes", "true")

    # Podcast namespace extensions
    chapters = _parse_chapters(entry)
    transcript_url = _get_podcast_namespace_value(entry, "transcript")
    funding_url = _get_podcast_namespace_value(entry, "funding")

    return Episode(
        guid=guid,
        title=title,
        description=description,
        pub_date=pub_date,
        duration=duration,
        enclosure_url=enclosure_url,
        enclosure_type=enclosure_type,
        enclosure_length=enclosure_length,
        artwork_url=artwork_url,
        season=season,
        episode=episode_num,
        explicit=explicit,
        chapters=chapters,
        transcript_url=transcript_url,
        funding_url=funding_url,
    )


def _parse_duration(duration_str: str) -> int:
    """
    Parse iTunes duration string to seconds.

    Formats: "HH:MM:SS", "MM:SS", or "SSSS"

    Args:
        duration_str: Duration string

    Returns:
        Duration in seconds
    """
    try:
        parts = duration_str.split(":")
        if len(parts) == 3:
            # HH:MM:SS
            return int(parts[0]) * 3600 + int(parts[1]) * 60 + int(parts[2])
        elif len(parts) == 2:
            # MM:SS
            return int(parts[0]) * 60 + int(parts[1])
        else:
            # Just seconds
            return int(duration_str)
    except (ValueError, AttributeError):
        return 0


def _parse_chapters(entry) -> List[Chapter]:
    """
    Parse podcast namespace chapters.

    Args:
        entry: feedparser entry

    Returns:
        List of Chapter objects
    """
    chapters = []

    # TODO: Parse podcast:chapters from entry
    # This requires checking for podcast namespace in the entry
    # For now, return empty list

    return chapters


def _get_podcast_namespace_value(entry, key: str) -> str:
    """
    Get value from podcast namespace.

    Args:
        entry: feedparser entry
        key: podcast namespace key (e.g., "transcript", "funding")

    Returns:
        Value string or empty string if not found
    """
    # TODO: Parse podcast namespace attributes
    # For now, return empty string

    return ""
