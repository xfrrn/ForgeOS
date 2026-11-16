// Package preview defines the private control-plane to Worker preview protocol.
package preview

import (
	"errors"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"
)

const InstanceHeader = "X-ForgeOS-Preview-Instance"

var InstanceID = regexp.MustCompile("^[0-9a-f]{48}$")

type Lease struct {
	Repo    string    `json:"repo"`
	Commit  string    `json:"commit"`
	Expires time.Time `json:"expires_at"`
}

func ManagerConfig() (addr, token string, err error) {
	addr, token = os.Getenv("FORGEOS_PREVIEW_MANAGER_ADDR"), os.Getenv("FORGEOS_PREVIEW_MANAGER_TOKEN")
	if addr == "" && token == "" {
		return
	}
	h, p, e := net.SplitHostPort(addr)
	port, e2 := strconv.Atoi(p)
	if e != nil || e2 != nil || h != "127.0.0.1" || port < 1 || port > 65535 || p != strconv.Itoa(port) || len(token) < 32 {
		return "", "", errors.New("configure a fixed loopback FORGEOS_PREVIEW_MANAGER_ADDR and a dedicated FORGEOS_PREVIEW_MANAGER_TOKEN of at least 32 characters")
	}
	return
}

// Only application data crosses the boundary. Authentication belongs to each hop.
func CopyRequestHeaders(dst, src http.Header) {
	for _, k := range []string{"Accept", "Accept-Language", "Content-Type", "Range", "If-None-Match", "If-Modified-Since"} {
		for _, v := range src.Values(k) {
			dst.Add(k, v)
		}
	}
}
func CopyResponseHeaders(dst, src http.Header) {
	for _, k := range []string{"Content-Type", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Content-Encoding", "Location"} {
		for _, v := range src.Values(k) {
			dst.Add(k, v)
		}
	}
}
