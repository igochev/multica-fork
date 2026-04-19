# Reviewer Plan Contract

This document formalizes the Builder → Reviewer plan-file contract used by Elite Mission Control in Multica.

## Contract rule

```markdown
Before reviewing code, you must:
1. Read the issue comments and find the canonical plan path posted by the Builder.
2. Check out the repository if it is not already present.
3. Read the plan file from the repo.
4. Review code changes against that plan first, then against general code quality.
5. Report explicit deviations from the plan.
```

## Deterministic issue-comment convention

The Builder must post this exact convention in an issue comment after creating the plan:

```markdown
Plan path: docs/plans/{issue-slug}.md
Review contract: this file is canonical.
```

## Grounded conclusion

- Repository access is automatic once the reviewer checks out the repo.
- Plan-path discovery is not automatic and therefore requires the issue comment convention above.
- No daemon change is required for this workflow.

## Operational notes

- The plan file becomes the canonical implementation contract once created.
- Review should measure both plan compliance and general code quality.
- If the builder omits the convention comment, the reviewer should flag that as missing review-contract setup rather than guessing.
