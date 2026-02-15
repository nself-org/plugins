"""
Feed refresh scheduler.

Automatically refreshes podcast feeds based on activity level.
"""

from enum import Enum
from typing import Optional
from datetime import datetime, timedelta


class FetchStatus(str, Enum):
    """Feed fetch status levels."""

    ACTIVE = "active"  # episode in last 30 days → check every 60min
    DORMANT = "dormant"  # last episode 31-90 days → check every 6hr
    INACTIVE = "inactive"  # last episode >90 days → check daily
    ERROR = "error"  # fetch failed → retry daily, alert after 7 days


# Refresh intervals in seconds
REFRESH_INTERVALS = {
    FetchStatus.ACTIVE: 3600,  # 60 min
    FetchStatus.DORMANT: 21600,  # 6 hr
    FetchStatus.INACTIVE: 86400,  # daily
    FetchStatus.ERROR: 86400,  # daily
}


def calculate_fetch_status(last_episode_date: Optional[datetime]) -> FetchStatus:
    """
    Calculate appropriate fetch status based on last episode date.

    Args:
        last_episode_date: Publication date of most recent episode

    Returns:
        FetchStatus enum value
    """
    if not last_episode_date:
        return FetchStatus.INACTIVE

    now = datetime.now()
    days_since_last = (now - last_episode_date).days

    if days_since_last <= 30:
        return FetchStatus.ACTIVE
    elif days_since_last <= 90:
        return FetchStatus.DORMANT
    else:
        return FetchStatus.INACTIVE


def get_refresh_interval(status: FetchStatus) -> int:
    """
    Get refresh interval in seconds for a fetch status.

    Args:
        status: FetchStatus enum value

    Returns:
        Interval in seconds
    """
    return REFRESH_INTERVALS[status]


async def schedule_feed_refresh(show_id: str, status: FetchStatus) -> None:
    """
    Schedule feed refresh based on show status.

    Uses APScheduler to schedule periodic feed refreshes.

    Args:
        show_id: Podcast show UUID
        status: Current fetch status

    TODO: Implement APScheduler integration
    """
    interval = get_refresh_interval(status)

    # TODO: Implement APScheduler job scheduling
    # For now, this is a stub
    pass
