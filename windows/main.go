// syncwatch-win: 「一起看」Windows 客户端 v0.1
// 控制台程序：连接同步服务器，接管 mpv（JSON IPC 命名管道），实现多人同步播放。
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/gorilla/websocket"
)

const (
	SOFT_DRIFT      = 0.3
	PIPE_NAME       = `\\.\pipe\syncwatch-mpv`
	HEARTBEAT_EVERY = 5 * time.Second
)

// ---------- 消息（与服务端协议一致） ----------

type Msg struct {
	Type     string       `json:"type"`
	Room     string       `json:"room,omitempty"`
	Password string       `json:"password,omitempty"`
	Name     string       `json:"name,omitempty"`
	Position float64      `json:"position,omitempty"`
	Rate     float64      `json:"rate,omitempty"`
	State    string       `json:"state,omitempty"`
	Operator string       `json:"operator,omitempty"`
	AnchorMs int64        `json:"anchorMs,omitempty"`
	Error    string       `json:"error,omitempty"`
	File     string       `json:"file,omitempty"`
	Size     int64        `json:"size,omitempty"`
	Duration float64      `json:"duration,omitempty"`
	Members  []MemberInfo `json:"members,omitempty"`
	// Created 仅 hello 携带：true=本次新建了房间，false=加入了已存在的房间
	Created bool `json:"created,omitempty"`
}

