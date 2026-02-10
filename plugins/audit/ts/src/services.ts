/**
 * Audit Plugin Services
 * SIEM integration, compliance reports, and export functionality
 */

import { createLogger } from '@nself/plugin-utils';
import { AuditDatabase } from './database.js';
import {
  AuditEventRecord,
  ExportFormat,
  ComplianceFramework,
  ComplianceReportResponse,
  ComplianceCheck,
  SiemExportResult,
} from './types.js';
import { config } from './config.js';

const logger = createLogger('audit:services');

export class AuditService {
  constructor(private db: AuditDatabase) {}

  /**
   * Export events to various formats
   */
  async exportEvents(
    events: AuditEventRecord[],
    format: ExportFormat
  ): Promise<string> {
    switch (format) {
      case 'csv':
        return this.exportCsv(events);
      case 'json':
        return this.exportJson(events);
      case 'jsonl':
        return this.exportJsonl(events);
      case 'cef':
        return this.exportCef(events);
      case 'leef':
        return this.exportLeef(events);
      case 'syslog':
        return this.exportSyslog(events);
      default:
        throw new Error(`Unsupported export format: ${format}`);
    }
  }

  /**
   * Export to CSV
   */
  private exportCsv(events: AuditEventRecord[]): string {
    if (events.length === 0) return '';

    const headers = [
      'id',
      'app_id',
      'source_plugin',
      'event_type',
      'actor_id',
      'actor_type',
      'resource_type',
      'resource_id',
      'action',
      'outcome',
      'severity',
      'ip_address',
      'user_agent',
      'location',
      'checksum',
      'created_at',
    ];

    const rows = events.map((event) => [
      event.id,
      event.app_id,
      event.source_plugin,
      event.event_type,
      event.actor_id || '',
      event.actor_type || '',
      event.resource_type || '',
      event.resource_id || '',
      event.action,
      event.outcome,
      event.severity,
      event.ip_address || '',
      event.user_agent || '',
      event.location || '',
      event.checksum,
      event.created_at.toISOString(),
    ]);

    return [headers.join(','), ...rows.map((row) => row.map(this.escapeCsv).join(','))].join('\n');
  }

  private escapeCsv(value: string): string {
    if (value.includes(',') || value.includes('"') || value.includes('\n')) {
      return `"${value.replace(/"/g, '""')}"`;
    }
    return value;
  }

  /**
   * Export to JSON
   */
  private exportJson(events: AuditEventRecord[]): string {
    return JSON.stringify(events, null, 2);
  }

  /**
   * Export to JSON Lines (JSONL)
   */
  private exportJsonl(events: AuditEventRecord[]): string {
    return events.map((event) => JSON.stringify(event)).join('\n');
  }

  /**
   * Export to CEF (Common Event Format) for SIEM
   */
  private exportCef(events: AuditEventRecord[]): string {
    return events
      .map((event) => {
        const severity = this.mapSeverityToCef(event.severity);
        const extensions = [
          `suser=${event.actor_id || 'unknown'}`,
          `src=${event.ip_address || 'unknown'}`,
          `act=${event.action}`,
          `outcome=${event.outcome}`,
          `rt=${event.created_at.getTime()}`,
          `deviceCustomString1Label=ResourceType`,
          `deviceCustomString1=${event.resource_type || 'unknown'}`,
          `deviceCustomString2Label=ResourceId`,
          `deviceCustomString2=${event.resource_id || 'unknown'}`,
          `deviceCustomString3Label=Checksum`,
          `deviceCustomString3=${event.checksum}`,
        ].join(' ');

        return `CEF:0|nself|audit|1.0|${event.event_type}|${event.action}|${severity}|${extensions}`;
      })
      .join('\n');
  }

  private mapSeverityToCef(severity: string): number {
    const mapping: Record<string, number> = {
      low: 3,
      medium: 5,
      high: 7,
      critical: 10,
    };
    return mapping[severity] || 5;
  }

  /**
   * Export to LEEF (Log Event Extended Format) for IBM QRadar
   */
  private exportLeef(events: AuditEventRecord[]): string {
    return events
      .map((event) => {
        const fields = [
          `devTime=${event.created_at.toISOString()}`,
          `src=${event.ip_address || 'unknown'}`,
          `sev=${this.mapSeverityToCef(event.severity)}`,
          `cat=${event.event_type}`,
          `usrName=${event.actor_id || 'unknown'}`,
          `action=${event.action}`,
          `outcome=${event.outcome}`,
          `resourceType=${event.resource_type || 'unknown'}`,
          `resourceId=${event.resource_id || 'unknown'}`,
          `checksum=${event.checksum}`,
        ].join('\t');

        return `LEEF:2.0|nself|audit|1.0|${event.event_type}|${fields}`;
      })
      .join('\n');
  }

