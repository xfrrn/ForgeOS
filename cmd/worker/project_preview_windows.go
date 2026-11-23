package main

import (
	"context"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"forgeos/internal/preview"
	"golang.org/x/sys/windows"
)

var previewTCPTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")
var previewInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

// Windows only reports listening socket owners as PIDs. Check that the exact
// loopback listener belongs to the private job before proxying to the port.
func previewOwnsListener(job windows.Handle, port int) bool {
	var size uint32
	if result, _, _ := previewTCPTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, 3, 0); result != 122 {
		return false
	}
	for i := 0; i < 3 && size >= 4 && size < 1<<24; i++ {
		buf := make([]byte, size)
		result, _, _ := previewTCPTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, 3, 0)
		if result == 122 {
			continue
		}
		if result != 0 {
			return false
		}
		count := binary.LittleEndian.Uint32(buf)
		for j := uint32(0); j < count && 4+24*(j+1) <= uint32(len(buf)); j++ {
			row := buf[4+24*j : 4+24*(j+1)]
			if string(row[4:8]) != "\x7f\x00\x00\x01" || int(binary.BigEndian.Uint16(row[8:10])) != port {
				continue
			}
			pid := binary.LittleEndian.Uint32(row[20:24])
			handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
			if err != nil {
				return false
			}
			var belongs int32
			ok, _, _ := previewInJob.Call(uintptr(handle), uintptr(job), uintptr(unsafe.Pointer(&belongs)))
			windows.CloseHandle(handle)
			return ok != 0 && belongs != 0
		}
		break
	}
	return false
}

func showProjectPreviewProcess(in io.Reader, out io.Writer) {
	var input projectPreviewProcess
	decoder := json.NewDecoder(io.LimitReader(in, 16<<10))
	if decoder.Decode(&input) != nil {
		return
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		_ = json.NewEncoder(out).Encode(projectPreviewReady{Error: "无法创建 Windows 预览进程组"})
		return
	}
	defer windows.CloseHandle(job)
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		_ = json.NewEncoder(out).Encode(projectPreviewReady{Error: "无法设置 Windows 预览进程组"})
		return
	}
	if err = windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		_ = json.NewEncoder(out).Encode(projectPreviewReady{Error: "无法隔离 Windows 预览进程组"})
		return
	}
	parentGone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, in)
		close(parentGone)
	}()
	if err := runProjectPreviewProcess(job, input, out, parentGone); err != nil {
		_ = json.NewEncoder(out).Encode(projectPreviewReady{Error: err.Error()})
	}
}

type windowsPreviewEndpoint struct {
	port       int
	healthPath string
	listener   net.Listener
	origin     *url.URL
	transport  *http.Transport
}

func reservePreviewEndpoint(job windows.Handle, healthPath string, preferredPort int) (*windowsPreviewEndpoint, error) {
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", preferredPort))
	if err != nil {
		return nil, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	address := listener.Addr().String()
	transport := &http.Transport{DisableKeepAlives: true, ResponseHeaderTimeout: 20 * time.Second,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			if !previewOwnsListener(job, port) {
				return nil, errors.New("preview listener is not owned by this instance")
			}
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, address)
		},
	}
	return &windowsPreviewEndpoint{port, healthPath, listener, &url.URL{Scheme: "http", Host: address}, transport}, nil
}

func (endpoint *windowsPreviewEndpoint) health() int {
	client := &http.Client{Transport: endpoint.transport, Timeout: 2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get(endpoint.origin.String() + endpoint.healthPath)
	if err != nil {
		return 0
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return response.StatusCode
	}
	return 0
}

func (endpoint *windowsPreviewEndpoint) proxy(responseProof string) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: endpoint.transport,
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(endpoint.origin)
			p.Out.Header.Del(preview.ProjectProofHeader)
		},
		ModifyResponse: func(response *http.Response) error {
			// Keep same-service redirects on the HTTPS origin the phone opened.
			location, err := url.Parse(response.Header.Get("Location"))
			if err == nil && location.Scheme == endpoint.origin.Scheme && location.Host == endpoint.origin.Host {
				location.Scheme, location.Host = "", ""
				response.Header.Set("Location", location.String())
			}
			response.Header.Set(preview.ProjectProofHeader, responseProof)
			return nil
		},
	}
}

