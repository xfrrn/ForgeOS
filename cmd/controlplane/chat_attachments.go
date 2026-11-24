package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	"forgeos/internal/store"
)

const (
	chatAttachmentLimit    = 10 << 20
	chatAttachmentMaxCount = 6
)

func cleanChatImage(contentType string, src []byte) (string, []byte, error) {
	if len(src) == 0 || len(src) > chatAttachmentLimit {
		return "", nil, errors.New("图片大小须在 1 字节至 10 MiB 之间。")
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	detected := http.DetectContentType(src)
	if mediaType == "" || mediaType == "application/octet-stream" {
		mediaType = detected
	}
	switch mediaType {
	case "image/png":
		cfg, err := png.DecodeConfig(bytes.NewReader(src))
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > screenshotLimit/cfg.Height {
			return "", nil, errors.New("无效的 PNG 图片或像素超过限制。")
		}
		im, err := png.Decode(bytes.NewReader(src))
		if err != nil {
			return "", nil, errors.New("无法解析 PNG 图片。")
		}
		flat := image.NewRGBA(im.Bounds())
		draw.Draw(flat, flat.Bounds(), image.White, image.Point{}, draw.Src)
		draw.Draw(flat, flat.Bounds(), im, im.Bounds().Min, draw.Over)
		var buf bytes.Buffer
		encoder := png.Encoder{CompressionLevel: png.BestSpeed}
		if err = encoder.Encode(&buf, flat); err != nil {
			return "", nil, err
		}
		if buf.Len() > chatAttachmentLimit {
			return "", nil, errors.New("图片编码后超过 10 MiB 限制。")
		}
		return "image/png", buf.Bytes(), nil
	case "image/jpeg":
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(src))
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > screenshotLimit/cfg.Height {
			return "", nil, errors.New("无效的 JPEG 图片或像素超过限制。")
		}
		im, err := jpeg.Decode(bytes.NewReader(src))
		if err != nil {
			return "", nil, errors.New("无法解析 JPEG 图片。")
		}
		var buf bytes.Buffer
		if err = jpeg.Encode(&buf, im, &jpeg.Options{Quality: 90}); err != nil {
			return "", nil, err
		}
		if buf.Len() > chatAttachmentLimit {
			return "", nil, errors.New("图片编码后超过 10 MiB 限制。")
		}
		return "image/jpeg", buf.Bytes(), nil
	case "image/gif":
		cfg, err := gif.DecodeConfig(bytes.NewReader(src))
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > screenshotLimit/cfg.Height {
			return "", nil, errors.New("无效的 GIF 图片或像素超过限制。")
		}
		im, err := gif.Decode(bytes.NewReader(src))
		if err != nil {
			return "", nil, errors.New("无法解析 GIF 图片。")
		}
		flat := image.NewRGBA(im.Bounds())
		draw.Draw(flat, flat.Bounds(), image.White, image.Point{}, draw.Src)
		draw.Draw(flat, flat.Bounds(), im, im.Bounds().Min, draw.Over)
		var buf bytes.Buffer
		encoder := png.Encoder{CompressionLevel: png.BestSpeed}
		if err = encoder.Encode(&buf, flat); err != nil {
			return "", nil, err
		}
		if buf.Len() > chatAttachmentLimit {
			return "", nil, errors.New("图片编码后超过 10 MiB 限制。")
		}
		return "image/png", buf.Bytes(), nil
	case "image/webp":
		if len(src) < 12 || string(src[:4]) != "RIFF" || string(src[8:12]) != "WEBP" {
			return "", nil, errors.New("无效的 WebP 图片。")
		}
		return "image/webp", src, nil
	default:
		return "", nil, errors.New("仅支持 PNG、JPEG、WebP 或 GIF 图片。")
	}
}

