// web/src/app/core/api-base.ts
// One place that knows where the API is reached. The proxy.conf.json handles
// the dev-time rewrite, so this returns the same /api/v1 prefix the rest of
// the codebase uses.

export function apiBasePath(): string {
  return '/api/v1';
}
