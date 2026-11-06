//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是暴露给前端的方法集合。
// 前端不拼协议消息、不直接操作 mpv，所有动作都从这里进。
type App struct {
	ctx context.Context

	mu       sync.Mutex
	loading  bool
	mainHWND uintptr
	hostHWND uintptr
	player   *Player
	sync     *SyncClient
	full     bool

	cfg       Config
	lastPos   float64
	lastDur   float64
	isPaused  bool
	roomState string // 已应用到播放器的房间状态，用于识别回声
	roomCode  string
	connState string // idle / connecting / connected / disconnected
	fileSent  bool
	videoName string
	videoSize int64
	videoDur  float64
}

// Config 持久化在 %APPDATA%\SyncWatch\config.json，不含任何密码。
type Config struct {
	Server    string `json:"server"`
	Name      string `json:"name"`
	LastVideo string `json:"lastVideo"`
}

func NewApp() *App { return &App{connState: "idle"} }

// logf 把诊断信息写到 %TEMP%\syncwatch-gui.log。
// GUI 程序没有控制台，出问题时这是唯一能看到内部状态的地方。
func logf(format string, a ...interface{}) {
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "syncwatch-gui.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, time.Now().Format("15:04:05.000 ")+format+"\n", a...)
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.cfg = loadConfig()

	// 分发包可以在 exe 同目录放一个 server.txt 预置服务器地址，
	// 这样把包发给朋友，对方打开就能连，不用手填。
	// 用户自己在设置里改过的地址（config.json）优先级更高。
	if a.cfg.Server == "" {
		if exe, err := os.Executable(); err == nil {
			dir := filepath.Dir(exe)
			for _, name := range []string{"server.txt"} {
				if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
					if v := strings.TrimSpace(string(b)); v != "" {
						a.cfg.Server = v
						logf("从 %s 读取服务器地址: %s", name, v)
						break
					}
				}
			}
		}
	}
}

// domReady 在页面加载完成后执行：此时 Wails 主窗口一定存在。
func (a *App) domReady(ctx context.Context) {
	a.mu.Lock()
	a.mainHWND = findAppWindow()
	found := a.mainHWND != 0
	a.mu.Unlock()

	logf("domReady: mainHWND=%d mpv=%q", a.mainHWND, findMpv())
	wailsruntime.EventsEmit(ctx, "host:ready", map[string]interface{}{
		"hwnd": found,
		"mpv":  findMpv() != "",
	})

}

// LogMsg 让前端把关键节点写进同一份日志，便于定位"界面没反应"这类问题。
func (a *App) LogMsg(s string) { logf("[ui] %s", s) }

// DebugIntent 仅供自动化联调：从环境变量读取"自动载片 + 自动进房"的意图。
// 正常使用不会设置这些变量，所以对用户没有影响。
func (a *App) DebugIntent() map[string]string {
	return map[string]string{
		"video": os.Getenv("SYNCWATCH_AUTO_VIDEO"),
		"room":  os.Getenv("SYNCWATCH_AUTO_ROOM"),
		"name":  os.Getenv("SYNCWATCH_AUTO_NAME"),
	}
}

// ---------- 窗口中的视频区 ----------

// SetVideoRect 由前端在布局变化时调用，把视频区的位置告诉 Go。
// 前端传 CSS 像素 + 视口尺寸，Go 按客户区物理像素等比换算，天然兼容各种 DPI。
func (a *App) SetVideoRect(x, y, w, h, vw, vh float64) {
	a.mu.Lock()
	main := a.mainHWND
	a.mu.Unlock()

	if main == 0 {
		main = findAppWindow()
		if main != 0 {
			a.mu.Lock()
			a.mainHWND = main
			a.mu.Unlock()
		}
	}
	if main == 0 || vw <= 0 || vh <= 0 || w <= 0 || h <= 0 {
		return
	}
	cw, ch := clientSize(main)
	if cw == 0 || ch == 0 {
		return
	}
	sx := float64(cw) / vw
	sy := float64(ch) / vh
	px, py := int(x*sx+.5), int(y*sy+.5)
	pw, ph := int(w*sx+.5), int(h*sy+.5)

	a.mu.Lock()
	if a.hostHWND == 0 {
		a.hostHWND = createMpvHost(main, px, py, pw, ph)
		hwnd := a.hostHWND
		a.mu.Unlock()
		if hwnd == 0 {
			wailsruntime.EventsEmit(a.ctx, "host:error", "创建播放器宿主窗口失败")
			return
		}
		logf("创建 mpv 宿主: hwnd=%d rect=(%d,%d) %dx%d", hwnd, px, py, pw, ph)
		wailsruntime.EventsEmit(a.ctx, "host:hostready", nil)
		return
	}
	host := a.hostHWND
	a.mu.Unlock()
	moveMpvHost(host, px, py, pw, ph)
}