func runProjectPreviewProcess(job windows.Handle, input projectPreviewProcess, out io.Writer, parentGone <-chan struct{}) error {
	// Only the gateway-facing listener stays open. The project gets its own
	// loopback port and is required to bind it before the preview is published.
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return errors.New("无法分配预览入口")
	}
	defer listener.Close()
	app, err := reservePreviewEndpoint(job, input.HealthPath, 0)
	if err != nil {
		return errors.New("无法分配应用端口")
	}
	defer app.listener.Close()
	var api *windowsPreviewEndpoint
	if input.API != nil {
		api, err = reservePreviewEndpoint(job, input.API.HealthPath, input.API.Port)
		if err != nil {
			return errors.New("无法分配 API 端口，请检查端口是否已被占用")
		}
		defer api.listener.Close()
	}
	command := exec.Command(input.PowerShell, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", input.Script)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Dir = input.Dir
	for _, entry := range harnessEnv() {
		name, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(name) {
		case "CODEX_HOME", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY":
		default:
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "HOST=127.0.0.1", fmt.Sprintf("PORT=%d", app.port))
	if api != nil {
		command.Env = append(command.Env, fmt.Sprintf("API_PORT=%d", api.port))
	}
	logFile, err := os.OpenFile(input.LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return errors.New("无法创建本机预览日志")
	}
	defer logFile.Close()
	logOutput := &environmentOutputWriter{dest: &cappedLog{dest: logFile, remaining: 256 << 10},
		output: modelOutput{secrets: outputSecrets("")}}
	command.Stdout, command.Stderr = logOutput, logOutput
	app.listener.Close()
	if api != nil {
		api.listener.Close()
	}
	if err := command.Start(); err != nil {
		return errors.New("无法运行项目预览脚本")
	}
	exited := make(chan struct{})
	go func() {
		_ = command.Wait()
		logOutput.flush()
		close(exited)
	}()
	readiness := func() int {
		status := app.health()
		if status != 0 && api != nil && api.health() == 0 {
			return 0
		}
		return status
	}
	status := 0
	deadline := time.NewTimer(time.Duration(input.Timeout) * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for status == 0 {
		select {
		case <-deadline.C:
			return errors.New("应用未在规定时间内通过健康检查")
		case <-exited:
			return errors.New("项目预览脚本在就绪前退出")
		case <-parentGone:
			return errors.New("Worker 已关闭预览启动")
		case <-tick.C:
			status = readiness()
		}
	}
	requestProof := preview.ProjectProof(input.Secret, input.Instance, "request")
	responseProof := preview.ProjectProof(input.Secret, input.Instance, "response")
	proxy := app.proxy(responseProof)
	var apiProxy *httputil.ReverseProxy
	if api != nil {
		apiProxy = api.proxy(responseProof)
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get(preview.ProjectProofHeader)), []byte(requestProof)) != 1 {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == preview.ProjectHealthPath {
			select {
			case <-exited:
				http.Error(w, "preview exited", http.StatusServiceUnavailable)
				return
			default:
			}
			status := readiness()
			if status == 0 {
				http.Error(w, "preview listener unavailable", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set(preview.ProjectProofHeader, responseProof)
			w.WriteHeader(status)
			return
		}
		if input.API.matches(r.URL.Path) {
			apiProxy.ServeHTTP(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()
	if err := json.NewEncoder(out).Encode(projectPreviewReady{Port: listener.Addr().(*net.TCPAddr).Port, HTTPStatus: status}); err != nil {
		return err
	}
	select {
	case <-parentGone:
	case <-exited:
	}
	return nil
}
