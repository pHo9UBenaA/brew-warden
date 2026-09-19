# Jev evaluation for the development harness

Date: 2026-09-19. Decision: defer integration. Keep deterministic scripts as the
required gates and the existing agent as the semantic reviewer. Reconsider Jev
for optional advisory classification if a measured workload warrants it.

This is a documentation-based assessment, not an API benchmark. No SDK, provider
skill, credentials, account, or integration was installed; no repository content
was submitted to an inference endpoint. The product remains all-in-one: this
decision concerns how its development harness is implemented.

## What was checked

The [requested article](https://zenn.dev/mizchi/articles/jev-is-gpu-for-llms)
describes experiments with batched structured judgments. Its GPU analogy is a
workload analogy, not evidence that this repository would run faster. Published
latency, cost, accuracy, and confidence thresholds are experiment-specific and
were not reproduced here.

The [official API](https://docs.typesafe.ai/api) accepts state and typed questions
over authenticated HTTP. Choice selects among declared options; Score evaluates
ordered criteria; Noul provides a yes/no probability. A bounded output vocabulary
does not establish that the selected answer is correct. Direct HTTP integration
is available, so a Python or JavaScript SDK is not inherently required. API keys,
network availability, data handling, model updates, and evaluation still add
dependencies even with a standard-library HTTP client.

The [official skill-suggestion cookbook](https://docs.typesafe.ai/cookbooks/skill_suggestion)
uses a large Hermes skill catalog, a ranking pass, and a second shortlist check.
It reports fewer wrong or needless loads under its experiment. This does not
establish a benefit for our two repository skills or for a different agent host.
Adding a SKILL.md does not automatically interpose a router in a host's selection
mechanism; that would require an integration point in the actual harness.

The [confidence documentation](https://docs.typesafe.ai/confidence) defines
Choice/Score confidence from the output probability distribution. Noul does not
carry that confidence field. A confidence value is not a measured correctness
rate for our repository, and a high value is not evidence of cryptographic trust.
Thresholds need evaluation on the actual workload.

The [jev-1.13 limitations](https://docs.typesafe.ai/model-jaggedness/jev-1.13),
reviewed by the vendor on 2026-09-17, recommend code for arithmetic and date
comparison and describe sensitivity to adversarial state. These are
version-specific observations, not a claim about every future model.

## Fit for this repository

| Task | Preferred mechanism now | Reason |
| --- | --- | --- |
| Import direction, local resolution, cycles | Go parser and graph checks | Exact rules; no inference needed |
| Conventional Commit syntax and text hygiene | Existing checker | Stable local feedback, offline, no API cost |
| Whether prose is meaningfully English | Author/reviewer judgment plus existing migration guard | CJK detection is not a semantic language test; no current volume justifies another service |
| Hash, signature, age, expiry, exception validity | Code and maintained cryptographic verifiers | Exact security conditions must not become probabilistic |
| Which of two repository skills to use | Descriptions and current agent | No demonstrated routing bottleneck |
| Large numbers of issues or review items | Optional future Jev classification | Repetitive semantic classification could be a useful workload |
| Which review lenses deserve attention | Optional future Jev suggestions | Useful only if it improves coverage or effort without suppressing required review |
| Approving commands or emergency updates | Existing authorization and deterministic policy | Model selection/confidence is not permission |

Do not replace the mandatory architecture, test, commit, or CI gates with model
judgments. Do not add a probabilistic command gate merely because the
[guardrails cookbook](https://docs.typesafe.ai/cookbooks/llm_guardrails) demonstrates
one: this repository already has concrete boundaries, and a classifier would
need its own false-negative, false-positive, and adversarial evaluation.

## A bounded future experiment

If review volume becomes repetitive, evaluate a development-only `review-hints`
operation. It could assign a sanitized diff to fixed lenses such as trust,
process execution, persistence, dependencies, documentation, or unknown. It
would return suggestions for the existing reviewer, never commands or an allow
decision. Do not add this command or new abstractions until the need is measured.

1. Assemble representative changes with independently reviewed labels, including
   mixed changes, misleading comments, prompt injection, and no-applicable-lens
   cases. Include critical examples where missing a lens would matter.
2. Compare path/rule-based routing, the current agent alone, and Jev-assisted
   routing. Use the same held-out examples and evaluation criteria.
3. Measure critical-lens recall, needless escalations, reviewer time, total
   latency including fallback, cost, and repeated-run disagreement. Faster
   inference alone is not a workflow improvement.
4. Record model ID, criteria, input digest, output, and baseline results. Use a
   fixed available model rather than silently changing a latest alias. Do not
   infer calibration from a confidence threshold copied from an article.
5. Adopt only if the results show a meaningful benefit without losing required
   coverage. Otherwise retain scripts and the existing review workflow.

Any experiment must be opt-in and keep data scope explicit. Public-OSS intent
does not make uncommitted source, credentials, environment values, personal paths,
or private security reports public. Establish acceptable provider data handling
before sending material. A missing key, timeout, malformed response, unknown
label, or unavailable model falls back to the existing complete review workflow;
it must not skip checks or block an otherwise offline contributor workflow.

If eventually implemented, keep the integration outside product packages and out
of hooks and mandatory CI. A small Go HTTP client could avoid a second language
runtime, but that is an implementation option rather than a reason to integrate.
No host-specific routing interception or model-performance guarantee is assumed.
