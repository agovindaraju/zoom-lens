# Contributing

Thanks for looking at Zoom-Lens. It is an early proof of concept, so small, focused changes are easiest to review.

## Before you open a pull request

```bash
make check       # go vet, gofmt check, unit tests, kustomize render, script syntax
make knowledge   # only if you touched knowledge/: validates and regenerates knowledge/generated
```

- Keep commits small and the message clear about *why*.
- Add or update a test for behaviour you change.
- Documentation lives in `docs/`; update it with the code.

## Where to start

- **A metric or alert rule that is missing:** [docs/knowledge-guide/07-maintaining-and-extending.md](docs/knowledge-guide/07-maintaining-and-extending.md).
- **A new metric source:** it needs a scrape template in `internal/render` as well as a pack in `knowledge/`.
- **Another actuator or agent:** [docs/ROADMAP.md](docs/ROADMAP.md).

## Licence

The licence is chosen by the project owners before the first release. Until a `LICENSE` file is in the
repository, do not assume any licence.