type MemberInfo struct {
	Name     string  `json:"name"`
	State    string  `json:"state"`
	File     string  `json:"file,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Online   bool    `json:"online"`
}

// ---------- mpv IPC ----------

type MpvClient struct {
	conn    net.Conn
	mu      sync.Mutex // 写锁
	nextID  int
	pending map[int]chan json.RawMessage
	pendMu  sync.Mutex
}

func dialMpv(pipe string) (*MpvClient, error) {
	timeout := 10 * time.Second
	conn, err := winio.DialPipe(pipe, &timeout)
	if err != nil {
		return nil, err
	}
	c := &MpvClient{conn: conn, pending: map[int]chan json.RawMessage{}}
	go c.readLoop()
	return c, nil
}

func (c *MpvClient) readLoop() {
	sc := bufio.NewScanner(c.conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var ev struct {
			Event     string          `json:"event"`
			ID        int             `json:"id"`
			Name      string          `json:"name"`
			Data      json.RawMessage `json:"data"`
			RequestID int             `json:"request_id"`
			Error     string          `json:"error"`
		}
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Event == "property-change" {
			handlePropertyChange(ev.ID, ev.Name, ev.Data)
			continue
		}
		if ev.Event == "seek" {
			handleMpvSeekEvent()
			continue
		}
		if ev.Event == "end-file" {
			log.Println(dim("[mpv]") + " 播放结束")
			continue
		}
		if ev.RequestID != 0 {
			c.pendMu.Lock()
			ch, ok := c.pending[ev.RequestID]
			if ok {
				delete(c.pending, ev.RequestID)
			}
			c.pendMu.Unlock()
			if ok {
				ch <- ev.Data
			}
		}
	}
	log.Println(dim("[mpv]") + " IPC 断开")
}

func (c *MpvClient) cmd(args ...interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, _ := json.Marshal(map[string]interface{}{"command": args})
	_, err := c.conn.Write(append(data, '\n'))
	return err
}

func (c *MpvClient) get(prop string) (json.RawMessage, error) {
	c.pendMu.Lock()
	id := c.nextID + 1000
	c.nextID++
	ch := make(chan json.RawMessage, 1)
	c.pending[id] = ch
	c.pendMu.Unlock()

	c.mu.Lock()
	data, _ := json.Marshal(map[string]interface{}{"command": []interface{}{"get_property", prop}, "request_id": id})
	_, err := c.conn.Write(append(data, '\n'))
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case v := <-ch:
		return v, nil
	case <-time.After(2 * time.Second):
		return nil, fmt.Errorf("mpv get_property timeout: %s", prop)
	}
}

func (c *MpvClient) getFloat(prop string) float64 {
	raw, err := c.get(prop)
	if err != nil {
		return 0
	}
	var f float64
	json.Unmarshal(raw, &f)
	return f
}

func (c *MpvClient) getString(prop string) string {
	raw, err := c.get(prop)
	if err != nil {
		return ""
	}
	var s string
	json.Unmarshal(raw, &s)
	return s
}

// ---------- 全局状态 ----------

var (
	mpv      *MpvClient
	ws       *websocket.Conn
	wsMu     sync.Mutex
	myName   string
	roomCode string
	filePath string

	suppressMu    sync.Mutex
	suppressUntil time.Time

	// 收到 hello 前忽略本地事件（避免 mpv 初始 --pause 触发误广播）
	helloOnce sync.Once
	helloRecv atomic.Bool

	// 服务器最近一次告知的房间状态（playing/paused）。
	// 本地播放器事件若与之一致，说明是"应用远端状态"引起的回声或 mpv 启动时的
	// 延迟事件，不应再上报——否则加入一个正在播放的房间会把全房间拉回 0 秒。
	roomState atomic.Value

	// 本地是否处于"用户暂停"状态，用于 memberState
	lastCachePause bool
)

func suppress(ms int) {
	suppressMu.Lock()
	suppressUntil = time.Now().Add(time.Duration(ms) * time.Millisecond)
	suppressMu.Unlock()
}

func suppressed() bool {
	suppressMu.Lock()
	defer suppressMu.Unlock()
	return time.Now().Before(suppressUntil)
}

func sendMsg(m Msg) {
	wsMu.Lock()
	defer wsMu.Unlock()
	if ws == nil {
		return
	}
	data, _ := json.Marshal(m)
	ws.WriteMessage(websocket.TextMessage, data)
}

// ---------- mpv 事件 → 服务器 ----------

func handlePropertyChange(id int, name string, data json.RawMessage) {
	switch name {
	case "pause":
		var paused bool
		json.Unmarshal(data, &paused)
		if suppressed() || !helloRecv.Load() {
			return
		}
		// 与房间当前状态一致 → 是回声或播放器启动期的延迟事件，不上报
		if cur, _ := roomState.Load().(string); (paused && cur == "paused") || (!paused && cur == "playing") {
			return
		}
		pos := mpv.getFloat("time-pos")
		if paused {
			roomState.Store("paused")
			log.Printf("%s 你暂停了 @%.1fs", cyan("[本地]"), pos)
			sendMsg(Msg{Type: "pause", Position: pos})
		} else {
			roomState.Store("playing")
			log.Printf("%s 你播放了 @%.1fs", cyan("[本地]"), pos)
			rate := mpv.getFloat("speed")
			sendMsg(Msg{Type: "play", Position: pos, Rate: rate})
		}
	case "paused-for-cache":
		var p bool
		json.Unmarshal(data, &p)
		if p == lastCachePause {
			return
		}
		lastCachePause = p
		if p {
			sendMsg(Msg{Type: "memberState", State: "buffering"})
		} else if !p {
			paused := mpv.getFloat("pause") > 0.5
			if !paused {
				sendMsg(Msg{Type: "memberState", State: "playing"})
			}
		}
	}
}

func handleMpvSeekEvent() {
	if suppressed() || !helloRecv.Load() {
		return
	}
	pos := mpv.getFloat("time-pos")
	log.Printf("%s 你跳转到了 %.1fs", cyan("[本地]"), pos)
	sendMsg(Msg{Type: "seek", Position: pos})
}

// ---------- 服务器消息 → mpv ----------

func applyRemoteState(state string, pos, rate float64, reason string) {
	cur := mpv.getFloat("time-pos")
	needSeek := abs(cur-pos) > SOFT_DRIFT
	roomState.Store(state)
	suppress(800)
	if needSeek {
		mpv.cmd("seek", pos, "absolute+exact")
	}
	if state == "playing" {
		mpv.cmd("set_property", "pause", false)
		if rate > 0 && rate != mpv.getFloat("speed") {
			mpv.cmd("set_property", "speed", rate)
		}
	} else {
		mpv.cmd("set_property", "pause", true)
	}
	log.Printf("%s %s → %s @%.1fs", magenta("[同步]"), reason, stateLabel(state), pos)
}

func stateLabel(s string) string {
	if s == "playing" {
		return "播放"
	}
	return "暂停"
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// ---------- WebSocket 读循环 ----------

func wsReadLoop() {
	for {
		wsMu.Lock()
		c := ws
		wsMu.Unlock()
		if c == nil {
			return
		}
		_, data, err := c.ReadMessage()
		if err != nil {
			log.Printf("%s 与服务器连接断开: %v，10 秒后重连…", yellow("[断线]"), err)
			reconnect()
			continue
		}
		var m Msg
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		switch m.Type {
		case "hello":
			helloOnce.Do(func() { helloRecv.Store(true) })
			requested := roomCode // 覆盖前先留住用户启动时填的码
			roomCode = m.Room
			printRoomBanner(m.Room, m.Created, requested)
			log.Printf("%s 成员: %s，状态: %s @%.1fs",
				green("[房间]"), namesOf(m.Members), stateLabel(m.State), m.Position)
			applyRemoteState(m.State, m.Position, m.Rate, "进入房间同步")
		case "sync":
			if m.Operator == myName {
				continue
			}
			applyRemoteState(m.State, m.Position, m.Rate, m.Operator+"操作")
		case "forceSeek":
			applyRemoteState(playStateOf(), m.Position, 0, "服务器纠偏")
		case "members":
			log.Printf("%s %s", cyan("[成员]"), namesOf(m.Members))
		case "join":
			log.Printf("%s %s 加入了房间", green("[成员]"), m.Name)
		case "leave":
			log.Printf("%s %s 离开了房间", dim("[成员]"), m.Name)
		case "joinError", "error":
			log.Printf("%s %s", red("[错误]"), red(m.Error))
			os.Exit(1)
		}
	}
}

func namesOf(ms []MemberInfo) string {
	var b strings.Builder
	for i, m := range ms {
		if i > 0 {
			b.WriteString("、")
		}
		fmt.Fprintf(&b, "%s(%s)", m.Name, m.State)
	}
	return b.String()
}

func playStateOf() string {
	if mpv.getFloat("pause") > 0.5 {
		return "paused"
	}
	return "playing"
}

// ---------- 断线重连 ----------

func reconnect() {
	for i := 0; i < 30; i++ {
		time.Sleep(10 * time.Second)
		log.Printf("%s 第 %d 次尝试…", yellow("[重连]"), i+1)
		c, _, err := websocket.DefaultDialer.Dial(serverURL(), nil)
		if err != nil {
			continue
		}
		wsMu.Lock()
		ws = c
		wsMu.Unlock()
		joinRoom(c)
		log.Printf("%s 成功", green("[重连]"))
		return
	}
	log.Println(red("[重连] 多次失败，退出"))
	os.Exit(1)
}

// ---------- 配置 ----------

var (
	serverAddr = flag.String("server", defaultServerAddr(), "服务器地址 host:port（默认读取 server.txt / 环境变量 SYNCWATCH_SERVER）")
	nameFlag   = flag.String("name", "", "你的昵称")
	roomFlag   = flag.String("room", "", "房间码（留空则由服务器创建新房间）")
	passFlag   = flag.String("password", "", "房间密码")
	fileFlag   = flag.String("file", "", "本地视频文件路径")
	mpvFlag    = flag.String("mpv", "", "mpv.exe 路径（默认自动寻找）")
)

// builtinServer 是编译期可注入的内置默认地址：
//
//	go build -ldflags "-X main.builtinServer=1.2.3.4:8995"
//
// build.bat 会自动读取 server.txt 注入，因此分发给别人的 exe 即使没有
// server.txt 也能直连你的服务器。
var builtinServer = "127.0.0.1:8995"

// defaultServerAddr 决定默认服务器地址，优先级：
//  1. 环境变量 SYNCWATCH_SERVER
//  2. 程序同目录的 server.txt（一行 host:port）
//  3. 编译期内置地址（默认是自己服务器；未注入时为回环地址）
//
// 这样换服务器不用改代码重新编译，改 server.txt 即可（该文件不纳入 git）。
func defaultServerAddr() string {
	if v := strings.TrimSpace(os.Getenv("SYNCWATCH_SERVER")); v != "" {
		return v
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "server.txt")
		if b, err := os.ReadFile(p); err == nil {
			if v := strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0]); v != "" {
				return v
			}
		}
	}
	return builtinServer
}

func serverURL() string {
	return "ws://" + *serverAddr + "/ws"
}

func joinRoom(c *websocket.Conn) {
	msg := Msg{Type: "join", Name: myName, Password: *passFlag}
	if *roomFlag != "" {
		msg.Room = *roomFlag
	}
	data, _ := json.Marshal(msg)
	c.WriteMessage(websocket.TextMessage, data)
	// 重连时上报文件信息
	sendFileInfo()
}

func sendFileInfo() {
	go func() {
		time.Sleep(500 * time.Millisecond)
		dur := mpv.getFloat("duration")
		size := int64(0)
		if st, err := os.Stat(filePath); err == nil {
			size = st.Size()
		}
		sendMsg(Msg{Type: "fileInfo", File: filepath.Base(filePath), Duration: dur, Size: size})
	}()
}

func findMpv() string {
	if *mpvFlag != "" {
		return *mpvFlag
	}
	if p, err := exec.LookPath("mpv"); err == nil {
		return p
	}
	exeDir, _ := os.Executable()
	dir := filepath.Dir(exeDir)
	for _, cand := range []string{
		filepath.Join(dir, "mpv.exe"),
		filepath.Join(dir, "mpv", "mpv.exe"),
	} {
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return "mpv"
}

// copyToClipboard 把房间码放进 Windows 剪贴板，方便直接粘给朋友。
// 通过环境变量传值，避免房间码里的特殊字符被 PowerShell 误解析。
// 失败不影响使用（比如极精简系统没有 powershell）。
func copyToClipboard(s string) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "Set-Clipboard -Value $env:SYNCWATCH_CLIP")
	cmd.Env = append(os.Environ(), "SYNCWATCH_CLIP="+s)
	if err := cmd.Run(); err == nil {
		fmt.Println("  （房间码已复制到剪贴板，可直接 Ctrl+V 发给朋友）")
	}
}

// stdinReader 读整行。原来用 fmt.Scanln 会在空格处截断，
// 而拖进来的视频路径经常带空格（D:\我的 电影\a.mkv），会直接读错文件。
var stdinReader = bufio.NewReader(os.Stdin)

func readLine(def string) string {
	line, err := stdinReader.ReadString('\n')
	if err != nil && line == "" {
		return def
	}
	s := strings.Trim(strings.TrimSpace(line), `"'`)
	if s == "" {
		return def
	}
	return s
}

