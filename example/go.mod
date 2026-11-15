module github.com/go-kanna/validator/example

go 1.27.0

toolchain go1.27.3

tool (
	github.com/go-kanna/validator/cmd/kanna-validator
	github.com/go-kanna/value/cmd/kanna-value
)

require (
	github.com/go-kanna/validator v0.0.27
	github.com/go-kanna/value v0.0.18
)

require (
	github.com/go-kanna/internal v0.0.34-0.20261115032209-5ca03e729550 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
)
