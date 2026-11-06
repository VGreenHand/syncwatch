//go:build windows

package main

import (
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// 本文件只做两件事：
//  1. 用 Win32 API 造一个"承载 mpv 的子窗口"，让 mpv 真正嵌进 Wails 主窗口内部；
//  2. 把这些窗口操作全部串行到一个专用 OS 线程上执行。
//
// 为什么必须有专用线程：CreateWindowEx 创建的窗口归属于调用它的线程，
// 而 Go 的 goroutine 会被调度到任意线程。窗口一旦跨线程被 ShowWindow /
// SetWindowPos 操作，系统会向窗口所属线程发同步消息，那个线程若没有消息处理，
// 调用就会一直阻塞（实测卡死在这里）。
//
// 层级结构（从下往上）：
//   主窗口背景 → mpv 宿主子窗口（本文件创建）→ WebView2（Wails 自带）
// mpv 宿主必须沉到最底层，WebView2 才能通过透明背景把控制条叠在视频上。

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pEnumWindows              = user32.NewProc("EnumWindows")
	pIsWindowVisible          = user32.NewProc("IsWindowVisible")
	pGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	pCreateWindowExW          = user32.NewProc("CreateWindowExW")
	pDestroyWindow            = user32.NewProc("DestroyWindow")
	pIsWindow                 = user32.NewProc("IsWindow")
	pSetWindowPos             = user32.NewProc("SetWindowPos")
	pGetClientRect            = user32.NewProc("GetClientRect")
	pGetClassNameW            = user32.NewProc("GetClassNameW")
	pShowWindow               = user32.NewProc("ShowWindow")
	pPeekMessageW             = user32.NewProc("PeekMessageW")
	pTranslateMessage         = user32.NewProc("TranslateMessage")
	pDispatchMessageW         = user32.NewProc("DispatchMessageW")
	pGetModuleHandleW         = kernel32.NewProc("GetModuleHandleW")
)

const (
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsClipChildren = 0x02000000

	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010

	hwndBottom = 1
	swShow     = 5
	swHide     = 0
	pmRemove   = 0x0001
)

type winRect struct {
	Left, Top, Right, Bottom int32
}

// winMsg 对应 Win32 的 MSG 结构（x64 下 48 字节，显式补齐对齐）。
type winMsg struct {
	Hwnd    uintptr
	Message uint32
	_       uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type uiCall struct {
	fn   func()
	done chan struct{}
}

var uiCh = make(chan uiCall, 64)

func init() {
	go func() {
		// 固定到一个 OS 线程，之后所有窗口都在这个线程上创建与操作
		runtime.LockOSThread()
		for {
			select {
			case c := <-uiCh:
				c.fn()
				close(c.done)
			default:
				// 顺带把积压的窗口消息消化掉，避免某些控件行为异常
				var m winMsg
				for {
					r, _, _ := pPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
					if r == 0 {
						break
					}
					pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
					pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
				}
				time.Sleep(4 * time.Millisecond)
			}
		}
	}()
}

// runOnUI 把窗口操作投递到专用线程执行，带超时保护（避免再次出现整条链路卡死）。
func runOnUI(fn func()) bool {
	done := make(chan struct{})
	select {
	case uiCh <- uiCall{fn: fn, done: done}:
	case <-time.After(2 * time.Second):
		logf("runOnUI: 投递超时")
		return false
	}
	select {
	case <-done:
		return true
	case <-time.After(3 * time.Second):
		logf("runOnUI: 执行超时")
		return false
	}
}

// windowClass 读取窗口类名。
func windowClass(hwnd uintptr) string {
	buf := make([]uint16, 256)
	n, _, _ := pGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// findAppWindow 找到本进程的 Wails 主窗口。
//
// 不能只靠 IsWindowVisible 判断：OnDomReady 触发时窗口往往还没显示，
// 会把主窗口漏掉，导致播放器宿主永远建不出来。
// 所以优先按类名 wailsWindow + PID 匹配，可见性只作为兜底。
func findAppWindow() uintptr {
	pid := uint32(os.Getpid())
	var byClass, byVisible uintptr
	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		var wpid uint32
		pGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&wpid)))
		if wpid != pid {
			return 1
		}
		if windowClass(hwnd) == "wailsWindow" {
			byClass = hwnd
			return 0 // 找到就停
		}
		if byVisible == 0 {
			if v, _, _ := pIsWindowVisible.Call(hwnd); v != 0 {
				byVisible = hwnd
			}
		}
		return 1
	})
	pEnumWindows.Call(cb, 0)
	if byClass != 0 {
		return byClass
	}
	return byVisible
}

// createMpvHost 在父窗口里创建一个子窗口，交给 mpv 做渲染目标。
// 用系统预定义的 STATIC 类，省掉注册窗口类的步骤。
func createMpvHost(parent uintptr, x, y, w, h int) uintptr {
	var result uintptr
	runOnUI(func() {
		cls, _ := syscall.UTF16PtrFromString("STATIC")
		title, _ := syscall.UTF16PtrFromString("")
		hInst, _, _ := pGetModuleHandleW.Call(0)

		hwnd, _, _ := pCreateWindowExW.Call(
			0,
			uintptr(unsafe.Pointer(cls)),
			uintptr(unsafe.Pointer(title)),
			uintptr(wsChild|wsVisible|wsClipChildren),
			uintptr(x), uintptr(y), uintptr(w), uintptr(h),
			parent, 0, hInst, 0,
		)
		if hwnd == 0 {
			return
		}
		// 关键一步：沉到同层最底，否则后创建的子窗口会盖住 WebView2，
		// 导致控制条被视频吃掉（这正是设计稿第 8 章要验证的点）。
		pSetWindowPos.Call(hwnd, hwndBottom, 0, 0, 0, 0,
			swpNoMove|swpNoSize|swpNoActivate)
		pShowWindow.Call(hwnd, swShow)
		result = hwnd
	})
	return result
}

// moveMpvHost 调整 mpv 宿主窗口的位置和大小（不改变层级）。
func moveMpvHost(hwnd uintptr, x, y, w, h int) {
	if hwnd == 0 {
		return
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	runOnUI(func() {
		if v, _, _ := pIsWindow.Call(hwnd); v == 0 {
			return
		}
		pSetWindowPos.Call(hwnd, 0,
			uintptr(x), uintptr(y), uintptr(w), uintptr(h),
			swpNoZOrder|swpNoActivate)
	})
}

// showMpvHost 显隐宿主窗口（切换页面时用，避免 mpv 浮在首页上）。
func showMpvHost(hwnd uintptr, show bool) {
	if hwnd == 0 {
		return
	}
	cmd := uintptr(swHide)
	if show {
		cmd = swShow
	}
	runOnUI(func() {
		if v, _, _ := pIsWindow.Call(hwnd); v == 0 {
			return
		}
		pShowWindow.Call(hwnd, cmd)
	})
}

func destroyMpvHost(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	runOnUI(func() {
		if v, _, _ := pIsWindow.Call(hwnd); v != 0 {
			pDestroyWindow.Call(hwnd)
		}
	})
}

// clientSize 返回主窗口客户区尺寸（物理像素）。
func clientSize(hwnd uintptr) (int, int) {
	if hwnd == 0 {
		return 0, 0
	}
	var w, h int
	runOnUI(func() {
		var r winRect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		w = int(r.Right - r.Left)
		h = int(r.Bottom - r.Top)
	})
	return w, h
}