func main() {
	flag.Parse()
	enableVT()
	log.SetFlags(log.Ltime)
	roomState.Store("paused")

	printHeader()

	myName = *nameFlag
	if myName == "" {
		printStep(1, 4, "你的昵称", "")
		myName = readLine("")
	}
	if myName == "" {
		myName = "观众"
	}
	filePath = *fileFlag
	if filePath == "" {
		printStep(2, 4, "视频文件完整路径", "把文件直接拖进这个窗口即可")
		filePath = readLine("")
	}
	if filePath == "" {
		log.Fatal(red("未指定视频文件"))
	}
	filePath = strings.Trim(filePath, `"' `)
	isLavfi := strings.HasPrefix(filePath, "av://")
	if !isLavfi {
		if _, err := os.Stat(filePath); err != nil {
			log.Fatalf("找不到文件: %s", filePath)
		}
	}
	roomCode = *roomFlag
	if roomCode == "" {
		printStep(3, 4, "房间码", "有朋友的房间码就填进去；直接回车 = 你自己创建一个新房间")
		roomCode = readLine("")
	}
	roomCode = strings.ToUpper(strings.TrimSpace(roomCode))
	if *passFlag == "" {
		printStep(4, 4, "房间密码", "房间没设密码就直接回车")
		*passFlag = readLine("")
	}
	fmt.Println()

	// 1. 启动 mpv（暂停待命，等 hello 同步）
	mpvPath := findMpv()
	log.Printf("启动 mpv: %s", mpvPath)
	mpvCmd := exec.Command(mpvPath,
		"--input-ipc-server="+PIPE_NAME,
		"--force-window=yes",
		"--title=一起看-"+filepath.Base(filePath),
		"--pause",
		filePath)
	mpvCmd.Stdout = nil
	mpvCmd.Stderr = nil
	if err := mpvCmd.Start(); err != nil {
		log.Fatalf("无法启动 mpv（%s）: %v。请安装 mpv 或用 -mpv 指定路径。", mpvPath, err)
	}

	// 2. 等待命名管道就绪
	var merr error
	for i := 0; i < 30; i++ {
		time.Sleep(300 * time.Millisecond)
		mpv, merr = dialMpv(PIPE_NAME)
		if merr == nil {
			break
		}
	}
	if merr != nil {
		log.Fatalf("连接 mpv IPC 失败: %v", merr)
	}
	mpv.cmd("observe_property", 1, "pause")
	mpv.cmd("observe_property", 2, "paused-for-cache")
	log.Println(green("[mpv]") + " 已接管播放器")

	// 3. 连接服务器
	log.Printf("%s %s", dim("[网络]"), serverURL())
	c, _, err := websocket.DefaultDialer.Dial(serverURL(), nil)
	if err != nil {
		log.Fatalf("连接服务器失败: %v", err)
	}
	wsMu.Lock()
	ws = c
	wsMu.Unlock()
	joinRoom(c)
	sendFileInfo()

	go wsReadLoop()

	// 4. 心跳
	go func() {
		for {
			time.Sleep(HEARTBEAT_EVERY)
			pos := mpv.getFloat("time-pos")
			rate := mpv.getFloat("speed")
			paused := mpv.getFloat("pause") > 0.5
			sendMsg(Msg{Type: "heartbeat", Position: pos, Rate: rate})
			if !paused && !lastCachePause {
				sendMsg(Msg{Type: "memberState", State: "playing"})
			}
		}
	}()

	// 5. Ctrl+C 优雅退出
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	printQuit()
	sendMsg(Msg{Type: "leave"})
	mpv.cmd("quit")
	time.Sleep(300 * time.Millisecond)
	os.Exit(0)
}
