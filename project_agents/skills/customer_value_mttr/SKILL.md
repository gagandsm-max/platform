---
name: customer_value_mttr
description: MTTR/value analytics with cohort methodology, data quality and no fabricated savings percentages.
---

# customer_value_mttr

## Purpose

Report factual operational value and MTTR/MTTA metrics with explicit methodology, cohorts and data-quality limits—never fabricated ROI, pricing or guaranteed savings.

## Use when

Value/MTTR dashboards, before/after incident metrics, customer success evidence packs, or any claim that Platform “saved X% / $Y.” Required for OIV value surfaces governed by `Platform_19`.

## Authoritative inputs

Always read:

1. `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` (mandatory)
2. Existing metric definitions/code on the exact head
3. Incident severity taxonomy and exclusion rules in product docs
4. Tenant isolation rules for analytics
5. Founder-reserved commercial boundary in Mother / AGENTS notes

Pricing, contractual SLA, guaranteed savings, guaranteed MTTR reduction and commercial ROI commitments are **Founder-reserved**—do not invent them.

## Invariants

- Metrics are factual operational measures with definitions (MTTA/MTTR, denominators, units).
- Baseline period and comparison population are stated.
- Exclusions (maintenance, false alerts, open incidents) are explicit.
- Incident severity mix is disclosed when it affects averages.
- Confidence / sample size / data-quality flags accompany results.
- Attribution vs correlation is labelled; do not claim causal savings without approved method.
- **No fabricated ROI** or marketing multiplier without evidence.
- Customer-specific data stays tenant-isolated.
- Before/after methodology is reproducible on the exact SHA.
- Missing data ⇒ `UNVERIFIED` / suppressed—not interpolated fantasy.
- Commercial claims remain Founder-owned.

## Required procedure

1. Name the metric, formula, denominator and time zone.
2. Define baseline vs comparison windows and population filters.
3. List exclusions and severity mix; show counts.
4. Compute with code/SQL on tenant-scoped data; retain query evidence.
5. Separate correlation (“incidents fell after enablement”) from causal attribution.
6. Surface data-quality gaps; mark incomplete series `UNVERIFIED`.
7. Refuse to emit pricing, SLA guarantees or ROI multipliers in product copy/code.
8. Route any commercial wording to Founder/control plane—do not invent.

## Negative / failure cases

- “Platform reduces MTTR 40%” with no cohort or baseline.
- Mixing tenants to inflate N.
- Counting still-open incidents as zero MTTR.
- Embedding dollar savings in UI without Founder-approved methodology.
- Quietly dropping high-severity outliers to beautify charts.

## Security / tenant implications

Value analytics are tenant-scoped. No cross-tenant benchmarks without explicit authorization. Exports redact customer-identifying fields per policy. Operators see only authorized scopes.

## Required evidence

- Metric glossary + formulas
- Cohort/baseline parameters
- Query/job IDs and SHA
- Data-quality / `UNVERIFIED` list
- Confirmation no Founder-reserved commercial claims were added

## Stop conditions

Stop when:

- Methodology cannot be stated
- Cross-tenant mixing would occur
- Commercial ROI/pricing/SLA guarantees are requested in-product
- Data insufficient yet a precise savings number is demanded

## Anti-patterns

- Marketing screenshots as SOURCE_FACT
- Single-customer anecdote as estate proof
- Writer self-certifying value claims

## Handoff / output

Return: methodology pack; numbers with caveats; Founder escalations if any; nextOwner (OIV verifier / Founder for commercial). Not MERGE_READY alone.

## Non-ownership

Does not set price, invent contractual SLA, or own alert policy. Owns honest MTTR/value measurement discipline.
