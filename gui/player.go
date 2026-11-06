//go:build windows

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio"
)

var pipeSeq int

// playerLogWriter 把 mpv 的 stdout/stderr 逐行转进应用日志。
type playerLogWriter struct{}

func (playerLogWriter) Write(p []byte) (int, error) {
	text := strings.TrimRight(string(p), "\r\n")
	if text != "" {
		for _, line := range strings.Split(text, "\n") {
			if line != "" {
				logf("[mpv] %s", line)
			}
		}
	}
	return len(p), nil
}

// Player 封装 mpv 的启动与 JSON IPC。
// 与旧控制台版的区别只有一处：启动 mpv 时带 --wid，让画面嵌进宿主子窗口，
// 而不是开一个独立窗口。其余命令与属性读取逻辑保持一致。
type Player struct {
	cmd  *exec.Cmd
	conn interface {
		Read(b []byte) (int, error)
		Write(b []byte) (int, error)
		Close() error
	}

	writeMu sync.Mutex
	pendMu  sync.Mutex
	nextID  int
	pending map[int]chan json.RawMessage

	dbg      atomic.Int32
	pipe     string
	exited   chan struct{}
	onEvent  func(name string, value interface{})
	onExited func()
}

func newPlayer(onEvent func(string, interface{}), onExited func()) *Player {
	return &Player{
		pending:  map[int]chan json.RawMessage{},
		exited:   make(chan struct{}),
		onEvent:  onEvent,
		onExited: onExited,
	}
}

// Start 启动 mpv 并让它渲染到 hostHWND。
func (p *Player) Start(mpvPath, videoPath string, hostHWND uintptr) error {
	if p.cmd != nil {
		return fmt.Errorf("播放器已在运行")
	}
	if hostHWND == 0 {
		return fmt.Errorf("播放器宿主窗口未就绪")
	}
	pipeSeq++
	p.pipe = `\\.\pipe\syncwatch-gui-` + strconv.Itoa(os.Getpid()) + "-" + strconv.Itoa(pipeSeq)

	args := []string{
		"--wid=" + strconv.FormatUint(uint64(hostHWND), 10),
		"--no-border",
		"--force-window=yes",
		"--keep-open=yes",
		"--pause=yes",
		"--no-terminal",
		"--osc=no",
		"--osd-level=0",
		"--input-ipc-server=" + p.pipe,
		"--no-input-default-bindings",
		"--volume=100",
	}
	// 调试用：SYNCWATCH_VO=gdi 可让画面走 GDI，便于用 PrintWindow 抓图验证
	if vo := os.Getenv("SYNCWATCH_VO"); vo != "" {
		args = append(args, "--vo="+vo)
	}
	args = append(args, videoPath)

	cmd := exec.Command(mpvPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	// mpv 的输出全接进日志：起不来、参数不认、解码失败都只能从这里看出来
	cmd.Stdout = playerLogWriter{}
	cmd.Stderr = playerLogWriter{}
	logf("启动 mpv: %s", mpvPath)
	logf("参数: %v", args)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 mpv 失败: %w", err)
	}
	p.cmd = cmd

	// 等 mpv 把命名管道建起来（冷启动通常 200~600ms）
	var conn interface {
		Read(b []byte) (int, error)
		Write(b []byte) (int, error)
		Close() error
	}
	deadline := time.Now().Add(12 * time.Second)
	for {
		timeout := 500 * time.Millisecond
		c, err := winio.DialPipe(p.pipe, &timeout)
		if err == nil {
			conn = c
			break
		}
		if time.Now().After(deadline) {
			p.kill()
			return fmt.Errorf("连接 mpv 命名管道超时")
		}
		time.Sleep(150 * time.Millisecond)
	}
	p.conn = conn

	go p.readLoop()
	go func() {
		cmd.Wait()
		select {
		case <-p.exited:
		default:
			close(p.exited)
		}
		if p.onExited != nil {
			p.onExited()
		}
	}()

	// 只订阅界面确实要用的属性，避免事件洪水
	for i, prop := range []string{"time-pos", "duration", "pause", "speed", "volume", "eof-reached"} {
		if err := p.send(map[string]interface{}{
			"command": []interface{}{"observe_property", i + 1, prop},
		}); err != nil {
			logf("observe %s 发送失败: %v", prop, err)
		}
	}
	logf("已订阅属性事件")
	return nil
}

func (p *Player) send(v interface{}) error {
	if p.conn == nil {
		return fmt.Errorf("播放器未就绪")
	}
	data, _ := json.Marshal(v)
	data = append(data, '\n')
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_, err := p.conn.Write(data)
	return err
}

