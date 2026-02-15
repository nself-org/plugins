"""
OPML import/export for podcast subscriptions.

Supports RSS 2.0 OPML format for podcast clients.
"""

from typing import List, Dict
from xml.etree import ElementTree as ET
from defusedxml import ElementTree as SafeET


def import_opml(opml_xml: str) -> List[str]:
    """
    Parse OPML XML and extract feed URLs.

    Args:
        opml_xml: OPML XML string

    Returns:
        List of feed URLs

    Raises:
        ValueError: If OPML is malformed
    """
    try:
        # Parse with defusedxml for security
        root = SafeET.fromstring(opml_xml)

        feed_urls = []

        # Find all <outline> elements with xmlUrl attribute
        for outline in root.iter("outline"):
            feed_url = outline.get("xmlUrl")
            if feed_url:
                feed_urls.append(feed_url)

        return feed_urls

    except ET.ParseError as e:
        raise ValueError(f"Failed to parse OPML: {e}")


def export_opml(subscriptions: List[Dict[str, str]]) -> str:
    """
    Generate OPML XML from subscription list.

    Args:
        subscriptions: List of dicts with 'title', 'xmlUrl' keys

    Returns:
        OPML XML string
    """
    root = ET.Element("opml", version="2.0")

    head = ET.SubElement(root, "head")
    title = ET.SubElement(head, "title")
    title.text = "nself-tv Podcast Subscriptions"

    body = ET.SubElement(root, "body")

    # Add each subscription as an outline element
    for sub in subscriptions:
        ET.SubElement(
            body,
            "outline",
            text=sub.get("title", "Unknown"),
            type="rss",
            xmlUrl=sub.get("xmlUrl", ""),
        )

    # Convert to string with XML declaration
    tree = ET.ElementTree(root)
    xml_str = ET.tostring(root, encoding="unicode", method="xml")

    return f'<?xml version="1.0" encoding="UTF-8"?>\n{xml_str}'
