# Updating Scenarios for a New Sonic Release

When a new Sonic version is released, the scenario suite under [scenarios/](scenarios)
has to be updated so the new version is covered where it matters. This document
defines how.

The new version is **not** added to every scenario. A blanket sweep inflates
runtime, and worse, it silently destroys the tests that depend on a *specific*
old version forking. What follows is a role-based procedure instead.

---

## 1. Background: how image references resolve

`imageName` values are resolved by [driver/docker/images.go](driver/docker/images.go):

| Reference                   | Resolution                                                   |
| --------------------------- | ------------------------------------------------------------ |
| omitted / `sonic:local`     | docker build from the [sonic](sonic) submodule               |
| `sonic:vX.Y.Z`              | docker build from `github.com/0xsoniclabs/sonic.git#vX.Y.Z`  |
| `sonic`                     | docker build from the remote repository's **default branch** |

Consequences:

- Referencing a released tag needs no registry and no manual build step —
  the tag alone is enough.
- Omitting `imageName` means the candidate. `DefaultClientDockerImageName` is
  `"sonic:local"`, so a scenario only needs `imageName` when it wants something
  *other* than the candidate. Scenarios therefore never write `sonic:local`
  explicitly — it is redundant with the default, so the candidate is always
  spelled as the absence of a pin.
- `sonic` — the bare, untagged ref — is **not** the default and is not the
  submodule. It builds from the remote default branch, ignoring the checked-out
  submodule entirely. It exists for deliberately testing against sonic main;
  ask for it explicitly or not at all.
- `sonic:local` is the **submodule**, not an arbitrary working copy.
  `DefaultSonicLocalPath` is `"sonic"`, resolved against the Norma build root,
  so the submodule pointer is what decides which code the candidate actually
  is. `--sonic-path` can override this for local experiments, but the committed
  submodule revision is what CI and release runs build.

Norma's Go dependency on `github.com/0xsoniclabs/sonic` comes from the module
proxy and has no `replace` directive pointing at `./sonic`. The submodule
revision therefore affects only the `sonic:local` docker build — never the
Norma binary.

### The genesis is a cross-version artifact

One genesis file is generated per network, by the Norma binary, and imported by
every node with the sonictool of its own image. Anything in it that a client
version reads differently therefore has to suit *every* pinned image of the
scenario, not just the candidate — see
[genesis/genesis.go](genesis/genesis.go). Two places currently depend on the
version:

- **Account balances** are written as JSON numbers, which every version decodes.
- **The subsidies registry** is installed in the shape the *oldest* client of the
  scenario understands. Its `getGasConfig` and `chooseFund` signatures changed
  with v2.2.0; clients from v2.2.0 on read both shapes, earlier ones only the old
  one, and a client that cannot read the registry silently stops sponsoring —
  which forks it off the nodes that do. So a scenario pinning any pre-v2.2.0
  image runs the legacy registry, and the extended one is only exercised where
  every node is v2.2.0 or newer. The images are collected from all `startNode`
  steps, so a node joining mid-scenario counts too.

  The legacy bytecode is fetched from the Sonic repository at the tag that
  shipped it rather than kept in Norma, so such a run needs network access to
  `raw.githubusercontent.com` even when the client images are already built.

---

## 2. Every version reference has a role

A scenario's version pins are not interchangeable values; each one plays a role.
The role, not the version string, decides what happens at release time.

| Role         | Meaning                                                                | On release of `vNEW`      |
| ------------ | ---------------------------------------------------------------------- | ------------------------- |
| **Candidate** | The code under test. Spelled as the *absence* of a pin. Deliberately contrasted against pinned versions in the same file. | unchanged, but graduates in fork-boundary scenarios (§4) |
| **Baseline**  | "Some older client that must keep working." Interchangeable.          | rotate to `sonic:vNEW`    |
| **Boundary**  | Pinned *because* it is the last client without feature X, so it must fork or fail. | **frozen forever** |
| **Feature peer** | A released client that *implements* the feature under test, present so the scenario exercises the feature across versions instead of between copies of one build. | **join** if `vNEW` implements the feature; existing peers stay |
| **Agnostic**  | Also unpinned, but the scenario tests topology, load or an app — it has no version dimension at all. | unchanged, and never graduates |

