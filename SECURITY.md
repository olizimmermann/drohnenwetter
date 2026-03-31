# Security Policy

## Supported Versions

Only the latest version on `main` is actively maintained and receives security fixes.

## Reporting a Vulnerability

**Please do not report security vulnerabilities via GitHub Issues.**

Send a description of the issue to: **public@ozimmermann.com**

Include as much detail as possible:
- Type of vulnerability (XSS, injection, authentication bypass, etc.)
- Affected component (Go handler, nginx config, template, etc.)
- Steps to reproduce
- Potential impact

You can expect an acknowledgement within **48 hours** and a fix or mitigation plan within **7 days** for confirmed issues.

## Scope

In scope:
- The Go web application (`go/`)
- nginx configuration (`nginx/`)
- API integration security (credential exposure, SSRF, etc.)

Out of scope:
- Vulnerabilities in upstream dependencies (report to the respective maintainers)
- Theoretical attacks with no realistic exploit path
- Rate limiting bypass via IP spoofing (by design — Cloudflare is the perimeter)

## Disclosure Policy

Once a fix is available, the vulnerability will be disclosed in the release notes. Credit will be given to reporters unless anonymity is requested.
