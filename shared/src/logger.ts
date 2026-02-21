/**
 * Logging utilities for nself plugins
 */

import type { LogLevel } from './types.js';

const LOG_LEVELS: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
};

const COLORS = {
  reset: '\x1b[0m',
  red: '\x1b[31m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  magenta: '\x1b[35m',
  cyan: '\x1b[36m',
  gray: '\x1b[90m',
};

export class Logger {
  private name: string;
  private level: LogLevel;
  private useColors: boolean;

  constructor(name: string, level: LogLevel = 'info', useColors = true) {
    this.name = name;
    this.level = level;
    this.useColors = useColors && process.stdout.isTTY;
  }

  private shouldLog(level: LogLevel): boolean {
    return LOG_LEVELS[level] >= LOG_LEVELS[this.level];
  }

  private formatTimestamp(): string {
    return new Date().toISOString();
  }

  private colorize(text: string, color: keyof typeof COLORS): string {
    if (!this.useColors) return text;
    return `${COLORS[color]}${text}${COLORS.reset}`;
  }

  private formatMessage(level: LogLevel, message: string, meta?: Record<string, unknown>): string {
    const timestamp = this.colorize(this.formatTimestamp(), 'gray');
    const levelStr = this.formatLevel(level);
    const name = this.colorize(`[${this.name}]`, 'cyan');

    let output = `${timestamp} ${levelStr} ${name} ${message}`;

    if (meta && Object.keys(meta).length > 0) {
      output += ` ${this.colorize(JSON.stringify(meta), 'gray')}`;
    }

    return output;
  }

  private formatLevel(level: LogLevel): string {
    const colors: Record<LogLevel, keyof typeof COLORS> = {
      debug: 'gray',
      info: 'blue',
      warn: 'yellow',
      error: 'red',
    };

    const labels: Record<LogLevel, string> = {
      debug: 'DEBUG',
      info: 'INFO ',
      warn: 'WARN ',
      error: 'ERROR',
    };

    return this.colorize(labels[level], colors[level]);
  }

  debug(message: string, meta?: Record<string, unknown>): void {
    if (this.shouldLog('debug')) {
      console.log(this.formatMessage('debug', message, meta));
    }
  }

  info(message: string, meta?: Record<string, unknown>): void {
    if (this.shouldLog('info')) {
      console.log(this.formatMessage('info', message, meta));
    }
  }

  warn(message: string, meta?: Record<string, unknown>): void {
    if (this.shouldLog('warn')) {
      console.warn(this.formatMessage('warn', message, meta));
    }
  }

  error(message: string, meta?: Record<string, unknown>): void {
    if (this.shouldLog('error')) {
      console.error(this.formatMessage('error', message, meta));
    }
  }

  success(message: string, meta?: Record<string, unknown>): void {
    if (this.shouldLog('info')) {
      const timestamp = this.colorize(this.formatTimestamp(), 'gray');
      const levelStr = this.colorize('OK   ', 'green');
      const name = this.colorize(`[${this.name}]`, 'cyan');

      let output = `${timestamp} ${levelStr} ${name} ${message}`;
      if (meta && Object.keys(meta).length > 0) {
        output += ` ${this.colorize(JSON.stringify(meta), 'gray')}`;
      }

      console.log(output);
    }
  }

  child(name: string): Logger {
    return new Logger(`${this.name}:${name}`, this.level, this.useColors);
  }

  setLevel(level: LogLevel): void {
    this.level = level;
  }
}

export function createLogger(name: string, level?: LogLevel): Logger {
  return new Logger(name, level ?? (process.env.LOG_LEVEL as LogLevel) ?? 'info');
}