### Distinguishing Baseline from Boundary

Both are often the same version string today (`sonic:v2.1.6`), so the string
tells you nothing. Ask:

> Does this node carry `failing: true`, **or** does the scenario's
> `InitialNetworkRules` / `updateRules` toggle a feature that this version
> predates?

- **Yes** → Boundary. The version *is* the test. Never rotate it. Rotating it
  turns a fork test into a no-op that still passes.
- **No** → Baseline. Rotate it.

Candidate and Agnostic are both written as an absent `imageName`, so they look
identical in YAML; they differ only in whether the scenario is *about* versions.
Read the scenario's intent, not its syntax, to tell them apart — it decides
whether §4 graduation applies.

---

## 3. Rotate, do not accumulate

The Baseline slot holds **exactly one** version: the newest release. It is
replaced, not appended to.

Accumulating a node per release would add ~15 validators across the suite every
cycle and grow release-test runtime without bound, for coverage that decays in
value as versions age out of support.

Old-version interop coverage lives in **one** designated file that is allowed to
accumulate: [scenarios/release_testing/validators/mixed_validator_versions.yml](scenarios/release_testing/validators/mixed_validator_versions.yml).
It carries the last two released versions plus the candidate. Nowhere else.

Two things are **not** accumulation and are not covered by this rule:

- **Feature peers.** A released client in a feature scenario is not a second
  Baseline; it is the only way that scenario can test the feature across
  versions at all. See §5a.
- **Migration source versions.** A migration scenario tests an *upgrade path*,
  and each supported source version is a different path, not a redundant node.
  [progressive_validator_state_migration_quorum.yml](scenarios/release_testing/liveness/progressive_validator_state_migration_quorum.yml)
  therefore starts two validators on v2.1.6 and two on v2.2.0 and migrates all
  four to the candidate. Rotating the older pair away would delete the
  v2.1.6 → candidate path, which is the one more likely to break. The node
  count stays fixed: split the existing validators, do not add more.

## 4. Graduation: the candidate becomes `sonic:vNEW`

This is the step that is easy to miss. During the `vNEW` development cycle,
the candidate *was* `vNEW`. Every fork-boundary scenario that proved
"the candidate survives the fork, the old client does not" has, at release,
proven that claim about a shipped artifact. Freeze it:

    <feature>_fork_<old>_local.yml   →   <feature>_fork_<old>_v<NEW>.yml

by pinning `sonic:vNEW` on the surviving side of the fork. The renamed file
becomes the permanent regression test for that boundary. It is a **rename, not a
copy** — a separate `_local` sibling would duplicate a boundary the candidate
already crosses in the unpinned `rules/enable_<feature>.yml` scenarios.

Graduating **splits** the surviving side, it does not convert it: `vNEW` joins as
a feature peer and the candidate keeps a node in the file, holding the ⅔ of §5a.
Converting the last unpinned node instead leaves a scenario made entirely of
released images, which no longer exercises the code being released —
`TestReleaseTestingScenarios_UseLocalClient` in
[scenarios/scenario_test.go](scenarios/scenario_test.go) fails if that happens.
The boundary node itself is untouched and stays frozen.

The repository already follows this convention:
`subsidies_fork_v215_v216.yml` is a graduated pair and has no `_v215_local`
sibling, because `local` moved past that boundary. It sits in
[scenarios/examples/](scenarios/examples) under the retirement rule below, where
the candidate requirement no longer applies.

### Retiring frozen scenarios

Keep frozen boundary scenarios for the **last two** hardfork boundaries. Once
both sides of a pair are outside the supported-version window, the scenario no
longer says anything about the current release: move it to
[scenarios/examples/](scenarios/examples) as documentation, or delete it.

---

