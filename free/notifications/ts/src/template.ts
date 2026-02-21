/**
 * Template engine using Handlebars
 */

import Handlebars from 'handlebars';
import { NotificationTemplate, TemplateVariables } from './types.js';

export class TemplateEngine {
  /**
   * Render a template with variables
   */
  static render(template: string, variables: TemplateVariables): string {
    const compiled = Handlebars.compile(template);
    return compiled(variables);
  }

  /**
   * Render notification from template
   */
  static renderNotification(
    template: NotificationTemplate,
    variables: TemplateVariables
  ): {
    subject?: string;
    body_text?: string;
    body_html?: string;
    push_title?: string;
    push_body?: string;
    sms_body?: string;
  } {
    return {
      subject: template.subject ? this.render(template.subject, variables) : undefined,
      body_text: template.body_text ? this.render(template.body_text, variables) : undefined,
      body_html: template.body_html ? this.render(template.body_html, variables) : undefined,
      push_title: template.push_title ? this.render(template.push_title, variables) : undefined,
      push_body: template.push_body ? this.render(template.push_body, variables) : undefined,
      sms_body: template.sms_body ? this.render(template.sms_body, variables) : undefined,
    };
  }

  /**
   * Validate template syntax
   */
  static validate(template: string): { valid: boolean; error?: string } {
    try {
      Handlebars.compile(template);
      return { valid: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return { valid: false, error: message };
    }
  }

  /**
   * Register custom helpers
   */
  static registerHelpers(): void {
    // Date formatting
    Handlebars.registerHelper('formatDate', (date: Date, format: string) => {
      // Simple date formatting (use date-fns in production)
      return date.toLocaleDateString();
    });

    // Currency formatting
    Handlebars.registerHelper('currency', (amount: number, currency: string = 'USD') => {
      return new Intl.NumberFormat('en-US', {
        style: 'currency',
        currency,
      }).format(amount);
    });

    // Uppercase
    Handlebars.registerHelper('upper', (str: string) => {
      return str ? str.toUpperCase() : '';
    });

    // Lowercase
    Handlebars.registerHelper('lower', (str: string) => {
      return str ? str.toLowerCase() : '';
    });

    // Capitalize
    Handlebars.registerHelper('capitalize', (str: string) => {
      return str ? str.charAt(0).toUpperCase() + str.slice(1) : '';
    });

    // Truncate
    Handlebars.registerHelper('truncate', (str: string, length: number) => {
      if (!str || str.length <= length) return str;
      return str.substring(0, length) + '...';
    });

    // Default value
    Handlebars.registerHelper('default', (value: unknown, defaultValue: unknown) => {
      return value || defaultValue;
    });

    // Conditional helpers
    Handlebars.registerHelper('eq', (a: unknown, b: unknown) => a === b);
    Handlebars.registerHelper('ne', (a: unknown, b: unknown) => a !== b);
    Handlebars.registerHelper('lt', (a: number, b: number) => a < b);
    Handlebars.registerHelper('gt', (a: number, b: number) => a > b);
    Handlebars.registerHelper('lte', (a: number, b: number) => a <= b);
    Handlebars.registerHelper('gte', (a: number, b: number) => a >= b);

    // Array helpers
    Handlebars.registerHelper('join', (array: unknown[], separator: string = ', ') => {
      return Array.isArray(array) ? array.join(separator) : '';
    });

    Handlebars.registerHelper('length', (array: unknown[]) => {
      return Array.isArray(array) ? array.length : 0;
    });
  }
}

// Register helpers on module load
TemplateEngine.registerHelpers();
