package edge

// KeyClick Tracker local change (not part of upstream go-webview2): lets the host program pass
// additional command line arguments to the WebView2 browser engine. It implements the small COM
// object ICoreWebView2EnvironmentOptions that CreateCoreWebView2EnvironmentWithOptions accepts.
// Upstream passes no options. The WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS environment variable is
// not read by the runtime in this setup, so the options object is the dependable way.

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// AdditionalBrowserArguments, when not empty, is given to the browser engine at start. Set it
// before the first WebView is created, and keep it the same for every WebView of the program.
var AdditionalBrowserArguments string

var (
	ole32           = windows.NewLazySystemDLL("ole32.dll")
	procCoTaskAlloc = ole32.NewProc("CoTaskMemAlloc")
)

type envOptionsVtbl struct {
	queryInterface, addRef, release                 uintptr
	getAdditionalBrowserArguments                   uintptr
	putAdditionalBrowserArguments                   uintptr
	getLanguage, putLanguage                        uintptr
	getTargetCompatibleBrowserVersion               uintptr
	putTargetCompatibleBrowserVersion               uintptr
	getAllowSingleSignOnUsingOSPrimaryAccount       uintptr
	putAllowSingleSignOnUsingOSPrimaryAccount       uintptr
}

type envOptions struct {
	vtbl *envOptionsVtbl
	args string
}

var (
	theEnvOptions *envOptions // kept for the life of the program: the engine holds the pointer
	envVtbl       *envOptionsVtbl
)

var (
	iidUnknown            = windows.GUID{Data1: 0x00000000, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidEnvironmentOptions = windows.GUID{Data1: 0x2fde08a8, Data2: 0x1e9a, Data3: 0x4766, Data4: [8]byte{0x8c, 0x05, 0x95, 0xa9, 0xce, 0xb9, 0xd1, 0xc5}}
)

// coString returns a CoTaskMemAlloc'ed wide string; the caller (the engine) frees it.
func coString(s string) uintptr {
	u, _ := windows.UTF16FromString(s)
	size := uintptr(len(u) * 2)
	p, _, _ := procCoTaskAlloc.Call(size)
	if p == 0 {
		return 0
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(u)), u)
	return p
}

func setOut(out uintptr, v uintptr) {
	if out != 0 {
		*(*uintptr)(unsafe.Pointer(out)) = v
	}
}

// newEnvOptions returns the options object (as a COM interface pointer) for the given arguments.
func newEnvOptions(args string) uintptr {
	if envVtbl == nil {
		envVtbl = &envOptionsVtbl{
			queryInterface: windows.NewCallback(func(this, riid, ppv uintptr) uintptr {
				if riid != 0 {
					id := *(*windows.GUID)(unsafe.Pointer(riid))
					if id == iidUnknown || id == iidEnvironmentOptions {
						setOut(ppv, this)
						return 0
					}
				}
				setOut(ppv, 0)
				return 0x80004002 // E_NOINTERFACE
			}),
			addRef:  windows.NewCallback(func(this uintptr) uintptr { return 1 }),
			release: windows.NewCallback(func(this uintptr) uintptr { return 1 }),
			getAdditionalBrowserArguments: windows.NewCallback(func(this, out uintptr) uintptr {
				o := (*envOptions)(unsafe.Pointer(this))
				setOut(out, coString(o.args))
				return 0
			}),
			putAdditionalBrowserArguments: windows.NewCallback(func(this, in uintptr) uintptr { return 0 }),
			getLanguage: windows.NewCallback(func(this, out uintptr) uintptr {
				setOut(out, coString(""))
				return 0
			}),
			putLanguage: windows.NewCallback(func(this, in uintptr) uintptr { return 0 }),
			getTargetCompatibleBrowserVersion: windows.NewCallback(func(this, out uintptr) uintptr {
				setOut(out, coString("86.0.616.0")) // the oldest runtime this program supports
				return 0
			}),
			putTargetCompatibleBrowserVersion: windows.NewCallback(func(this, in uintptr) uintptr { return 0 }),
			getAllowSingleSignOnUsingOSPrimaryAccount: windows.NewCallback(func(this, out uintptr) uintptr {
				if out != 0 {
					*(*int32)(unsafe.Pointer(out)) = 0
				}
				return 0
			}),
			putAllowSingleSignOnUsingOSPrimaryAccount: windows.NewCallback(func(this, in uintptr) uintptr { return 0 }),
		}
	}
	theEnvOptions = &envOptions{vtbl: envVtbl, args: args}
	return uintptr(unsafe.Pointer(theEnvOptions))
}