## 5. Per-directory policy

| Directory                                  | Policy                                                                                  |
| ------------------------------------------ | --------------------------------------------------------------------------------------- |
| `load_generators/`                         | **Never add a version.** One validator, no `imageName`. Purpose is "does this generator emit valid transactions" — there is no version dimension. |
| `examples/`                                | Documentation surface. Never widen. Bump a pin only when it falls out of the support window, so the docs do not cite ancient tags. |
| `release_testing/features/*` (unpinned)    | **Add `vNEW` as a feature peer** if `vNEW` implements the feature (§5a). Only stay all-candidate while no release implements it. |
| `release_testing/features/*` (compat)      | Do **not** rotate. The interesting pin is the *oldest* client that supports the feature — and `vNEW` joins it. |
| `release_testing/liveness/`, `validators/` (migration) | Rotate, except migration *source* versions, which accumulate one node per supported upgrade path (§3). |
| `release_testing/stress/`                  | Rotate the Baseline pin. Leave single-version scenarios alone.                          |
| `release_testing/rules/`                   | Do **not** rotate — the pin is the oldest client supporting that rule, and `vNEW` joins as a feature peer (§5a). |
| `rules_upgrades/`                          | Boundary pins, frozen. Only the graduation step applies.                                |

The "compat vs migration" split is the one judgement call. A *feature
compatibility* scenario asks "do all clients that have feature X interoperate?"
— its most interesting participant is the oldest such client, so the pin stays.
A *migration or interop* scenario asks "does an older client keep up / can its
database be upgraded?" — there the realistic pin is the newest release, so it
rotates.

### 5a. Feature scenarios: derive who *can* participate

A feature scenario written during the `vNEW` development cycle has every node
unpinned, because no release implemented the feature yet. Once `vNEW` ships, that
is no longer true, and leaving the scenario all-candidate means the feature is
never tested across versions — the failure this suite exists to catch.

So at each release, for every feature scenario, work out which versions *can*
join. Do not infer it from the `Upgrades` struct: the flag often exists a release
before the implementation. Count implementation files per tag instead:

```
for tag in v2.1.6 v2.2.0; do
  git -C sonic grep -l 'TransactionBundles' $tag -- '*.go' | grep -v _test | wc -l
done
```

A handful of hits means the rule plumbing only; a dozen or more means the feature
is really there. As of v2.2.0:

| Feature                      | v2.1.6              | v2.2.0 | candidate |
| ---------------------------- | ------------------- | ------ | --------- |
| Allegro                      | ✅                  | ✅     | ✅        |
| GasSubsidies                 | ✅                  | ✅     | ✅        |
| SingleProposerBlockFormation | ✅                  | ✅     | ✅        |
| Brio                         | ❌ rule only, forks | ✅     | ✅        |
| TransactionBundles           | ❌ absent           | ✅     | ✅        |

Three-way coverage is therefore possible for Allegro, subsidies and single
proposer; Brio and bundles can only be two-way (`v2.2.0` + candidate) until the
next release, and v2.1.6 appears there **only** as a Boundary node that forks.

Three constraints when adding a peer:

1. **Keep the node count flat.** Convert an existing node rather than appending
   one: pin one of the `instances` in the candidate group, or repin a
   same-versioned sibling. `rules/enable_brio.yml` went from `instances: 3` to
   `instances: 2` plus a `v2.2.0` node, so runtime is unchanged and the scenario
   gained a version dimension.
2. **Give the candidate more than ⅔ of the stake**, with explicit stakes if the
   group was using defaults. Then any combination of peer divergence leaves a
   live network and `blockHashes` names the diverging node; below that threshold
   a peer fork *stalls* consensus, which reports as "no blocks produced" and
   tells you nothing about who was wrong. At equal stakes three validators put
   any two at exactly ⅔ — not a quorum — so defaults are usually wrong here.
   Migration scenarios are the one exemption: every validator there starts on
   a released version, so the candidate cannot hold ⅔ by construction.
