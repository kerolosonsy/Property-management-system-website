// web/src/app/core/session.service.ts
// Holds the signed-in user as a signal. The server remains the authority on
// role and active state (FR-034); this service caches what the server last
// told us and exposes a refresh() that reads /auth/me afresh.

import { Injectable, inject, signal, computed } from '@angular/core';
import { CurrentUser } from '../api/model/current-user.model';
import { AuthService } from '../api/api/auth.service';
import { ApiError } from './api-error';

type SessionState =
  | { kind: 'unknown' }
  | { kind: 'signed-out' }
  | { kind: 'signed-in'; user: CurrentUser };

@Injectable({ providedIn: 'root' })
export class SessionService {
  private readonly auth = inject(AuthService);

  private readonly state = signal<SessionState>({ kind: 'unknown' });

  readonly user = computed<CurrentUser | null>(() => {
    const s = this.state();
    return s.kind === 'signed-in' ? s.user : null;
  });

  readonly isAdmin = computed(() => this.user()?.role === 'admin');

  readonly mustChangePassword = computed(() => this.user()?.mustChangePassword === true);

  signIn(user: CurrentUser): void {
    this.state.set({ kind: 'signed-in', user });
  }

  setUnknown(): void {
    this.state.set({ kind: 'unknown' });
  }

  signOut(): void {
    this.state.set({ kind: 'signed-out' });
  }

  // Refresh hits /auth/me. If it 401s, the session is gone; if it 403s with
  // password_change_required, the user must change their password.
  refresh(): Promise<void> {
    return new Promise((resolve) => {
      this.auth.getCurrentUser('body').subscribe({
        next: (user) => {
          this.state.set({ kind: 'signed-in', user });
          resolve();
        },
        error: (err: ApiError) => {
          if (err.code === 'not_authenticated') {
            this.state.set({ kind: 'signed-out' });
          } else {
            // Surface but keep the previous state so UI can react.
          }
          resolve();
        },
      });
    });
  }
}