export interface ApiResponse<T> {
  code: number;
  message: string;
  data: T;
  requestId?: string;
  traceId?: string;
  timestamp?: string;
}

export interface ApiErrorField {
  field: string;
  code?: string;
  message: string;
}

export interface ApiErrorDetails {
  type?: string;
  hint?: string;
  fields?: ApiErrorField[];
  errorId?: string;
  cause?: string;
  stack?: string[];
}

export interface ApiErrorPayload {
  code?: number;
  message?: string;
  requestId?: string;
  traceId?: string;
  timestamp?: string;
  details?: ApiErrorDetails;
}

export interface HttpErrorShape {
  status?: number;
  payload?: ApiErrorPayload;
  network?: boolean;
}
