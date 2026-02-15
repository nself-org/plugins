"""
Tests for RSS/Atom feed parser.
"""

import pytest
from datetime import datetime
from src.parser import parse_feed, _parse_duration, Episode


# Sample RSS 2.0 feed
SAMPLE_RSS_FEED = b"""<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
  <channel>
    <title>Test Podcast</title>
    <itunes:author>Test Author</itunes:author>
    <description>A test podcast feed</description>
    <language>en</language>
    <itunes:image href="https://example.com/artwork.jpg" />
    <itunes:explicit>no</itunes:explicit>
    <itunes:category text="Technology" />

    <item>
      <guid>episode-1</guid>
      <title>Episode 1</title>
      <description>First episode description</description>
      <pubDate>Mon, 01 Jan 2024 12:00:00 GMT</pubDate>
      <itunes:duration>3600</itunes:duration>
      <enclosure url="https://example.com/episode1.mp3" type="audio/mpeg" length="50000000" />
      <itunes:season>1</itunes:season>
      <itunes:episode>1</itunes:episode>
      <itunes:explicit>no</itunes:explicit>
    </item>

    <item>
      <guid>episode-2</guid>
      <title>Episode 2</title>
      <description>Second episode description</description>
      <pubDate>Mon, 08 Jan 2024 12:00:00 GMT</pubDate>
      <itunes:duration>45:30</itunes:duration>
      <enclosure url="https://example.com/episode2.mp3" type="audio/mpeg" length="60000000" />
    </item>
  </channel>
</rss>
"""


# Malformed XML
MALFORMED_XML = b"""<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Broken Feed
  </channel>
</rss>
"""


# Atom feed
SAMPLE_ATOM_FEED = b"""<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Test Atom Feed</title>
  <author><name>Atom Author</name></author>
  <subtitle>An Atom feed</subtitle>

  <entry>
    <id>atom-episode-1</id>
    <title>Atom Episode 1</title>
    <summary>Atom episode description</summary>
    <updated>2024-01-01T12:00:00Z</updated>
    <link rel="enclosure" type="audio/mpeg" href="https://example.com/atom1.mp3" length="30000000"/>
  </entry>
</feed>
"""


def test_parse_rss_feed():
    """Test parsing a valid RSS 2.0 podcast feed."""
    feed = parse_feed(SAMPLE_RSS_FEED)

    # Check feed metadata
    assert feed.title == "Test Podcast"
    assert feed.author == "Test Author"
    assert feed.description == "A test podcast feed"
    assert feed.language == "en"
    assert feed.artwork_url == "https://example.com/artwork.jpg"
    assert feed.explicit is False
    assert "Technology" in feed.categories

    # Check episodes
    assert len(feed.episodes) == 2

    # Check first episode
    ep1 = feed.episodes[0]
    assert ep1.guid == "episode-1"
    assert ep1.title == "Episode 1"
    assert ep1.description == "First episode description"
    assert ep1.duration == 3600  # 1 hour
    assert ep1.enclosure_url == "https://example.com/episode1.mp3"
    assert ep1.enclosure_type == "audio/mpeg"
    assert ep1.enclosure_length == 50000000
    assert ep1.season == 1
    assert ep1.episode == 1
    assert ep1.explicit is False

    # Check second episode
    ep2 = feed.episodes[1]
    assert ep2.title == "Episode 2"
    assert ep2.duration == 2730  # 45:30 = 2730 seconds


def test_parse_atom_feed():
    """Test parsing an Atom feed."""
    feed = parse_feed(SAMPLE_ATOM_FEED)

    assert feed.title == "Test Atom Feed"
    assert feed.author == "Atom Author"
    assert len(feed.episodes) == 1

    ep = feed.episodes[0]
    assert ep.guid == "atom-episode-1"
    assert ep.title == "Atom Episode 1"
    assert ep.enclosure_url == "https://example.com/atom1.mp3"


def test_parse_malformed_feed():
    """Test handling of malformed XML."""
    # feedparser is very forgiving, so malformed feeds may still parse
    # but with bozo flag set
    try:
        feed = parse_feed(MALFORMED_XML)
        # If it parses, check that it has minimal valid data
        assert feed.title is not None
    except ValueError as e:
        # Or it may raise ValueError for severely broken feeds
        assert "Failed to parse feed" in str(e)


def test_parse_duration_formats():
    """Test duration parsing for different formats."""
    # HH:MM:SS format
    assert _parse_duration("01:30:00") == 5400

    # MM:SS format
    assert _parse_duration("45:30") == 2730

    # Seconds only
    assert _parse_duration("3600") == 3600

    # Invalid format
    assert _parse_duration("invalid") == 0
    assert _parse_duration("") == 0


def test_parse_empty_feed():
    """Test parsing a feed with no episodes."""
    empty_feed = b"""<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Empty Podcast</title>
    <description>No episodes yet</description>
  </channel>
</rss>
"""

    feed = parse_feed(empty_feed)

    assert feed.title == "Empty Podcast"
    assert len(feed.episodes) == 0


def test_episode_without_enclosure():
    """Test that episodes without enclosure are skipped."""
    no_enclosure = b"""<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Podcast</title>

    <item>
      <guid>no-audio</guid>
      <title>Episode Without Audio</title>
      <description>This episode has no enclosure</description>
    </item>
  </channel>
</rss>
"""

    feed = parse_feed(no_enclosure)

    # Episode should be skipped
    assert len(feed.episodes) == 0
