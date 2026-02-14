/**
 * EPG Recording Trigger
 * Orchestrates recording rules, scheduled recordings, conflict detection,
 * and AntServer notification.
 */

import { createLogger } from '@nself/plugin-utils';
import type { EpgDatabase } from './database.js';
import type {
  ProgramRecord,
  ScheduledRecordingRecord,
} from './types.js';

const logger = createLogger('epg:recording-trigger');

/**
 * Schedule a recording for a specific program.
 * Gets program/schedule details, checks for conflicts,
 * creates a scheduled recording entry, and notifies AntServer.
 */
export async function scheduleRecording(
  db: EpgDatabase,
  programId: string,
  ruleId: string | null,
  channelId: string,
  antserverUrl?: string
): Promise<ScheduledRecordingRecord> {
  // Get program details
  const program = await db.getProgram(programId);
  if (!program) {
    throw new Error(`Program not found: ${programId}`);
  }

  // Get the schedule entry for this program on this channel
  const scheduleResult = await db.query<{
    start_time: Date;
    end_time: Date;
  }>(
    `SELECT start_time, end_time FROM np_epg_schedules
     WHERE source_account_id = $1
       AND program_id = $2
       AND channel_id = $3
       AND start_time >= NOW()
     ORDER BY start_time ASC
     LIMIT 1`,
    [db.getCurrentSourceAccountId(), programId, channelId]
  );

  if (scheduleResult.rows.length === 0) {
    throw new Error(`No upcoming schedule found for program ${programId} on channel ${channelId}`);
  }

  const scheduleEntry = scheduleResult.rows[0];

  // Apply padding from the rule if one exists
  let paddingStartMinutes = 1;
  let paddingEndMinutes = 3;

  if (ruleId) {
    const rule = await db.getRecordingRule(ruleId);
    if (rule) {
      paddingStartMinutes = rule.start_padding_minutes;
      paddingEndMinutes = rule.end_padding_minutes;
    }
  }

  const scheduledStart = new Date(scheduleEntry.start_time.getTime() - paddingStartMinutes * 60 * 1000);
  const scheduledEnd = new Date(scheduleEntry.end_time.getTime() + paddingEndMinutes * 60 * 1000);

  // Wrap conflict check + recording creation in a transaction to prevent race conditions.
  // Lock relevant rows to ensure atomicity of the check-and-create operation.
  let recording: ScheduledRecordingRecord;
  let status: 'scheduled' | 'conflict' = 'scheduled';

  try {
    await db.execute('BEGIN');

    // Lock existing scheduled/recording entries for this channel to prevent concurrent conflicts
    await db.query(
      `SELECT id FROM np_epg_scheduled_recordings
       WHERE channel_id = $1 AND status IN ('scheduled', 'recording')
       FOR UPDATE`,
      [channelId]
    );

    // Check for conflicts within the transaction
    const conflicts = await checkConflicts(db, scheduledStart, scheduledEnd, channelId);

    if (conflicts.length > 0) {
      logger.warn('Scheduling conflict detected', {
        programId,
        channelId,
        conflictCount: conflicts.length,
      });
      status = 'conflict';
    }

    // Create the scheduled recording (uses ON CONFLICT for duplicate safety)
    recording = await db.createScheduledRecording({
      recording_rule_id: ruleId,
      program_id: programId,
      channel_id: channelId,
      scheduled_start: scheduledStart,
      scheduled_end: scheduledEnd,
      status,
    });

    await db.execute('COMMIT');
  } catch (error) {
    await db.execute('ROLLBACK');
    throw error;
  }

  logger.info('Recording scheduled', {
    recordingId: recording.id,
    programTitle: program.title,
    status,
    scheduledStart: scheduledStart.toISOString(),
    scheduledEnd: scheduledEnd.toISOString(),
  });

  // Notify AntServer if configured and no conflicts
  if (status === 'scheduled' && antserverUrl) {
    const jobId = await notifyAntServer(antserverUrl, recording);
    if (jobId) {
      await db.updateScheduledRecording(recording.id, { antserver_job_id: jobId });
      recording.antserver_job_id = jobId;
    }
  }

  return recording;
}

/**
 * Check for scheduling conflicts in a given time range.
 */
export async function checkConflicts(
  db: EpgDatabase,
  start: Date,
  end: Date,
  channelId?: string,
  excludeId?: string
): Promise<ScheduledRecordingRecord[]> {
  return db.findConflicts(start, end, channelId, excludeId);
}

/**
 * Resolve conflicts using priority-based resolution.
 * Higher priority recordings win; lower priority recordings
 * get their status set to 'conflict'.
 */
