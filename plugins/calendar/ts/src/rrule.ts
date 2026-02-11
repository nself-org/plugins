/**
 * RFC 5545 RRULE utilities for recurring event expansion
 */

import { rrulestr } from 'rrule';
import { createLogger } from '@nself/plugin-utils';
import type { CalendarEventRecord, EventOccurrence } from './types.js';

const logger = createLogger('calendar:rrule');

/**
 * Parse an RRULE string and expand occurrences within a date range
 */
export function expandRecurrence(
  event: CalendarEventRecord,
  startDate: Date,
  endDate: Date,
  maxOccurrences = 365
): EventOccurrence[] {
  if (!event.recurrence_rule) {
    // Non-recurring event - return as single occurrence if in range
    if (event.start_at >= startDate && event.start_at <= endDate) {
      return [mapEventToOccurrence(event, event.start_at, event.end_at)];
    }
    return [];
  }

  try {
    // Parse RRULE string
    const rrule = rrulestr(event.recurrence_rule, {
      dtstart: event.start_at,
      tzid: event.timezone,
    });

    // Get occurrences within date range
    const occurrences = rrule.between(startDate, endDate, true);

    // Limit to max occurrences
    const limited = occurrences.slice(0, maxOccurrences);

    // Map to EventOccurrence objects
    const eventOccurrences = limited.map(occurrenceStart => {
      const duration = calculateEventDuration(event);
      const occurrenceEnd = duration > 0 ? new Date(occurrenceStart.getTime() + duration) : null;
      return mapEventToOccurrence(event, occurrenceStart, occurrenceEnd);
    });

    return eventOccurrences;
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to expand recurrence', {
      eventId: event.id,
      rrule: event.recurrence_rule,
      error: message,
    });
    return [];
  }
}

/**
 * Calculate event duration in milliseconds
 */
function calculateEventDuration(event: CalendarEventRecord): number {
  if (!event.end_at) {
    return 0;
  }
  return event.end_at.getTime() - event.start_at.getTime();
}

/**
 * Map a base event to an occurrence instance
 */
function mapEventToOccurrence(
  event: CalendarEventRecord,
  occurrenceStart: Date,
  occurrenceEnd: Date | null
): EventOccurrence {
  return {
    ...event,
    occurrence_start: occurrenceStart,
    occurrence_end: occurrenceEnd,
    is_recurring_instance: Boolean(event.recurrence_rule),
  };
}

/**
 * Validate an RRULE string
 */
export function validateRRule(rruleString: string): { valid: boolean; error?: string } {
  try {
    rrulestr(rruleString);
    return { valid: true };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Invalid RRULE';
    return { valid: false, error: message };
  }
}

/**
 * Generate RRULE string for common patterns
 */
export function generateRRule(pattern: {
  frequency: 'DAILY' | 'WEEKLY' | 'MONTHLY' | 'YEARLY';
  interval?: number;
  count?: number;
  until?: Date;
  byDay?: string[]; // e.g., ['MO', 'WE', 'FR']
  byMonthDay?: number[]; // e.g., [1, 15]
  byMonth?: number[]; // e.g., [1, 6, 12]
}): string {
  const parts: string[] = [`FREQ=${pattern.frequency}`];

  if (pattern.interval && pattern.interval > 1) {
    parts.push(`INTERVAL=${pattern.interval}`);
  }

  if (pattern.count) {
    parts.push(`COUNT=${pattern.count}`);
  }

  if (pattern.until) {
    const untilStr = pattern.until.toISOString().replace(/[-:]/g, '').split('.')[0] + 'Z';
    parts.push(`UNTIL=${untilStr}`);
  }

  if (pattern.byDay && pattern.byDay.length > 0) {
    parts.push(`BYDAY=${pattern.byDay.join(',')}`);
  }

  if (pattern.byMonthDay && pattern.byMonthDay.length > 0) {
    parts.push(`BYMONTHDAY=${pattern.byMonthDay.join(',')}`);
  }

  if (pattern.byMonth && pattern.byMonth.length > 0) {
    parts.push(`BYMONTH=${pattern.byMonth.join(',')}`);
  }

  return parts.join(';');
}

/**
 * Get human-readable description of RRULE
 */
export function describeRRule(rruleString: string): string {
  try {
    const rrule = rrulestr(rruleString);
    return rrule.toText();
  } catch (error) {
    return 'Invalid recurrence rule';
  }
}
