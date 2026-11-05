// syncwatch-server: 「一起看」同步观影信令服务器
//
// 只负责房间管理与播放指令转发，不接触任何视频数据。
// 协议文档：docs/02-同步协议.md
// 部署说明：docs/03-服务端部署.md
package main

import (
	"encoding/json"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	MAX_MEMBERS   = 5 // 房间人数上限
	SEEK_DEBOUNCE = 120 * time.Millisecond
	DRIFT_SOFT    = 0.3 // 秒：忽略
	DRIFT_HARD    = 1.0 // 秒：强制纠偏
	// 空房保留时长。原来最后一个成员一离开就删房，断线重连必然回到 0 秒。
	EMPTY_ROOM_TTL = 15 * time.Minute
	HEARTBEAT_LATE = 30 * time.Second // 心跳超时踢出
	PING_PERIOD    = 20 * time.Second
	PONG_TIMEOUT   = 60 * time.Second
	RATE_LIMIT     = 40            // 每秒每连接最大消息数(粗略)
	ROOM_IDLE_TTL  = 2 * time.Hour // 空房间保留时长
)

// ---------- 消息 ----------

type Msg struct {
	Type     string       `json:"type"`
	Room     string       `json:"room,omitempty"`
	Password string       `json:"password,omitempty"`
	Name     string       `json:"name,omitempty"`
	Position float64      `json:"position,omitempty"`
	Rate     float64      `json:"rate,omitempty"`
	State    string       `json:"state,omitempty"` // playing/paused/buffering (memberState)
	Operator string       `json:"operator,omitempty"`
	AnchorMs int64        `json:"anchorMs,omitempty"`
	Error    string       `json:"error,omitempty"`
	File     string       `json:"file,omitempty"`
	Size     int64        `json:"size,omitempty"`
	Duration float64      `json:"duration,omitempty"`
	Members  []MemberInfo `json:"members,omitempty"`
	Reason   string       `json:"reason,omitempty"`
	// Created 仅在 hello 中出现：true 表示本次 join 新建了房间，
	// false 表示加入了一个已存在的房间。客户端用它提示用户
	// “你输入的房间码不存在，已为你新建房间”，避免误以为进了朋友的房间。
	Created bool `json:"created,omitempty"`
}

