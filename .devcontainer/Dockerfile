# Official dev container image: Go (latest 1.x), gopls, delve,
# golangci-lint and gcc for CGO are already included.
FROM mcr.microsoft.com/devcontainers/go:1-trixie

# sqlite3 CLI for inspecting the DB, make for task shortcuts; headless
# chromium, fonts and python3-venv (for Playwright) to check the UI at
# phone widths
RUN apt-get update \
 && export DEBIAN_FRONTEND=noninteractive \
 && apt-get install -y --no-install-recommends sqlite3 make \
      chromium fonts-dejavu-core python3-venv \
 && apt-get clean && rm -rf /var/lib/apt/lists/*

# Project tools: sqlc (codegen from SQL), goose (migrations), air (live reload),
# gofumpt (formatting)
# Consider pinning versions later, e.g. @v1.30.0
RUN go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest \
 && go install github.com/pressly/goose/v3/cmd/goose@latest \
 && go install github.com/air-verse/air@latest \
 && go install mvdan.cc/gofumpt@latest \
 && go clean -cache -modcache \
 && chown -R vscode:golang /go
