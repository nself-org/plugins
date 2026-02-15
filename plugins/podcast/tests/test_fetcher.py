"""
Tests for feed fetcher with conditional GET support.
"""

import pytest
import httpx
from unittest.mock import Mock, AsyncMock, patch
from src.fetcher import fetch_feed, USER_AGENT


@pytest.mark.asyncio
async def test_fetch_feed_success():
    """Test successful feed fetch."""
    mock_response = Mock()
    mock_response.status_code = 200
    mock_response.content = b"<rss>test</rss>"
    mock_response.headers = httpx.Headers(
        {"ETag": "abc123", "Last-Modified": "Mon, 01 Jan 2024 12:00:00 GMT"}
    )
    mock_response.raise_for_status = Mock()

    with patch("httpx.AsyncClient") as mock_client:
        mock_client.return_value.__aenter__.return_value.get = AsyncMock(
            return_value=mock_response
        )

        xml_data, metadata = await fetch_feed("https://example.com/feed.xml")

        assert xml_data == b"<rss>test</rss>"
        assert metadata["etag"] == "abc123"
        assert metadata["last_modified"] == "Mon, 01 Jan 2024 12:00:00 GMT"
        assert metadata["not_modified"] is False


@pytest.mark.asyncio
async def test_fetch_feed_not_modified():
    """Test feed fetch with 304 Not Modified response."""
    mock_response = Mock()
    mock_response.status_code = 304

    with patch("httpx.AsyncClient") as mock_client:
        mock_client.return_value.__aenter__.return_value.get = AsyncMock(
            return_value=mock_response
        )

        xml_data, metadata = await fetch_feed(
            "https://example.com/feed.xml", etag="abc123"
        )

        assert xml_data == b""
        assert metadata["not_modified"] is True


@pytest.mark.asyncio
async def test_fetch_feed_with_conditional_headers():
    """Test that conditional GET headers are sent."""
    mock_response = Mock()
    mock_response.status_code = 200
    mock_response.content = b"<rss>test</rss>"
    mock_response.headers = httpx.Headers({})
    mock_response.raise_for_status = Mock()

    with patch("httpx.AsyncClient") as mock_client:
        mock_get = AsyncMock(return_value=mock_response)
        mock_client.return_value.__aenter__.return_value.get = mock_get

        await fetch_feed(
            "https://example.com/feed.xml",
            etag="test-etag",
            last_modified="Mon, 01 Jan 2024 12:00:00 GMT",
        )

        # Check that headers were included in request
        call_kwargs = mock_get.call_args.kwargs
        assert "headers" in call_kwargs
        headers = call_kwargs["headers"]
        assert headers["User-Agent"] == USER_AGENT
        assert headers["If-None-Match"] == "test-etag"
        assert headers["If-Modified-Since"] == "Mon, 01 Jan 2024 12:00:00 GMT"


@pytest.mark.asyncio
async def test_fetch_feed_error():
    """Test feed fetch with HTTP error."""
    mock_response = Mock()
    mock_response.status_code = 404
    mock_response.raise_for_status = Mock(
        side_effect=httpx.HTTPStatusError(
            "Not Found", request=Mock(), response=mock_response
        )
    )

    with patch("httpx.AsyncClient") as mock_client:
        mock_client.return_value.__aenter__.return_value.get = AsyncMock(
            return_value=mock_response
        )

        with pytest.raises(httpx.HTTPStatusError):
            await fetch_feed("https://example.com/notfound.xml")


@pytest.mark.asyncio
async def test_fetch_feed_user_agent():
    """Test that User-Agent header is set correctly."""
    mock_response = Mock()
    mock_response.status_code = 200
    mock_response.content = b"<rss>test</rss>"
    mock_response.headers = httpx.Headers({})
    mock_response.raise_for_status = Mock()

    with patch("httpx.AsyncClient") as mock_client:
        mock_get = AsyncMock(return_value=mock_response)
        mock_client.return_value.__aenter__.return_value.get = mock_get

        await fetch_feed("https://example.com/feed.xml")

        # Verify User-Agent was sent
        call_kwargs = mock_get.call_args.kwargs
        assert call_kwargs["headers"]["User-Agent"] == USER_AGENT