  /**
   * Export to Syslog format (RFC 5424)
   */
  private exportSyslog(events: AuditEventRecord[]): string {
    return events
      .map((event) => {
        const priority = this.calculateSyslogPriority(event.severity);
        const timestamp = event.created_at.toISOString();
        const hostname = 'nself-audit';
        const appName = event.source_plugin;
        const procId = event.id;
        const msgId = event.event_type;
        const structuredData = `[audit actor="${event.actor_id || 'unknown'}" resource="${event.resource_type || 'unknown'}:${event.resource_id || 'unknown'}" action="${event.action}" outcome="${event.outcome}" checksum="${event.checksum}"]`;
        const message = `${event.action} on ${event.resource_type}:${event.resource_id}`;

        return `<${priority}>1 ${timestamp} ${hostname} ${appName} ${procId} ${msgId} ${structuredData} ${message}`;
      })
      .join('\n');
  }

  private calculateSyslogPriority(severity: string): number {
    // Facility: User (1), Severity: based on audit severity
    const facility = 1;
    const severityMap: Record<string, number> = {
      critical: 2, // Critical
      high: 3, // Error
      medium: 5, // Notice
      low: 6, // Informational
    };
    const syslogSeverity = severityMap[severity] || 6;
    return facility * 8 + syslogSeverity;
  }

  /**
   * Send events to SIEM providers
   */
  async sendToSiem(events: AuditEventRecord[]): Promise<SiemExportResult[]> {
    const results: SiemExportResult[] = [];

    // Splunk HEC
    if (config.siem.splunk) {
      try {
        await this.sendToSplunk(events);
        results.push({
          provider: 'splunk',
          success: true,
          eventCount: events.length,
        });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to send to Splunk', { error: message });
        results.push({
          provider: 'splunk',
          success: false,
          eventCount: 0,
          error: message,
        });
      }
    }

    // Elasticsearch/ELK
    if (config.siem.elk) {
      try {
        await this.sendToElk(events);
        results.push({
          provider: 'elk',
          success: true,
          eventCount: events.length,
        });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to send to ELK', { error: message });
        results.push({
          provider: 'elk',
          success: false,
          eventCount: 0,
          error: message,
        });
      }
    }

    // Datadog
    if (config.siem.datadog) {
      try {
        await this.sendToDatadog(events);
        results.push({
          provider: 'datadog',
          success: true,
          eventCount: events.length,
        });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to send to Datadog', { error: message });
        results.push({
          provider: 'datadog',
          success: false,
          eventCount: 0,
          error: message,
        });
      }
    }

    return results;
  }

