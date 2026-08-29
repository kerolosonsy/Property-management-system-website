// web/src/app/core/api-error.ts
// Typed representation of the API's single error envelope
// (contracts/openapi.yaml components.schemas.Error).

export type ErrorCode =
  | 'invalid_request'
  | 'invalid_credentials'
  | 'too_soon'
  | 'not_authenticated'
  | 'forbidden'
  | 'not_found'
  | 'conflict'
  | 'password_change_required'
  | 'internal_error';

export interface ApiErrorBody {
  code: ErrorCode;
  message: string;
  fields?: Record<string, string> | null;
  retryAfterSeconds?: number;
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: ErrorCode;
  readonly fields: Record<string, string> | null;
  readonly retryAfterSeconds: number | null;

  constructor(status: number, body: ApiErrorBody) {
    super(body.message);
    this.status = status;
    this.code = body.code;
    this.fields = body.fields ?? null;
    this.retryAfterSeconds = body.retryAfterSeconds ?? null;
  }
}