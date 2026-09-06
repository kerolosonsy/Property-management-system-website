// web/src/app/features/properties/custom-values.ts
// Shared helpers for rendering custom field values on the property form and
// the property detail screen. The data shape is the OpenAPI-generated
// CustomFieldValue (model/custom-field-value.model.ts).

import { CustomField } from '../../api/model/custom-field.model';
import { CustomFieldValue } from '../../api/model/custom-field-value.model';

export interface RenderedValue {
  field: CustomField;
  /** Plain text for text fields, the choice label for dropdown, yes/no for checkbox. */
  display: string;
  /** Set on multiselect to render a comma list. */
  choiceLabels?: string[];
  /** True when the value is sensitive AND the field has a value. */
  isSensitive: boolean;
}

/**
 * Render a property's stored values for the detail screen. Empty fields are
 * included so the screen shows every defined field (FR-027c). Sensitive values
 * arrive decrypted by the server, which is the only place decryption happens
 * (Constitution VII).
 */
export function renderCustomValues(
  fields: CustomField[],
  values: CustomFieldValue[],
): RenderedValue[] {
  const byField = new Map<string, CustomFieldValue>();
  for (const v of values) {
    byField.set(v.fieldId, v);
  }
  const out: RenderedValue[] = [];
  for (const f of fields) {
    const v = byField.get(f.id);
    out.push(renderOne(f, v));
  }
  return out;
}

function renderOne(f: CustomField, v: CustomFieldValue | undefined): RenderedValue {
  if (!v) {
    return { field: f, display: '—', isSensitive: f.isSensitive && f.fieldType === 'text' };
  }
  switch (f.fieldType) {
    case 'text':
    case 'autocomplete':
      // autocomplete is stored and read exactly like text; only its form
      // control differs (it suggests values from the rest of the register).
      return {
        field: f,
        display: v.text ?? '—',
        isSensitive: f.isSensitive && (v.text ?? '') !== '—',
      };
    case 'checkbox':
      return {
        field: f,
        display: v.checked === true ? 'نعم' : v.checked === false ? 'لا' : '—',
        isSensitive: false,
      };
    case 'dropdown': {
      const id = v.choiceId ?? '';
      const ch = f.choices.find((c) => c.id === id);
      return { field: f, display: ch ? ch.label : '—', isSensitive: false };
    }
    case 'multiselect': {
      const ids = v.choiceIds ?? [];
      const labels = ids
        .map((id) => f.choices.find((c) => c.id === id)?.label ?? '')
        .filter((s) => s !== '');
      return { field: f, display: labels.join('، ') || '—', isSensitive: false };
    }
  }
  return { field: f, display: '—', isSensitive: false };
}

/** Western digits for everything user-visible (Constitution II). */
export function toWesternDigits(s: string): string {
  if (!s) return s;
  return s
    .replace(/[\u0660-\u0669]/g, (c) => String(c.charCodeAt(0) - 0x0660))
    .replace(/[\u06F0-\u06F9]/g, (c) => String(c.charCodeAt(0) - 0x06f0));
}

/** Convert a Date or ISO string to a Western-digit date string in Africa/Cairo. */
export function formatCairoDate(input: string | Date | null | undefined): string {
  if (!input) return '—';
  const d = typeof input === 'string' ? new Date(input) : input;
  if (isNaN(d.getTime())) return '—';
  // Africa/Cairo is UTC+2 with no DST.
  const parts = new Intl.DateTimeFormat('en-GB', {
    timeZone: 'Africa/Cairo',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).formatToParts(d);
  const get = (t: string) => parts.find((p) => p.type === t)?.value ?? '';
  return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')}`;
}
