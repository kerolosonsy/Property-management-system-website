// web/src/app/features/records/records.component.ts
// US4 — read recorded actions, newest first, filterable, paged.
// Filters survive paging (FR-045). The screen offers an undo for every
// record that can be undone, and marks records that have already been
// undone (Constitution VIII as amended in v1.4.0).
//
// FR-044 calls for filtering by the person who acted and by the account
// affected. UUID inputs would force the operator to look those up
// elsewhere, so the pickers below load the account list once (the screen is
// administrator-only, and the GET /users endpoint already exists) and resolve
// each selection back to the user id the API expects.

import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormBuilder, ReactiveFormsModule } from '@angular/forms';
import { RecordsService } from '../../api/api/records.service';
import { UsersService } from '../../api/api/users.service';
import { LookupsService } from '../../api/api/lookups.service';
import { CustomFieldsService } from '../../api/api/custom-fields.service';
import { AuditRecord } from '../../api/model/audit-record.model';
import { AuditAction } from '../../api/model/audit-action.model';
import { CustomField } from '../../api/model/custom-field.model';
import { Lookup } from '../../api/model/lookup.model';
import { User } from '../../api/model/user.model';
import {
  ARABIC_MESSAGES,
  AUDIT_ACTION_LABELS,
  AUDIT_FIELD_LABELS,
  format,
} from '../../shared/messages';
import { ApiError } from '../../core/api-error';
import { CloseOnEscapeDirective } from '../../shared/close-on-escape.directive';

interface FilterValues {
  actorId: string;
  targetId: string;
  action: string;
  from: string;
  to: string;
}

