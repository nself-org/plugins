# Meetings Plugin - Full Implementation Guide

## Overview

The Meetings plugin provides calendar synchronization and meeting management with support for **Google Calendar** and **Microsoft Outlook**. Enables event sync, availability checking, and meeting creation.

## Current Status

**Infrastructure Status**: ✅ Complete (database, API endpoints)
**Provider Integration Status**: ⚠️ Placeholder (requires OAuth and SDK implementation)

## What's Already Built

- ✅ Database schema for calendar events, attendees, sync tracking
- ✅ REST API endpoints
- ✅ Multi-tenant support
- ✅ Conflict detection logic

## What Needs Implementation

**Provider SDK Integration**:
- OAuth 2.0 authentication flow
- `syncEvents()` - Fetch calendar events
- `createEvent()` - Create meeting
- `updateEvent()` - Update meeting
- `deleteEvent()` - Cancel meeting
- `checkAvailability()` - Free/busy lookup

---

## Required Packages

```bash
# Google Calendar API
pnpm add googleapis @google-cloud/local-auth

# Microsoft Graph API (Outlook)
pnpm add @microsoft/microsoft-graph-client @azure/msal-node

# OAuth helpers
pnpm add axios date-fns
```

---

## Complete Implementation Code

### 1. Provider Integration Module

Create `ts/src/providers.ts`:

```typescript
/**
 * Calendar Provider Integration
 * Supports Google Calendar and Microsoft Outlook
 */

import { google, calendar_v3 } from 'googleapis';
import { OAuth2Client } from 'google-auth-library';
import { Client as GraphClient } from '@microsoft/microsoft-graph-client';
import { ConfidentialClientApplication } from '@azure/msal-node';
import { parseISO, formatISO } from 'date-fns';

export interface CalendarEvent {
  external_id: string;
  calendar_id: string;
  title: string;
  description?: string;
  location?: string;
  start_time: Date;
  end_time: Date;
  all_day: boolean;
  attendees?: string[];
  status: 'confirmed' | 'tentative' | 'cancelled';
  meeting_url?: string;
}

export interface CreateEventRequest {
  calendar_id: string;
  title: string;
  description?: string;
  location?: string;
  start_time: Date;
  end_time: Date;
  attendees?: string[];
}

/**
 * Google Calendar Provider
 */
export class GoogleCalendarProvider {
  private oauth2Client: OAuth2Client;
  private calendar: calendar_v3.Calendar;

  constructor(credentials: { clientId: string; clientSecret: string; redirectUri: string }) {
    this.oauth2Client = new google.auth.OAuth2(
      credentials.clientId,
      credentials.clientSecret,
      credentials.redirectUri
    );

    this.calendar = google.calendar({ version: 'v3', auth: this.oauth2Client });
  }

  /**
   * Set access token from OAuth flow
   */
  setTokens(tokens: { access_token: string; refresh_token?: string }) {
    this.oauth2Client.setCredentials(tokens);
  }

  /**
   * Get authorization URL for OAuth flow
   */
  getAuthUrl(): string {
    return this.oauth2Client.generateAuthUrl({
      access_type: 'offline',
      scope: ['https://www.googleapis.com/auth/calendar.readonly'],
    });
  }

  /**
   * Exchange authorization code for tokens
   */
  async getTokens(code: string) {
    const { tokens } = await this.oauth2Client.getToken(code);
    return tokens;
  }

  /**
   * Sync calendar events
   */
  async syncEvents(calendarId: string, since?: Date, until?: Date): Promise<CalendarEvent[]> {
    try {
      const response = await this.calendar.events.list({
        calendarId,
        timeMin: since ? formatISO(since) : undefined,
        timeMax: until ? formatISO(until) : undefined,
        singleEvents: true,
        orderBy: 'startTime',
      });

      const events = response.data.items ?? [];

      return events.map(event => this.parseGoogleEvent(event, calendarId));
    } catch (error) {
      throw new Error(`Google Calendar sync failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Create calendar event
   */
  async createEvent(request: CreateEventRequest): Promise<CalendarEvent> {
    try {
      const event = {
        summary: request.title,
        description: request.description,
        location: request.location,
        start: {
          dateTime: formatISO(request.start_time),
          timeZone: 'UTC',
        },
        end: {
          dateTime: formatISO(request.end_time),
          timeZone: 'UTC',
        },
        attendees: request.attendees?.map(email => ({ email })),
      };

      const response = await this.calendar.events.insert({
        calendarId: request.calendar_id,
        requestBody: event,
      });

      return this.parseGoogleEvent(response.data, request.calendar_id);
    } catch (error) {
      throw new Error(`Google Calendar create event failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Update event
   */
  async updateEvent(calendarId: string, eventId: string, updates: Partial<CreateEventRequest>): Promise<CalendarEvent> {
    try {
      const event: calendar_v3.Schema$Event = {};

      if (updates.title) event.summary = updates.title;
      if (updates.description) event.description = updates.description;
      if (updates.location) event.location = updates.location;
      if (updates.start_time) event.start = { dateTime: formatISO(updates.start_time), timeZone: 'UTC' };
      if (updates.end_time) event.end = { dateTime: formatISO(updates.end_time), timeZone: 'UTC' };
      if (updates.attendees) event.attendees = updates.attendees.map(email => ({ email }));

      const response = await this.calendar.events.patch({
        calendarId,
        eventId,
        requestBody: event,
      });

      return this.parseGoogleEvent(response.data, calendarId);
    } catch (error) {
      throw new Error(`Google Calendar update event failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Delete event
   */
  async deleteEvent(calendarId: string, eventId: string): Promise<void> {
    try {
      await this.calendar.events.delete({ calendarId, eventId });
    } catch (error) {
      throw new Error(`Google Calendar delete event failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Check availability (free/busy)
   */
  async checkAvailability(calendarId: string, start: Date, end: Date): Promise<Array<{ start: Date; end: Date }>> {
    try {
      const response = await this.calendar.freebusy.query({
        requestBody: {
          timeMin: formatISO(start),
          timeMax: formatISO(end),
          items: [{ id: calendarId }],
        },
      });

      const busy = response.data.calendars?.[calendarId]?.busy ?? [];

      return busy.map(period => ({
        start: parseISO(period.start!),
        end: parseISO(period.end!),
      }));
    } catch (error) {
      throw new Error(`Google Calendar availability check failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Parse Google Calendar event to CalendarEvent
   */
  private parseGoogleEvent(event: calendar_v3.Schema$Event, calendarId: string): CalendarEvent {
    const start = event.start?.dateTime ? parseISO(event.start.dateTime) : parseISO(event.start?.date!);
    const end = event.end?.dateTime ? parseISO(event.end.dateTime) : parseISO(event.end?.date!);

    return {
      external_id: event.id!,
      calendar_id: calendarId,
      title: event.summary ?? 'Untitled Event',
      description: event.description,
      location: event.location,
      start_time: start,
      end_time: end,
      all_day: !event.start?.dateTime,
      attendees: event.attendees?.map(a => a.email!),
      status: event.status === 'confirmed' ? 'confirmed' : 'tentative',
      meeting_url: event.hangoutLink,
    };
  }
}

/**
 * Microsoft Outlook Provider
 */
export class OutlookProvider {
  private msalClient: ConfidentialClientApplication;
  private graphClient?: GraphClient;

  constructor(credentials: { clientId: string; clientSecret: string; tenantId: string }) {
    this.msalClient = new ConfidentialClientApplication({
      auth: {
        clientId: credentials.clientId,
        clientSecret: credentials.clientSecret,
        authority: `https://login.microsoftonline.com/${credentials.tenantId}`,
      },
    });
  }

  /**
   * Set access token
   */
  setAccessToken(accessToken: string) {
    this.graphClient = GraphClient.init({
      authProvider: done => done(null, accessToken),
    });
  }

  /**
   * Get authorization URL
   */
  getAuthUrl(redirectUri: string): string {
    return `https://login.microsoftonline.com/common/oauth2/v2.0/authorize?client_id=${this.msalClient.config.auth.clientId}&response_type=code&redirect_uri=${redirectUri}&scope=Calendars.ReadWrite`;
  }

  /**
   * Exchange code for token
   */
  async getTokens(code: string, redirectUri: string) {
    const result = await this.msalClient.acquireTokenByCode({
      code,
      scopes: ['Calendars.ReadWrite'],
      redirectUri,
    });

    return {
      access_token: result!.accessToken,
      refresh_token: result!.refreshToken,
    };
  }

  /**
   * Sync calendar events
   */
  async syncEvents(calendarId: string, since?: Date, until?: Date): Promise<CalendarEvent[]> {
    if (!this.graphClient) {
      throw new Error('Access token not set');
    }

    try {
      let query = `/me/calendars/${calendarId}/events?$orderby=start/dateTime`;
      if (since) query += `&$filter=start/dateTime ge ${formatISO(since)}`;
      if (until) query += ` and end/dateTime le ${formatISO(until)}`;

      const response = await this.graphClient.api(query).get();

      return response.value.map((event: Record<string, unknown>) => this.parseOutlookEvent(event, calendarId));
    } catch (error) {
      throw new Error(`Outlook sync failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Create event
   */
  async createEvent(request: CreateEventRequest): Promise<CalendarEvent> {
    if (!this.graphClient) {
      throw new Error('Access token not set');
    }

    try {
      const event = {
        subject: request.title,
        body: {
          contentType: 'HTML',
          content: request.description ?? '',
        },
        start: {
          dateTime: formatISO(request.start_time),
          timeZone: 'UTC',
        },
        end: {
          dateTime: formatISO(request.end_time),
          timeZone: 'UTC',
        },
        location: {
          displayName: request.location,
        },
        attendees: request.attendees?.map(email => ({
          emailAddress: { address: email },
          type: 'required',
        })),
      };

      const response = await this.graphClient
        .api(`/me/calendars/${request.calendar_id}/events`)
        .post(event);

      return this.parseOutlookEvent(response, request.calendar_id);
    } catch (error) {
      throw new Error(`Outlook create event failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Parse Outlook event
   */
  private parseOutlookEvent(event: Record<string, unknown>, calendarId: string): CalendarEvent {
    const start = event.start as { dateTime: string };
    const end = event.end as { dateTime: string };

    return {
      external_id: event.id as string,
      calendar_id: calendarId,
      title: event.subject as string,
      description: (event.body as { content: string })?.content,
      location: (event.location as { displayName: string })?.displayName,
      start_time: parseISO(start.dateTime),
      end_time: parseISO(end.dateTime),
      all_day: event.isAllDay as boolean,
      attendees: (event.attendees as Array<{ emailAddress: { address: string } }>)?.map(a => a.emailAddress.address),
      status: event.responseStatus === 'accepted' ? 'confirmed' : 'tentative',
      meeting_url: (event.onlineMeeting as { joinUrl: string })?.joinUrl,
    };
  }
}

/**
 * Provider factory
 */
export function createMeetingsProvider(
  provider: string,
  config: Record<string, string>
): GoogleCalendarProvider | OutlookProvider {
  switch (provider.toLowerCase()) {
    case 'google':
      if (!config.MEETINGS_GOOGLE_CLIENT_ID || !config.MEETINGS_GOOGLE_CLIENT_SECRET) {
        throw new Error('Google Calendar credentials required');
      }
      return new GoogleCalendarProvider({
        clientId: config.MEETINGS_GOOGLE_CLIENT_ID,
        clientSecret: config.MEETINGS_GOOGLE_CLIENT_SECRET,
        redirectUri: config.MEETINGS_GOOGLE_REDIRECT_URI ?? 'http://localhost:3000/oauth/callback',
      });

    case 'outlook':
      if (!config.MEETINGS_OUTLOOK_CLIENT_ID || !config.MEETINGS_OUTLOOK_CLIENT_SECRET || !config.MEETINGS_OUTLOOK_TENANT_ID) {
        throw new Error('Outlook credentials required');
      }
      return new OutlookProvider({
        clientId: config.MEETINGS_OUTLOOK_CLIENT_ID,
        clientSecret: config.MEETINGS_OUTLOOK_CLIENT_SECRET,
        tenantId: config.MEETINGS_OUTLOOK_TENANT_ID,
      });

    default:
      throw new Error(`Unsupported meetings provider: ${provider}`);
  }
}
```

---

## Configuration

### Environment Variables

**Google Calendar**:
```bash
MEETINGS_PROVIDERS=google
MEETINGS_GOOGLE_CLIENT_ID=xxx.apps.googleusercontent.com
MEETINGS_GOOGLE_CLIENT_SECRET=GOCSPX-xxx
MEETINGS_GOOGLE_REDIRECT_URI=http://localhost:3000/oauth/callback
```

**Microsoft Outlook**:
```bash
MEETINGS_PROVIDERS=outlook
MEETINGS_OUTLOOK_CLIENT_ID=xxx
MEETINGS_OUTLOOK_CLIENT_SECRET=xxx
MEETINGS_OUTLOOK_TENANT_ID=common
```

### Get API Credentials

**Google Calendar**:
1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create project
3. Enable **Google Calendar API**
4. Create **OAuth 2.0 credentials**
5. Add redirect URI: `http://localhost:3000/oauth/callback`

**Microsoft Outlook**:
1. Go to [Azure Portal](https://portal.azure.com/)
2. Register app in **Azure AD**
3. Add **Calendars.ReadWrite** permission
4. Create client secret

---

## OAuth Flow

Users must authorize the app to access their calendar. Implement OAuth callback endpoint:

```typescript
app.get('/oauth/callback', async (request, reply) => {
  const { code } = request.query as { code: string };

  const provider = createMeetingsProvider('google', process.env as Record<string, string>);
  const tokens = await provider.getTokens(code);

  // Store tokens in database for user
  await db.storeUserTokens(userId, tokens);

  reply.redirect('/calendar');
});
```

---

## Testing

```bash
cd plugins/meetings/ts
pnpm install
pnpm add googleapis @microsoft/microsoft-graph-client @azure/msal-node axios date-fns
pnpm build
pnpm start
```

**Initiate OAuth**:
```bash
# Visit in browser
http://localhost:3000/oauth/google
```

**Sync Calendar**:
```bash
curl -X POST http://localhost:3000/sync \
  -H "X-API-Key: test-key" \
  -H "X-User-Id: user_123"
```

---

## Activation Checklist

- [ ] Install SDKs
- [ ] Create `providers.ts`
- [ ] Implement OAuth callback endpoints
- [ ] Add credentials to `.env`
- [ ] Build & start
- [ ] Test OAuth flow
- [ ] Test event sync

---

## Security Notes

- **Never commit OAuth secrets** to git
- Store user tokens encrypted in database
- Refresh tokens before expiry
- Use HTTPS for redirect URIs in production