type MemberInfo struct {
	Name     string  `json:"name"`
	State    string  `json:"state"` // playing/paused/buffering
	File     string  `json:"file,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Online   bool    `json:"online"`
}

// ---------- 房间与成员 ----------

type Member struct {
	conn     *websocket.Conn
	send     chan []byte
	done     chan struct{}
	name     string
	room     *Room
	file     string
	duration float64
	state    string // playing/paused/buffering
	lastHB   time.Time
	lastPos  float64
	rate     float64 // 最近一次心跳时的本地倍速
	quota    int     // 简单限流
	quotaAt  time.Time
	closed   sync.Once
	// 上一次心跳是否已经超阈值。连续两次才纠偏，单次抖动不折腾用户。
	driftWarned bool
}

type Room struct {
	mu        sync.Mutex
	code      string
	password  string
	state     string  // playing / paused
	position  float64 // anchor 时刻的进度(秒)
	anchor    time.Time
	rate      float64
	members   map[*Member]bool
	lastSeek  time.Time
	seekTimer *time.Timer
	created   time.Time
	// 房间变空的时刻。空房不立即销毁——手机切后台、WiFi 抖动都会短暂断开，
	// 立刻删房会让重连的客户端拿到一个全新房间（进度 0），表现为"跳回片头"。
	emptySince time.Time
}

var (
	hubMu sync.Mutex
	rooms = map[string]*Room{}
)

// sanitizePos 过滤明显异常的位置：NaN / Inf / 负数 / 超过 24 小时。
// 这是"跳回片头"的最后一道防线——即使某个客户端报错，也不会污染房间状态。
func sanitizePos(p float64) (float64, bool) {
	if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 24*3600 {
		return 0, false
	}
	return p, true
}

// 当前权威进度
func (r *Room) currentPos() float64 {
	if r.state == "playing" {
		return r.position + time.Since(r.anchor).Seconds()*r.rate
	}
	return r.position
}

func (r *Room) snapshot() (state string, pos float64, anchorMs int64, rate float64) {
	pos = r.currentPos()
	if r.state == "playing" {
		// 以当前时刻为新锚点返回，客户端计算简单
		r.position, r.anchor = pos, time.Now()
	}
	return r.state, r.position, r.anchor.UnixMilli(), r.rate
}

func (r *Room) memberInfos() []MemberInfo {
	infos := make([]MemberInfo, 0, len(r.members))
	for m := range r.members {
		infos = append(infos, MemberInfo{Name: m.name, State: m.state, File: m.file, Duration: m.duration, Online: true})
	}
	return infos
}

func (r *Room) broadcast(m *Member, msg Msg) {
	data, _ := json.Marshal(msg)
	for mem := range r.members {
		if mem == m && msg.Type == "sync" {
			continue // 发起者本地已执行，不需要回显（hello 除外）
		}
		select {
		case mem.send <- data:
		default: // 发送缓冲满，踢掉
			go mem.close("send buffer full")
		}
	}
}

func (m *Member) close(reason string) {
	m.closed.Do(func() {
		if m.room != nil {
			m.room.mu.Lock()
			delete(m.room.members, m)
			room := m.room
			m.room.mu.Unlock()
			room.broadcast(nil, Msg{Type: "leave", Name: m.name})
			room.broadcast(nil, Msg{Type: "members", Members: room.memberInfos()})
			cleanupRoom(room)
		}
		close(m.done)
		m.conn.Close()
	})
}

// cleanupRoom 只把房间标记为"空"，不销毁。
//
// v0.3 之前是最后一个成员一离开就 delete(rooms, code)，于是任何一次短暂断线
// （手机切后台、WiFi 抖动、心跳超时）都会让重连的客户端拿到一个全新房间：
// 房间码失效、进度归零，用户看到的就是"时不时跳回片头"。
// 现在交给 reaper 在 EMPTY_ROOM_TTL 后回收，房间状态可以跨断线保留。
func cleanupRoom(r *Room) {
	r.mu.Lock()
	if len(r.members) == 0 {
		r.emptySince = time.Now()
	} else {
		r.emptySince = time.Time{}
	}
	r.mu.Unlock()
}

// ---------- 房间操作 ----------

// getOrCreateRoom 返回房间以及“本次是否新建”。
// 注意：传入一个不存在但非空的 code 也会新建房间（兼容旧客户端），
// 新建标记通过 hello.created 告知客户端。
func getOrCreateRoom(code, password string) (*Room, bool) {
	hubMu.Lock()
	defer hubMu.Unlock()
	r, ok := rooms[code]
	created := false
	if !ok {
		r = &Room{
			code:     code,
			password: password,
			state:    "paused",
			rate:     1.0,
			members:  map[*Member]bool{},
			created:  time.Now(),
		}
		rooms[code] = r
		created = true
	}
	return r, created
}

func randomCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

// ---------- WebSocket 主逻辑 ----------

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // 客户端为原生应用，无 Origin 限制
}

func serveWS(w http.ResponseWriter, req *http.Request) {
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	m := &Member{conn: conn, send: make(chan []byte, 64), done: make(chan struct{}), state: "paused", lastHB: time.Now()}
	go m.writeLoop()
	m.readLoop()
}

func (m *Member) writeLoop() {
	ping := time.NewTicker(PING_PERIOD)
	defer ping.Stop()
	for {
		select {
		case <-m.done:
			return
		case data, ok := <-m.send:
			if !ok {
				return
			}
			m.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := m.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				m.close("write error")
				return
			}
		case <-ping.C:
			m.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := m.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				m.close("ping error")
				return
			}
		}
	}
}

func (m *Member) sendErr(typ, text string) {
	data, _ := json.Marshal(Msg{Type: typ, Error: text})
	select {
	case m.send <- data:
	default:
	}
}

func (m *Member) readLoop() {
	defer m.close("read end")
	m.conn.SetReadLimit(8 * 1024)
	m.conn.SetReadDeadline(time.Now().Add(PONG_TIMEOUT))
	m.conn.SetPongHandler(func(string) error {
		m.conn.SetReadDeadline(time.Now().Add(PONG_TIMEOUT))
		return nil
	})

	var joined bool
	for {
		_, data, err := m.conn.ReadMessage()
		if err != nil {
			return
		}
		// 限流
		now := time.Now()
		if now.Sub(m.quotaAt) > time.Second {
			m.quotaAt, m.quota = now, RATE_LIMIT
		}
		m.quota--
		if m.quota < 0 {
			m.sendErr("error", "rate limit exceeded")
			continue
		}

		var msg Msg
		if err := json.Unmarshal(data, &msg); err != nil {
			m.sendErr("error", "bad json")
			continue
		}

		if !joined {
			if msg.Type != "join" {
				m.sendErr("error", "join first")
				continue
			}
			if err := m.handleJoin(msg); err != "" {
				m.sendErr("joinError", err)
				return
			}
			joined = true
			continue
		}

		// 调试日志：把每条控制消息连同操作者与位置记下来，
		// 排查"谁把房间拉回片头"时第一眼就能看到。
		switch msg.Type {
		case "play", "pause", "seek", "heartbeat":
			if msg.Type != "heartbeat" {
				log.Printf("[msg] %s %s pos=%.2f rate=%.2f", m.name, msg.Type, msg.Position, msg.Rate)
			}
		}

		m.room.mu.Lock()
		switch msg.Type {
		case "play":
			// 位置写进房间权威状态前必须过关：客户端可能因为读不到 time-pos
			// 而报一个 0 或 NaN 上来，一旦落库，全房间就被拽到片头。
			if p, ok := sanitizePos(msg.Position); ok {
				m.room.position = p
				m.room.anchor = time.Now()
			} else {
				log.Printf("[room %s] 丢弃异常位置 %v（play by %s）", m.room.code, msg.Position, m.name)
			}
			m.room.state = "playing"
			if msg.Rate > 0 && msg.Rate < 8 {
				m.room.rate = msg.Rate
			}
			m.state = "playing"
			m.broadcastSyncLocked(m)
		case "pause":
			if p, ok := sanitizePos(msg.Position); ok {
				m.room.position = p
				m.room.anchor = time.Now()
			} else {
				log.Printf("[room %s] 丢弃异常位置 %v（pause by %s）", m.room.code, msg.Position, m.name)
			}
			m.room.state = "paused"
			m.state = "paused"
			m.broadcastSyncLocked(m)
		case "seek":
			p, ok := sanitizePos(msg.Position)
			if !ok {
				log.Printf("[room %s] 丢弃异常位置 %v（seek by %s）", m.room.code, msg.Position, m.name)
				break
			}
			// 防抖：120ms 窗口内只广播最后一次
			m.room.position = p
			m.room.anchor = time.Now()
			m.state = m.room.state
			debounced := time.Since(m.room.lastSeek) < SEEK_DEBOUNCE
			m.room.lastSeek = time.Now()
			if !debounced {
				if m.room.seekTimer != nil {
					m.room.seekTimer.Stop()
				}
				timer := time.AfterFunc(SEEK_DEBOUNCE, func() { m.emitSync(nil) })
				m.room.seekTimer = timer
			}
		case "heartbeat":
			m.lastHB = time.Now()
			// 心跳位置只用于漂移判定，脏数据同样要挡：否则一个 0 会让服务端
			// 认为该成员落后了几十秒，进而给它发 forceSeek。
			if p, ok := sanitizePos(msg.Position); ok {
				m.lastPos = p
			} else {
				m.room.mu.Unlock()
				continue
			}
			if msg.Rate > 0 {
				m.rate = msg.Rate
			}
			m.room.checkDriftLocked(m)
		case "memberState":
			if msg.State == "buffering" || msg.State == "playing" || msg.State == "paused" {
				m.state = msg.State
				members := m.room.memberInfos()
				m.room.mu.Unlock()
				m.room.broadcast(nil, Msg{Type: "members", Members: members})
				continue
			}
		case "fileInfo":
			m.file = msg.File
			m.duration = msg.Duration
			members := m.room.memberInfos()
			m.room.mu.Unlock()
			m.room.broadcast(nil, Msg{Type: "members", Members: members})
			continue
		case "chat", "leave":
			// 预留
		}
		m.room.mu.Unlock()
	}
}

func (m *Member) handleJoin(msg Msg) string {
	if msg.Room == "" {
		msg.Room = randomCode() // 创建房间：服务器生成房间码
	}
	if msg.Name == "" {
		return "name required"
	}
	r, created := getOrCreateRoom(msg.Room, msg.Password)
	r.mu.Lock()
	if r.password != "" && msg.Password != r.password {
		r.mu.Unlock()
		return "wrong room password"
	}
	if len(r.members) >= MAX_MEMBERS {
		r.mu.Unlock()
		return "room full (max " + strconv.Itoa(MAX_MEMBERS) + ")"
	}
	// 重名检查
	for mem := range r.members {
		if mem.name == msg.Name {
			r.mu.Unlock()
			return "name already taken in room"
		}
	}
	r.members[m] = true
	r.emptySince = time.Time{} // 有人回来了，取消空置计时
	m.room, m.name = r, msg.Name
	m.state = "paused"
	state, pos, anchor, rate := r.snapshot()
	members := r.memberInfos()
	r.mu.Unlock()

	// hello：全量状态
	hello, _ := json.Marshal(Msg{Type: "hello", Room: r.code, State: state, Position: pos, AnchorMs: anchor, Rate: rate, Members: members, Created: created})
	select {
	case m.send <- hello:
	default:
	}
	r.broadcast(nil, Msg{Type: "join", Name: m.name})
	r.broadcast(nil, Msg{Type: "members", Members: members})
	log.Printf("[room %s] %s joined, %d members", r.code, m.name, len(members))
	return ""
}

// 广播 sync（需持有 room.mu，operator 为发起者）
func (m *Member) broadcastSyncLocked(operator *Member) {
	state, pos, anchor, rate := m.room.snapshot()
	m.room.broadcast(operator, Msg{Type: "sync", State: state, Position: pos, AnchorMs: anchor, Rate: rate, Operator: m.name})
}

// 定时器回调用（自动加锁）
func (m *Member) emitSync(operator *Member) {
	if m.room == nil {
		return
	}
	m.room.mu.Lock()
	m.broadcastSyncLocked(nil)
	m.room.mu.Unlock()
}

// 漂移检测：偏差 > DRIFT_HARD 时只纠正该成员。
//
// 原实现有三个问题，都是用户能直接感受到的：
//  1. 单次心跳就纠偏 → 解码卡顿一下就被 seek，画面一顿一顿；
//  2. broadcast 给全房间 → 一个成员的偏差把所有人都拽走；
//  3. 顺手重置 r.position/r.anchor → 房间权威进度被个别成员的异常值污染。
func (r *Room) checkDriftLocked(m *Member) {
	if r.state != "playing" {
		m.driftWarned = false
		return
	}
	expected := r.currentPos()
	drift := m.lastPos - expected
	if drift <= DRIFT_HARD && drift >= -DRIFT_HARD {
		m.driftWarned = false
		return
	}
	if !m.driftWarned {
		// 第一次只记标记，下一次心跳仍偏才动手
		m.driftWarned = true
		return
	}
	data, _ := json.Marshal(Msg{Type: "forceSeek", Position: expected, Operator: "server(drift)"})
	select {
	case m.send <- data:
		log.Printf("[room %s] drift %.2fs by %s -> forceSeek %.1f（仅纠正该成员）", r.code, drift, m.name, expected)
	default:
	}
}

// ---------- 房间过期清理 ----------

func reaper() {
	for {
		time.Sleep(time.Minute)
		hubMu.Lock()
		for code, r := range rooms {
			r.mu.Lock()
			empty := len(r.members) == 0
			expired := empty && !r.emptySince.IsZero() && time.Since(r.emptySince) > EMPTY_ROOM_TTL
			r.mu.Unlock()
			if expired {
				delete(rooms, code)
				log.Printf("[room %s] 空置超过 %s，已回收", code, EMPTY_ROOM_TTL)
			}
		}
		hubMu.Unlock()
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())
	port := os.Getenv("PORT")
	if port == "" {
		port = "8995"
	}
	go reaper()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		hubMu.Lock()
		n := len(rooms)
		hubMu.Unlock()
		w.Write([]byte(`{"status":"ok","rooms":` + strconv.Itoa(n) + `}`))
	})
	http.HandleFunc("/ws", serveWS)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("syncwatch-server v0.4 ok"))
	})

	log.Printf("syncwatch-server listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
