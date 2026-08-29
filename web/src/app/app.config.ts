import {
  ApplicationConfig,
  provideAppInitializer,
  provideBrowserGlobalErrorListeners,
  provideZonelessChangeDetection,
  inject,
} from '@angular/core';
import { provideRouter, withComponentInputBinding } from '@angular/router';
import { provideHttpClient, withInterceptors } from '@angular/common/http';
import { provideApi } from './api/provide-api';
import { routes } from './app.routes';
import { apiErrorInterceptor } from './core/api.interceptor';
import { SessionService } from './core/session.service';

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideZonelessChangeDetection(),
    provideRouter(routes, withComponentInputBinding()),
    provideHttpClient(withInterceptors([apiErrorInterceptor])),
    // The Angular dev server proxies /api/v1 to https://localhost:8443/api/v1.
    // In production the same path serves the bundled API behind a reverse
    // proxy. Either way, /api/v1 is the origin-relative base path; the
    // browser sends the HttpOnly session cookie on same-origin requests.
    provideApi({ basePath: '/api/v1', withCredentials: true }),
    // Resolve the session before the router activates any route. Angular waits
    // for this promise during bootstrap, so every guard sees the real
    // signed-in/signed-out state instead of the initial 'unknown'. Without it a
    // signed-in visitor is shown the sign-in screen on every page load, because
    // requireSignedOutGuard reads a null user that /auth/me has not filled yet.
    provideAppInitializer(() => inject(SessionService).refresh()),
  ],
};