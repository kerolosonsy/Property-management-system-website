// web/src/app/features/users/users-list.component.ts
// US2 — list of accounts, paging, role-gated (manager → 403 from server).

import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { UsersService } from '../../api/api/users.service';
import { User } from '../../api/model/user.model';
import { ARABIC_MESSAGES, format } from '../../shared/messages';
import { ApiError } from '../../core/api-error';

@Component({
  selector: 'app-users-list',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, RouterLink],
  template: `
    <section class="pms-page">
      <div class="pms-card">
        <header style="display: flex; align-items: center; gap: 1rem;">
          <h1 style="margin: 0; flex: 1;">{{ msgs.usersList }}</h1>
          <a routerLink="/users/new" class="pms-button">{{ msgs.create }}</a>
        </header>

        <div class="pms-field" style="flex-direction: row; align-items: center; gap: 0.5rem;">
          <label for="includeInactive" style="margin: 0;">{{ msgs.inactive }}</label>
          <input id="includeInactive" type="checkbox" [formControl]="includeInactiveCtrl" />
        </div>

        @if (errorMessage(); as msg) {
          <div class="pms-error-banner">{{ msg }}</div>
        }

        @if (loading()) {
          <p class="pms-muted">{{ msgs.loading }}</p>
        } @else if (items().length === 0) {
          <p class="pms-empty">{{ msgs.noRecordsMatch }}</p>
        } @else {
          <table class="pms-table">
            <thead>
              <tr>
                <th>{{ msgs.username }}</th>
                <th>{{ msgs.displayName }}</th>
                <th>{{ msgs.role }}</th>
                <th>{{ msgs.active }}</th>
                <th>{{ msgs.mustChangePassword }}</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              @for (u of items(); track u.id) {
                <tr>
                  <td>{{ u.username }}</td>
                  <td>{{ u.displayName }}</td>
                  <td>{{ roleLabel(u.role) }}</td>
                  <td>{{ u.isActive ? msgs.yes : msgs.no }}</td>
                  <td>{{ u.mustChangePassword ? msgs.yes : msgs.no }}</td>
                  <td>
                    <a [routerLink]="['/users', u.id]">{{ msgs.edit }}</a>
                  </td>
                </tr>
              }
            </tbody>
          </table>

          <div class="pms-paging">
            <span>{{ format(totalLabel, { count: totalItems() }) }}</span>
            <div class="pms-toolbar-spacer"></div>
            <button type="button" class="pms-button pms-button-secondary" (click)="prev()" [disabled]="page() <= 1">
              {{ msgs.pagePrevious }}
            </button>
            <span>{{ format(pageLabel, { page: page() }) }}</span>
            <button type="button" class="pms-button pms-button-secondary" (click)="next()" [disabled]="page() * pageSize() >= totalItems()">
              {{ msgs.pageNext }}
            </button>
          </div>
        }
      </div>
    </section>
  `,
})
export class UsersListComponent implements OnInit {
  protected readonly msgs = ARABIC_MESSAGES;
  protected readonly format = format;
  protected readonly totalLabel = ARABIC_MESSAGES.totalItems;
  protected readonly pageLabel = ARABIC_MESSAGES.pageOf;

  private readonly users = inject(UsersService);

  protected readonly items = signal<User[]>([]);
  protected readonly totalItems = signal(0);
  protected readonly page = signal(1);
  protected readonly pageSize = signal(25);
  protected readonly loading = signal(false);
  protected readonly errorMessage = signal<string | null>(null);

  protected readonly includeInactiveCtrl = new FormControl(true, { nonNullable: true });

  ngOnInit(): void {
    this.includeInactiveCtrl.valueChanges.subscribe(() => {
      this.page.set(1);
      this.refresh();
    });
    this.refresh();
  }

  protected roleLabel(role: string): string {
    return role === 'admin' ? this.msgs.roleAdmin : this.msgs.roleManager;
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

  private refresh(): void {
    this.loading.set(true);
    this.errorMessage.set(null);
    this.users.listUsers(
      {
        page: this.page(),
        pageSize: this.pageSize(),
        includeInactive: this.includeInactiveCtrl.value,
      },
      'body',
    ).subscribe({
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