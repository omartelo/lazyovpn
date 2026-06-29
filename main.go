package main

import "github.com/omartelo/lazyovpn/cmd"

// version is overridden at release via ldflags (-X main.version); this is the
// fallback for plain `go build`/`go run`.
var version = "dev"

func main() {
	cmd.Execute(version)
}
