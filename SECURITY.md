# Security Policy

## Supported versions

Security fixes are released for the latest published version of GitContribute.
Please upgrade before reporting a problem that may already be fixed.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability. Use
[GitHub private vulnerability reporting](https://github.com/morluto/gitcontribute/security/advisories/new)
to share the details privately with the maintainer.

Include enough information to reproduce and assess the issue when possible:

- the affected GitContribute version and platform;
- the capability involved, such as corpus reads, GitHub access, acquisition,
  validation, or MCP;
- a minimal reproduction or proof of concept;
- the expected and observed security boundary; and
- any known impact or suggested mitigation.

Please avoid accessing data that is not yours, disrupting third-party systems,
or publishing details before a fix is available. Reports will be acknowledged
and evaluated as promptly as practical.

## Security boundaries

GitContribute separates corpus reads, network reads, local writes, process
execution, and GitHub mutation. Read [the architecture guide](docs/architecture.md)
for the security model and explicit non-goals.
