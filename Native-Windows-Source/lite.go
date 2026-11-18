package main

// v2.2.0 Lite mode and idle behaviour.
//
// The window is a WebView2 (Microsoft Edge engine) page. Its largest costs are the GPU process,
// the extra helper processes and continuous animation. Two things lower them:
//   - Lite mode: no animations or visual effects in the page (applied at once), and a lighter
//     engine (software drawing, one renderer, no background services) that WebView2 only reads
//     when it starts, so it applies after the next start of the app.
//   - Hidden window: when the window is hidden to the tray or minimised, the page is told so it
//     can stop animating and redrawing. This applies in normal mode too.

import (
	"os"
	"strings"
	"sync/atomic"

	"github.com/jchv/go-webview2/pkg/edge"
)

// liteBrowserArgs are extra command line options for the WebView2 browser engine in Lite mode.
// Options an engine version does not know are ignored by it.
const liteBrowserArgs = "--enable-low-end-device-mode --disable-gpu --disable-gpu-compositing --in-process-gpu " +
	"--renderer-process-limit=1 --disable-extensions --disable-component-update --disable-background-networking " +
	"--disable-sync --no-pings --disable-breakpad " +
	"--disable-features=Translate,OptimizationHints,MediaRouter,AutofillServerCommunication,msSmartScreenProtection,msEdgeShoppingUI " +
	"--js-flags=--max-old-space-size=96"

// applyLiteEngine sets the engine options for Lite mode. It must run before the first WebView2
// window is created, because the engine reads them once, when it starts.
func applyLiteEngine() {
	storeMu.RLock()
	on := store.Settings != nil && store.Settings.LiteMode
	storeMu.RUnlock()
	args := ""
	if on {
		args = liteBrowserArgs
		logLine("Lite mode is on: lighter browser engine options")
	}
	if os.Getenv("KEYCLICK_DEV") == "1" { // test copies only: extra engine options for measuring
		args = strings.TrimSpace(args + " " + os.Getenv("KEYCLICK_EXTRA_ARGS"))
	}
	// The options go to the engine through the environment options object. The environment
	// variable WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS is not read in this setup.
	edge.AdditionalBrowserArguments = args
}

var uiVisibleFlag atomic.Bool

func init() { uiVisibleFlag.Store(true) }

// setUIVisible tells the page whether its window can be seen, only when that changes.
func setUIVisible(v bool) {
	if uiVisibleFlag.Swap(v) == v {
		return
	}
	if v {
		jsEval("window.kcVisible&&window.kcVisible(true)")
	} else {
		jsEval("window.kcVisible&&window.kcVisible(false)")
	}
}
