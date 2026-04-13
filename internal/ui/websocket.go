package ui

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"nhooyr.io/websocket"
)

func (s *Server) handleWSLogs(w http.ResponseWriter, r *http.Request) {
	appName := r.PathValue("name")
	if err := validateAppName(appName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	process := r.URL.Query().Get("process")
	if process == "" {
		process = "web"
	}
	lines := r.URL.Query().Get("lines")
	if lines == "" {
		lines = "100"
	}
	if _, err := strconv.Atoi(lines); err != nil {
		http.Error(w, "invalid lines parameter", http.StatusBadRequest)
		return
	}

	// Find the server this app is on
	exec, _, err := s.withApp(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("websocket accept error: %v", err)
		return
	}
	defer conn.CloseNow()

	ctx := conn.CloseRead(r.Context())

	// Find the container name
	containerFilter := fmt.Sprintf("--filter label=teploy.app=%s --filter label=teploy.process=%s",
		shellQuote(appName), shellQuote(process))
	findCmd := fmt.Sprintf("docker ps --format '{{.Names}}' %s", containerFilter)
	containerName, err := exec.Run(ctx, findCmd)
	if err != nil || strings.TrimSpace(containerName) == "" {
		conn.Write(ctx, websocket.MessageText, []byte("No container found for "+appName+"-"+process+"\n"))
		conn.Close(websocket.StatusNormalClosure, "no container")
		return
	}
	containerName = strings.TrimSpace(strings.Split(containerName, "\n")[0])

	logCmd := fmt.Sprintf("docker logs --follow --tail %s %s", shellQuote(lines), shellQuote(containerName))

	// Stream logs via SSH to WebSocket
	writer := &wsWriter{conn: conn, ctx: ctx}
	err = exec.RunStream(ctx, logCmd, writer, writer)
	if err != nil && ctx.Err() == nil {
		conn.Write(ctx, websocket.MessageText, []byte("Stream error: "+err.Error()+"\n"))
	}

	conn.Close(websocket.StatusNormalClosure, "stream ended")
}

// wsWriter adapts a WebSocket connection to io.Writer for log streaming.
type wsWriter struct {
	conn *websocket.Conn
	ctx  context.Context
}

func (w *wsWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if err := w.conn.Write(w.ctx, websocket.MessageText, p); err != nil {
		return 0, err
	}
	return len(p), nil
}
