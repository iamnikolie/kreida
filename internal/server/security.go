package server

import (
	"net"
	"net/http"
	"strings"
)

// local says whether a request may use the editor API. The editor API is for this machine's own pages only. A
// loopback socket is not enough: a web page can make the browser send requests here (CSRF), and DNS rebinding gives
// a foreign site a loopback socket under its own Host. So the Host must be a loopback name with this server's port,
// and a request that names its Origin must come from this server.
func (s *Server) local(r *http.Request) bool {
	if !loopbackPeer(r.RemoteAddr) {
		return false
	}

	s.mu.Lock()
	hosts := s.loopHosts
	s.mu.Unlock()

	if !hosts[r.Host] {
		return false
	}

	if origin := r.Header.Get("Origin"); origin != "" {
		if rest, ok := strings.CutPrefix(origin, "https://"); ok {
			origin = rest
		} else {
			origin = strings.TrimPrefix(origin, "http://")
		}

		if !hosts[origin] {
			return false
		}
	}

	return r.Header.Get("Sec-Fetch-Site") != "cross-site"
}

// loopbackPeer: the socket's peer is 127.0.0.1 or ::1 (also as an IPv4-mapped address), not any other 127.x.x.x.
func loopbackPeer(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	return ip.Equal(net.IPv4(127, 0, 0, 1)) || ip.Equal(net.IPv6loopback)
}
