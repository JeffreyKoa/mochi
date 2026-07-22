package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: voiceflow <token>")
	}
	url := "ws://localhost:8081/ws/voice?token=" + os.Args[1]
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	// read session_start + animation idle
	readUntil(c, 2*time.Second)

	// send fake PCM (silence then "speech")
	pcm := make([]byte, 640*10) // 200ms silence
	sendAudio(c, pcm, 1)

	// simulate speech with louder PCM
	loud := make([]byte, 640*20)
	for i := 0; i < len(loud)/2; i++ {
		loud[i*2] = 0xFF
		loud[i*2+1] = 0x7F
	}
	sendAudio(c, loud, 2)

	// end speech
	send(c, "audio_end", map[string]any{})

	// read responses for 3s
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, msg, err := c.ReadMessage()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}
		fmt.Println(string(msg))
	}
}

func sendAudio(c *websocket.Conn, pcm []byte, seq int64) {
	send(c, "audio", map[string]any{
		"pcm": base64.StdEncoding.EncodeToString(pcm),
		"seq": seq,
	})
}

func send(c *websocket.Conn, typ string, data any) {
	b, _ := json.Marshal(map[string]any{"type": typ, "data": data, "ts": time.Now().Unix()})
	c.WriteMessage(websocket.TextMessage, b)
}

func readUntil(c *websocket.Conn, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		_, msg, err := c.ReadMessage()
		if err != nil {
			return
		}
		fmt.Println(string(msg))
	}
}
