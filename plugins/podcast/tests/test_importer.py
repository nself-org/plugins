"""
Tests for OPML import/export.
"""

import pytest
from src.importer import import_opml, export_opml


# Sample OPML file
SAMPLE_OPML = """<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head>
    <title>Test Subscriptions</title>
  </head>
  <body>
    <outline text="Test Podcast 1" type="rss" xmlUrl="https://example.com/feed1.xml" />
    <outline text="Test Podcast 2" type="rss" xmlUrl="https://example.com/feed2.xml" />
    <outline text="Test Podcast 3" type="rss" xmlUrl="https://example.com/feed3.xml" />
  </body>
</opml>
"""


# Malformed OPML
MALFORMED_OPML = """<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <body>
    <outline text="Broken
  </body>
</opml>
"""


def test_import_opml_success():
    """Test successful OPML import."""
    feed_urls = import_opml(SAMPLE_OPML)

    assert len(feed_urls) == 3
    assert "https://example.com/feed1.xml" in feed_urls
    assert "https://example.com/feed2.xml" in feed_urls
    assert "https://example.com/feed3.xml" in feed_urls


def test_import_opml_empty():
    """Test OPML import with no subscriptions."""
    empty_opml = """<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <body></body>
</opml>
"""

    feed_urls = import_opml(empty_opml)

    assert len(feed_urls) == 0


def test_import_opml_malformed():
    """Test error handling for malformed OPML."""
    with pytest.raises(ValueError) as exc_info:
        import_opml(MALFORMED_OPML)

    assert "Failed to parse OPML" in str(exc_info.value)


def test_import_opml_missing_xmlurl():
    """Test OPML import with missing xmlUrl attributes."""
    missing_url = """<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <body>
    <outline text="No URL" type="rss" />
    <outline text="Has URL" type="rss" xmlUrl="https://example.com/feed.xml" />
  </body>
</opml>
"""

    feed_urls = import_opml(missing_url)

    # Only the one with xmlUrl should be imported
    assert len(feed_urls) == 1
    assert feed_urls[0] == "https://example.com/feed.xml"


def test_export_opml_success():
    """Test successful OPML export."""
    subscriptions = [
        {"title": "Podcast 1", "xmlUrl": "https://example.com/feed1.xml"},
        {"title": "Podcast 2", "xmlUrl": "https://example.com/feed2.xml"},
    ]

    opml_xml = export_opml(subscriptions)

    # Verify XML structure
    assert '<?xml version="1.0" encoding="UTF-8"?>' in opml_xml
    assert '<opml version="2.0">' in opml_xml
    assert "<title>nself-tv Podcast Subscriptions</title>" in opml_xml
    assert 'text="Podcast 1"' in opml_xml
    assert 'xmlUrl="https://example.com/feed1.xml"' in opml_xml
    assert 'text="Podcast 2"' in opml_xml
    assert 'xmlUrl="https://example.com/feed2.xml"' in opml_xml


def test_export_opml_empty():
    """Test OPML export with no subscriptions."""
    opml_xml = export_opml([])

    # Should still have valid OPML structure
    assert '<opml version="2.0">' in opml_xml
    assert "<body>" in opml_xml
    assert "</body>" in opml_xml


def test_opml_roundtrip():
    """Test OPML import/export round-trip."""
    # Export subscriptions to OPML
    original_subs = [
        {"title": "Podcast A", "xmlUrl": "https://example.com/a.xml"},
        {"title": "Podcast B", "xmlUrl": "https://example.com/b.xml"},
    ]

    opml_xml = export_opml(original_subs)

    # Import the OPML back
    feed_urls = import_opml(opml_xml)

    # Verify URLs match
    assert len(feed_urls) == 2
    assert "https://example.com/a.xml" in feed_urls
    assert "https://example.com/b.xml" in feed_urls
