// 模拟双客户端协议联调：A 创建房间等待，B 加入后发 play/pause/seek，验证同步广播
package main

import (
	"encoding/json"
	"flag"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type Msg struct {
	Type     string      `json:"type"`
	Room     string      `json:"room,omitempty"`
	Password string      `json:"password,omitempty"`
	Name     string      `json:"name,omitempty"`
	Position float64     `json:"position,omitempty"`
	Rate     float64     `json:"rate,omitempty"`
	State    string      `json:"state,omitempty"`
	Operator string      `json:"operator,omitempty"`
	Error    string      `json:"error,omitempty"`
	Created  bool        `json:"created,omitempty"`
	Members  []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"members,omitempty"`
}

var (
	role = flag.String("role", "b", "a=创建者等待, b=加入者发指令")
	addr = flag.String("addr", "127.0.0.1:8995", "服务器")
	room = flag.String("room", "TEST01", "房间码")
	nameFlag = flag.String("name", "", "昵称（默认按 role 取 A/B）")
)

func main() {
	flag.Parse()
	log.SetFlags(log.Lmicroseconds)
	c, _, err := websocket.DefaultDialer.Dial("ws://"+*addr+"/ws", nil)
	if err != nil {
		log.Fatal("dial: ", err)
	}
	defer c.Close()

	name := "A"
	if *role == "b" {
		name = "B"
	}
	if *nameFlag != "" {
		name = *nameFlag
	}
	join, _ := json.Marshal(Msg{Type: "join", Room: *room, Name: name, Password: ""})
	c.WriteMessage(websocket.TextMessage, join)

	if *role == "b" {
		time.Sleep(1 * time.Second)
		send(c, Msg{Type: "play", Position: 100.0, Rate: 1.0})
		time.Sleep(600 * time.Millisecond)
		send(c, Msg{Type: "pause", Position: 101.5})
		time.Sleep(600 * time.Millisecond)
		send(c, Msg{Type: "seek", Position: 200.0})
		time.Sleep(600 * time.Millisecond)
		send(c, Msg{Type: "heartbeat", Position: 200.8, Rate: 1.0})
		time.Sleep(1 * time.Second)
		log.Println("B: 测试指令已发完，退出")
		return
	}

	// A: 读所有消息并打印
	deadline := time.Now().Add(8 * time.Second)
	c.SetReadDeadline(deadline)
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			log.Println("A: 连接结束: ", err)
			return
		}
		var m Msg
		json.Unmarshal(data, &m)
		log.Printf("A 收到: %s (state=%s pos=%.1f operator=%s members=%d created=%v err=%s)",
			m.Type, m.State, m.Position, m.Operator, len(m.Members), m.Created, m.Error)
	}
}

func send(c *websocket.Conn, m Msg) {
	data, _ := json.Marshal(m)
	c.WriteMessage(websocket.TextMessage, data)
	log.Printf("%s 发送: %s pos=%.1f", m.Name, m.Type, m.Position)
}