export async function resolveConflicts(
  db: EpgDatabase,
  recordings: ScheduledRecordingRecord[]
): Promise<void> {
  if (recordings.length <= 1) return;

  // Get priorities from associated rules
  const recordingsWithPriority: Array<{
    recording: ScheduledRecordingRecord;
    priority: number;
  }> = [];

  for (const recording of recordings) {
    let priority = 50; // default priority
    if (recording.recording_rule_id) {
      const rule = await db.getRecordingRule(recording.recording_rule_id);
      if (rule) {
        priority = rule.priority;
      }
    }
    recordingsWithPriority.push({ recording, priority });
  }

  // Sort by priority descending (highest priority first)
  recordingsWithPriority.sort((a, b) => b.priority - a.priority);

  // The first (highest priority) recording stays scheduled
  // All others become 'conflict'
  const winner = recordingsWithPriority[0];
  logger.info('Resolving conflict', {
    winnerId: winner.recording.id,
    winnerPriority: winner.priority,
  });

  // Ensure winner is scheduled
  if (winner.recording.status === 'conflict') {
    await db.updateScheduledRecording(winner.recording.id, { status: 'scheduled' });
  }

  // Mark losers as conflict
  for (let i = 1; i < recordingsWithPriority.length; i++) {
    const loser = recordingsWithPriority[i];
    if (loser.recording.status !== 'conflict' && loser.recording.status !== 'cancelled') {
      await db.updateScheduledRecording(loser.recording.id, { status: 'conflict' });
      logger.info('Recording marked as conflict', {
        recordingId: loser.recording.id,
        priority: loser.priority,
      });
    }
  }
}

/**
 * Match series/keyword rules against newly imported programs.
 * Auto-schedules recordings for any matches found.
 * Returns the count of new recordings scheduled.
 */
export async function matchSeriesRules(
  db: EpgDatabase,
  newPrograms: ProgramRecord[],
  antserverUrl?: string
): Promise<number> {
  if (newPrograms.length === 0) return 0;

  const matches = await db.matchRulesAgainstPrograms(newPrograms);

  if (matches.length === 0) {
    logger.debug('No recording rule matches found for imported programs');
    return 0;
  }

  let scheduledCount = 0;

  for (const match of matches) {
    try {
      // Check if we already have a recording for this program+channel+time combo
      const existingConflicts = await db.findConflicts(
        match.schedule.start_time,
        match.schedule.end_time,
        match.schedule.channel_id
      );

      const alreadyScheduled = existingConflicts.some(
        r => r.program_id === match.program.id && r.channel_id === match.schedule.channel_id
      );

      if (alreadyScheduled) {
        logger.debug('Recording already exists for matched program', {
          programId: match.program.id,
          ruleId: match.rule.id,
        });
        continue;
      }

      await scheduleRecording(
        db,
        match.program.id,
        match.rule.id,
        match.schedule.channel_id,
        antserverUrl
      );
      scheduledCount++;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to auto-schedule recording from rule match', {
        ruleId: match.rule.id,
        programId: match.program.id,
        error: message,
      });
    }
  }

  logger.info('Series/keyword rule matching complete', {
    programsChecked: newPrograms.length,
    matchesFound: matches.length,
    recordingsScheduled: scheduledCount,
  });

  return scheduledCount;
}

/**
 * Notify AntServer about a scheduled recording via HTTP POST.
 * Returns the antserver_job_id on success, null on failure.
 * Gracefully handles unreachable AntServer (logs warning, does not fail).
 */
export async function notifyAntServer(
  url: string,
  recording: ScheduledRecordingRecord
): Promise<string | null> {
  if (!url) return null;

  const endpoint = `${url.replace(/\/+$/, '')}/api/recordings`;

  try {
    const response = await fetch(endpoint, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        recording_id: recording.id,
        channel_id: recording.channel_id,
        program_id: recording.program_id,
        scheduled_start: recording.scheduled_start,
        scheduled_end: recording.scheduled_end,
      }),
      signal: AbortSignal.timeout(10000),
    });

    if (!response.ok) {
      logger.warn('AntServer notification failed with non-OK status', {
        status: response.status,
        recordingId: recording.id,
      });
      return null;
    }

    const data = await response.json() as { job_id?: string };
    const jobId = data.job_id ?? null;

    if (jobId) {
      logger.info('AntServer notified successfully', {
        recordingId: recording.id,
        jobId,
      });
    }

    return jobId;
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.warn('AntServer unreachable, recording will proceed without notification', {
      url: endpoint,
      recordingId: recording.id,
      error: message,
    });
    return null;
  }
}
