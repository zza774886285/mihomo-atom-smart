package dns

import (
	"context"
	"net"
	"sync"

	"github.com/metacubex/mihomo/common/sockopt"
	"github.com/metacubex/mihomo/component/resolver"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"

	D "github.com/miekg/dns"
)

var (
	address   string
	serverMux sync.Mutex
	server    = &Server{}

	dnsDefaultTTL uint32 = 600
)

type Server struct {
	serviceMux sync.RWMutex
	service    resolver.Service
	tcpServer  *D.Server
	udpServer  *D.Server
}

type serverHandler struct {
	*Server
	isUDP bool
}

// ServeDNS implement D.Handler ServeDNS
func (s serverHandler) ServeDNS(w D.ResponseWriter, r *D.Msg) {
	s.serviceMux.RLock()
	service := s.service
	s.serviceMux.RUnlock()
	msg, err := service.ServeMsg(context.Background(), r)
	if err != nil {
		m := new(D.Msg)
		m.SetRcode(r, D.RcodeServerFailure)
		// does not matter if this write fails
		w.WriteMsg(m)
		return
	}
	if s.isUDP {
		// RFC 6891: fit the reply into the client's advertised buffer size,
		// setting the TC bit if records must be dropped; 512 when no OPT present
		msg.Truncate(resolver.RequestUDPSize(r))
	}
	msg.Compress = true
	w.WriteMsg(msg)
}

func (s *Server) UDPHandler() D.Handler {
	return serverHandler{Server: s, isUDP: true}
}

func (s *Server) TCPHandler() D.Handler {
	return serverHandler{Server: s, isUDP: false}
}

func (s *Server) SetService(service resolver.Service) {
	s.serviceMux.Lock()
	defer s.serviceMux.Unlock()
	s.service = service
}

func Ready() bool {
	serverMux.Lock()
	defer serverMux.Unlock()
	return server.tcpServer != nil && server.udpServer != nil
}

func ReCreateServer(addr string, lc C.InboundListenConfig, service resolver.Service) {
	serverMux.Lock()
	defer serverMux.Unlock()
	if addr == address && service != nil && server.tcpServer != nil && server.udpServer != nil {
		server.SetService(service)
		return
	}

	if server.tcpServer != nil {
		_ = server.tcpServer.Shutdown()
		server.tcpServer = nil
	}

	if server.udpServer != nil {
		_ = server.udpServer.Shutdown()
		server.udpServer = nil
	}

	address = ""

	if addr == "" || lc == nil || service == nil {
		return
	}

	var err error
	defer func() {
		if err != nil {
			log.Errorln("Start DNS server error: %s", err.Error())
		}
	}()

	_, port, err := net.SplitHostPort(addr)
	if port == "0" || port == "" || err != nil {
		return
	}

	address = addr
	server = &Server{service: service}

	p, err := lc.ListenPacket(context.Background(), "udp", addr)
	if err != nil {
		return
	}
	if reuseErr := sockopt.UDPReuseaddr(p); reuseErr != nil {
		log.Warnln("Failed to set DNS UDP address reuse: %s", reuseErr)
	}

	l, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		_ = p.Close()
		return
	}
	log.Infoln("DNS server listening on TCP and UDP at: %s", addr)

	udpServer := &D.Server{Addr: addr, PacketConn: p, Handler: server.UDPHandler()}
	tcpServer := &D.Server{Addr: addr, Listener: l, Handler: server.TCPHandler()}
	server.udpServer = udpServer
	server.tcpServer = tcpServer
	for _, listener := range []*D.Server{udpServer, tcpServer} {
		started := make(chan struct{})
		listener.NotifyStartedFunc = func() { close(started) }
		done := make(chan struct{})
		go func(listener *D.Server) {
			defer close(done)
			_ = listener.ActivateAndServe()
		}(listener)
		select {
		case <-started:
		case <-done:
			if listener == udpServer {
				_ = p.Close()
				server.udpServer = nil
			} else {
				_ = l.Close()
				server.tcpServer = nil
			}
		}
	}
}
