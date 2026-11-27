# Instructions for AI coding agents

## Engineering standards (non-negotiable)

The full text is `docs/ENGINEERING.md` in the golang-template repository (github.com/jetlio/golang-template; locally `~/Work/golang-template/docs/ENGINEERING.md`). Read it before designing or changing code, and say in the plan how the change meets it. References: golang-template, and in backend-mono `apps/monitor` (layout, wiring, lint-enforced layers, apperr) and `apps/assistant` (huma). **Never copy patterns from older services.**

1. **One source of truth.** Generate from the owner of a fact: OpenAPI → clients, catalogs → constants, migrations → sqlc. Never hand-edit generated code; CI checks for drift.
2. **Clean layers.** handler → service → repo, with dto and mapper beside them, enforced by lint.
3. **Dependency injection.** Use `Deps`/`Options` in one composition root. No global state, no singletons.
4. **Typed vocabularies.** Enums, statuses, events and error codes are typed constants or generated. No magic strings or numbers. Configuration is declared once.
5. **Secure by design.** Deny by default. Guard every route from the permission catalog; scope tenants in the query itself. Hash or seal secrets, wire up validation, send outbound calls through the SSRF-safe client, redact logs, keep an audit trail.
6. **Reliable.** A deadline on every outside call, bounded retries, atomic changes, an outbox for side effects, supervised workers (never a bare `go func()`), graceful shutdown.
7. **Bounded and scalable.** Sizes, pages and concurrency are capped. Nothing assumes a single replica.
8. **Errors.** Domain errors map to RFC 9457 problems with enumerated codes. Internals never leak.
9. **Tested at the boundaries,** including authorization denials. Every security fix gets a regression test.
10. **Process.** Docs ship with the change. Short-lived branches and PRs, commits with explicit paths, pinned tool versions. Spikes are labelled as spikes.
