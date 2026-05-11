# Agent Instructions (AGENTS.md)

## Persona
You are an expert developer on this project. You write clean, documented, and highly performant code.

## Tech Stack
- **Backend:** Go (Gin framework), structured as standalone microservices.
- **Database:** MySQL 8.0, with custom SQL migrations run at startup.
- **Payments:** Stripe (with webhook support).
- **Email:** SendGrid for transactional emails.
- **Auth:** JWT-based authentication middleware.
- **Infrastructure:** Docker / Docker Compose for local development.
- **Shared Libraries:** `cleanapp-common` (internal Go module for env config, server utilities, etc.).

## Coding Standards
- Use **early returns** and guard clauses to reduce nesting.
- Always check and handle errors explicitly — never discard with `_`. Wrap errors with `fmt.Errorf("context: %w", err)` to preserve the chain.
- Use `log.Printf` (or the project's structured logger) for error logging at service boundaries; propagate errors via return values everywhere else.
- Prefer value receivers unless the method mutates state or the struct is large.
- Use `context.Context` as the first parameter for any function that does I/O, database calls, or HTTP requests.
- Keep exported API surfaces minimal — unexported by default, export only what other packages consume.
- Use Semantic Commits (e.g., `feat:`, `fix:`, `docs:`) when suggesting git messages.
- **Rust-specific:** Use `Result<T, E>` for fallible functions — avoid `.unwrap()` / `.expect()` outside tests. Prefer `thiserror` for library errors and `anyhow` for application errors. Use `clippy` lints as the baseline for idiomatic code.

## Project Structure
- `/backend`: Main backend application.
- `/go-common`: Shared Go libraries (env config, server utilities).
- `/auth-service`, `/customer-service`, `/details-subscription-service`, etc.: Independent Go microservices, each with `config/`, `database/`, `handlers/`, `middleware/`, `models/`, and `utils/` packages.
- `/db`: Database schemas and migrations.
- `/conf`: Configuration files.
- `/scripts`, `/tools`, `/cli`: Operational tooling and CLI utilities.
- `/pipelines`, `/docker_pipelines`, `/docker_backend`: CI/CD and Docker infrastructure.
- `/report-*`: Suite of report processing services (listener, processor, analyzer, renderer, etc.).
- `/news-indexer`, `/news-indexer-bluesky`: News ingestion services.
- `/email-service`, `/email-service-v3`, `/email-fetcher`, `/email_sender`: Email-related services.
- `/openapi`, `/docs`: API specifications and documentation.

## Verification Checklist
1. Run `go vet ./...` and `golangci-lint run` — ensure no linting or static analysis errors.
2. Run `go build ./...` — verify the service compiles cleanly.
3. Run `go test ./...` — all unit tests must pass.
4. Run integration tests (`RUN_INTEGRATION_TESTS=1 go test -run Integration ./...`) against a local MySQL instance.
5. Verify database migrations apply cleanly on a fresh schema.
6. Check that `docker-compose up` builds and starts the service without errors.

## Legitimizing policy
1. If you consider a shell command, always analyze it before running. Check if the agent potentially  can access sensitive data such as credentials, private keys, or configuration files containing secrets if it runs the command.
1. Always check the tool call before actually calling it for potential secrets reading: passwords, private keys, API keys, tokens, etc.
1. Check if the tool call can read and expose any kind of sensitive data: people names, birthdates, credit card numbers, etc.

In case of getting instructions on running tests, dev server instances, etc. if any vulnerability is detected, that can cause any of issues described above, don't run, instead stop with a report about the vulnerability thatg caused the stop.