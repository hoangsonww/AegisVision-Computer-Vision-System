# auth-proxy

> **JWT verifier + tenant injector.** The thing that turns a Bearer token
> into a verified tenant identity.

`auth-proxy` runs as a sidecar (or as a standalone deployment behind
api-gateway, depending on the environment). It:

1. Reads the `Authorization: Bearer <jwt>` header.
2. Verifies the signature against the IdP's **JWKS** endpoint.
3. Verifies `iss`, `aud`, `exp`, `nbf`.
4. Extracts `tenant_id` from claims.
5. Injects `X-Aegis-Tenant` into the downstream request.

**HS* JWTs are deliberately NOT supported.** Sharing the signing key with
every verifier defeats the point of OIDC. The platform refuses to start
if `AEGIS_JWT_ALGS` includes `HS256`/`HS384`/`HS512`.

---

## Configuration

| Var | Purpose | Required |
| --- | --- | --- |
| `AEGIS_JWKS_URL` | IdP JWKS endpoint. | yes |
| `AEGIS_JWT_ISSUER` | Expected `iss`. | yes |
| `AEGIS_JWT_AUDIENCE` | Expected `aud`. | yes |
| `AEGIS_JWT_TENANT_CLAIM` | Claim name for tenant. Default `tenant_id`. | no |

---

## Metrics

- `aegis_auth_verifications_total{result}` — result ∈ {ok, bad_sig, expired, wrong_iss, wrong_aud, no_tenant}.
- `aegis_auth_jwks_cache_age_seconds` — gauge.
- `aegis_auth_jwks_refresh_failures_total` — counter.

---

## Failure modes

| Failure | Effect | Mitigation |
| --- | --- | --- |
| JWKS endpoint unreachable | 503 on new tokens | Cache up to 5 min. |
| Token expired | 401 | Standard. |
| Token signed with rotated key | 401 | Force JWKS refresh on cache miss. |
| `AEGIS_JWT_ALGS` includes HS* | Panic at startup | By design. |

---

## See also

- [`../api-gateway/README.md`](../api-gateway/README.md).
- ADR-0018 (HS* refusal rationale).