func (a *App) ShowVideoHost(show bool) {
	a.mu.Lock()
	host := a.hostHWND
	a.mu.Unlock()
	if host != 0 {
		showMpvHost(host, show)
	}
}

// ---------- 配置 ----------

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "SyncWatch", "config.json")
}

func loadConfig() Config {
	var c Config
	data, err := os.ReadFile(configPath())
	if err == nil {
		json.Unmarshal(data, &c)
	}
	return c
}

func (a *App) GetConfig() Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

func (a *App) SaveConfig(c Config) error {
	a.mu.Lock()
	if c.Server != "" {
		a.cfg.Server = strings.TrimSpace(c.Server)
	}
	if c.Name != "" {
		a.cfg.Name = c.Name
	}
	saved := a.cfg
	a.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(saved, "", "  ")
	return os.WriteFile(configPath(), data, 0o644)
}

// PingServer 快速检查服务器是否可达。
func (a *App) PingServer(server string) map[string]interface{} {
	server = strings.TrimSpace(server)
	if server == "" {
		return map[string]interface{}{"ok": false, "message": "未填写服务器地址"}
	}
	start := time.Now()
	sc := newSyncClient(nil, nil)
	err := sc.Connect(server, "PING01", "ping", "")
	if err != nil {
		return map[string]interface{}{"ok": false, "message": "连接失败：" + err.Error()}
	}
	ms := time.Since(start).Milliseconds()
	sc.Close()
	return map[string]interface{}{"ok": true, "message": fmt.Sprintf("连接正常（%d ms）", ms), "ms": ms}
}

// ---------- 视频 ----------

func (a *App) ChooseVideo() (map[string]interface{}, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择要一起看的视频",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "视频文件", Pattern: "*.mp4;*.mkv;*.avi;*.rmvb;*.mov;*.flv;*.wmv;*.ts;*.m2ts;*.webm;*.mpg;*.mpeg;*.m4v"},
			{DisplayName: "所有文件", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}
	return a.LoadVideo(path)
}

// LoadVideo 启动嵌入播放器并加载视频（此时还没进房间）。
func (a *App) LoadVideo(path string) (map[string]interface{}, error) {
	a.mu.Lock()
	if a.loading {
		a.mu.Unlock()
		return nil, fmt.Errorf("正在加载上一个视频，请稍候")
	}
	a.loading = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.loading = false
		a.mu.Unlock()
	}()

	if path == "" {
		return nil, fmt.Errorf("未指定视频文件")
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("文件不存在或无法访问：%s", filepath.Base(path))
	}
	mpv := findMpv()
	if mpv == "" {
		return nil, fmt.Errorf("未找到 mpv.exe，请把它放到程序目录的 mpv 子目录下")
	}

	a.mu.Lock()
	host := a.hostHWND
	main := a.mainHWND
	a.mu.Unlock()

	// 选片发生在首页/表单页，此时播放页还没渲染，宿主窗口尚不存在。
	// 先建一个 1x1 的隐藏宿主把 mpv 拉起来，等进入播放页再由 SetVideoRect 摆正。
	if host == 0 {
		if main == 0 {
			main = findAppWindow()
		}
		if main != 0 {
			host = createMpvHost(main, 0, 0, 1, 1)
			if host != 0 {
				a.mu.Lock()
				a.hostHWND = host
				a.mu.Unlock()
				showMpvHost(host, false)
				logf("预创建隐藏宿主: hwnd=%d", host)
			}
		}
	}
	if host == 0 {
		return nil, fmt.Errorf("播放区域尚未就绪，请稍后重试")
	}

	a.StopVideo()

	p := newPlayer(
		func(name string, v interface{}) { a.onPlayerEvent(name, v) },
		func() { wailsruntime.EventsEmit(a.ctx, "player:exited", nil) },
	)
	if err := p.Start(mpv, path, host); err != nil {
		logf("Player.Start 失败: %v", err)
		return nil, err
	}

	a.mu.Lock()
	a.player = p
	a.lastPos, a.lastDur, a.isPaused = 0, 0, true
	a.videoName = filepath.Base(path)
	a.videoSize = st.Size()
	a.fileSent = false
	a.mu.Unlock()

	showMpvHost(host, false) // 由播放页的 ShowVideoHost(true) 决定何时露出

	info := map[string]interface{}{
		"path": path,
		"name": filepath.Base(path),
		"size": st.Size(),
		"ext":  filepath.Ext(path),
	}
	wailsruntime.EventsEmit(a.ctx, "player:loaded", info)
	return info, nil
}

