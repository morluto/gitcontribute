# ADR 0003: Explicit execution boundaries

Status: accepted

## Context

Repository code and contribution validation are useful but untrusted. A local
search command should never imply network access, process execution, or GitHub
mutation.

## Decision

The application distinguishes read-only corpus operations, network reads,
local writes, process execution, and external mutations as separate
capabilities. Version 1 exposes no GitHub mutation. Repository-controlled code
is not executed by crawl, indexing, search, or dossier operations.

Validation commands record the exact command, directory, commit, environment
policy, timing, exit status, and captured output. Unattended execution remains
deferred until an isolation contract is implemented and tested.

## Consequences

- MCP annotations can accurately describe side effects.
- Agents cannot gain execution authority through a read tool.
- Some workflows require an explicit additional command from the user.
