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
//   - 位置永远取自服务器锚点推算，不信任任何"取不到就返回 0"的默认值。

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
	Kind string      // joined / state / members / notice / error / closed
	Data interface{} // 具体载荷，前端按 Kind 解读
}

type SyncClient struct {
	mu     sync.Mutex
	conn   *websocket.Conn
	server string
	room   string
	name   string
	pass   string

	// 服务器最近一次告知的权威状态
	roomState  string
	anchorPos  float64
	anchorMs   int64
	rate       float64
	members    []MemberInfo
	lastFile   string
	lastDur    float64
	helloDone  bool
	closed     bool
	retryCount int

	onEvent     func(SyncEvent)
	posProvider func() (float64, bool)
	stopHB      chan struct{}
	stopAll     chan struct{}
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
func (c *SyncClient) Connect(server, room, name, password string) error {
	c.mu.Lock()
	c.server, c.name, c.pass = server, name, password
	if room != "" {
		c.room = room
	}
	c.closed = false
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

	join := Msg{Type: "join", Name: name, Password: password}
	if room != "" {
		join.Room = room
	}
	if err := c.send(join); err != nil {
		conn.Close()
		return err
	}

	go c.readLoop()
	go c.heartbeatLoop()
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
	return conn.WriteMessage(websocket.TextMessage, data)
}

func (c *SyncClient) readLoop() {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return
	}

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			alreadyClosed := c.closed
			c.mu.Unlock()
			if !alreadyClosed {
				c.emit("closed", err.Error())
			}
			return
		}
		var m Msg
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		c.handle(m)
	}
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
		c.mu.Unlock()

		// 重连后要重新上报片源信息，否则成员列表会丢时长
		if !first && c.lastFile != "" {
			c.send(Msg{Type: "fileInfo", File: c.lastFile, Duration: c.lastDur})
		}
		c.emit("joined", map[string]interface{}{
			"room":     m.Room,
			"created":  m.Created,
			"state":    m.State,
			"position": c.targetPos(m.State, m.Position, m.AnchorMs, c.rate),
			"members":  m.Members,
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
		c.closed = true
		c.mu.Unlock()
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
	c.send(Msg{Type: "memberState", State: "playing"})
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
	c.send(Msg{Type: "memberState", State: "paused"})
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

func (c *SyncClient) SetFileInfo(name string, size int64, dur float64) {
	c.mu.Lock()
	c.lastFile, c.lastDur = name, dur
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

	select {
	case <-c.stopHB:
	default:
		close(c.stopHB)
	}
	if conn != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"leave"}`))
		conn.Close()
	}
}
