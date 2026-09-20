# Role-Based Access Control (RBAC)

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 09, 10, 17, 18

## Product objective

Enforce tenant, resource and action authorization server-side across UI, APIs, diagnostics, integrations and privileged changes.

## Authorization dimensions

Effective access may include:

- tenant/customer;
- site/region;
- group/dynamic group;
- entity/device/resource;
- service;
- data class;
- action;
- privilege/risk tier;
- approval/JIT/MFA requirements.

UI hiding alone is never authorization.

## Role examples

Conceptual roles may include:

- MSP Admin / Operator;
- Customer Admin;
- Operator;
- Viewer;
- specialized NCM/integration/report/security roles.

Exact roles are configurable/governed; capabilities should not be hard-coded solely around names.

## Action classes

Examples:

- view telemetry;
- view raw logs/user-attributed Flow;
- manage monitoring;
- manage credentials by reference;
- run read-only diagnostics;
- open CLI;
- execute configuration/remediation;
- manage integrations/routes;
- replay/dead-letter operations;
- manage users/roles;
- export/report;
- audit access.

## Privileged actions

Higher-risk actions may require:

- JIT/step-up MFA;
- reason/ticket;
- maker-checker/approval;
- command/action policy;
- blast-radius bounds;
- audit;
- post-action validation.

## Credentials

Permission to use a Credential Profile does not imply permission to reveal its secret.

## Access Simulator

Platform should be able to answer "Can user X perform action Y on resource Z?" and explain the effective permission path without exposing secrets.

## Audit

Security-relevant authorization changes and privileged actions are audited with actor, tenant, object, action, result, time and correlation context.

## Tenant isolation

Cross-tenant authorization tests are release blockers. All query, diagnostics, Flow/Logs, integrations and data export enforce scope server-side.
