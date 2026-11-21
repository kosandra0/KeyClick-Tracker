//go:build windows

// v2.5.0: the Microsoft Edge WebView2 Runtime.
//
// The app window is drawn by Microsoft's WebView2 Runtime (a free, signed Microsoft component
// that is part of Windows 11 and of most Windows 10 PCs, but not of a fresh Windows 10 or of
// Windows Sandbox). Before v2.5.0 a PC without it got an empty white window and the app page in
// the default web browser. Now the app (and its setup program) installs the runtime from
// Microsoft, checks Microsoft's signature on the downloaded installer, and only then opens the
// real app window. The app never falls back to a browser.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2/webviewloader"
)

// Microsoft's permanent link to the small "Evergreen Bootstrapper" (about 1.7 MB). It downloads
// and installs the current runtime. https://developer.microsoft.com/microsoft-edge/webview2/
const webView2BootstrapperURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

var (
	pPeekMessageW = user32.NewProc("PeekMessageW")
	pUpdateWindow = user32.NewProc("UpdateWindow")
)

// webView2Version returns the installed runtime version, or "" when there is none.
func webView2Version() string {
	v, err := webviewloader.GetInstalledVersion()
	if err != nil {
		logLine("WebView2 version check failed: " + err.Error())
		return ""
	}
	return v
}

// downloadFile saves url to path. Only https is accepted.
func downloadFile(url, path string, timeout time.Duration) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("refusing a non-https address")
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download answered HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n < 100*1024 {
		err = fmt.Errorf("download is only %d bytes", n)
	}
	return err
}

