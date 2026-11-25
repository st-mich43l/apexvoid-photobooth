# Go integration tests

Black-box Go integration tests that exercise exported application APIs belong in this directory. Package unit tests stay next to the package they test when they need access to unexported implementation details.

Run the integration suite from the repository root with `go test ./test/...`.
