// web/src/app/core/api.interceptor.ts
// Maps the API's single error envelope into an ApiError instance and ensures
// every request carries the session cookie (withCredentials so the HttpOnly
// cookie is sent cross-origin in dev, where Angular runs on 4200 and the API
// on 8443). On 401 it clears the local session and redirects to /sign-in so
// an expired session returns the user to sign-in instead of leaving them on a
// broken screen (FR-019, FR-020).

import { HttpErrorResponse, HttpHandlerFn, HttpInterceptorFn, HttpRequest } from '@angular/common/http';
import { inject } from '@angular/core';
import { Router } from '@angular/router';
import { catchError, throwError } from 'rxjs';
import { ApiError, ApiErrorBody } from './api-error';
import { SessionService } from './session.service';

export const apiErrorInterceptor: HttpInterceptorFn = (req: HttpRequest<unknown>, next: HttpHandlerFn) => {
  const router = inject(Router);
  const session = inject(SessionService);

  const withCreds = req.clone({ withCredentials: true });
  return next(withCreds).pipe(
    catchError((err: unknown) => {
      if (err instanceof HttpErrorResponse) {
        const body = err.error as Partial<ApiErrorBody> | null;
        // 401 → end any local session assumption and send the user to sign-in.
        // Do this for any 401, not just for /auth/me, so the interceptor
        // covers every code path that might run while the session has lapsed.
        if (err.status === 401) {
          session.signOut();
          if (!router.url.startsWith('/sign-in')) {
            void router.navigate(['/sign-in']);
          }
        }
        if (body && typeof body === 'object' && 'code' in body && 'message' in body) {
          return throwError(() => new ApiError(err.status, body as ApiErrorBody));
        }
        // Network failure or empty body: surface as internal_error.
        return throwError(() => new ApiError(err.status || 0, {
          code: 'internal_error',
          message: 'حدث خطأ غير متوقع. الرجاء المحاولة مرة أخرى.',
        }));
      }
      return throwError(() => err);
    })
  );
};