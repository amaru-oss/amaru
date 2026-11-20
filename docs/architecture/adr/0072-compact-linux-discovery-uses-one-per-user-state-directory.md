---
title: Resolve compact Linux discovery from one per-user state directory
status: accepted
date: 2026-11-20
deciders: Ricardo Lüders
---

# 0072. Resolve compact Linux discovery from one per-user state directory

## Context

[ADR-0034](0034-local-web-client-authentication-with-launch-code-and-cookie.md) requires one compact Instance per OS user and private native discovery. [ADR-0068](0068-platform-process-readiness-and-bounded-shutdown.md) owns publication through the Platform lifecycle. Platform startup and the native launcher need the same location across working directories.

## Decision

On Linux, resolve the compact discovery directory to `$XDG_STATE_HOME/amaru` when `XDG_STATE_HOME` is absolute. Otherwise use `$HOME/.local/state/amaru`, requiring an absolute home directory. Follow the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir/). M3 exposes no Amaru-specific path override. The Platform creates and validates its private directory and publishes discovery with the existing permission and locking rules. Native discovery resolves the path without creating it.

## Consequences

Bundled and slim compact installs share the same location. Native clients use the discovery adapter to find the running Instance and the approved launch-code flow to open a browser. The path decision applies to Linux. Additional platforms require native private discovery and validation.

## Alternatives considered

A repository-relative path ties discovery to the working directory. An Amaru-specific CLI flag or environment setting can split discovery and lifetime locking across different locations for the same user. The XDG state directory provides a stable location across process restarts.

## Approval

Proposed by: Codex. Approved by: Ricardo Lüders on 2026-11-20 in the M3 implementation conversation.