// StopVideo 结束播放器，确保不残留 mpv 进程。
func (a *App) StopVideo() {
	a.mu.Lock()
	p := a.player
	host := a.hostHWND
	a.player = nil
	a.lastPos, a.lastDur = 0, 0
	a.mu.Unlock()

	if p != nil {
		p.Close()
	}
	if host != 0 {
		showMpvHost(host, false)
	}
}

// ---------- 房间 ----------

func (a *App) startSync(server, room, name, password string) error {
	a.stopSync()

	sc := newSyncClient(func(ev SyncEvent) { a.onSyncEvent(ev) }, func() (float64, bool) {
		a.mu.Lock()
		p := a.player
		a.mu.Unlock()
		if p == nil {
			return 0, false
		}
		return p.Position()
	})

	a.mu.Lock()
	a.sync = sc
	a.connState = "connecting"
	a.mu.Unlock()

	wailsruntime.EventsEmit(a.ctx, "room:connecting", map[string]interface{}{"room": room})
	if err := sc.Connect(server, room, name, password); err != nil {
		a.mu.Lock()
		a.connState = "disconnected"
		a.mu.Unlock()
		return err
	}
	return nil
}

func (a *App) stopSync() {
	a.mu.Lock()
	sc := a.sync
	a.sync = nil
	a.roomState = ""
	a.roomCode = ""
	a.connState = "idle"
	a.mu.Unlock()
	if sc != nil {
		sc.Close()
	}
}

// CreateRoom 用当前已载入的视频创建房间。
func (a *App) CreateRoom(name, password string) (map[string]interface{}, error) {
	a.mu.Lock()
	hasVideo := a.player != nil
	server := a.cfg.Server
	a.mu.Unlock()

	if !hasVideo {
		return nil, fmt.Errorf("请先选择一部本地视频")
	}
	if strings.TrimSpace(server) == "" {
		return nil, fmt.Errorf("还没有配置服务器地址，请打开设置填写")
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("请填写昵称")
	}
	if err := a.startSync(server, "", name, password); err != nil {
		return nil, err
	}
	return map[string]interface{}{"ok": true}, nil
}

// JoinRoom 加入已有房间。
func (a *App) JoinRoom(room, name, password string) (map[string]interface{}, error) {
	a.mu.Lock()
	hasVideo := a.player != nil
	server := a.cfg.Server
	a.mu.Unlock()

	room = strings.ToUpper(strings.TrimSpace(room))
	if room == "" {
		return nil, fmt.Errorf("请填写房间码")
	}
	if !hasVideo {
		return nil, fmt.Errorf("请先选择一部本地视频")
	}
	if strings.TrimSpace(server) == "" {
		return nil, fmt.Errorf("还没有配置服务器地址，请打开设置填写")
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("请填写昵称")
	}
	if err := a.startSync(server, room, name, password); err != nil {
		return nil, err
	}
	return map[string]interface{}{"ok": true}, nil
}

// LeaveRoom 离开房间并停止播放。
func (a *App) LeaveRoom() {
	a.stopSync()
	a.StopVideo()
	wailsruntime.EventsEmit(a.ctx, "room:left", nil)
}

// CopyText 复制到剪贴板（房间码、诊断信息）。
func (a *App) CopyText(s string) {
	wailsruntime.ClipboardSetText(a.ctx, s)
}