3. **Do not write a `blockHashes` check.** Hash agreement is what makes a peer
   worth adding — `blocksProduced` cannot see a fork that leaves a quorum behind
   — but the parser already appends `advanceEpoch`, `advanceEpoch` and
   `checks: [blockHashes, blockHeights]` to every scenario (`setDefaults` in
   [driver/parser/scenario.go](driver/parser/scenario.go)), so writing it out
   duplicates the harness. `blockHeights` there uses the same slack of 5 that
   the explicit checks were passing as `tolerance: 5`.

   `blockHashes` walks the chain from block 0, so the implicit run also covers
   every earlier point in the scenario. An explicit check mid-scenario is
   therefore only worth writing when the implicit one cannot see what it sees:

   - a node is stopped before the end and never restarted, so it is absent from
     `GetActiveNodes()` at the end;
   - a node restarts **without** a `dataVolume`, which re-syncs it from the
     network and erases the divergence from its own history — the case in
     [bundles_survive_validator_restart.yml](scenarios/release_testing/features/bundles/bundles_survive_validator_restart.yml),
     the one place in the suite that still writes the check;
   - the scenario needs the check with `failing: true`, which the implicit one
     does not supply.

   Set `DisableEndChecks: true` only when the scenario deliberately ends in a
   state where hash agreement is not expected.

---

## 6. Procedure

### Step 1 — Characterise the release

Before editing anything, establish from the Sonic repository:

1. **Does the tag build?** `sonic:vNEW` resolves to the git tag; confirm it exists.
2. **Does `vNEW` introduce a hardfork or consensus-relevant semantic change?**
   Read `changelog.md` and diff the `Upgrades` struct *and its semantics* against
   the previous release.
   - **Yes** → the previous newest version now forks. A new Boundary scenario is
     needed, and §4 graduation applies.
   - **No** (pure RPC/tooling release) → rotation only. No new files.
3. **Which optional feature flags does `vNEW` newly support?** Each one that
   older clients lack warrants one compatibility scenario.
4. **Is `sonic:local` still consensus-compatible with `vNEW`?** If HEAD carries
   no further hardfork, `local` and `vNEW` agree, and boundary scenarios can
   graduate cleanly.

### Step 2 — Advance the submodule past the release

The candidate must be **newer** than the baseline it is tested against.
Advance the [sonic](sonic) submodule to the post-release development head:

```
git -C sonic fetch origin
git -C sonic checkout --detach origin/main
git -C sonic describe --tags          # must be vNEW-<n>-g<sha>, not vNEW-rc.*
git add sonic
```

Confirm the release tag is an ancestor, so `local` really does contain
everything `vNEW` shipped:

```
git -C sonic merge-base --is-ancestor vNEW origin/main && echo ok
```

Skipping this step is the subtlest way to get a green suite that proves
nothing. If the submodule still points at a release candidate, `sonic:local`
is *older* than the newly rotated baseline, every "old client keeps up"
scenario runs backwards, and nothing fails.

### Step 3 — Classify every pin

Enumerate the current pins and assign a role to each:

```
grep -rn -B3 'imageName' scenarios/ | grep -E 'startNode|imageName|stake|failing'
```

Apply the §2 role test. Record the classification in the pull request — it is the
part a reviewer needs to check, and it is what a blanket sweep gets wrong.

### Step 4 — Apply the edits

In this order:

1. **Graduate** boundary scenarios (§4): `git mv`, then pin the previously
   unpinned node to `sonic:vNEW`.
2. **Rotate** Baseline pins: `sonic:vOLD` → `sonic:vNEW`.
3. **Join** every feature and `rules/` scenario whose feature `vNEW` implements
   (§5a), keeping the node count flat and adding `blockHashes`.
4. **Split** migration scenarios so each supported source version has a node (§3).
5. **Widen** `mixed_validator_versions.yml` only.
6. **Leave** Boundary and agnostic scenarios untouched.

When a pin changes, also update the things that encode the version in prose,
or the suite becomes actively misleading:

