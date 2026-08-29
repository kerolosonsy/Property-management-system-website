// web/src/app/features/records/records.component.ts
// US4 — read recorded actions, newest first, filterable, paged.
// Filters survive paging (FR-045). The screen offers no edit/delete
// affordance — none exists (FR-046).
//
// FR-044 calls for filtering by the person who acted and by the account
// affected. UUID inputs would force the operator to look those up
// elsewhere, so the pickers below load the account list once (the screen is
// administrator-only, and the GET /users endpoint already exists) and resolve
// each selection back to the user id the API expects.

import { Component, inject, signal, OnInit, computed } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormBuilder, ReactiveFormsModule } from '@angular/forms';
import { RecordsService } from '../../api/api/records.service';
import { UsersService } from '../../api/api/users.service';
import { AuditRecord } from '../../api/model/audit-record.model';
import { AuditAction } from '../../api/model/audit-action.model';
import { User } from '../../api/model/user.model';
import { ARABIC_MESSAGES, format } from '../../shared/messages';
import { ApiError } from '../../core/api-error';

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
  imports: [CommonModule, ReactiveFormsModule],
  template: `
    <section class="pms-page">
      <div class="card">
        <h1>{{ msgs.recordsList }}</h1>

        <form [formGroup]="form" (ngSubmit)="applyFilters()" novalidate>
          <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr)); gap: 1rem;">
            <div class="field pms-field">
              <label for="actorId">{{ msgs.filterByActor }}</label>
              <select class="input" id="actorId" formControlName="actorId">
                <option [ngValue]="''">{{ msgs.allAccounts }}</option>
                @for (u of users(); track u.id) {
                  <option [ngValue]="u.id">{{ u.displayName }} ({{ u.username }})</option>
                }
              </select>
            </div>
            <div class="field pms-field">
              <label for="targetId">{{ msgs.filterByTarget }}</label>
              <select class="input" id="targetId" formControlName="targetId">
                <option [ngValue]="''">{{ msgs.allAccounts }}</option>
                @for (u of users(); track u.id) {
                  <option [ngValue]="u.id">{{ u.displayName }} ({{ u.username }})</option>
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
            <button type="button" class="btn btn-secondary" (click)="clearFilters()">{{ msgs.clearFilters }}</button>
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
              </tr>
            </thead>
            <tbody>
              @for (r of items(); track r.id) {
                <tr>
                  <td>{{ formatTime(r.occurredAt) }}</td>
                  <td>{{ actionLabel(r.action) }}</td>
                  <td>{{ r.actorUsername }} @if (r.actorRole) { · {{ roleLabel(r.actorRole) }} }</td>
                  <td>{{ r.targetUsername ?? msgs.noTarget }}</td>
                  <td>{{ r.sourceIp }}</td>
                </tr>
              }
            </tbody>
          </table>

          <div class="pms-paging">
            <span>{{ format(totalLabel, { count: totalItems() }) }}</span>
            <div class="pms-toolbar-spacer"></div>
            <button type="button" class="btn btn-secondary" (click)="prev()" [disabled]="page() <= 1">
              {{ msgs.pagePrevious }}
            </button>
            <span>{{ format(pageLabel, { page: page() }) }}</span>
            <button type="button" class="btn btn-secondary" (click)="next()" [disabled]="page() * pageSize() >= totalItems()">
              {{ msgs.pageNext }}
            </button>
          </div>
        }

        <p class="text-muted" style="margin-block-start: 1rem;">{{ msgs.cannotEditRecord }}</p>
      </div>
    </section>
  `,
})
export class RecordsComponent implements OnInit {
  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly format = format;
  protected readonly totalLabel = ARABIC_MESSAGES.totalItems;
  protected readonly pageLabel = ARABIC_MESSAGES.pageOf;
  protected readonly allActions: AuditAction[] = [
    AuditAction.SignInSucceeded,
    AuditAction.SignInFailed,
    AuditAction.SignOut,
    AuditAction.PasswordChanged,
    AuditAction.PasswordReset,
    AuditAction.AccountCreated,
    AuditAction.AccountRoleChanged,
    AuditAction.AccountActivated,
    AuditAction.AccountDeactivated,
    AuditAction.SessionsInvalidated,
    AuditAction.AdminRecoveryUsed,
  ];

  private readonly fb = inject(FormBuilder);
  private readonly records = inject(RecordsService);
  private readonly usersApi = inject(UsersService);

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
  protected readonly usersLoaded = signal(false);

  ngOnInit(): void {
    this.loadUsers();
    this.refresh();
  }

  protected roleLabel(role: string): string {
    return role === 'admin' ? this.msgs.roleAdmin : this.msgs.roleManager;
  }

  protected actionLabel(a: string): string {
    const labels: Record<string, string> = {
      sign_in_succeeded: 'تسجيل دخول ناجح',
      sign_in_failed: 'فشل تسجيل الدخول',
      sign_out: 'تسجيل خروج',
      password_changed: 'تغيير كلمة المرور',
      password_reset: 'إعادة تعيين كلمة المرور',
      account_created: 'إنشاء حساب',
      account_role_changed: 'تغيير دور',
      account_activated: 'تفعيل حساب',
      account_deactivated: 'إلغاء تفعيل',
      sessions_invalidated: 'إلغاء الجلسات',
      admin_recovery_used: 'استخدام أداة الاسترداد',
    };
    return labels[a] ?? a;
  }

  protected formatTime(iso: string): string {
    const d = new Date(iso);
    // Render in Africa/Cairo with Western digits. The browser's Intl uses the
    // host locale, so we set timeZone explicitly and a Western locale.
    const fmt = new Intl.DateTimeFormat('en-GB', {
      timeZone: 'Africa/Cairo',
      year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', second: '2-digit',
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

  private refresh(): void {
    this.loading.set(true);
    this.errorMessage.set(null);

    const v: FilterValues = this.form.getRawValue();
    const params = {
      page: this.page(),
      pageSize: this.pageSize(),
      actorId: v.actorId || undefined,
      targetId: v.targetId || undefined,
      action: v.action || undefined,
      from: v.from || undefined,
      to: v.to || undefined,
    };

    this.records.listAuditRecords(params as any, 'body').subscribe({
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
