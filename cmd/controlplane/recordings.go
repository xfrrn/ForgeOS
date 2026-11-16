package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

const recordingLimit = 64 << 20

// Remux only the visible video track. A seekable output gives browser recordings a duration and a cue index.
func cleanRecording(ctx context.Context, src []byte) ([]byte, error) {
	if !bytes.HasPrefix(src, []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		return nil, errors.New("请选择有效的 WebM 录屏")
	}
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("录屏归档需要 FFmpeg: %w", exec.ErrNotFound)
	}
	f, err := os.CreateTemp("", "forgeos-recording-*.webm")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	if err = f.Close(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-nostdin", "-v", "error", "-xerror", "-abort_on", "empty_output",
		"-protocol_whitelist", "pipe", "-f", "matroska", "-i", "pipe:0",
		"-map", "0:V:0", "-c:v", "copy", "-an", "-sn", "-dn", "-map_metadata", "-1", "-map_metadata:s:v", "-1",
		"-map_chapters", "-1", "-fflags", "+bitexact", "-fs", strconv.Itoa(recordingLimit+1), "-f", "webm", "-y", f.Name())
	cmd.Stdin = bytes.NewReader(src)
	if err = cmd.Run(); err != nil {
		return nil, errors.New("录屏缺少可归档的 WebM 视频轨道或读取失败")
	}
	st, err := os.Stat(f.Name())
	if err != nil {
		return nil, err
	}
	if st.Size() <= 0 || st.Size() > recordingLimit {
		return nil, errors.New("归档的录屏须在 64 MiB 以内")
	}
	return os.ReadFile(f.Name())
}
