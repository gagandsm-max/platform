<!-- generated from project_agents/skills/sbom_provenance/SKILL.md sha256:9e54cc95006eca3ce40759aff1493cc5f09034a9bd063643223fea0a3c13c5e7 -->

---
name: sbom_provenance
description: SBOM, provenance, CVE/licence review and supply-chain evidence before new OSS.
---

# sbom_provenance

## Purpose
Admit only catalogue-reviewed dependencies.

## Use when
New OSS, version bumps, or release SBOMs.

## Required procedure
1. Catalogue + transitive licence/CVE review.
2. No dependency added to dodge a failure.
3. Record provenance of artifacts.
4. Do not claim certification by specification alone.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `sbom_provenance` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
