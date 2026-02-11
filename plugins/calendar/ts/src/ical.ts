/**
 * iCalendar (RFC 5545) export utilities
 */

import type { CalendarRecord, CalendarEventRecord, AttendeeRecord } from './types.js';

/**
 * Format a date for iCalendar (YYYYMMDDTHHMMSSZ)
 */
function formatICalDate(date: Date, allDay = false): string {
  if (allDay) {
    // All-day events use date format without time
    return date.toISOString().split('T')[0].replace(/-/g, '');
  }

  // Regular events use datetime format
  return date.toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '');
}

/**
 * Escape special characters for iCalendar text fields
 */
function escapeICalText(text: string): string {
  return text
    .replace(/\\/g, '\\\\')
    .replace(/;/g, '\\;')
    .replace(/,/g, '\\,')
    .replace(/\n/g, '\\n');
}

/**
 * Fold long lines to 75 characters per RFC 5545
 */
function foldLine(line: string): string {
  if (line.length <= 75) {
    return line;
  }

  const folded: string[] = [];
  let remaining = line;

  // First line can be 75 chars
  folded.push(remaining.substring(0, 75));
  remaining = remaining.substring(75);

  // Continuation lines are prefixed with space and can be 74 chars
  while (remaining.length > 0) {
    folded.push(' ' + remaining.substring(0, 74));
    remaining = remaining.substring(74);
  }

  return folded.join('\r\n');
}

/**
 * Generate VEVENT component for a single event
 */
function generateVEvent(event: CalendarEventRecord, attendees: AttendeeRecord[]): string {
  const lines: string[] = [];

  lines.push('BEGIN:VEVENT');

  // Required fields
  lines.push(`UID:${event.id}@nself-calendar`);
  lines.push(`DTSTAMP:${formatICalDate(new Date())}`);

  // Start/end times
  if (event.all_day) {
    lines.push(`DTSTART;VALUE=DATE:${formatICalDate(event.start_at, true)}`);
    if (event.end_at) {
      lines.push(`DTEND;VALUE=DATE:${formatICalDate(event.end_at, true)}`);
    }
  } else {
    lines.push(`DTSTART:${formatICalDate(event.start_at)}`);
    if (event.end_at) {
      lines.push(`DTEND:${formatICalDate(event.end_at)}`);
    }
  }

  // Summary (title)
  lines.push(`SUMMARY:${escapeICalText(event.title)}`);

  // Description
  if (event.description) {
    lines.push(`DESCRIPTION:${escapeICalText(event.description)}`);
  }

  // Location
  if (event.location_name) {
    lines.push(`LOCATION:${escapeICalText(event.location_name)}`);
  }

  // Geo coordinates
  if (event.location_lat !== null && event.location_lon !== null) {
    lines.push(`GEO:${event.location_lat};${event.location_lon}`);
  }

  // Status
  const status = event.status.toUpperCase();
  if (['CONFIRMED', 'TENTATIVE', 'CANCELLED'].includes(status)) {
    lines.push(`STATUS:${status}`);
  }

  // Visibility/Class
  const visibility = event.visibility.toUpperCase();
  if (visibility === 'PUBLIC') {
    lines.push('CLASS:PUBLIC');
  } else if (visibility === 'PRIVATE') {
    lines.push('CLASS:PRIVATE');
  } else if (visibility === 'CONFIDENTIAL') {
    lines.push('CLASS:CONFIDENTIAL');
  }

  // URL
  if (event.url) {
    lines.push(`URL:${event.url}`);
  }

  // Organizer
  if (event.organizer_id) {
    lines.push(`ORGANIZER:mailto:organizer-${event.organizer_id}@nself-calendar`);
  }

  // Attendees
  for (const attendee of attendees) {
    const email = attendee.email ?? `user-${attendee.user_id}@nself-calendar`;
    const name = attendee.name ? `;CN="${escapeICalText(attendee.name)}"` : '';
    const role = attendee.role === 'optional' ? ';ROLE=OPT-PARTICIPANT' : ';ROLE=REQ-PARTICIPANT';
    const partstat = attendee.rsvp_status.toUpperCase();
    const status = ['ACCEPTED', 'DECLINED', 'TENTATIVE', 'NEEDS-ACTION'].includes(partstat)
      ? `;PARTSTAT=${partstat === 'PENDING' ? 'NEEDS-ACTION' : partstat}`
      : ';PARTSTAT=NEEDS-ACTION';

    lines.push(`ATTENDEE${name}${role}${status}:mailto:${email}`);
  }

  // Recurrence rule
  if (event.recurrence_rule) {
    lines.push(`RRULE:${event.recurrence_rule}`);
  }

  // Recurrence ID for exceptions
  if (event.is_exception && event.original_start_at) {
    lines.push(`RECURRENCE-ID:${formatICalDate(event.original_start_at)}`);
  }

  // Alarms (reminders)
  for (const minutes of event.reminder_minutes) {
    lines.push('BEGIN:VALARM');
    lines.push('ACTION:DISPLAY');
    lines.push(`DESCRIPTION:${escapeICalText(event.title)}`);
    lines.push(`TRIGGER:-PT${minutes}M`);
    lines.push('END:VALARM');
  }

  // Timestamps
  lines.push(`CREATED:${formatICalDate(event.created_at)}`);
  lines.push(`LAST-MODIFIED:${formatICalDate(event.updated_at)}`);

  lines.push('END:VEVENT');

  // Fold all lines to 75 characters
  return lines.map(foldLine).join('\r\n');
}

