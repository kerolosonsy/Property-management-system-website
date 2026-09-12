// web/src/app/features/sign-in/change-password/change-password.component.ts
// Password-change screen, reached two ways:
//   - forced, when mustChangePassword is set after creation or an
//     administrator reset (FR-007, FR-010); and
//   - chosen, from the entry point in the application shell (FR-011).
// The prompt distinguishes the two so the chosen path does not read as a
// demand. Either way the server revokes every session for the account
// (FR-020), so the user signs in again afterwards; the screen says so rather
// than letting that look like a fault.

import { Component, computed, inject, input, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { AuthService } from '../../../api/api/auth.service';
import { ARABIC_MESSAGES } from '../../../shared/messages';
import { ApiError } from '../../../core/api-error';
import { SessionService } from '../../../core/session.service';

@Component({
  selector: 'app-change-password',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule],
  template: `
    <section [class.pms-auth-page]="!embedded()">
      <div
        class="card blueprint pms-card"
        [class.elev-md]="!embedded()"
        [class.pms-auth-card]="!embedded()"
      >
        <i class="corner tl"></i><i class="corner tr"></i> <i class="corner bl"></i
        ><i class="corner br"></i>

        @if (embedded()) {
          <h2>{{ msgs.passwordChangeTitle }}</h2>
          <p class="text-muted">{{ msgs.passwordChangeOptionalPrompt }}</p>
        } @else {
          <div class="pms-auth-head">
            <div>
              <div class="pms-auth-title">{{ msgs.passwordChangeTitle }}</div>
              <div class="pms-auth-sub">
                {{ forced() ? msgs.passwordChangePrompt : msgs.passwordChangeOptionalPrompt }}
              </div>
            </div>
          </div>
        }
        <p class="text-muted">{{ msgs.passwordChangeEndsSessions }}</p>

        <form id="pms-change-password-form" [formGroup]="form" (ngSubmit)="onSubmit()" novalidate>
          <div class="field pms-field">
            <label for="currentPassword">{{ msgs.currentPassword }}</label>
            <input
              class="input"
              id="currentPassword"
              type="password"
              formControlName="currentPassword"
              autocomplete="current-password"
            />
            @if (
              form.controls.currentPassword.touched &&
              form.controls.currentPassword.hasError('required')
            ) {
              <div class="pms-field-error">{{ msgs.requiredField }}</div>
            }
          </div>

          <div class="field pms-field">
            <label for="newPassword">{{ msgs.newPassword }}</label>
            <input
              class="input"
              id="newPassword"
              type="password"
              formControlName="newPassword"
              autocomplete="new-password"
            />
            @if (
              form.controls.newPassword.touched && form.controls.newPassword.hasError('required')
            ) {
              <div class="pms-field-error">{{ msgs.requiredField }}</div>
            }
            @if (
              form.controls.newPassword.touched && form.controls.newPassword.hasError('minlength')
            ) {
              <div class="pms-field-error">{{ msgs.passwordTooShort }}</div>
            }
          </div>

          @if (errorMessage(); as msg) {
            <div class="pms-error-banner" role="alert">{{ msg }}</div>
          }

          <!-- Standalone (the forced change) keeps the design's submit at the
               foot of the centred card. Embedded (the profile screen) the
               submit moves into a pinned foot so it stays reachable; it
               reaches this same form by id. One button either way, same
               bindings. -->
          @if (!embedded()) {
            <button
              type="submit"
              class="btn btn-primary btn-block"
              [disabled]="submitting() || form.invalid"
            >
              {{ submitting() ? msgs.loading : msgs.passwordChangeSubmit }}
            </button>
          }
        </form>
      </div>

      @if (embedded()) {
        <footer class="pms-view-foot">
          <button
            type="submit"
            class="btn btn-primary"
            [disabled]="submitting() || form.invalid"
            form="pms-change-password-form"
          >
            {{ submitting() ? msgs.loading : msgs.passwordChangeSubmit }}
          </button>
        </footer>
      }
    </section>
  `,
})
export class ChangePasswordComponent {
  // Standalone (false) renders the design's centred blueprint card, used for
  // the forced change after first sign-in or an administrator reset. Embedded
  // (true) renders just the form, for the profile screen which supplies its
  // own heading. One component, so the two flows cannot drift apart.
  readonly embedded = input(false);

  protected readonly msgs = ARABIC_MESSAGES;
  private readonly fb = inject(FormBuilder);
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly session = inject(SessionService);

  // Forced when the server says a change is outstanding; otherwise the user
  // chose to be here. Declared after `session` because TypeScript initialises
  // class fields in order.
  protected readonly forced = computed(() => this.session.mustChangePassword());

  protected readonly form = this.fb.nonNullable.group({
    currentPassword: ['', [Validators.required]],
    newPassword: ['', [Validators.required, Validators.minLength(12)]],
  });

  protected readonly submitting = signal(false);
  protected readonly errorMessage = signal<string | null>(null);

  protected onSubmit(): void {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    this.submitting.set(true);
    this.errorMessage.set(null);

    const { currentPassword, newPassword } = this.form.getRawValue();

    this.auth
      .changeOwnPassword({ changeOwnPasswordRequest: { currentPassword, newPassword } }, 'response')
      .subscribe({
        next: () => {
          this.session.signOut();
          this.submitting.set(false);
          // After password change the server revokes every session — there is
          // no auto-sign-in. Send the user back to sign-in.
          void this.router.navigate(['/sign-in']);
        },
        error: (err: ApiError) => {
          this.submitting.set(false);
          if (err.code === 'invalid_credentials') {
            this.errorMessage.set(this.msgs.passwordWrongCurrent);
          } else if (err.code === 'invalid_request' && err.fields?.['newPassword']) {
            this.errorMessage.set(err.fields['newPassword']);
          } else if (err.message) {
            this.errorMessage.set(err.message);
          } else {
            this.errorMessage.set(this.msgs.internalError);
          }
        },
      });
  }
}
