package version

// Version is set at build time via ldflags.
// Example: go build -ldflags "-X github.com/hhiroshell/kubectl-realname-diff/pkg/version.Version=v1.0.0"
var Version = "dev"