// GetState 返回界面需要的当前状态（前端每秒拉一次作为兜底）。
func (a *App) GetState() map[string]interface{} {
	a.mu.Lock()
	p := a.player
	pos, dur := a.lastPos, a.lastDur
	paused := a.isPaused
	room := a.roomCode
	conn := a.connState
	sc := a.sync
	name := a.videoName
	size := a.videoSize
	a.mu.Unlock()

	if p != nil {
		if v, ok := p.Position(); ok {
			pos = v
		}
		if v, ok := p.Duration(); ok && v > 0 {
			dur = v
		}
	}
	members := []MemberInfo{}
	if sc != nil {
		if m := sc.Members(); m != nil {
			members = m
		}
	}
	return map[string]interface{}{
		"position":  pos,
		"duration":  dur,
		"paused":    paused,
		"room":      room,
		"conn":      conn,
		"members":   members,
		"videoName": name,
		"videoSize": size,
	}
}

// ---------- 播放控制 ----------

func (a *App) TogglePlay() (bool, error) {
	a.mu.Lock()
	p := a.player
	paused := a.isPaused
	a.mu.Unlock()
	if p == nil {
		return paused, fmt.Errorf("播放器未就绪")
	}
	if paused {
		a.setPaused(false)
		pos, _ := p.Position()
		rate := 1.0
		if r, ok := p.getFloat("speed"); ok && r > 0 {
			rate = r
		}
		p.Play()
		a.reportPlay(pos, rate)
		return false, nil
	}
	a.setPaused(true)
	pos, _ := p.Position()
	p.Pause()
	a.reportPause(pos)
	return true, nil
}

func (a *App) Seek(pos float64) error {
	if pos < 0 {
		pos = 0
	}
	a.mu.Lock()
	p := a.player
	sc := a.sync
	a.mu.Unlock()
	if p == nil {
		return fmt.Errorf("播放器未就绪")
	}
	if err := p.Seek(pos); err != nil {
		return err
	}
	if sc != nil {
		sc.ReportSeek(pos)
	}
	return nil
}

func (a *App) SetRate(rate float64) error {
	a.mu.Lock()
	p := a.player
	sc := a.sync
	a.mu.Unlock()
	if p == nil {
		return fmt.Errorf("播放器未就绪")
	}
	if err := p.SetRate(rate); err != nil {
		return err
	}
	if sc != nil {
		sc.SetRate(rate)
	}
	return nil
}

func (a *App) SetVolume(v float64) error {
	a.mu.Lock()
	p := a.player
	a.mu.Unlock()
	if p == nil {
		return fmt.Errorf("播放器未就绪")
	}
	return p.SetVolume(v)
}

func (a *App) ToggleFullscreen() bool {
	a.mu.Lock()
	a.full = !a.full
	full := a.full
	a.mu.Unlock()
	if full {
		wailsruntime.WindowFullscreen(a.ctx)
	} else {
		wailsruntime.WindowUnfullscreen(a.ctx)
	}
	return full
}

// ---------- 同步事件 ----------

func (a *App) onSyncEvent(ev SyncEvent) {
	switch ev.Kind {
	case "joined":
		d, _ := ev.Data.(map[string]interface{})
		room, _ := d["room"].(string)
		created, _ := d["created"].(bool)
		state, _ := d["state"].(string)
		pos, _ := d["position"].(float64)
		members, _ := d["members"].([]MemberInfo)

		logf("joined room=%s created=%v state=%s pos=%.2f members=%d", room, created, state, pos, len(members))
		a.mu.Lock()
		a.roomCode = room
		a.connState = "connected"
		a.fileSent = false
		name, size, dur := a.videoName, a.videoSize, a.videoDur
		sc := a.sync
		a.mu.Unlock()

		a.applyRemote(state, pos)

		// 进房后上报片源，供成员列表显示时长一致性
		if sc != nil {
			sc.SetFileInfo(name, size, dur)
			a.mu.Lock()
			a.fileSent = true
			a.mu.Unlock()
		}

		wailsruntime.EventsEmit(a.ctx, "room:joined", map[string]interface{}{
			"room": room, "created": created, "state": state,
			"position": pos, "members": members,
		})

	case "state":
		d, _ := ev.Data.(map[string]interface{})
		state, _ := d["state"].(string)
		pos, _ := d["position"].(float64)
		op, _ := d["operator"].(string)
		reason, _ := d["reason"].(string)
		a.applyRemote(state, pos)
		wailsruntime.EventsEmit(a.ctx, "room:state", map[string]interface{}{
			"state": state, "position": pos, "operator": op, "reason": reason,
		})

	case "members":
		wailsruntime.EventsEmit(a.ctx, "room:members", ev.Data)

	case "notice":
		wailsruntime.EventsEmit(a.ctx, "room:notice", ev.Data)

	case "error":
		a.mu.Lock()
		a.connState = "disconnected"
		a.mu.Unlock()
		wailsruntime.EventsEmit(a.ctx, "room:error", ev.Data)

	case "closed":
		a.mu.Lock()
		a.connState = "disconnected"
		a.mu.Unlock()
		wailsruntime.EventsEmit(a.ctx, "room:closed", ev.Data)
	}
}