@Component({
  selector: 'app-records',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, CloseOnEscapeDirective],
  template: `
    <section class="pms-page">
      <div class="card">
        <h1>{{ msgs.recordsList }}</h1>

        <form [formGroup]="form" (ngSubmit)="applyFilters()" novalidate>
          <div
            style="display: grid; grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr)); gap: 1rem;"
          >
            <div class="field pms-field">
              <label for="actorId">{{ msgs.filterByActor }}</label>
              <select class="input" id="actorId" formControlName="actorId">
                <option [ngValue]="''">{{ msgs.allAccounts }}</option>
                @for (u of users(); track u.id) {
                  <option [ngValue]="u.id">{{ u.displayName }} ({{ u.username }>)</option>
                }
              </select>
            </div>
            <div class="field pms-field">
              <label for="targetId">{{ msgs.filterByTarget }}</label>
              <select class="input" id="targetId" formControlName="targetId">
                <option [ngValue]="''">{{ msgs.allAccounts }}</option>
                @for (u of users(); track u.id) {
                  <option [ngValue]="u.id">{{ u.displayName }} ({{ u.username }>)</option>
                }
              </select>
            </div>
            <div class="field pms-field">
              <label for="action">{{ msgs.filterByAction }}</label>
              <select class="input" id="action" formControlName="action">
                <option [ngValue]="''">{{ msgs.allActions }}</option>
                @for (a of allActions; track a) {
                  <option [ngValue]="a">{{ actionLabel(a) }}</option>
                }
              </select>
            </div>
            <div class="field pms-field">
              <label for="from">{{ msgs.filterByDateFrom }}</label>
              <input class="input" id="from" type="date" formControlName="from" />
            </div>
            <div class="field pms-field">
              <label for="to">{{ msgs.filterByDateTo }}</label>
              <input class="input" id="to" type="date" formControlName="to" />
            </div>
          </div>
          <div style="display: flex; gap: 0.75rem;">
            <button type="submit" class="btn btn-primary">{{ msgs.applyFilters }}</button>
            <button type="button" class="btn btn-secondary" (click)="clearFilters()">
              {{ msgs.clearFilters }}
            </button>
          </div>
        </form>

        @if (errorMessage(); as msg) {
          <div class="pms-error-banner">{{ msg }}</div>
        }

        @if (loading()) {
          <p class="text-muted">{{ msgs.loading }}</p>
        } @else if (items().length === 0) {
          <p class="pms-empty">{{ msgs.noRecordsMatch }}</p>
        } @else {
          <table class="table">
            <thead>
              <tr>
                <th>{{ msgs.occurredAt }}</th>
                <th>{{ msgs.action }}</th>
                <th>{{ msgs.actor }}</th>
                <th>{{ msgs.target }}</th>
                <th>{{ msgs.sourceIp }}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              @for (r of items(); track r.id) {
                <tr [class.pms-record-undone]="r.revertedByAuditId != null">
                  <td>{{ formatTime(r.occurredAt) }}</td>
                  <td>{{ actionLabel(r.action) }}</td>
                  <td>
                    {{ r.actorUsername }}
                    @if (r.actorRole) {
                      · {{ roleLabel(r.actorRole) }}
                    }
                  </td>
                  <td>{{ r.targetUsername ?? msgs.noTarget }}</td>
                  <td>{{ r.sourceIp }}</td>
                  <td>
                    @if (r.revertedByAuditId != null) {
                      <span class="pms-pill pms-pill-warn">{{ msgs.auditRecordUndone }}</span>
                    } @else if (canUndo(r)) {
                      <button type="button" class="btn btn-secondary" (click)="askUndo(r)">
                        {{ msgs.auditUndo }}
                      </button>
                    } @else {
                      <span class="text-muted">{{ msgs.auditCannotUndo }}</span>
                    }
                  </td>
                </tr>
                @if (hasDiff(r)) {
                  <tr class="pms-record-diff">
                    <td colspan="6">
                      @if (r.before && asRecord(r.before) | keyvalue; as fields) {
                        @for (field of fields; track field.key) {
                          <div>
                            <strong>{{ fieldLabel(field.key) }}:</strong>
                            {{ formatValue(field.key, field.value) }} →
                            {{ formatValue(field.key, getAfter(r, field.key)) }}
                          </div>
                        }
                      }
                    </td>
                  </tr>
                }
              }
            </tbody>
          </table>

          <div class="pms-paging">
            <span>{{ format(totalLabel, { count: totalItems() }) }}</span>
            <div class="pms-toolbar-spacer"></div>
            <button
              type="button"
              class="btn btn-secondary"
              (click)="prev()"
              [disabled]="page() <= 1"
            >
              {{ msgs.pagePrevious }}
            </button>
            <span>{{ format(pageLabel, { page: page() }) }}</span>
            <button
              type="button"
              class="btn btn-secondary"
              (click)="next()"
              [disabled]="page() * pageSize() >= totalItems()"
            >
              {{ msgs.pageNext }}
            </button>
          </div>
        }

        <p class="text-muted" style="margin-block-start: 1rem;">{{ msgs.cannotEditRecord }}</p>
      </div>
    </section>

    @if (confirmUndo(); as r) {
      <div class="pms-modal-backdrop" (click)="cancelUndo()" [pmsCloseOnEscape]="cancelUndo">
        <div class="pms-modal" (click)="$event.stopPropagation()">
          <h2>{{ msgs.auditUndoTitle }}</h2>
          <p>{{ msgs.auditUndoPrompt }}</p>
          <p>{{ format(msgs.auditUndoAction, { action: actionLabel(r.action) }) }}</p>
          <div class="pms-modal-actions">
            <button type="button" class="btn btn-secondary" (click)="cancelUndo()">
              {{ msgs.cancel }}
            </button>
            <button
              type="button"
              class="btn btn-primary"
              (click)="doUndo(r)"
              [disabled]="undoing()"
            >
              {{ msgs.auditUndo }}
            </button>
          </div>
        </div>
      </div>
    }
  `,
  styles: [
    `
      .pms-record-undone {
        opacity: 0.65;
      }
      .pms-record-diff {
        background: color-mix(in srgb, var(--color-accent) 5%, transparent);
      }
      .pms-record-diff div {
        font-size: 12px;
        padding: 2px 0;
      }
    `,
  ],
})
export class RecordsComponent implements OnInit {
  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly format = format;
  protected readonly totalLabel = ARABIC_MESSAGES.totalItems;
  protected readonly pageLabel = ARABIC_MESSAGES.pageOf;
  protected readonly allActions = Object.values(AuditAction) as AuditAction[];

  private readonly fb = inject(FormBuilder);
  private readonly records = inject(RecordsService);
  private readonly usersApi = inject(UsersService);
  private readonly lookupsApi = inject(LookupsService);
  private readonly customFieldsApi = inject(CustomFieldsService);

  protected readonly form = this.fb.nonNullable.group({
    actorId: [''],
    targetId: [''],
    action: [''],
    from: [''],
    to: [''],
  });

