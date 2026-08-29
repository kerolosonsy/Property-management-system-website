// web/src/app/features/users/edit/user-edit.component.ts
// US2 — edit one account: display name, role, is_active, reset password.

import { Component, inject, signal, OnInit, Input } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterLink } from '@angular/router';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { UsersService } from '../../../api/api/users.service';
import { WesternDigitsDirective } from '../../../shared/western-digits.directive';
import { ARABIC_MESSAGES } from '../../../shared/messages';
import { ApiError } from '../../../core/api-error';
import { User } from '../../../api/model/user.model';
import { Role } from '../../../api/model/role.model';
import { SessionService } from '../../../core/session.service';

@Component({
  selector: 'app-user-edit',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, RouterLink, WesternDigitsDirective],
  template: `
    <section class="pms-page">
      <div class="card" style="max-inline-size: 36rem; margin-inline: auto;">
        <header style="display: flex; align-items: center; gap: 1rem;">
          <h1 style="margin: 0; flex: 1;">{{ msgs.edit }}</h1>
          <a routerLink="/settings/users">{{ msgs.backToList }}</a>
        </header>

        @if (loading()) {
          <p class="text-muted">{{ msgs.loading }}</p>
        } @else if (user(); as u) {
          <form [formGroup]="form" (ngSubmit)="onSave()" novalidate>
            <div class="field pms-field">
              <label>{{ msgs.username }}</label>
              <input class="input" type="text" [value]="u.username" disabled />
            </div>

            <div class="field pms-field">
              <label for="displayName">{{ msgs.displayName }}</label>
              <input class="input" id="displayName" type="text" formControlName="displayName" appWesternDigits />
              @if (fieldError('displayName'); as msg) {
                <div class="pms-field-error">{{ msg }}</div>
              }
            </div>

            <div class="field pms-field">
              <label for="role">{{ msgs.role }}</label>
              <select class="input" id="role" formControlName="role">
                <option [ngValue]="'admin'">{{ msgs.roleAdmin }}</option>
                <option [ngValue]="'manager'">{{ msgs.roleManager }}</option>
              </select>
              @if (fieldError('role'); as msg) {
                <div class="pms-field-error">{{ msg }}</div>
              }
            </div>

            <div class="field pms-field" style="display: flex; flex-direction: row; align-items: center; gap: 0.5rem;">
              <label for="isActive" style="margin: 0;">{{ msgs.active }}</label>
              <input id="isActive" type="checkbox" formControlName="isActive" />
            </div>

            @if (errorMessage(); as msg) {
              <div class="pms-error-banner">{{ msg }}</div>
            }

            <div style="display: flex; gap: 0.75rem;">
              <button type="submit" class="btn btn-primary" [disabled]="submitting() || form.invalid">
                {{ submitting() ? msgs.loading : msgs.save }}
              </button>
              <a routerLink="/settings/users" class="btn btn-secondary">{{ msgs.cancel }}</a>
            </div>
          </form>

          <hr class="hr" style="margin-block: 2rem;" />

          <h2>{{ msgs.resetPassword }}</h2>
          <form [formGroup]="resetForm" (ngSubmit)="onResetPassword()" novalidate>
            <div class="field pms-field">
              <label for="newPassword">{{ msgs.newPassword }}</label>
              <input class="input" id="newPassword" type="password" formControlName="newPassword" autocomplete="new-password" />
              @if (resetForm.controls.newPassword.touched && resetForm.controls.newPassword.invalid) {
                <div class="pms-field-error">
                  @if (resetForm.controls.newPassword.hasError('required')) { {{ msgs.requiredField }} }
                  @else { {{ msgs.passwordTooShort }} }
                </div>
              }
            </div>
            @if (resetMessage(); as msg) {
              <div class="pms-error-banner">{{ msg }}</div>
            }
            <button type="submit" class="btn btn-primary" [disabled]="resetting() || resetForm.invalid">
              {{ resetting() ? msgs.loading : msgs.resetPassword }}
            </button>
          </form>
        } @else {
          <p>{{ msgs.noRecordsMatch }}</p>
        }
      </div>
    </section>
  `,
})
export class UserEditComponent implements OnInit {
  protected readonly msgs = ARABIC_MESSAGES;
  private readonly fb = inject(FormBuilder);
  private readonly users = inject(UsersService);
  private readonly router = inject(Router);
  private readonly session = inject(SessionService);

  @Input() id?: string;

  protected readonly user = signal<User | null>(null);
  protected readonly loading = signal(false);
  protected readonly submitting = signal(false);
  protected readonly resetting = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  protected readonly resetMessage = signal<string | null>(null);
  private readonly fieldErrors = signal<Record<string, string>>({});

  protected readonly form = this.fb.nonNullable.group({
    displayName: ['', [Validators.required, Validators.minLength(1), Validators.maxLength(120)]],
    role: ['manager' as Role, [Validators.required]],
    isActive: [true, [Validators.required]],
  });

  protected readonly resetForm = this.fb.nonNullable.group({
    newPassword: ['', [Validators.required, Validators.minLength(12)]],
  });

  protected fieldError(name: string): string | undefined {
    return this.fieldErrors()[name];
  }

  ngOnInit(): void {
    if (!this.id) {
      return;
    }
    this.loading.set(true);
    this.users.getUser({ userId: this.id }, 'body').subscribe({
      next: (u) => {
        this.user.set(u);
        this.form.patchValue({
          displayName: u.displayName,
          role: u.role,
          isActive: u.isActive,
        });
        this.loading.set(false);
      },
      error: (err: ApiError) => {
        this.loading.set(false);
        this.errorMessage.set(err.message || this.msgs.internalError);
      },
    });
  }

  protected onSave(): void {
    if (this.form.invalid || !this.id) return;
    this.submitting.set(true);
    this.errorMessage.set(null);
    this.fieldErrors.set({});

    const { displayName, role, isActive } = this.form.getRawValue();
    this.users.updateUser({
      userId: this.id!,
      updateUserRequest: { displayName, role, isActive },
    }, 'body').subscribe({
      next: () => {
        this.submitting.set(false);
        void this.router.navigate(['/settings/users']);
      },
      error: (err: ApiError) => {
        this.submitting.set(false);
        if (err.code === 'conflict') {
          this.errorMessage.set(err.message || this.msgs.lastAdmin);
        } else if (err.code === 'forbidden') {
          this.errorMessage.set(this.msgs.forbidden);
        } else if (err.fields) {
          this.fieldErrors.set(err.fields);
          this.errorMessage.set(err.message);
        } else {
          this.errorMessage.set(err.message || this.msgs.internalError);
        }
      },
    });
  }

  protected onResetPassword(): void {
    if (this.resetForm.invalid || !this.id) return;
    this.resetting.set(true);
    this.resetMessage.set(null);
    const { newPassword } = this.resetForm.getRawValue();
    this.users.resetUserPassword({
      userId: this.id!,
      resetUserPasswordRequest: { newPassword },
    }, 'response').subscribe({
      next: () => {
        this.resetting.set(false);
        this.resetForm.reset({ newPassword: '' });
        this.resetMessage.set(this.msgs.passwordChanged);
      },
      error: (err: ApiError) => {
        this.resetting.set(false);
        this.resetMessage.set(err.message || this.msgs.internalError);
      },
    });
  }
}