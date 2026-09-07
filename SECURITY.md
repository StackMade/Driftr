# Security Policy

## Supported versions

Only the latest release receives security fixes. Driftr is a single static binary, so
upgrading means replacing it — `driftr self-update` does that in place.

## Reporting a vulnerability

Report vulnerabilities privately through GitHub, using
[Report a vulnerability](https://github.com/StackMade/Driftr/security/advisories/new).
Please do not open a public issue for anything exploitable.

Include the version (`driftr --version`), your OS and architecture, and the steps that
reproduce the problem. If the issue depends on a particular project layout — a
`.driftr.toml`, a `package.json` pin, a shim on `PATH` — say so; the resolver walks
several config sources and the details matter.

Expect an acknowledgement within a few days. Once a fix is ready it ships in a release
and the advisory is published with credit, unless you would rather stay anonymous.

## What is in scope

Driftr downloads archives over the network, verifies them, extracts them, and executes
the binaries it installed. Findings in that path are the interesting ones:

- checksum or SRI verification that can be bypassed
- archive extraction that writes outside the version directory
- a resolved version or binary path that an untrusted repository can control
- shims or `PATH` setup that let a checked-out project execute code unexpectedly
- anything that leaks credentials from the environment into a downloaded process

Out of scope: vulnerabilities in Node.js, npm, or bun themselves — report those upstream.
The same goes for problems that require an attacker to already have write access to
`~/.driftr` or to the user's shell configuration.