  protected readonly items = signal<AuditRecord[]>([]);
  protected readonly totalItems = signal(0);
  protected readonly page = signal(1);
  protected readonly pageSize = signal(25);
  protected readonly loading = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly users = signal<User[]>([]);
  protected readonly propertyTypes = signal<Lookup[]>([]);
  protected readonly areas = signal<Lookup[]>([]);
  protected readonly customFields = signal<CustomField[]>([]);
  private readonly propertyTypesLoaded = signal(false);
  private readonly areasLoaded = signal(false);
  private readonly customFieldsLoaded = signal(false);
  protected readonly usersLoaded = signal(false);
  protected readonly confirmUndo = signal<AuditRecord | null>(null);
  protected readonly undoing = signal(false);

  // The set of actions we know how to undo. Anything else shows the
  // "cannot undo" hint.
  private readonly undoableActions = new Set<string>([
    AuditAction.PropertyModified,
    AuditAction.PropertyArchived,
    AuditAction.PropertyRestored,
    AuditAction.PropertyCodeChanged,
    AuditAction.LookupRenamed,
    AuditAction.CustomFieldRenamed,
    AuditAction.CustomFieldChoiceAdded,
    AuditAction.CustomFieldChoiceRemoved,
    AuditAction.AttachmentDescribed,
  ]);

  ngOnInit(): void {
    this.loadUsers();
    this.loadAuditReferences();
    this.refresh();
  }

  protected roleLabel(role: string): string {
    return role === 'admin' ? this.msgs.roleAdmin : this.msgs.roleManager;
  }

  protected actionLabel(action: string): string {
    return AUDIT_ACTION_LABELS[action as AuditAction] ?? action;
  }

  protected formatTime(iso: string): string {
    const d = new Date(iso);
    // Render in Africa/Cairo with Western digits. The browser's Intl uses the
    // host locale, so we set timeZone explicitly and a Western locale.
    const fmt = new Intl.DateTimeFormat('en-GB', {
      timeZone: 'Africa/Cairo',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    });
    return fmt.format(d);
  }

  protected applyFilters(): void {
    this.page.set(1);
    this.refresh();
  }

  protected clearFilters(): void {
    this.form.reset({ actorId: '', targetId: '', action: '', from: '', to: '' });
    this.page.set(1);
    this.refresh();
  }

  protected prev(): void {
    if (this.page() > 1) {
      this.page.set(this.page() - 1);
      this.refresh();
    }
  }

  protected next(): void {
    if (this.page() * this.pageSize() < this.totalItems()) {
      this.page.set(this.page() + 1);
      this.refresh();
    }
  }

  protected canUndo(r: AuditRecord): boolean {
    if (r.revertedByAuditId != null) return false;
    if (r.reversesAuditId != null) return false;
    return this.undoableActions.has(r.action);
  }

  protected hasDiff(r: AuditRecord): boolean {
    return r.before != null && r.after != null;
  }

  protected asRecord(rawValue: unknown): Record<string, unknown> {
    return rawValue && typeof rawValue === 'object' ? (rawValue as Record<string, unknown>) : {};
  }

  protected getAfter(r: AuditRecord, key: string): unknown {
    if (!r.after) return undefined;
    return this.asRecord(r.after)[key];
  }

  protected formatValue(key: string, rawValue: unknown): string {
    if (key === 'propertyTypeId') {
      return this.lookupLabel(rawValue, this.propertyTypes(), this.propertyTypesLoaded());
    }
    if (key === 'areaId') return this.lookupLabel(rawValue, this.areas(), this.areasLoaded());
    if (key === 'customValues') return this.formatCustomValues(rawValue);
    if (key === 'sensitiveFields') return this.formatSensitiveFields(rawValue);
    if (rawValue === null || rawValue === undefined) return '—';
    if (typeof rawValue === 'string') return rawValue;
    if (typeof rawValue === 'boolean') return rawValue ? this.msgs.yes : this.msgs.no;
    if (typeof rawValue === 'number') return String(rawValue);
    if (Array.isArray(rawValue)) return rawValue.map(String).join(', ');
    return JSON.stringify(rawValue);
  }

  protected fieldLabel(key: string): string {
    return AUDIT_FIELD_LABELS[key] ?? key;
  }

  protected askUndo(r: AuditRecord): void {
    this.confirmUndo.set(r);
    this.errorMessage.set(null);
  }
  protected cancelUndo(): void {
    this.confirmUndo.set(null);
  }
  protected doUndo(r: AuditRecord): void {
    this.undoing.set(true);
    this.records.undoAuditRecord({ recordId: r.id }, 'body').subscribe({
      next: () => {
        this.undoing.set(false);
        this.confirmUndo.set(null);
        this.refresh();
      },
      error: (err: ApiError) => {
        this.undoing.set(false);
        this.confirmUndo.set(null);
        this.errorMessage.set(err.message || this.msgs.internalError);
      },
    });
  }

