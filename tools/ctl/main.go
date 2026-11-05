// 单指令控制客户端：向指定房间发一条命令，用于联调验证
package main

import (
	"encoding/json"
	"flag"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type Msg struct {
	Type     string  `json:"type"`
	Room     string  `json:"room,omitempty"`
	Name     string  `json:"name,omitempty"`
	Position float64 `json:"position,omitempty"`
	Rate     float64 `json:"rate,omitempty"`
}

func main() {
	room := flag.String("room", "", "房间码")
	name := flag.String("name", "remote", "昵称")
	cmd := flag.String("cmd", "play", "play/pause/seek")
	pos := flag.Float64("pos", 0, "进度秒")
	addr := flag.String("addr", "127.0.0.1:8995", "服务器")
	hold := flag.Duration("hold", 3*time.Second, "发完保持连接时长")
	flag.Parse()

	c, _, err := websocket.DefaultDialer.Dial("ws://"+*addr+"/ws", nil)
	if err != nil {
		log.Fatal("dial: ", err)
	}
	join, _ := json.Marshal(Msg{Type: "join", Room: *room, Name: *name})
	c.WriteMessage(websocket.TextMessage, join)
	time.Sleep(500 * time.Millisecond)

	m := Msg{Type: *cmd, Name: *name, Position: *pos}
	if *cmd == "play" {
		m.Rate = 1.0
	}
	data, _ := json.Marshal(m)
	c.WriteMessage(websocket.TextMessage, data)
	log.Printf("已发送 %s @%.1fs 到房间 %s", *cmd, *pos, *room)

	// 读掉服务器广播，避免阻塞
	go func() {
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}()
	time.Sleep(*hold)
	c.Close()
}
