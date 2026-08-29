// web/src/app/features/sign-in/change-password/change-password.component.ts
// Forced password-change screen reached when mustChangePassword is true on
// the current user. The route guard also routes here from anywhere the
// server replies with password_change_required.

import { Component, inject, signal } from '@angular/core';
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
    <section class="pms-page">
      <div class="pms-card" style="max-inline-size: 32rem; margin-inline: auto;">
        <h1>{{ msgs.passwordChangeTitle }}</h1>
        <p class="pms-muted">{{ msgs.passwordChangePrompt }}</p>

        <form [formGroup]="form" (ngSubmit)="onSubmit()" novalidate>
          <div class="pms-field">
            <label for="currentPassword">{{ msgs.currentPassword }}</label>
            <input id="currentPassword" type="password" formControlName="currentPassword" autocomplete="current-password" />
            @if (form.controls.currentPassword.touched && form.controls.currentPassword.hasError('required')) {
              <div class="pms-field-error">{{ msgs.requiredField }}</div>
            }
          </div>

          <div class="pms-field">
            <label for="newPassword">{{ msgs.newPassword }}</label>
            <input id="newPassword" type="password" formControlName="newPassword" autocomplete="new-password" />
            @if (form.controls.newPassword.touched && form.controls.newPassword.hasError('required')) {
              <div class="pms-field-error">{{ msgs.requiredField }}</div>
            }
            @if (form.controls.newPassword.touched && form.controls.newPassword.hasError('minlength')) {
              <div class="pms-field-error">{{ msgs.passwordTooShort }}</div>
            }
          </div>

          @if (errorMessage(); as msg) {
            <div class="pms-error-banner" role="alert">{{ msg }}</div>
          }

          <button type="submit" class="pms-button" [disabled]="submitting() || form.invalid">
            {{ submitting() ? msgs.loading : msgs.passwordChangeSubmit }}
          </button>
        </form>
      </div>
    </section>
  `,
})
export class ChangePasswordComponent {
  protected readonly msgs = ARABIC_MESSAGES;
  private readonly fb = inject(FormBuilder);
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly session = inject(SessionService);

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

    this.auth.changeOwnPassword({ changeOwnPasswordRequest: { currentPassword, newPassword } }, 'response').subscribe({
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