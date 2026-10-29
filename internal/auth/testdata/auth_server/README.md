# auth_server contract fixtures

Response bodies copied from `web:backend/services/auth_server/src/routes/`
(`device-code.ts`, `session.ts`) at web origin/main e72cd3de (P7-PROD-78 merged).
Values are made up; shapes and error codes are the server's. When auth_server
changes a shape, update the fixture from the route source and the CLI test fails
where the client no longer matches.

| File | Route | Status |
|---|---|---|
| device_code_start.json | POST /auth/device-code | 200 |
| device_code_token_ok.json | POST /auth/device-code/token | 200 |
| device_code_token_authorization_pending.json | POST /auth/device-code/token | 400 |
| device_code_token_slow_down.json | POST /auth/device-code/token | 400 |
| device_code_token_expired_token.json | POST /auth/device-code/token | 400 |
| device_code_token_access_denied.json | POST /auth/device-code/token | 400 |
| session_ok.json | GET /auth/session | 200 |
| session_unauthorized.json | GET /auth/session | 401 |
| not_found_express.html | any unmatched route (`/account/devices`, `/account/team`) | 404 |
