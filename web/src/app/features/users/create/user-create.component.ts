// web/src/app/features/users/create/user-create.component.ts
// US2 — create an account. Role-gated; a manager cookie gets 403 from the API.

import { Component, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterLink } from '@angular/router';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { UsersService } from '../../../api/api/users.service';
import { WesternDigitsDirective } from '../../../shared/western-digits.directive';
import { ARABIC_MESSAGES } from '../../../shared/messages';
import { ApiError } from '../../../core/api-error';
import { Role } from '../../../api/model/role.model';

@Component({
  selector: 'app-user-create',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, RouterLink, WesternDigitsDirective],
  template: `
    <section class="pms-page">
      <header class="pms-view-head">
        <div>
          <h1>{{ msgs.userCreateTitle }}</h1>
          <div class="pms-crumb">{{ msgs.settings }} / {{ msgs.usersList }}</div>
        </div>
        <div class="pms-toolbar-spacer"></div>
        <a routerLink="/settings/users" class="btn btn-secondary">{{ msgs.backToList }}</a>
      </header>

      <div class="card" style="max-inline-size: 48rem; margin-inline: auto;">
        <form id="pms-user-create-form" [formGroup]="form" (ngSubmit)="onSubmit()" novalidate>
          <div class="pms-form-grid">
            <div class="field pms-field">
              <label for="username">{{ msgs.username }}</label>
              <input
                class="input"
                id="username"
                type="text"
                formControlName="username"
                appWesternDigits
                autocomplete="off"
                autocapitalize="off"
                spellcheck="false"
              />
              @if (form.controls.username.touched && form.controls.username.invalid) {
                <div class="pms-field-error">
                  @if (form.controls.username.hasError('required')) {
                    {{ msgs.requiredField }}
                  } @else {
                    {{ msgs.usernameLength }}
                  }
                </div>
              }
              @if (fieldError('username'); as msg) {
                <div class="pms-field-error">{{ msg }}</div>
              }
            </div>

            <div class="field pms-field">
              <label for="displayName">{{ msgs.displayName }}</label>
              <input
                class="input"
                id="displayName"
                type="text"
                formControlName="displayName"
                appWesternDigits
              />
              @if (form.controls.displayName.touched && form.controls.displayName.invalid) {
                <div class="pms-field-error">
                  @if (form.controls.displayName.hasError('required')) {
                    {{ msgs.requiredField }}
                  } @else {
                    {{ msgs.displayNameLength }}
                  }
                </div>
              }
              @if (fieldError('displayName'); as msg) {
                <div class="pms-field-error">{{ msg }}</div>
              }
            </div>

            <div class="field pms-field">
              <label for="role">{{ msgs.role }}</label>
              <select class="input" id="role" formControlName="role">
                <option [ngValue]="''" disabled>{{ msgs.selectRole }}</option>
                <option [ngValue]="'admin'">{{ msgs.roleAdmin }}</option>
                <option [ngValue]="'manager'">{{ msgs.roleManager }}</option>
              </select>
              @if (form.controls.role.touched && form.controls.role.hasError('required')) {
                <div class="pms-field-error">{{ msgs.requiredField }}</div>
              }
              @if (fieldError('role'); as msg) {
                <div class="pms-field-error">{{ msg }}</div>
              }
            </div>

            <div class="field pms-field">
              <label for="initialPassword">{{ msgs.initialPassword }}</label>
              <input
                class="input"
                id="initialPassword"
                type="password"
                formControlName="initialPassword"
                autocomplete="new-password"
              />
              @if (form.controls.initialPassword.touched && form.controls.initialPassword.invalid) {
                <div class="pms-field-error">
                  @if (form.controls.initialPassword.hasError('required')) {
                    {{ msgs.requiredField }}
                  } @else {
                    {{ msgs.passwordTooShort }}
                  }
                </div>
              }
              @if (fieldError('initialPassword'); as msg) {
                <div class="pms-field-error">{{ msg }}</div>
              }
            </div>
          </div>

          @if (errorMessage(); as msg) {
            <div class="pms-error-banner">{{ msg }}</div>
          }
        </form>
      </div>

      <!-- The action bar pins to the foot; its submit reaches the form by
           id. Same buttons, same bindings as the row it replaces. -->
      <footer class="pms-view-foot">
        <button
          type="submit"
          class="btn btn-primary"
          [disabled]="submitting() || form.invalid"
          form="pms-user-create-form"
        >
          {{ submitting() ? msgs.loading : msgs.create }}
        </button>
        <a routerLink="/settings/users" class="btn btn-secondary">{{ msgs.cancel }}</a>
      </footer>
    </section>
  `,
})
export class UserCreateComponent {
  protected readonly msgs = ARABIC_MESSAGES;
  private readonly fb = inject(FormBuilder);
  private readonly users = inject(UsersService);
  private readonly router = inject(Router);

  protected readonly form = this.fb.nonNullable.group({
    username: ['', [Validators.required, Validators.minLength(3), Validators.maxLength(32)]],
    displayName: ['', [Validators.required, Validators.minLength(1), Validators.maxLength(120)]],
    role: ['' as Role | '', [Validators.required]],
    initialPassword: ['', [Validators.required, Validators.minLength(12)]],
  });

  protected readonly submitting = signal(false);
  protected readonly errorMessage = signal<string | null>(null);
  private readonly fieldErrors = signal<Record<string, string>>({});

  protected fieldError(name: string): string | undefined {
    return this.fieldErrors()[name];
  }

  protected onSubmit(): void {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    this.submitting.set(true);
    this.errorMessage.set(null);
    this.fieldErrors.set({});

    const raw = this.form.getRawValue();
    const role = raw.role as Role;
    this.users
      .createUser(
        {
          createUserRequest: {
            username: raw.username,
            displayName: raw.displayName,
            role,
            initialPassword: raw.initialPassword,
          },
        },
        'body',
      )
      .subscribe({
        next: () => {
          this.submitting.set(false);
          void this.router.navigate(['/settings/users']);
        },
        error: (err: ApiError) => {
          this.submitting.set(false);
          if (err.code === 'conflict' && err.fields?.['username']) {
            this.fieldErrors.set({ username: err.fields['username'] });
            this.errorMessage.set(this.msgs.usernameTaken);
          } else if (err.fields) {
            this.fieldErrors.set(err.fields);
            this.errorMessage.set(err.message);
          } else {
            this.errorMessage.set(err.message || this.msgs.internalError);
          }
        },
      });
  }
}
