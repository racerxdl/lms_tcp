package rtltcp

import (
	"github.com/quan-to/slog"
	"net"
)

type Session struct {
	id   string
	conn net.Conn
	log  slog.Instance
}