- the node identifier (`validator-v216` → `validator-v220`)
- `Name:`
- `Description:`
- the filename, where it names versions
- any leading comment block

### Step 5 — Verify

```
go test ./scenarios/                        # parses every scenario, and asserts
                                            # each release_testing one runs the
                                            # candidate on at least one node
build/norma build scenarios/release_testing # pre-builds all referenced images
build/norma run scenarios/release_testing   # runs the suite (accepts directories)
```

Then check the two properties this document is about — that every feature has a
peer where one is possible, and that no scenario restates the implicit checks:

```
# version coverage per scenario
for f in $(find scenarios/release_testing -name '*.yml'); do
  printf '%-62s %s %s\n' "$f" \
    "$(grep -o 'sonic:v[0-9.]*' $f | sort -u | tr '\n' ' ')" \
    "$([ $(grep -c imageName $f) -lt $(grep -c startNode $f) ] && echo local)"
done

# explicit hash/height checks - each hit needs a §5a reason next to it
grep -rn 'blockHashes\|blockHeights' scenarios/
```

The `local` column above is the one property now asserted in Go, so treat it as
a cross-check rather than the guard. Everything else here is prose: `go test
./scenarios/` will not catch a Boundary pin that was wrongly rotated, or a peer
added below the ⅔ threshold. Those failure modes are invisible to CI — the
scenario still passes, it just no longer tests what it claims. Classification
review in Step 3 is the only guard.

---

## 7. Worked example: v2.2.0

**Step 1 — characterisation.** v2.2.0 adds Brio support and Transaction
Bundles. It is exactly what `sonic:local` represented throughout the v2.2.0
cycle. Sonic HEAD is v2.2.0 plus a handful of commits whose only changes are
`trace_` RPC response formats — no further hardfork — so `sonic:local` and
`sonic:v2.2.0` are consensus-compatible and boundary scenarios graduate cleanly.
Version roles therefore rotate as:

| Role      | Before   | After    |
| --------- | -------- | -------- |
| Candidate | `local`  | `local`  |
| Baseline  | `v2.1.6` | `v2.2.0` |
| Boundary (pre-Brio)     | `v2.1.6` | `v2.1.6` (frozen) |
| Boundary (pre-Subsidies) | `v2.1.5` | `v2.1.5` (frozen) |

**Step 2 — submodule.** The submodule was pinned at `v2.2.0-rc.2-3`, older than
the release it was about to be tested against. Advanced to `v2.2.0-30`
(`origin/main`, with `v2.2.0` confirmed as an ancestor) so `sonic:local` is the
post-v2.2.0 candidate.

**Steps 3/4 — classification and edits.** Seven files change their version pins:

| Action       | File                                                             |
| ------------ | ---------------------------------------------------------------- |
| **Graduate** | `liveness/supermajority_after_brio_fork_v216_local.yml` → `liveness/brio_fork_supermajority_v216_v220.yml`, `local` → `v2.2.0` |
| **Rotate**   | `liveness/progressive_validator_state_migration_quorum.yml` (4 nodes) |
| **Rotate**   | `validators/restart_validator_migrated_volume.yml`    |
| **Rotate**   | `stress/multi_node_mix_payload_constant.yml`                      |
| **Rotate**   | `stress/multi_node_single_payload_slope.yml`                      |
| **Rotate**   | `examples/rejoin_upgraded_with_data_volume.yml`                                 |
| **Widen**    | `validators/mixed_validator_versions.yml` → `v2.1.6` + `v2.2.0` + `local` |

Frozen — v2.2.0 supports Brio and Subsidies, so substituting it here would
convert a fork test into a silent no-op:

- `liveness/forked_db_stays_forked.yml` (v2.1.6)
- `rules_upgrades/allegro_to_brio_mixed_versions.yml` (v2.1.6)
- `rules_upgrades/sonic_to_allegro_mixed_versions.yml` (v2.1.5, v2.1.6)
- `examples/brio_fork_rejects_old_clients.yml`,
  `examples/single_proposer_brio_fork.yml` (v2.1.6)
