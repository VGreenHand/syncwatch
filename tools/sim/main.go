// 模拟双客户端协议联调：A 创建房间等待，B 加入后发 play/pause/seek，验证同步广播
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type Msg struct {
	Type       string  `json:"type"`
	Room       string  `json:"room,omitempty"`
	Password   string  `json:"password,omitempty"`
	Name       string  `json:"name,omitempty"`
	Position   float64 `json:"position,omitempty"`
	Rate       float64 `json:"rate,omitempty"`
	State      string  `json:"state,omitempty"`
	Operator   string  `json:"operator,omitempty"`
	Error      string  `json:"error,omitempty"`
	Created    bool    `json:"created,omitempty"`
	VoiceUrl   string  `json:"voiceUrl,omitempty"`
	VoiceToken string  `json:"voiceToken,omitempty"`
	Members    []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"members,omitempty"`
}

var (
	role         = flag.String("role", "b", "a=创建者等待, b=加入者发指令")
	addr         = flag.String("addr", "127.0.0.1:8995", "服务器")
	room         = flag.String("room", "TEST01", "房间码")
	nameFlag     = flag.String("name", "", "昵称（默认按 role 取 A/B）")
	verifySecret = flag.String("verify-secret", "", "仅本地校验 voiceToken 的 HS256 签名；不打印该值")
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
		log.Printf("A 收到: %s (state=%s pos=%.1f operator=%s members=%d created=%v err=%s) %s",
			m.Type, m.State, m.Position, m.Operator, len(m.Members), m.Created, m.Error,
			voiceSummary(m.VoiceUrl, m.VoiceToken, *verifySecret))
	}
}

func send(c *websocket.Conn, m Msg) {
	data, _ := json.Marshal(m)
	c.WriteMessage(websocket.TextMessage, data)
	log.Printf("%s 发送: %s pos=%.1f", m.Name, m.Type, m.Position)
}

func voiceSummary(url, token, secret string) string {
	if url == "" && token == "" {
		return "voice=off"
	}
	if url == "" || token == "" {
		return fmt.Sprintf("voice=partial url=%q tokenLen=%d", url, len(token))
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Sprintf("voice=malformed url=%s tokenLen=%d", url, len(token))
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Sprintf("voice=bad-header url=%s tokenLen=%d", url, len(token))
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Sprintf("voice=bad-payload url=%s tokenLen=%d", url, len(token))
	}
	var header struct {
		Alg string `json:"alg"`
	}
	var claims struct {
		Sub   string `json:"sub"`
		Name  string `json:"name"`
		Nbf   int64  `json:"nbf"`
		Exp   int64  `json:"exp"`
		Video struct {
			Room              string   `json:"room"`
			RoomJoin          bool     `json:"roomJoin"`
			CanPublish        bool     `json:"canPublish"`
			CanPublishSources []string `json:"canPublishSources"`
			CanSubscribe      bool     `json:"canSubscribe"`
			CanPublishData    bool     `json:"canPublishData"`
			RoomAdmin         bool     `json:"roomAdmin"`
			RoomCreate        bool     `json:"roomCreate"`
			RoomRecord        bool     `json:"roomRecord"`
			Recorder          bool     `json:"recorder"`
		} `json:"video"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return fmt.Sprintf("voice=bad-header url=%s tokenLen=%d", url, len(token))
	}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return fmt.Sprintf("voice=bad-claims url=%s tokenLen=%d", url, len(token))
	}
	sig := "skip"
	if secret != "" {
		sig = verifyVoiceSig(token, secret)
	}
	ttl := time.Duration(claims.Exp-claims.Nbf) * time.Second
	return fmt.Sprintf("voiceUrl=%s tokenLen=%d sig=%s alg=%s sub=%s name=%s room=%s ttl=%s join=%v publish=%v sources=%v subscribe=%v data=%v admin=%v create=%v record=%v recorder=%v",
		url, len(token), sig, header.Alg, claims.Sub, claims.Name, claims.Video.Room, ttl,
		claims.Video.RoomJoin, claims.Video.CanPublish, claims.Video.CanPublishSources,
		claims.Video.CanSubscribe, claims.Video.CanPublishData, claims.Video.RoomAdmin,
		claims.Video.RoomCreate, claims.Video.RoomRecord, claims.Video.Recorder)
}

func verifyVoiceSig(token, secret string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "bad"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if hmac.Equal([]byte(expected), []byte(parts[2])) {
		return "ok"
	}
	return "bad"
}