func serveChatAttachment(w http.ResponseWriter, r *http.Request, data *store.Store, project, id int64) {
	att, err := data.ChatAttachment(r.Context(), project, id)
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	name, err := artifactPath(att.Key)
	if err != nil {
		problem(w, 503, "附件存储不可用。")
		return
	}
	f, err := os.Open(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			problem(w, 404, "找不到该图片附件。")
			return
		}
		problem(w, 503, "无法读取图片附件。")
		return
	}
	defer f.Close()
	if att.Bytes <= 0 || att.Bytes > chatAttachmentLimit {
		problem(w, 503, "图片附件大小异常。")
		return
	}
	content, err := io.ReadAll(io.LimitReader(f, att.Bytes+1))
	sum := sha256.Sum256(content)
	if err != nil || int64(len(content)) != att.Bytes || hex.EncodeToString(sum[:]) != att.SHA256 {
		problem(w, 503, "图片附件完整性校验失败。")
		return
	}
	w.Header().Set("Content-Type", att.MIME)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.FormatInt(att.Bytes, 10))
	w.Write(content)
}

func registerChatAttachments(mux *http.ServeMux, data *store.Store, owner string, nodes chatNodes) {
	mux.HandleFunc("POST /projects/{id}/chat/attachments", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		var filename, contentType string
		var raw []byte
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			r.Body = http.MaxBytesReader(w, r.Body, chatAttachmentLimit+(1<<20))
			reader, err := r.MultipartReader()
			if err != nil {
				problem(w, 400, "无法读取上传的图片。")
				return
			}
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					problem(w, 400, "上传图片不完整或超过 10 MiB 限制。")
					return
				}
				if part.FormName() != "file" {
					part.Close()
					continue
				}
				filename = part.FileName()
				contentType = part.Header.Get("Content-Type")
				dataBytes, readErr := io.ReadAll(io.LimitReader(part, chatAttachmentLimit+1))
				part.Close()
				if readErr != nil || len(dataBytes) > chatAttachmentLimit {
					problem(w, 413, "单张图片不能超过 10 MiB。")
					return
				}
				raw = dataBytes
				break
			}
			if len(raw) == 0 {
				problem(w, 400, "请选择或粘贴一张图片。")
				return
			}
		} else {
			contentType = r.Header.Get("Content-Type")
			filename = r.Header.Get("X-Attachment-Name")
			dataBytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, chatAttachmentLimit+1))
			if err != nil || len(dataBytes) > chatAttachmentLimit {
				problem(w, 413, "单张图片不能超过 10 MiB。")
				return
			}
			raw = dataBytes
		}
		cleanMIME, content, err := cleanChatImage(contentType, raw)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		sum := sha256.Sum256(content)
		att, err := data.SaveChatAttachment(r.Context(), project, filename, cleanMIME, hex.EncodeToString(sum[:]), int64(len(content)), func(key string) error {
			return saveArtifact(key, content)
		})
		respond(w, http.StatusCreated, att, err)
	}))

	mux.HandleFunc("GET /projects/{id}/chat/attachments/{attachment}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		id, err := strconv.ParseInt(r.PathValue("attachment"), 10, 64)
		if err != nil || id <= 0 {
			problem(w, 400, "无效的附件编号。")
			return
		}
		serveChatAttachment(w, r, data, project, id)
	}))

	mux.HandleFunc("DELETE /projects/{id}/chat/attachments/{attachment}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		id, err := strconv.ParseInt(r.PathValue("attachment"), 10, 64)
		if err != nil || id <= 0 {
			problem(w, 400, "无效的附件编号。")
			return
		}
		key, err := data.DeleteDraftChatAttachment(r.Context(), project, id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		_ = removeArtifact(key)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /internal/management-chat/attachments/{attachment}", nodes.authorize(func(project int64, w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("attachment"), 10, 64)
		if err != nil || id <= 0 {
			problem(w, 400, "invalid attachment ID")
			return
		}
		serveChatAttachment(w, r, data, project, id)
	}))
}