func (p *Player) readLoop() {
	sc := bufio.NewScanner(p.conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var ev struct {
			Event     string          `json:"event"`
			Name      string          `json:"name"`
			Data      json.RawMessage `json:"data"`
			RequestID int             `json:"request_id"`
		}
		if n := p.dbg.Add(1); n <= 30 {
			logf("RAW[%d]: %s", n, string(sc.Bytes()))
		}
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			logf("事件解析失败: %v", err)
			continue
		}
		if ev.Event == "property-change" {
			if n := p.dbg.Add(1); n <= 20 {
				logf("event property-change name=%q data=%s", ev.Name, string(ev.Data))
			}
		}
		if ev.Event == "property-change" && p.onEvent != nil {
			switch ev.Name {
			case "time-pos", "duration", "speed", "volume":
				var f float64
				if json.Unmarshal(ev.Data, &f) == nil {
					p.onEvent(ev.Name, f)
				}
			case "pause", "eof-reached":
				var b bool
				if json.Unmarshal(ev.Data, &b) == nil {
					p.onEvent(ev.Name, b)
				}
			}
			continue
		}
		if ev.RequestID != 0 {
			p.pendMu.Lock()
			ch, ok := p.pending[ev.RequestID]
			if ok {
				delete(p.pending, ev.RequestID)
			}
			p.pendMu.Unlock()
			if ok {
				ch <- ev.Data
			}
		}
	}
}

func (p *Player) command(args ...interface{}) error {
	return p.send(map[string]interface{}{"command": args})
}

// getRaw 发 get_property 并等回包。超时或出错一律 ok=false——
// 绝不返回 0 冒充有效值，否则一个 0 会被当成"跳到片头"广播出去（v0.3 的坑）。
func (p *Player) getRaw(prop string) (json.RawMessage, bool) {
	p.pendMu.Lock()
	id := p.nextID + 1
	p.nextID++
	ch := make(chan json.RawMessage, 1)
	p.pending[id] = ch
	p.pendMu.Unlock()

	drop := func() {
		p.pendMu.Lock()
		delete(p.pending, id)
		p.pendMu.Unlock()
	}

	if err := p.send(map[string]interface{}{
		"command":    []interface{}{"get_property", prop},
		"request_id": id,
	}); err != nil {
		drop()
		return nil, false
	}
	select {
	case raw := <-ch:
		if len(raw) == 0 || string(raw) == "null" {
			return nil, false
		}
		return raw, true
	case <-time.After(2 * time.Second):
		drop() // 不回包时也要把 pending 清掉，否则长时间播放会越攒越多
		return nil, false
	}
}

func (p *Player) getFloat(prop string) (float64, bool) {
	raw, ok := p.getRaw(prop)
	if !ok {
		return 0, false
	}
	var f float64
	if json.Unmarshal(raw, &f) != nil {
		return 0, false
	}
	return f, true
}

// getBool 读布尔属性。mpv 的 pause / seeking 是 JSON 布尔，
// 用 getFloat 去解会失败并静默得到 0（= false），v0.4 踩过这个坑。
func (p *Player) getBool(prop string) (bool, bool) {
	raw, ok := p.getRaw(prop)
	if !ok {
		return false, false
	}
	var b bool
	if json.Unmarshal(raw, &b) != nil {
		return false, false
	}
	return b, true
}

func (p *Player) Play() error  { return p.command("set_property", "pause", false) }
func (p *Player) Pause() error { return p.command("set_property", "pause", true) }

// IsPaused 问 mpv 真实暂停状态（界面点击播放/暂停时以此为准）。
func (p *Player) IsPaused() (bool, bool) { return p.getBool("pause") }

func (p *Player) Seek(pos float64) error {
	return p.command("seek", strconv.FormatFloat(pos, 'f', 3, 64), "absolute+exact")
}

func (p *Player) SetRate(rate float64) error {
	return p.command("set_property", "speed", rate)
}

func (p *Player) SetVolume(v float64) error {
	return p.command("set_property", "volume", v)
}

// Position 返回当前播放位置，取不到时 ok=false（绝不返回 0 冒充有效值）。
func (p *Player) Position() (float64, bool) { return p.getFloat("time-pos") }

func (p *Player) Duration() (float64, bool) { return p.getFloat("duration") }

func (p *Player) kill() {
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
}

// Close 关闭 IPC 并结束 mpv 进程，确保不残留。
func (p *Player) Close() {
	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
		p.cmd = nil
	}
	select {
	case <-p.exited:
	default:
		close(p.exited)
	}
}