  private loadUsers(): void {
    // The records screen is administrator-only, so GET /users is authorized
    // here. One large page is enough to populate both pickers; if the
    // account count ever exceeds the page size, only the first page is
    // listed, and the operator can fall back to date or action filters.
    this.usersApi.listUsers({ page: 1, pageSize: 100, includeInactive: true }, 'body').subscribe({
      next: (resp) => {
        this.users.set(resp.items);
        this.usersLoaded.set(true);
      },
      error: (_err: ApiError) => {
        // Pickers will simply be empty; the date and action filters remain
        // usable.
        this.usersLoaded.set(true);
      },
    });
  }

  private loadAuditReferences(): void {
    this.lookupsApi.listPropertyTypes('body').subscribe({
      next: (lookups) => {
        this.propertyTypes.set(lookups);
        this.propertyTypesLoaded.set(true);
      },
      error: () => this.propertyTypesLoaded.set(false),
    });
    this.lookupsApi.listAreas('body').subscribe({
      next: (lookups) => {
        this.areas.set(lookups);
        this.areasLoaded.set(true);
      },
      error: () => this.areasLoaded.set(false),
    });
    this.customFieldsApi.listCustomFields('body').subscribe({
      next: (fields) => {
        this.customFields.set(fields);
        this.customFieldsLoaded.set(true);
      },
      error: () => this.customFieldsLoaded.set(false),
    });
  }

  private lookupLabel(rawReference: unknown, lookups: Lookup[], loaded: boolean): string {
    if (rawReference === null || rawReference === undefined) return '—';
    const id = String(rawReference);
    const label = lookups.find((lookup) => lookup.id === id)?.label;
    return label ?? (loaded ? this.deletedReference(id) : id);
  }

  private formatCustomValues(rawValue: unknown): string {
    const values = this.asRecord(rawValue);
    return Object.entries(values)
      .map(([fieldId, fieldValue]) => {
        const field = this.customFields().find((candidate) => candidate.id === fieldId);
        const label = field?.label ?? this.missingCustomFieldLabel(fieldId);
        return `${label}: ${this.formatCustomFieldValue(field, fieldValue)}`;
      })
      .join('، ');
  }

  private formatCustomFieldValue(field: CustomField | undefined, rawValue: unknown): string {
    if (rawValue === null || rawValue === undefined) return '—';
    if (field?.fieldType === 'checkbox' && typeof rawValue === 'boolean') {
      return rawValue ? this.msgs.yes : this.msgs.no;
    }
    if (field?.fieldType === 'dropdown') {
      return this.choiceLabel(field, String(rawValue));
    }
    if (field?.fieldType === 'multiselect' && Array.isArray(rawValue)) {
      return rawValue.map((choiceId) => this.choiceLabel(field, String(choiceId))).join('، ');
    }
    if (Array.isArray(rawValue)) return rawValue.map(String).join('، ');
    return String(rawValue);
  }

  private formatSensitiveFields(rawValue: unknown): string {
    if (!Array.isArray(rawValue)) return '—';
    return rawValue
      .map((fieldId) => {
        const id = String(fieldId);
        return (
          this.customFields().find((field) => field.id === id)?.label ??
          this.missingCustomFieldLabel(id)
        );
      })
      .join('، ');
  }

  private choiceLabel(field: CustomField, choiceId: string): string {
    return (
      field.choices.find((choice) => choice.id === choiceId)?.label ??
      this.deletedReference(choiceId)
    );
  }

  private deletedReference(id: string): string {
    return `${id} (${this.msgs.auditDeletedReference})`;
  }

  private missingCustomFieldLabel(id: string): string {
    return this.customFieldsLoaded() ? this.deletedReference(id) : id;
  }

  private refresh(): void {
    this.loading.set(true);
    this.errorMessage.set(null);

    const v: FilterValues = this.form.getRawValue();
    const params = {
      page: this.page(),
      pageSize: this.pageSize(),
      actorId: v.actorId || undefined,
      targetId: v.targetId || undefined,
      action: (v.action as AuditAction | '') || undefined,
      from: v.from || undefined,
      to: v.to || undefined,
    };

    this.records.listAuditRecords(params, 'body').subscribe({
      next: (resp) => {
        this.items.set(resp.items);
        this.totalItems.set(resp.totalItems);
        this.loading.set(false);
      },
      error: (err: ApiError) => {
        this.loading.set(false);
        this.errorMessage.set(err.message || this.msgs.internalError);
      },
    });
  }
}