// applyRemote 把服务器状态落到本地播放器。
// 状态切换时严格（0.5s），纯位置同步时宽松（1.5s），避免播放中频繁打断。
func (a *App) applyRemote(state string, pos float64) {
	a.mu.Lock()
	p := a.player
	prev := a.roomState
	a.roomState = state
	a.isPaused = state != "playing"
	a.mu.Unlock()

	if p == nil {
		return
	}
	threshold := 1.5
	if prev != state {
		threshold = 0.5
	}
	if cur, ok := p.Position(); !ok || math.Abs(cur-pos) > threshold {
		logf("applyRemote seek %.2f -> %.2f (cur=%.2f thr=%.1f state=%s)", cur, pos, cur, threshold, state)
		p.Seek(pos)
	}
	if state == "playing" {
		p.Play()
	} else {
		p.Pause()
	}
}

// onPlayerEvent 处理 mpv 属性变化；本地操作在这里上报，回声在这里被过滤。
func (a *App) onPlayerEvent(name string, v interface{}) {
	switch name {
	case "time-pos":
		if f, ok := v.(float64); ok && f >= 0 {
			a.mu.Lock()
			a.lastPos = f
			a.mu.Unlock()
		}
	case "duration":
		if f, ok := v.(float64); ok && f > 0 {
			a.mu.Lock()
			a.lastDur = f
			a.videoDur = f
			sc := a.sync
			sent := a.fileSent
			vname, vsize := a.videoName, a.videoSize
			a.mu.Unlock()
			if sc != nil && !sent {
				sc.SetFileInfo(vname, vsize, f)
				a.mu.Lock()
				a.fileSent = true
				a.mu.Unlock()
			}
		}
	case "pause":
		b, ok := v.(bool)
		if !ok {
			return
		}
		a.mu.Lock()
		p := a.player
		st := a.roomState
		sc := a.sync
		a.mu.Unlock()

		a.setPaused(b)
		if sc == nil || st == "" {
			return
		}
		// 与房间状态一致 → 是我们自己下发指令造成的回声，不上报
		if b && st == "paused" {
			return
		}
		if !b && st == "playing" {
			return
		}
		pos := a.lastPos
		if p != nil {
			if v, ok := p.Position(); ok {
				pos = v
			}
		}
		if b {
			a.reportPause(pos)
		} else {
			a.reportPlay(pos, 1.0)
		}
	}
}

func (a *App) reportPlay(pos, rate float64) {
	a.mu.Lock()
	sc := a.sync
	a.mu.Unlock()
	if sc != nil {
		sc.ReportPlay(pos, rate)
	}
}

func (a *App) reportPause(pos float64) {
	a.mu.Lock()
	sc := a.sync
	a.mu.Unlock()
	if sc != nil {
		sc.ReportPause(pos)
	}
}

func (a *App) setPaused(v bool) {
	a.mu.Lock()
	a.isPaused = v
	a.mu.Unlock()
	wailsruntime.EventsEmit(a.ctx, "player:paused", v)
}

// ---------- 日志 ----------

func (a *App) GetLogs() string {
	data, err := os.ReadFile(filepath.Join(os.TempDir(), "syncwatch-gui.log"))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 300 {
		lines = lines[len(lines)-300:]
	}
	return strings.Join(lines, "\n")
}

func (a *App) ClearLogs() {
	os.Remove(filepath.Join(os.TempDir(), "syncwatch-gui.log"))
}

// findMpv 按优先级找 mpv.exe：环境变量 → 程序目录 → 开发期上级目录 → PATH。
func findMpv() string {
	if p := os.Getenv("SYNCWATCH_MPV"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "mpv", "mpv.exe"),
			filepath.Join(dir, "mpv.exe"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "mpv", "mpv.exe"),
			filepath.Join(wd, "..", "windows", "mpv", "mpv.exe"),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if p, err := exec.LookPath("mpv.exe"); err == nil {
		return p
	}
	return ""
}