  private async sendToSplunk(events: AuditEventRecord[]): Promise<void> {
    if (!config.siem.splunk) return;

    const payload = events.map((event) => ({
      time: Math.floor(event.created_at.getTime() / 1000),
      source: event.source_plugin,
      sourcetype: 'nself:audit',
      event: {
        id: event.id,
        app_id: event.app_id,
        event_type: event.event_type,
        actor_id: event.actor_id,
        actor_type: event.actor_type,
        resource_type: event.resource_type,
        resource_id: event.resource_id,
        action: event.action,
        outcome: event.outcome,
        severity: event.severity,
        ip_address: event.ip_address,
        user_agent: event.user_agent,
        location: event.location,
        details: event.details,
        metadata: event.metadata,
        checksum: event.checksum,
      },
    }));

    const response = await fetch(config.siem.splunk.hecUrl, {
      method: 'POST',
      headers: {
        Authorization: `Splunk ${config.siem.splunk.hecToken}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(payload),
    });

    if (!response.ok) {
      throw new Error(`Splunk HEC returned ${response.status}: ${await response.text()}`);
    }
  }

  private async sendToElk(events: AuditEventRecord[]): Promise<void> {
    if (!config.siem.elk) return;

    // Bulk index to Elasticsearch
    const bulkBody = events.flatMap((event) => [
      { index: { _index: config.siem.elk!.index } },
      {
        '@timestamp': event.created_at.toISOString(),
        id: event.id,
        app_id: event.app_id,
        source_plugin: event.source_plugin,
        event_type: event.event_type,
        actor_id: event.actor_id,
        actor_type: event.actor_type,
        resource_type: event.resource_type,
        resource_id: event.resource_id,
        action: event.action,
        outcome: event.outcome,
        severity: event.severity,
        ip_address: event.ip_address,
        user_agent: event.user_agent,
        location: event.location,
        details: event.details,
        metadata: event.metadata,
        checksum: event.checksum,
      },
    ]);

    const response = await fetch(`${config.siem.elk.url}/_bulk`, {
      method: 'POST',
      headers: {
        Authorization: `ApiKey ${config.siem.elk.apiKey}`,
        'Content-Type': 'application/x-ndjson',
      },
      body: bulkBody.map((line) => JSON.stringify(line)).join('\n') + '\n',
    });

    if (!response.ok) {
      throw new Error(`Elasticsearch returned ${response.status}: ${await response.text()}`);
    }
  }

  private async sendToDatadog(events: AuditEventRecord[]): Promise<void> {
    if (!config.siem.datadog) return;

    const payload = events.map((event) => ({
      ddsource: 'nself',
      ddtags: `app_id:${event.app_id},source_plugin:${event.source_plugin},severity:${event.severity}`,
      hostname: 'nself-audit',
      message: `${event.action} on ${event.resource_type}:${event.resource_id}`,
      service: 'audit',
      timestamp: event.created_at.getTime(),
      ...event,
    }));

    const response = await fetch(`https://http-intake.logs.${config.siem.datadog.site}/api/v2/logs`, {
      method: 'POST',
      headers: {
        'DD-API-KEY': config.siem.datadog.apiKey,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(payload),
    });

    if (!response.ok) {
      throw new Error(`Datadog returned ${response.status}: ${await response.text()}`);
    }
  }

  /**
   * Generate compliance report
   */
  async generateComplianceReport(
    framework: ComplianceFramework,
    startDate: Date,
    endDate: Date
  ): Promise<ComplianceReportResponse> {
    // Query events for the period
    const { events, total } = await this.db.queryEvents({
      startDate: startDate.toISOString(),
      endDate: endDate.toISOString(),
      limit: 1000000, // Large limit for reports
    });

    // Calculate summary statistics
    const criticalEvents = events.filter((e) => e.severity === 'critical').length;
    const highSeverityEvents = events.filter((e) => e.severity === 'high').length;
    const failedActions = events.filter((e) => e.outcome === 'failure').length;

    const uniqueActors = new Set(events.map((e) => e.actor_id).filter(Boolean)).size;
    const uniqueResources = new Set(
      events.map((e) => `${e.resource_type}:${e.resource_id}`).filter((r) => r !== ':')
    ).size;

    // Event distribution
    const eventsByType: Record<string, number> = {};
    const eventsBySeverity: Record<string, number> = {};
    const eventsByOutcome: Record<string, number> = {};

    for (const event of events) {
      eventsByType[event.event_type] = (eventsByType[event.event_type] || 0) + 1;
      eventsBySeverity[event.severity] = (eventsBySeverity[event.severity] || 0) + 1;
      eventsByOutcome[event.outcome] = (eventsByOutcome[event.outcome] || 0) + 1;
    }

    // Top actors
    const actorCounts: Record<string, number> = {};
    for (const event of events) {
      if (event.actor_id) {
        actorCounts[event.actor_id] = (actorCounts[event.actor_id] || 0) + 1;
      }
    }
    const topActors = Object.entries(actorCounts)
      .sort((a, b) => b[1] - a[1])
      .slice(0, 10)
      .map(([actorId, eventCount]) => ({ actorId, eventCount }));

    // Top resources
    const resourceCounts: Record<string, number> = {};
    for (const event of events) {
      if (event.resource_type && event.resource_id) {
        const key = `${event.resource_type}:${event.resource_id}`;
        resourceCounts[key] = (resourceCounts[key] || 0) + 1;
      }
    }
    const topResources = Object.entries(resourceCounts)
      .sort((a, b) => b[1] - a[1])
      .slice(0, 10)
      .map(([resource, eventCount]) => {
        const [resourceType, resourceId] = resource.split(':');
        return { resourceType, resourceId, eventCount };
      });

    // Get alert rules triggered
    const alertRules = await this.db.getAlertRules();
    const alertsTriggered = alertRules.reduce((sum, rule) => sum + rule.trigger_count, 0);

    // Generate compliance checks based on framework
    const complianceChecks = this.generateComplianceChecks(framework, events);

    return {
      framework,
      period: {
        startDate: startDate.toISOString(),
        endDate: endDate.toISOString(),
      },
      summary: {
        totalEvents: total,
        criticalEvents,
        highSeverityEvents,
        failedActions,
        uniqueActors,
        uniqueResources,
      },
      eventsByType,
      eventsBySeverity,
      eventsByOutcome,
      topActors,
      topResources,
      alertsTriggered,
      complianceChecks,
      generatedAt: new Date().toISOString(),
    };
  }

  private generateComplianceChecks(
    framework: ComplianceFramework,
    events: AuditEventRecord[]
  ): ComplianceCheck[] {
    switch (framework) {
      case 'SOC2':
        return this.generateSoc2Checks(events);
      case 'HIPAA':
        return this.generateHipaaChecks(events);
      case 'GDPR':
        return this.generateGdprChecks(events);
      case 'PCI':
        return this.generatePciChecks(events);
      default:
        return [];
    }
  }

  private generateSoc2Checks(events: AuditEventRecord[]): ComplianceCheck[] {
    const checks: ComplianceCheck[] = [];

    // CC6.1: Logical access security
    const authEvents = events.filter((e) => e.source_plugin === 'auth');
    checks.push({
      control: 'CC6.1',
      requirement: 'Logical and physical access controls',
      status: authEvents.length > 0 ? 'pass' : 'fail',
      details: `${authEvents.length} authentication events logged`,
    });

    // CC7.2: System monitoring
    checks.push({
      control: 'CC7.2',
      requirement: 'System monitoring for security incidents',
      status: events.length > 0 ? 'pass' : 'warning',
      details: `${events.length} total audit events recorded`,
    });

    // CC7.3: Incident response
    const criticalEvents = events.filter((e) => e.severity === 'critical');
    checks.push({
      control: 'CC7.3',
      requirement: 'Evaluation and response to security incidents',
      status: criticalEvents.length === 0 ? 'pass' : 'warning',
      details: `${criticalEvents.length} critical security events require review`,
    });

    return checks;
  }

  private generateHipaaChecks(events: AuditEventRecord[]): ComplianceCheck[] {
    const checks: ComplianceCheck[] = [];

    // 164.312(b): Audit controls
    checks.push({
      control: '164.312(b)',
      requirement: 'Implement hardware, software, and/or procedural mechanisms that record and examine activity',
      status: events.length > 0 ? 'pass' : 'fail',
      details: `${events.length} audit events recorded`,
    });

    // 164.308(a)(1)(ii)(D): Information system activity review
    const last30Days = events.filter(
      (e) => new Date(e.created_at).getTime() > Date.now() - 30 * 24 * 60 * 60 * 1000
    );
    checks.push({
      control: '164.308(a)(1)(ii)(D)',
      requirement: 'Regular review of information system activity',
      status: last30Days.length > 0 ? 'pass' : 'warning',
      details: `${last30Days.length} events in last 30 days`,
    });

    // 164.312(a)(2)(i): Unique user identification
    const uniqueActors = new Set(events.map((e) => e.actor_id).filter(Boolean));
    checks.push({
      control: '164.312(a)(2)(i)',
      requirement: 'Unique user identification',
      status: uniqueActors.size > 0 ? 'pass' : 'warning',
      details: `${uniqueActors.size} unique actors identified`,
    });

    return checks;
  }

  private generateGdprChecks(events: AuditEventRecord[]): ComplianceCheck[] {
    const checks: ComplianceCheck[] = [];

    // Art. 30: Records of processing activities
    checks.push({
      control: 'Article 30',
      requirement: 'Records of processing activities',
      status: events.length > 0 ? 'pass' : 'fail',
      details: `${events.length} processing activities logged`,
    });

    // Art. 32: Security of processing
    const securityEvents = events.filter(
      (e) => e.event_type.includes('security') || e.severity === 'critical'
    );
    checks.push({
      control: 'Article 32',
      requirement: 'Security of processing',
      status: 'pass',
      details: `${securityEvents.length} security-related events monitored`,
    });

    // Art. 33: Notification of a personal data breach
    const breachEvents = events.filter((e) => e.event_type.includes('breach'));
    checks.push({
      control: 'Article 33',
      requirement: 'Breach notification',
      status: breachEvents.length === 0 ? 'pass' : 'warning',
      details: `${breachEvents.length} potential breach events detected`,
    });

    return checks;
  }

  private generatePciChecks(events: AuditEventRecord[]): ComplianceCheck[] {
    const checks: ComplianceCheck[] = [];

    // Requirement 10.1: Implement audit trails
    checks.push({
      control: '10.1',
      requirement: 'Implement audit trails to link all access to system components',
      status: events.length > 0 ? 'pass' : 'fail',
      details: `${events.length} audit trail entries`,
    });

    // Requirement 10.2: Automated audit trails for all system components
    const criticalActions = ['create', 'update', 'delete', 'access'];
    const criticalEvents = events.filter((e) => criticalActions.includes(e.action.toLowerCase()));
    checks.push({
      control: '10.2',
      requirement: 'Log critical security events',
      status: criticalEvents.length > 0 ? 'pass' : 'warning',
      details: `${criticalEvents.length} critical security events logged`,
    });

    // Requirement 10.3: Record audit trail entries
    const requiredFields = events.every(
      (e) => e.actor_id && e.event_type && e.created_at && e.outcome
    );
    checks.push({
      control: '10.3',
      requirement: 'Record required audit trail entries',
      status: requiredFields ? 'pass' : 'fail',
      details: 'User identification, event type, timestamp, and success/failure recorded',
    });

    return checks;
  }
}
