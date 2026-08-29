// web/src/app/features/sign-in/sign-in.component.ts
// User Story 1 — the Arabic sign-in screen.
//
// On submit: POST /auth/login → 200 with CurrentUser, or refusal. The refusal
// message is identical for an unknown username, wrong password, and
// deactivated account (FR-013). The digit directive converts any Eastern
// digits typed into the username field (FR-033).

import { Component, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { Router } from '@angular/router';
import { landingPath } from '../../core/auth.guard';
import { AuthService } from '../../api/api/auth.service';
import { WesternDigitsDirective } from '../../shared/western-digits.directive';
import { ARABIC_MESSAGES, format } from '../../shared/messages';
import { ApiError } from '../../core/api-error';
import { SessionService } from '../../core/session.service';

@Component({
  selector: 'app-sign-in',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, WesternDigitsDirective],
  template: `
    <section class="pms-page">
      <div class="pms-card" style="max-inline-size: 28rem; margin-inline: auto;">
        <h1>{{ msgs.signInTitle }}</h1>
        <p class="pms-muted">{{ msgs.signInPrompt }}</p>

        <form [formGroup]="form" (ngSubmit)="onSubmit()" novalidate>
          <div class="pms-field">
            <label for="username">{{ msgs.username }}</label>
            <input id="username" type="text" formControlName="username" autocomplete="username"
                   appWesternDigits autocapitalize="off" spellcheck="false" />
            @if (form.controls.username.touched && form.controls.username.invalid) {
              <div class="pms-field-error">
                @if (form.controls.username.hasError('required')) { {{ msgs.requiredField }} }
                @else if (form.controls.username.hasError('minlength') || form.controls.username.hasError('maxlength')) { {{ msgs.usernameLength }} }
              </div>
            }
          </div>

          <div class="pms-field">
            <label for="password">{{ msgs.password }}</label>
            <input id="password" type="password" formControlName="password" autocomplete="current-password" />
            @if (form.controls.password.touched && form.controls.password.hasError('required')) {
              <div class="pms-field-error">{{ msgs.requiredField }}</div>
            }
          </div>

          @if (errorMessage(); as msg) {
            <div class="pms-error-banner" role="alert">{{ msg }}</div>
          }

          <button type="submit" class="pms-button" [disabled]="submitting() || form.invalid">
            {{ submitting() ? msgs.loading : msgs.signInSubmit }}
          </button>
        </form>
      </div>
    </section>
  `,
})
export class SignInComponent {
  protected readonly msgs = ARABIC_MESSAGES;
  private readonly fb = inject(FormBuilder);
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly session = inject(SessionService);

  protected readonly form = this.fb.nonNullable.group({
    username: ['', [Validators.required, Validators.minLength(3), Validators.maxLength(32)]],
    password: ['', [Validators.required]],
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

    const { username, password } = this.form.getRawValue();

    this.auth.signIn({ signInRequest: { username, password } }, 'body').subscribe({
      next: (user) => {
        this.session.signIn(user);
        this.submitting.set(false);
        // landingPath honours mustChangePassword and role: a manager has no
        // account screen, so sending everyone to /users bounced them off the
        // administrator guard.
        void this.router.navigateByUrl(landingPath(this.session));
      },
      error: (err: ApiError) => {
        this.submitting.set(false);
        if (err.code === 'too_soon' && err.retryAfterSeconds !== null) {
          this.errorMessage.set(format(this.msgs.tooSoonWithRemaining, { seconds: err.retryAfterSeconds }));
        } else if (err.code === 'invalid_credentials') {
          this.errorMessage.set(this.msgs.signInFailed);
        } else {
          this.errorMessage.set(err.message || this.msgs.internalError);
        }
      },
    });
  }
}