# Control surface compatibility policy

This policy governs changes to operator and integration interfaces. It does not
select which interfaces the project supports. The authoritative list and each
surface's classification belong in the [operator entrypoint inventory](operator-entrypoints.md)
([issue #1](https://github.com/Amosel/personal-search/issues/1)). This policy
does not by itself select a surface for retirement.

## Classify before changing

Each inventoried surface must be classified as one of:

- **Supported operator interface**: documented for a person operating the
  project.
- **Supported integration contract**: documented for a program or agent
  integrating with the project.
- **Internal or diagnostic interface**: available for implementation,
  troubleshooting, or development; not promised as a stable integration.

Classification attaches to a specific contract, not a whole implementation.
For example, a command's supported flags may be stable while an internal
package it calls remains changeable. The inventory should identify the relevant
contract and its documentation.

## Compatibility rules

For supported interfaces:

- Preserve documented inputs, outputs, defaults, and failure behavior across
  compatible changes.
- Add optional inputs or output fields only when existing callers can continue
  to work unchanged. Do not silently change the meaning of an existing input,
  default, or output field.
- Treat changes to CLI commands, flags and environment variables; Make targets;
  HTTP routes and payloads; MCP tool names, arguments and results; and wrapper
  commands as changes to their respective contracts when those surfaces are
  classified as supported.
- Keep source-specific ingestion and search behavior separate from the
  interface policy. A new data source may add its own adapter and contracts
  without changing the compatibility promise of existing sources.

Internal or diagnostic interfaces may change without a compatibility promise.
Keep them identified as internal or diagnostic in the inventory and do not make
supported workflows depend on them without reclassifying them.

## Deprecation and retirement

Do not remove or make a breaking change to a supported contract in the same
change that first announces its deprecation. Before retirement:

1. Open or update an issue naming the affected contract, reason, replacement
   when one exists, and migration steps.
2. Mark the contract deprecated in its operator-facing documentation and in
   runtime help or diagnostics where the interface allows it. Keep it working
   during the deprecation period.
3. Migrate supported workflows, in-repository callers, and contract tests to
   the documented replacement. Update examples and cross-references.
4. Retire it in a separate reviewed change after the project has announced the
   deprecation in a release or equivalent user-facing project notice. If the
   project has no release or notice mechanism, keep the contract until a
   dedicated issue and review explicitly approve retirement and its migration
   is documented.
5. Update the control surface inventory and compatibility documentation in the
   retirement change.

If a contract has no replacement, explain the impact and migration options
before retirement. A security or data-integrity defect may require an earlier
breaking change; document the reason, affected contract, and safest available
migration in the same change.

## Review checklist

For a change to a surface listed in the operator entrypoint inventory, its pull
request should state:

- the affected contract and its inventory classification;
- whether the change is compatible or breaking, and why;
- any caller, documentation, and test migrations;
- for deprecation or retirement, the issue and notice that satisfy the steps
  above.

Changes to data-source adapters should also follow their source contract; this
policy does not define that contract. See [issue #3](https://github.com/Amosel/personal-search/issues/3).
