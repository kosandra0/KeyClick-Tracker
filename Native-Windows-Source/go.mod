module keyclicktracker

go 1.26.0

require (
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	golang.org/x/sys v0.50.0
)

require github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect

// KeyClick Tracker uses a local copy of go-webview2 that can pass arguments to the browser engine
// (Lite mode). The change is in third_party/go-webview2/pkg/edge/envoptions.go (MIT licence kept).
replace github.com/jchv/go-webview2 => ./third_party/go-webview2
