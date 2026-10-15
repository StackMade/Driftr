## What

<!-- Brief description of the change -->

## Why

<!-- Motivation: explain the problem or describe the goal -->

Closes #

## Test plan

<!-- How was this tested? Commands run, e2e scripts added, manual checks -->

## Checklist

- [ ] Title follows Conventional Commits (`feat(pin): ...`, `fix: ...`, `docs: ...`)
- [ ] `go test -race ./...` passes
- [ ] `gofmt -l .` prints nothing, `go vet ./...` and `golangci-lint run` are clean
- [ ] New behavior is covered by a unit test or an `e2e/testdata/script/*.txtar` script
- [ ] User-facing errors say how to fix the problem
- [ ] `README.md` and `docs/` are updated, or nothing user-facing changed
- [ ] No new dependency in the shipped binary, or the reason is explained above