/**
 * Generate complete iCalendar document
 */
export function generateICalendar(
  calendar: CalendarRecord,
  events: CalendarEventRecord[],
  attendeesMap: Map<string, AttendeeRecord[]>
): string {
  const lines: string[] = [];

  // Calendar header
  lines.push('BEGIN:VCALENDAR');
  lines.push('VERSION:2.0');
  lines.push('PRODID:-//nself//Calendar Plugin//EN');
  lines.push('CALSCALE:GREGORIAN');
  lines.push('METHOD:PUBLISH');
  lines.push(`X-WR-CALNAME:${escapeICalText(calendar.name)}`);
  lines.push(`X-WR-TIMEZONE:${calendar.timezone}`);

  if (calendar.description) {
    lines.push(`X-WR-CALDESC:${escapeICalText(calendar.description)}`);
  }

  // Add timezone component if needed
  if (calendar.timezone !== 'UTC') {
    lines.push('BEGIN:VTIMEZONE');
    lines.push(`TZID:${calendar.timezone}`);
    // Note: Full timezone definitions would be added here
    // For simplicity, we're using a basic definition
    lines.push('END:VTIMEZONE');
  }

  // Add events
  for (const event of events) {
    const attendees = attendeesMap.get(event.id) ?? [];
    lines.push(generateVEvent(event, attendees));
  }

  lines.push('END:VCALENDAR');

  // Fold all header lines
  const header = lines.slice(0, lines.indexOf('BEGIN:VEVENT') || lines.length)
    .map(foldLine)
    .join('\r\n');

  const eventContent = lines.slice(lines.indexOf('BEGIN:VEVENT'))
    .join('\r\n');

  return header + (eventContent ? '\r\n' + eventContent : '');
}

/**
 * Generate iCalendar for a single event (useful for invitations)
 */
export function generateEventICalendar(event: CalendarEventRecord, attendees: AttendeeRecord[]): string {
  const lines: string[] = [];

  lines.push('BEGIN:VCALENDAR');
  lines.push('VERSION:2.0');
  lines.push('PRODID:-//nself//Calendar Plugin//EN');
  lines.push('METHOD:REQUEST');

  lines.push(generateVEvent(event, attendees));

  lines.push('END:VCALENDAR');

  return lines.map(foldLine).join('\r\n');
}
