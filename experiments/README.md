# Experiments

Scenarios kept for reference that are deliberately outside the release testing
suite in [scenarios/](../scenarios): too long or too heavy to run on every
release, but worth having written down.

Nothing here is parsed by `go test ./scenarios/` or picked up by CI. These
scenarios usually run past the default ten minute budget, so give them a
`--timeout`:

```sh
build/norma run --timeout 25m experiments/testnet_brio_hardfork.yml
```
