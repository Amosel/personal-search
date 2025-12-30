# AGENT.md

## Purpose

Agents contribute to this project by **making assumptions visible, boundaries explicit, and execution faster**.
The goal is a **clean, fail-fast pipeline** whose correctness comes from contracts, not cleverness.

---

## Core Principles

### 1. Boundary-First Design

Agents focus on **boundaries between components**.

* Each boundary defines inputs, outputs, and failure modes.
* Internal logic assumes validated input and stays simple.
* Boundaries exist to absorb complexity so downstream code is straightforward.

**Goal:** complexity is localized, not spread.

---

### 2. Contract-First Reasoning

Agents treat **contracts as the primary artifact**.

* A contract answers: *what comes in, what goes out, and how failure is handled*.
* Testing validates contracts, not internal behavior.
* If a contract is unclear, the correct outcome is to **surface that ambiguity**.

**Goal:** make correctness provable and tests trivial.

---

### 3. Epistemic Clarity Before Resolution

By default, agents operate in **Epistemic Review mode**.

* The task is to **expose assumptions, gaps, and implicit decisions**.
* Ambiguity is a valid and valuable finding.
* Resolution happens only after assumptions are fully visible.

**Goal:** avoid premature closure and hidden design debt.

---

### 4. Pipeline Discipline

Agents reason in terms of a **one-way pipeline**.

* Each stage produces validated output for the next stage.
* Re-ingestion and re-execution must be safe and deterministic.
* Failures stop the pipeline immediately and clearly.

**Goal:** predictable execution and easy parallelization.

---

## Operating Modes

### Mode: Epistemic Review (default)

**Intent:** Stress-test the existing design.

**Agent actions:**

* Work strictly within the current task list
* Identify implicit assumptions and weak boundaries
* State whether each issue is *defined, implicit, or undefined*
* Express findings as **deltas**, not narratives

**Value:** reveals where the system needs clarity.

---

### Mode: Design Resolution (explicitly requested)

**Intent:** Make a deliberate, narrow decision.

**Agent actions:**

* Resolve only issues previously surfaced
* Propose minimal, bounded changes
* Tie every decision to a specific contract

**Value:** converts clarity into commitment.

---

### Mode: Implementation Support (explicitly requested)

**Intent:** Implement an agreed contract.

**Agent actions:**

* Produce code or tests that enforce the contract
* Preserve existing boundaries
* Keep implementation simple once inputs are validated

**Value:** fast, reliable execution.

---

## Output Structure

Agents communicate using **structured, local outputs**.

### Dependency DAG Deltas

```
REMOVE: T7 → T3
ADD:    T6 → T2
```

### Boundary Clarification

```
Task: T9
Boundary: Adapter → Indexer
State: implicit
Failure mode: undefined
```

### Determinism Check

```
Task: T19
Concern: External dependency
Deterministic substitute: hash-based embedder
```

**Goal:** enable parallel work and precise discussion.

---

## Testing Doctrine

* Tests validate **inputs, outputs, and failure modes**
* Valid input implies trusted execution
* Invalid input halts immediately
* Deterministic tests are preferred

**Goal:** tests stay simple because complexity lives at boundaries.

---

## Scope Discipline

Agents improve the system by:

* sharpening contracts
* adjusting dependencies
* clarifying ownership of validation

All changes are expressed as **deltas**, not expansions.

**Goal:** progress without scope creep.

---

## Success Criteria

An agent contribution is successful when:

* Assumptions become explicit
* Boundaries become sharper
* Tests become simpler
* Parallel execution becomes easier
* Failures happen earlier and more clearly

---

## Closing Principle

This project optimizes for:

* **epistemic clarity**
* **execution speed**
* **operational correctness**

Agents support this by **revealing structure**, not by adding abstraction.
