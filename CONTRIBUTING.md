# Contributing to drohnenwetter.de

Thank you for your interest in contributing. This is a personal project but contributions are welcome within the scope below.

## What contributions are welcome

- **Bug fixes** — especially safety assessment logic or API integration issues
- **German airspace / regulation corrections** — if you spot outdated or incorrect zone handling
- **UI/UX improvements** — the results page and map interactions
- **Translations** — corrections to the DE/EN bilingual copy
- **Documentation** — improving README, inline comments

## What is out of scope

- New data sources that require paid API keys the maintainer can't sustain
- Features unrelated to drone flight safety in Germany
- Breaking changes to the Go module structure without prior discussion

## Getting started

1. Fork the repository and create a branch from `main`
2. Build with Docker (Go is not required locally):
   ```bash
   docker build -t drohnenwetter-dev ./go
   ```
3. Run locally:
   ```bash
   docker run --rm -p 8080:8080 --env-file .env drohnenwetter-dev
   ```
4. Run smoke tests:
   ```bash
   ./checks/smoke.sh http://localhost:8080
   ```

## Pull request guidelines

- Keep PRs focused — one fix or feature per PR
- Describe *why* the change is needed, not just what changed
- Make sure the Docker build succeeds and smoke tests pass before submitting
- Do not commit `.env` or any file containing API keys

## License

By contributing, you agree that your contributions will be licensed under the same [PolyForm Noncommercial License 1.0.0](LICENSE) as this project.
