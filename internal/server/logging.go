package server

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// NewLogger writes one stable, grep-friendly event per line to the supplied stream.
func NewLogger(output io.Writer) *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(output)
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&lineFormatter{})
	return logger
}

type lineFormatter struct{}

var orderedLogFields = []string{"event", "room_id", "client_id", "match_id", "request_id", "conn_id", "remote_addr", "reason"}

func (f *lineFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var out bytes.Buffer
	writeLogField(&out, "time", entry.Time.UTC().Format(time.RFC3339Nano))
	writeLogField(&out, "level", entry.Level.String())
	seen := make(map[string]bool, len(entry.Data))
	for _, key := range orderedLogFields {
		if value, ok := entry.Data[key]; ok {
			writeLogField(&out, key, fmt.Sprint(value))
			seen[key] = true
		}
	}
	keys := make([]string, 0, len(entry.Data))
	for key := range entry.Data {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		writeLogField(&out, key, fmt.Sprint(entry.Data[key]))
	}
	writeLogField(&out, "message", entry.Message)
	out.WriteByte('\n')
	return out.Bytes(), nil
}

func writeLogField(out *bytes.Buffer, key, value string) {
	if out.Len() > 0 {
		out.WriteByte(' ')
	}
	out.WriteString(key)
	out.WriteByte('=')
	if value == "" || strings.ContainsAny(value, " \t\r\n\"=") {
		out.WriteString(strconv.Quote(value))
	} else {
		out.WriteString(value)
	}
}

// Wire IDs are Guid.ToByteArray() in the C# protocol. The first three fields
// must be byte-reversed to match Guid.ToString("N") in plugin logs.
func formatID(raw []byte) string {
	if len(raw) != 16 {
		return ""
	}
	ordered := []byte{raw[3], raw[2], raw[1], raw[0], raw[5], raw[4], raw[7], raw[6]}
	ordered = append(ordered, raw[8:]...)
	return hex.EncodeToString(ordered)
}

func (s *Server) logFor(c *connection) *logrus.Entry {
	fields := logrus.Fields{"conn_id": c.connID, "remote_addr": c.conn.RemoteAddr().String()}
	_, clientID, roomID := c.binding()
	if value := formatID(roomID); value != "" {
		fields["room_id"] = value
	}
	if value := formatID(clientID); value != "" {
		fields["client_id"] = value
	}
	return s.logger.WithFields(fields)
}