- `examples/subsidies_fork_v215_v216.yml` (v2.1.5) — since
  `features/subsidies/subsidies_semantic_change.yml` was removed as a duplicate,
  this retired scenario is the only remaining coverage of v2.1.5 forking on
  subsidies, and it sits in `examples/` rather than the release gate.

Joined by `v2.2.0` as a feature peer (§5a) — the existing v2.1.6 pin stays, since
it is the oldest client supporting the feature, and `v2.2.0` is added beside it:

- `features/subsidies/subsidies_across_client_versions.yml`
- `features/subsidies/enable_subsidies_by_rules_update.yml`
- `features/single_proposer/proposer_switch_with_old_client.yml`
- `rules/enable_allegro.yml`, `rules/toggle_subsidies_off_and_on.yml`

These four were all-candidate because Brio and bundles had never shipped. v2.2.0
is the first release with both, so it joins them too — replacing one candidate
node rather than adding a fourth, so runtime is unchanged:

- `rules/enable_brio.yml`, `rules/toggle_bundles_off_and_on.yml`
  (`instances: 3` → `instances: 2` + a `v2.2.0` node at 20% stake)
- `features/bundles/bundles_activation_with_late_join.yml` (the peer becomes
  `v2.2.0`; the late joiner stays the candidate)
- `features/bundles/bundles_survive_validator_restart.yml` (one of three
  identical candidates becomes `v2.2.0`; the restarted node stays the candidate)

Split by source version (§3) — `liveness/progressive_validator_state_migration_quorum.yml`
keeps four validators but starts two on v2.1.6 and two on v2.2.0, so both
supported upgrade paths to the candidate are covered in one run.

**Checks removed.** Hash agreement in all of these comes from the implicit end
checks, so the 26 explicit `blockHashes` and `blockHeights` entries the suite had
accumulated across 17 files were restating the harness and were dropped. Only
`features/bundles/bundles_survive_validator_restart.yml` still writes one, for
the reason given in §5a. What the mixed scenarios do need explicitly is
`blocksProduced`, which is *not* implicit —
`validators/mixed_validator_versions.yml` had no explicit check at all and now
asserts it.

Untouched — genuinely no version dimension: `event_throttler/` (flag-driven),
`single_proposer/high_load_five_validators`,
`single_proposer/{join_after_proposer_switch,network_recovers_from_blackout}`,
`liveness/blackout_recovery_after_quorum_loss`, `rules_upgrades/change_*`,
`stress/{single_node_*,progressive_node_addition_*,*bundle*}`,
`validators/{mixed_validator_stakes,restart_validator_keep_stake_quorum}`.

**Retired.** `liveness/supermajority_after_subsidies_fork_v215_v216.yml` moved to
`examples/subsidies_fork_v215_v216.yml`. Both sides are now outside the
supported-version window, so it no longer says anything about the current
release.

**Separately: the default was wrong.** All 18 `load_generators/` scenarios plus
`stress/single_node_*`, `stress/progressive_node_addition_*`,
`validators/mixed_validator_stakes`, `validators/restart_validator_keep_stake_quorum`
and `features/single_proposer/network_recovers_from_blackout` omit `imageName`,
and `DefaultClientDockerImageName` was `"sonic"` — the remote default branch. So
34 node steps across 24 files were gating sonic main rather than the release
candidate, and the submodule revision they were reviewed against was ignored.

Fixed at the root by changing the default to `"sonic:local"` rather than pinning
each scenario, so the scenarios stay clean and every future version-agnostic
scenario is correct by default instead of by remembering to add a line. The 48
pre-existing explicit `sonic:local` pins were removed for the same reason: they
restate the default, so the only pins left in the suite are the ones that carry
meaning.

For contrast, the reverted `Add v2.2.0` commit (6da14c6) appended a v2.2.0
validator to 67 files, including all 18 load generators, and rewrote the frozen
fork scenarios above.
