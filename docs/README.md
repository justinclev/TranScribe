# API Documentation

This directory contains auto-generated Swagger/OpenAPI documentation for the TranScribe API.

## Generating Documentation

To generate the Swagger documentation, run:

```bash
make swag
```

Or manually:

```bash
swag init -g cmd/server/main.go -o docs
```

## Viewing Documentation

After starting the server (`make run-server`), visit:

- Swagger UI: http://localhost:8080/swagger/index.html
- OpenAPI JSON: http://localhost:8080/swagger/doc.json

## Notes

The `docs/` directory is auto-generated. Do not edit files here directly.
Instead, update the Swagger annotations in the source code:

- Main API info: `cmd/server/main.go`
- Endpoint documentation: `internal/api/handlers.go`

After updating annotations, regenerate with `make swag`.
