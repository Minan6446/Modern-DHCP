# Auth Module Flow (Feature-Sliced)

## Scope
Covers login, token storage, refresh, and profile validation in `modules/auth`.

## Key Responsibilities
- API: `modules/auth/api` (login/logout/refresh/getProfile)
- Store: `modules/auth/store` (tokens, remember flag, user, error/loading)
- View: `modules/auth/views/Login.vue` (form, validation, UX)
- Errors: `modules/auth/hooks/errors.ts` (status→message mapping)
- Types: `modules/auth/types` (LoginRequest/Response, TokenPair, AuthErrorCode)

## Flows
1. **Login**
   - Form validates patterns; submit guarded by `isLoading` and clears error.
   - Store `login` saves tokens (session for access, remember-controlled storage for refresh), caches user/lastUsername/remember.
   - Success clears error and password; redirect respects safe redirect (starts with '/').
2. **CheckAuth**
   - Restores tokens/user/remember from storage.
   - If access will expire soon and refresh exists → refresh.
   - If access exists → call `getProfile`; on failure, logout and surface error.
3. **Refresh**
   - Uses refresh token and remember flag to persist new tokens.
   - Errors mapped via `mapAuthError`; expired clears state.
4. **Logout**
   - Calls API then clears tokens, user, remember, and errors.
5. **Error Handling**
   - `mapAuthError` covers 401/423/429/498/5xx/network/other; store assigns friendly text and clears on success/inputs.

## Storage Strategy
- Access token: sessionStorage
- Refresh token: sessionStorage or localStorage (controlled by remember flag)
- Remember flag: REMEMBER_KEY (session/local)
- User + lastUsername: session/local per remember flag

## Rollback
- Legacy wrappers remain (views/system/Login.vue, api/system/auth.ts, store/auth.ts re-export); switch imports back to legacy paths if needed.