// verifyMicrosoftSignature accepts the file only if Windows reports a valid Authenticode
// signature whose signer is Microsoft Corporation.
func verifyMicrosoftSignature(path string) error {
	// The path is placed in the script as a single-quoted PowerShell string. (PowerShell does not
	// fill $args when -Command is used, which is what the first v2.5.0 build relied on.)
	script := signatureScript(path)
	c := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("signature check could not run: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return checkSignatureAnswer(strings.TrimSpace(string(out)))
}

// signatureScript builds the PowerShell that prints "<Status>|<Subject>" for a file.
func signatureScript(path string) string {
	q := "'" + strings.ReplaceAll(path, "'", "''") + "'"
	return `$s = Get-AuthenticodeSignature -LiteralPath ` + q + `; $n = ''; if ($s.SignerCertificate) { $n = $s.SignerCertificate.Subject }; Write-Output ($s.Status.ToString() + '|' + $n)`
}

// checkSignatureAnswer decodes "<Status>|<Subject>" from the check above.
func checkSignatureAnswer(answer string) error {
	status, subject, _ := strings.Cut(answer, "|")
	if status != "Valid" {
		return fmt.Errorf("the downloaded installer is not validly signed (%s)", status)
	}
	if !strings.Contains(subject, "O=Microsoft Corporation") {
		return fmt.Errorf("the downloaded installer is not signed by Microsoft Corporation")
	}
	return nil
}

// installWebView2Runtime downloads Microsoft's bootstrapper, checks its signature, runs it
// silently and waits until the runtime shows up. It needs an internet connection.
func installWebView2Runtime(say func(string)) error {
	dir, err := os.MkdirTemp("", "KeyClickTracker-WebView2-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir) // only the folder created just above
	setup := filepath.Join(dir, "MicrosoftEdgeWebview2Setup.exe")
	say("downloading Microsoft's WebView2 installer")
	if err := downloadFile(webView2BootstrapperURL, setup, 5*time.Minute); err != nil {
		return fmt.Errorf("could not download the installer from Microsoft (is this PC online?): %v", err)
	}
	if err := verifyMicrosoftSignature(setup); err != nil {
		return err
	}
	say("installer signature verified (Microsoft Corporation); installing")
	c := exec.Command(setup, "/silent", "/install")
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	done := make(chan error, 1)
	if err := c.Start(); err != nil {
		return fmt.Errorf("could not start Microsoft's installer: %v", err)
	}
	go func() { done <- c.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			say("installer exit: " + err.Error()) // the version check below decides
		}
	case <-time.After(10 * time.Minute):
		_ = c.Process.Kill()
		return fmt.Errorf("Microsoft's installer did not finish within 10 minutes")
	}
	for i := 0; i < 20; i++ { // the registration can lag a moment behind the installer
		if v := webView2Version(); v != "" {
			say("WebView2 Runtime " + v + " is installed")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("the installer finished but the WebView2 Runtime was not found afterwards")
}

// ------------------------------------------------ a small "please wait" window

type pt struct{ X, Y int32 }
type peekMsg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      pt
}

// withWaitWindow shows a small centred window with text while work runs, and keeps it
// responsive. It must be called on the thread that owns the UI.
func withWaitWindow(text string, work func()) {
	var finished atomic.Bool
	go func() { work(); finished.Store(true) }()
	cx, _, _ := pGetSystemMetrics.Call(0)
	cy, _, _ := pGetSystemMetrics.Call(1)
	w, h := 520, 140
	// A bordered popup made of one centred STATIC text control, always on top.
	// WS_POPUP | WS_BORDER | SS_CENTER, extended style WS_EX_TOPMOST
	hwnd, _, _ := pCreateWindowExW.Call(0x8, uintptr(unsafe.Pointer(utf16p("STATIC"))), uintptr(unsafe.Pointer(utf16p("\n"+text))),
		0x80800000|0x1,
		(cx-uintptr(w))/2, (cy-uintptr(h))/2, uintptr(w), uintptr(h), 0, 0, 0, 0)
	if hwnd != 0 {
		font, _, _ := pCreateFontW.Call(^uintptr(18-1), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(utf16p("Segoe UI"))))
		pSendMessageW.Call(hwnd, 0x30, font, 1) // WM_SETFONT
		pShowWindow.Call(hwnd, swShow)
		pUpdateWindow.Call(hwnd)
	}
	var m peekMsg
	for !finished.Load() {
		for {
			r, _, _ := pPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
			if r == 0 {
				break
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		time.Sleep(30 * time.Millisecond)
	}
	if hwnd != 0 {
		pDestroyWindow.Call(hwnd)
	}
}

// ensureWebView2 makes sure the runtime exists. quiet = no questions and no windows (used by the
// setup program, which has already asked). Returns nil when the runtime is available.
func ensureWebView2(quiet bool, say func(string)) error {
	if v := webView2Version(); v != "" {
		say("WebView2 Runtime " + v + " found")
		return nil
	}
	say("WebView2 Runtime not found")
	if !quiet {
		const yes = 6
		r, _, _ := pMessageBoxW.Call(0, uintptr(unsafe.Pointer(utf16p(
			appName+" draws its window with the Microsoft Edge WebView2 Runtime, a free component from Microsoft. "+
				"It is not installed on this computer yet.\n\n"+
				"Install it now? This needs an internet connection and takes about a minute. "+
				"The installer comes from Microsoft and its signature is checked first."))),
			uintptr(unsafe.Pointer(utf16p(appName+" needs one more component"))), 0x24) // MB_YESNO | MB_ICONQUESTION
		if r != yes {
			return fmt.Errorf("the WebView2 Runtime was not installed (declined)")
		}
	}
	var err error
	run := func() { err = installWebView2Runtime(say) }
	if quiet {
		run()
	} else {
		withWaitWindow("Installing the Microsoft Edge WebView2 Runtime...\nThis takes about a minute. "+appName+" opens when it is done.", run)
	}
	return err
}

// runInstallWebView2 is the "--install-webview2" switch: used by the setup program. Exit code 0
// means the runtime is available. "--quiet" suppresses every window.
func runInstallWebView2(quiet bool) int {
	logPath := filepath.Join(os.TempDir(), "KeyClickTracker-WebView2.log")
	say := func(s string) {
		if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), s)
			f.Close()
		}
	}
	if err := ensureWebView2(quiet, say); err != nil {
		say("FAILED: " + err.Error())
		return 1
	}
	return 0
}
