//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 本文件是 WebSocket 同步客户端，协议与 windows/main.go、Android 端完全一致。
// 服务端不需要任何改动。
//
// 关键设计（都是 v0.3 / v0.4 踩过坑之后定下来的）：
//   - 只在收到 hello 之后才允许上报本地操作，否则播放器启动期的初始事件会污染房间；
//   - 本地状态与服务器最近告知的状态一致时不上报，避免 A→B→A 回声；
//   - 位置永远取自服务器锚点推算，不信任任何"取不到就返回 0"的默认值；
//   - 写操作必须串行（v0.5.1）：gorilla/websocket 对并发写是直接 panic 的，
//     心跳线程与用户操作同时发包会整个进程崩掉；
//   - 断线要自己接回来（v0.5.1）：没有重连时，一次抖动就变成"两个人都还在看，
//     但谁也收不到谁"，这正是用户报的"互相看不到对方"的另一半成因。

type MemberInfo struct {
	Name     string  `json:"name"`
	State    string  `json:"state"`
	File     string  `json:"file,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Online   bool    `json:"online"`
}

type Msg struct {
	Type     string       `json:"type"`
	Room     string       `json:"room,omitempty"`
	Name     string       `json:"name,omitempty"`
	Password string       `json:"password,omitempty"`
	Position float64      `json:"position,omitempty"`
	Rate     float64      `json:"rate,omitempty"`
	State    string       `json:"state,omitempty"`
	Operator string       `json:"operator,omitempty"`
	AnchorMs int64        `json:"anchorMs,omitempty"`
	Members  []MemberInfo `json:"members,omitempty"`
	Error    string       `json:"error,omitempty"`
	File     string       `json:"file,omitempty"`
	Size     int64        `json:"size,omitempty"`
	Duration float64      `json:"duration,omitempty"`
	Created  bool         `json:"created,omitempty"`
}

// SyncEvent 是同步层推给界面的结构化事件。
type SyncEvent struct {
	Kind string      // joined / state / members / notice / error / closed / reconnecting
	Data interface{} // 具体载荷，前端按 Kind 解读
}

type SyncClient struct {
	mu      sync.Mutex
	writeMu sync.Mutex // gorilla 不允许并发写，所有 WriteMessage 都过这把锁
	conn    *websocket.Conn
	server  string
	room    string
	name    string
	pass    string

	// 服务器最近一次告知的权威状态
	roomState    string
	anchorPos    float64
	anchorMs     int64
	rate         float64
	members      []MemberInfo
	lastFile     string
	lastDur      float64
	lastSize     int64
	helloDone    bool
	closed       bool
	reconnecting bool
	retryCount   int

	// 上一次上报过的成员状态，避免每 5 秒刷一遍
	lastMemberState string

	onEvent     func(SyncEvent)
	posProvider func() (float64, bool)
	stopHB      chan struct{}
	stopAll     chan struct{}
	closeOnce   sync.Once
}

func newSyncClient(onEvent func(SyncEvent), posProvider func() (float64, bool)) *SyncClient {
	return &SyncClient{
		onEvent:     onEvent,
		posProvider: posProvider,
		stopHB:      make(chan struct{}),
		stopAll:     make(chan struct{}),
	}
}

func (c *SyncClient) emit(kind string, data interface{}) {
	if c.onEvent != nil {
		c.onEvent(SyncEvent{Kind: kind, Data: data})
	}
}

// Connect 建立连接并发送 join。room 为空表示创建新房间。
// 首次连不上直接返回错误给界面（用户需要马上知道地址填错了），
// 连上之后的断线由 readLoop 里的 reconnect 负责。
func (c *SyncClient) Connect(server, room, name, password string) error {
	c.mu.Lock()
	c.server, c.name, c.pass = normalizeServer(server), name, password
	if room != "" {
		c.room = room
	}
	c.closed = false
	c.helloDone = false
	c.lastMemberState = ""
	c.mu.Unlock()

	if err := c.dialAndJoin(); err != nil {
		return err
	}
	go c.readLoop()
	go c.heartbeatLoop()
	return nil
}

// dialAndJoin 建一条新连接并发出 join（首次连接与重连共用）。
func (c *SyncClient) dialAndJoin() error {
	c.mu.Lock()
	server, room, name, pass := c.server, c.room, c.name, c.pass
	c.mu.Unlock()

	url := fmt.Sprintf("ws://%s/ws", server)
	dialer := websocket.Dialer{HandshakeTimeout: 8 * time.Second}
	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("无法连接服务器 %s：%w", server, err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	join := Msg{Type: "join", Name: name, Password: pass}
	if room != "" {
		join.Room = room
	}
	if err := c.send(join); err != nil {
		conn.Close()
		return err
	}
	logf("join 已发送 server=%s room=%q name=%s", server, room, name)
	return nil
}

func (c *SyncClient) send(m Msg) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("未连接")
	}
	data, _ := json.Marshal(m)

	// gorilla/websocket 检测到并发写会 panic，心跳与用户操作必须排队
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, data)
}

func (c *SyncClient) readLoop() {
	for {
		c.mu.Lock()
		conn := c.conn
		closed := c.closed
		c.mu.Unlock()
		if conn == nil || closed {
			return
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			closed = c.closed
			c.mu.Unlock()
			if closed {
				return
			}
			logf("与服务器断开: %v", err)
			// 先通知界面"正在重连"，再进入重连循环；重连成功就继续读这条新连接
			c.emit("closed", err.Error())
			if c.reconnect() {
				continue
			}
			c.emit("error", "与服务器的连接已断开，重连失败")
			return
		}

		var m Msg
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		c.handle(m)
	}
}

// reconnect 指数退避重连（1s→10s，最多 20 次，约两分半）。
//
// 房间在服务端是保留 15 分钟的，所以断线回来还是同一个房间、同一个进度；
// 用的房间码必须是 c.room（服务器在 hello 里回填的），绝不能拿用户输入框的值，
// 否则"重连＝新建一个空房间"，v0.3 踩过。
func (c *SyncClient) reconnect() bool {
	backoff := time.Second
	for i := 0; i < 20; i++ {
		select {
		case <-c.stopAll:
			return false
		case <-time.After(backoff):
		}
		if c.isClosed() {
			return false
		}
		c.mu.Lock()
		c.reconnecting = true
		c.retryCount = i + 1
		c.mu.Unlock()
		c.emit("reconnecting", i+1)

		if err := c.dialAndJoin(); err != nil {
			logf("重连第 %d 次失败: %v", i+1, err)
			if backoff < 10*time.Second {
				backoff *= 2
			}
			continue
		}
		logf("重连成功（第 %d 次）", i+1)
		c.mu.Lock()
		c.reconnecting = false
		c.mu.Unlock()
		return true
	}
	return false
}

func (c *SyncClient) handle(m Msg) {
	switch m.Type {
	case "hello":
		c.mu.Lock()
		first := !c.helloDone
		c.helloDone = true
		c.room = m.Room
		c.roomState = m.State
		c.anchorPos = m.Position
		c.anchorMs = m.AnchorMs
		c.rate = m.Rate
		if c.rate == 0 {
			c.rate = 1
		}
		c.members = m.Members
		c.retryCount = 0
		c.reconnecting = false
		rate := c.rate
		file, dur, size := c.lastFile, c.lastDur, c.lastSize
		c.mu.Unlock()

		// 重连后要重新上报片源信息，否则成员列表会丢时长
		if !first && file != "" {
			c.send(Msg{Type: "fileInfo", File: file, Size: size, Duration: dur})
		}
		c.emit("joined", map[string]interface{}{
			"room":     m.Room,
			"created":  m.Created,
			"state":    m.State,
			"position": c.targetPos(m.State, m.Position, m.AnchorMs, rate),
			"members":  m.Members,
			"again":    !first, // 重连回来的 hello，界面不要再弹"已加入房间"
		})

	case "sync":
		logf("recv sync state=%s pos=%.2f op=%s", m.State, m.Position, m.Operator)
		c.mu.Lock()
		c.roomState = m.State
		c.anchorPos = m.Position
		c.anchorMs = m.AnchorMs
		if m.Rate > 0 {
			c.rate = m.Rate
		}
		pos := c.targetPos(m.State, m.Position, m.AnchorMs, c.rate)
		op := m.Operator
		state := m.State
		c.mu.Unlock()

		c.emit("state", map[string]interface{}{
			"state":    state,
			"position": pos,
			"operator": op,
			"reason":   "sync",
		})

	case "forceSeek":
		logf("recv forceSeek pos=%.2f op=%s", m.Position, m.Operator)
		c.mu.Lock()
		c.anchorPos = m.Position
		c.anchorMs = time.Now().UnixMilli()
		state := c.roomState
		c.mu.Unlock()
		c.emit("state", map[string]interface{}{
			"state":    state,
			"position": m.Position,
			"operator": m.Operator,
			"reason":   "forceSeek",
		})

	case "members":
		c.mu.Lock()
		c.members = m.Members
		c.mu.Unlock()
		c.emit("members", m.Members)

	case "join":
		c.emit("notice", m.Name+" 加入了房间")

	case "leave":
		c.emit("notice", m.Name+" 离开了房间")

	case "joinError":
		c.mu.Lock()
		retrying := c.reconnecting
		if !retrying {
			c.closed = true
		}
		c.mu.Unlock()
		// 重连时最常撞到的是"昵称被自己那条还没超时的旧连接占着"，
		// 服务端 60 秒读超时后会自己清掉，等下一轮再试即可。
		if retrying {
			logf("重连被拒绝: %s（稍后重试）", m.Error)
			return
		}
		c.emit("error", m.Error)

	case "error":
		c.emit("error", m.Error)
	}
}

// targetPos 用服务器锚点推算当前播放位置。
func (c *SyncClient) targetPos(state string, pos float64, anchorMs int64, rate float64) float64 {
	if state != "playing" {
		return pos
	}
	if anchorMs <= 0 {
		return pos
	}
	delta := float64(time.Now().UnixMilli()-anchorMs) / 1000 * rate
	if delta < 0 {
		delta = 0
	}
	return pos + delta
}

// RoomState 返回服务器最近告知的播放状态。
func (c *SyncClient) RoomState() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.roomState
}

func (c *SyncClient) Room() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.room
}

func (c *SyncClient) Members() []MemberInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.members
}

func (c *SyncClient) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// ---------- 本地操作上报 ----------

// ReportPlay 上报"我开始播放了"。位置与状态都与房间一致时不重复发送。
func (c *SyncClient) ReportPlay(pos, rate float64) {
	c.mu.Lock()
	if !c.helloDone || c.roomState == "playing" {
		c.mu.Unlock()
		return
	}
	c.roomState = "playing"
	c.anchorPos, c.anchorMs, c.rate = pos, time.Now().UnixMilli(), rate
	c.mu.Unlock()
	logf("send play pos=%.2f rate=%.2f", pos, rate)
	c.send(Msg{Type: "play", Position: pos, Rate: rate})
	c.ReportMemberState("playing")
}

func (c *SyncClient) ReportPause(pos float64) {
	c.mu.Lock()
	if !c.helloDone || c.roomState == "paused" {
		c.mu.Unlock()
		return
	}
	c.roomState = "paused"
	c.anchorPos, c.anchorMs = pos, time.Now().UnixMilli()
	c.mu.Unlock()
	logf("send pause pos=%.2f", pos)
	c.send(Msg{Type: "pause", Position: pos})
	c.ReportMemberState("paused")
}

func (c *SyncClient) ReportSeek(pos float64) {
	c.mu.Lock()
	if !c.helloDone {
		c.mu.Unlock()
		return
	}
	c.anchorPos, c.anchorMs = pos, time.Now().UnixMilli()
	c.mu.Unlock()
	logf("send seek pos=%.2f", pos)
	c.send(Msg{Type: "seek", Position: pos})
}

// ReportMemberState 上报自己的播放器状态（有变化才发）。
// 不这么做的话，成员面板里的"谁在播放"要等到下一次心跳才更新。
func (c *SyncClient) ReportMemberState(state string) {
	switch state {
	case "playing", "paused", "buffering":
	default:
		return
	}
	c.mu.Lock()
	if !c.helloDone || c.lastMemberState == state {
		c.mu.Unlock()
		return
	}
	c.lastMemberState = state
	c.mu.Unlock()
	c.send(Msg{Type: "memberState", State: state})
}

func (c *SyncClient) SetFileInfo(name string, size int64, dur float64) {
	c.mu.Lock()
	c.lastFile, c.lastDur, c.lastSize = name, dur, size
	c.mu.Unlock()
	c.send(Msg{Type: "fileInfo", File: name, Size: size, Duration: dur})
}

func (c *SyncClient) SetRate(rate float64) {
	c.mu.Lock()
	c.rate = rate
	pos := c.anchorPos
	state := c.roomState
	c.mu.Unlock()
	if state == "playing" {
		c.send(Msg{Type: "play", Position: pos, Rate: rate})
	}
	// 暂停时调速，服务器不会收到任何消息（play 会顺带把房间改成 playing），
	// 所以只把倍速记在本地，等真正播放时随 play 一起带上。
}

// ---------- 心跳 / 断线重连 ----------

func (c *SyncClient) heartbeatLoop() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.stopHB:
			return
		case <-t.C:
			c.mu.Lock()
			done := c.helloDone
			c.mu.Unlock()
			if !done {
				continue
			}
			var pos float64
			var ok bool
			if c.posProvider != nil {
				pos, ok = c.posProvider()
			}
			if !ok {
				continue // 取不到位置就不发，绝不用 0 冒充
			}
			c.send(Msg{Type: "heartbeat", Position: pos, Rate: 1.0})
		}
	}
}

// Close 主动断开（离开房间）。
func (c *SyncClient) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()

	c.closeOnce.Do(func() { close(c.stopAll) })
	select {
	case <-c.stopHB:
	default:
		close(c.stopHB)
	}
	if conn != nil {
		c.writeMu.Lock()
		conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"leave"}`))
		c.writeMu.Unlock()
		conn.Close()
	}
}
