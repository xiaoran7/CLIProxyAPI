## Description

Please include a summary of the change, the motivation behind it, and which issue or requirement it resolves.

Fixes #(issue)

## Type of Change

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Refactoring / Architectural cleanup
- [ ] Documentation update

## Checklist

- [ ] My code follows the code conventions of this project (`gofmt -w .`, English comments only).
- [ ] Payload configuration remains the final semantic barrier before dispatching upstream requests.
- [ ] I have verified that unit tests pass locally (`go test ./...`).
- [ ] I have verified that the project builds cleanly (`go build -o test-output ./cmd/server && rm test-output`).
- [ ] I have updated `CHANGELOG.md` with relevant details under the Unreleased or current version section.
- [ ] I have added regression tests for any bug fixes or new request paths.
