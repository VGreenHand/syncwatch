// ui.go: Windows 控制台界面层（配色、VT 支持、房间码横幅）
//
// 设计原则：
//   - 只用 ASCII 画框，不用 ┌─┐ 等字符。控制台代码页可能是 936(GBK)，
//     Unicode 框线会变乱码；ASCII 在 65001/936 下都不会坏。
//   - 颜色靠 ANSI 转义，Windows 10+ 需要先开启
//     ENABLE_VIRTUAL_TERMINAL_PROCESSING；开启失败就自动降级为无色输出，
//     绝不因为配色问题让程序崩掉或吐出乱码。
package main

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const AppVersion = "v0.4"

var vtEnabled atomic.Bool

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode  = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode  = kernel32.NewProc("SetConsoleMode")
	procSetConsoleTitle = kernel32.NewProc("SetConsoleTitleW")
)

const enableVirtualTerminalProcessing = 0x0004

// enableVT 打开控制台的 ANSI 转义支持（stdout/stderr 各开一次）。
func enableVT() {
	ok := false
	for _, f := range []*os.File{os.Stdout, os.Stderr} {
		h := syscall.Handle(f.Fd())
		var mode uint32
		if r, _, _ := procGetConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode))); r == 0 {
			continue
		}
		if r, _, _ := procSetConsoleMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing)); r != 0 {
			ok = true
		}
	}
	vtEnabled.Store(ok)
}

// paint 给文本上色；终端不支持 ANSI 时原样返回。
func paint(code, s string) string {
	if !vtEnabled.Load() {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// 语义化配色，改主题只动这里
func green(s string) string   { return paint("1;32", s) }
func yellow(s string) string  { return paint("1;33", s) }
func cyan(s string) string    { return paint("1;36", s) }
func magenta(s string) string { return paint("1;35", s) }
func red(s string) string     { return paint("1;31", s) }
func dim(s string) string     { return paint("2", s) }
func bold(s string) string    { return paint("1", s) }

// setConsoleTitle 改窗口标题，切到后台也能一眼看到房间码。
func setConsoleTitle(s string) {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return
	}
	procSetConsoleTitle.Call(uintptr(unsafe.Pointer(p)))
}

// printHeader 启动横幅。
func printHeader() {
	line := strings.Repeat("=", 50)
	fmt.Println()
	fmt.Println(cyan(line))
	fmt.Printf("%s  %s\n", bold("一起看 · SyncWatch"), dim(AppVersion))
	fmt.Println(dim("  本地视频零传输 · 多人同步播放"))
	fmt.Println(cyan(line))
}

// printStep 带序号和颜色的输入提示，让四步输入看起来是一套流程。
// sub 是标签下方的补充说明，必须打在 ">" 之前，否则会被输入箭头挤到后面去。
func printStep(no, total int, label, sub string) {
	fmt.Printf("\n%s %s\n", dim(fmt.Sprintf("[%d/%d]", no, total)), bold(label))
	if sub != "" {
		fmt.Println(dim("       " + sub))
	}
	fmt.Printf("%s ", cyan(">"))
}

// printRoomBanner 房间码横幅——整个程序最重要的信息，必须一眼看到。
//
// requested 是用户启动时填的房间码（空表示他本来就想新建房间）。
// 填了码却仍然新建房间，说明那个房间不存在或已解散，必须点破，
// 否则他会以为进了朋友的房间，结果两人各看各的。
func printRoomBanner(code string, created bool, requested string) {
	line := strings.Repeat("-", 50)
	fmt.Println()
	if created {
		fmt.Println(yellow(line))
		fmt.Println(yellow("  已创建新房间，你就是房主"))
		fmt.Printf("  房间码：%s\n", bold(yellow("  "+code+"  ")))
		if requested != "" && !strings.EqualFold(requested, code) {
			fmt.Println(red(line))
			fmt.Printf("%s 你输入的房间码 %s 不存在或已解散，\n", red("[注意]"), bold(requested))
			fmt.Println(red("        所以新建了一个房间，并没有进到朋友的房间！"))
			fmt.Println(red("        请核对朋友发来的房间码后重新启动。"))
		} else {
			fmt.Println(dim("  把房间码发给朋友，对方填入后即可和你一起看"))
		}
		fmt.Println(yellow(line))
	} else {
		fmt.Println(green(line))
		fmt.Println(green("  已加入房间"))
		fmt.Printf("  房间码：%s\n", bold(green("  "+code+"  ")))
		fmt.Println(green(line))
	}
	copyToClipboard(code)
	fmt.Println()
	setConsoleTitle("一起看 - 房间 " + code)
}

// printQuit 退出时的收尾提示，避免窗口一闪而过让人以为崩了。
func printQuit() {
	fmt.Println()
	fmt.Println(dim(strings.Repeat("-", 50)))
	fmt.Println(dim("  已退出房间，感谢使用。下回见。"))
	fmt.Println(dim(strings.Repeat("-", 50)))
	fmt.Println()
}
