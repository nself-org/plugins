"""
Tests for feed refresh scheduler.
"""

import pytest
from datetime import datetime, timedelta
from src.scheduler import (
    FetchStatus,
    calculate_fetch_status,
    get_refresh_interval,
    REFRESH_INTERVALS,
)


def test_calculate_fetch_status_active():
    """Test status calculation for active podcast (recent episode)."""
    # Episode published 15 days ago
    last_episode = datetime.now() - timedelta(days=15)
    status = calculate_fetch_status(last_episode)

    assert status == FetchStatus.ACTIVE


def test_calculate_fetch_status_dormant():
    """Test status calculation for dormant podcast (31-90 days)."""
    # Episode published 60 days ago
    last_episode = datetime.now() - timedelta(days=60)
    status = calculate_fetch_status(last_episode)

    assert status == FetchStatus.DORMANT


def test_calculate_fetch_status_inactive():
    """Test status calculation for inactive podcast (>90 days)."""
    # Episode published 120 days ago
    last_episode = datetime.now() - timedelta(days=120)
    status = calculate_fetch_status(last_episode)

    assert status == FetchStatus.INACTIVE


def test_calculate_fetch_status_no_episodes():
    """Test status calculation when no episodes exist."""
    status = calculate_fetch_status(None)

    assert status == FetchStatus.INACTIVE


def test_calculate_fetch_status_boundary_active():
    """Test boundary at 30 days (active/dormant threshold)."""
    # Exactly 30 days should be ACTIVE
    last_episode = datetime.now() - timedelta(days=30)
    status = calculate_fetch_status(last_episode)

    assert status == FetchStatus.ACTIVE


def test_calculate_fetch_status_boundary_dormant():
    """Test boundary at 90 days (dormant/inactive threshold)."""
    # Exactly 90 days should be DORMANT
    last_episode = datetime.now() - timedelta(days=90)
    status = calculate_fetch_status(last_episode)

    assert status == FetchStatus.DORMANT


def test_get_refresh_interval_active():
    """Test refresh interval for active status."""
    interval = get_refresh_interval(FetchStatus.ACTIVE)

    assert interval == REFRESH_INTERVALS[FetchStatus.ACTIVE]
    assert interval == 3600  # 60 minutes


def test_get_refresh_interval_dormant():
    """Test refresh interval for dormant status."""
    interval = get_refresh_interval(FetchStatus.DORMANT)

    assert interval == REFRESH_INTERVALS[FetchStatus.DORMANT]
    assert interval == 21600  # 6 hours


def test_get_refresh_interval_inactive():
    """Test refresh interval for inactive status."""
    interval = get_refresh_interval(FetchStatus.INACTIVE)

    assert interval == REFRESH_INTERVALS[FetchStatus.INACTIVE]
    assert interval == 86400  # 24 hours


def test_get_refresh_interval_error():
    """Test refresh interval for error status."""
    interval = get_refresh_interval(FetchStatus.ERROR)

    assert interval == REFRESH_INTERVALS[FetchStatus.ERROR]
    assert interval == 86400  # 24 hours


def test_refresh_intervals_defined():
    """Test that all fetch statuses have defined intervals."""
    for status in FetchStatus:
        assert status in REFRESH_INTERVALS
        assert REFRESH_INTERVALS[status] > 0
