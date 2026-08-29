// web/src/app/core/auth.guard.ts
// PRESENTATION-ONLY guards.
//
// Constitution I is explicit: route guards are user-experience, not security.
// The server checks every request, and a manager's cookie gets 403 on /users
// and /audit-records regardless of what the UI shows. The comments below name
// this so the rule cannot be missed.
//
// These guards are only correct because the session is resolved BEFORE the
// router runs: app.config.ts registers provideAppInitializer(() => refresh()),
// so SessionService already knows signed-in vs signed-out by the time any guard
// executes. Without that, every guard reads a null user on a page load and a
// signed-in visitor is shown the sign-in screen.

import { CanActivateFn, Router, UrlTree } from '@angular/router';
import { inject } from '@angular/core';
import { SessionService } from './session.service';

// Where a signed-in user belongs. Managers have no screen of their own in this
// feature, so they land on /home; admins go straight to the account list.
export function landingPath(session: SessionService): string {
  const user = session.user();
  if (!user) {
    return '/sign-in';
  }
  if (user.mustChangePassword) {
    return '/change-password';
  }
  return session.isAdmin() ? '/settings/users' : '/home';
}

// Root route: send each visitor to the right place rather than always to
// /sign-in, which previously bounced signed-in users back and forth.
export const landingGuard: CanActivateFn = (): UrlTree => {
  const session = inject(SessionService);
  return inject(Router).parseUrl(landingPath(session));
};

export const requireSignedInGuard: CanActivateFn = () => {
  const session = inject(SessionService);
  const router = inject(Router);
  if (session.user()) {
    return true;
  }
  return router.createUrlTree(['/sign-in']);
};

// Sign-in is for signed-out visitors. A signed-in one is sent to their landing
// page, never to '/' — '/' resolves through landingGuard and would loop.
export const requireSignedOutGuard: CanActivateFn = () => {
  const session = inject(SessionService);
  const router = inject(Router);
  if (!session.user()) {
    return true;
  }
  return router.parseUrl(landingPath(session));
};

// FR-007 and FR-010: while a password change is outstanding the user must not
// reach any other screen. The API enforces this; the UI must not offer it.
export const requirePasswordChangedGuard: CanActivateFn = () => {
  const session = inject(SessionService);
  const router = inject(Router);
  if (!session.mustChangePassword()) {
    return true;
  }
  return router.createUrlTree(['/change-password']);
};

export const requireAdminGuard: CanActivateFn = () => {
  // Presentation only. The server is the authority on role (Principle I).
  // A manager who reaches /users by URL still gets 403 from the API.
  const session = inject(SessionService);
  const router = inject(Router);
  if (session.isAdmin()) {
    return true;
  }
  return router.parseUrl(landingPath(session));
};
