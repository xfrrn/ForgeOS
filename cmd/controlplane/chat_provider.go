package main

import (
	"context"
	"net/http"
	"strings"

	"forgeos/internal/chatkernel"
	"forgeos/internal/store"
	"github.com/google/uuid"
)

type chatProviders interface {
	ChatProvider(context.Context, int64) (chatkernel.Provider, error) // legacy project credentials
	ModelConnection(context.Context, uuid.UUID) (chatkernel.Provider, error)
	SetModelConnection(context.Context, uuid.UUID, chatkernel.Provider) error
}

func chatModelMetadata(p chatkernel.Provider) store.ChatModel {
	return store.ChatModel{Name: p.Model, Model: p.Model, BaseURL: p.BaseURL, OutputFormat: p.OutputFormat, Revision: uuid.New()}
}

type chatModelInput struct {
	chatkernel.Provider
	ID       uuid.UUID `json:"id"`
	Revision uuid.UUID `json:"revision"`
	Name     string    `json:"name"` // Accepted from older clients; the public name is always the model ID.
}

func readChatModelInput(w http.ResponseWriter, r *http.Request, data *store.Store, vault chatProviders, reuse bool) *chatModelInput {
	var in chatModelInput
	if !decodeLimited(w, r, &in, 16<<10) {
		return nil
	}
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	in.Model = strings.TrimSpace(in.Model)
	if reuse && in.APIKey == "" && in.Revision != uuid.Nil && in.ID != uuid.Nil {
		active, err := data.CurrentChatModelRevision(r.Context(), in.ID, in.Revision)
		if err != nil {
			respond(w, 0, nil, err)
			return nil
		}
		if !active {
			problem(w, 409, "模型配置已更改，请重新读取。")
			return nil
		}
		previous, err := vault.ModelConnection(r.Context(), in.Revision)
		if err != nil {
			problem(w, 503, "无法读取已有连接。")
			return nil
		}
		if previous.BaseURL != in.BaseURL {
			problem(w, 400, "更换 Base URL 时请重新填写 API Key。")
			return nil
		}
		in.APIKey = previous.APIKey
	}
	if !in.Valid() {
		problem(w, 400, "请检查 Base URL、模型 ID 和 API Key。远程地址须使用 HTTPS。")
		return nil
	}
	return &in
}

func registerChatProvider(mux *http.ServeMux, data *store.Store, owner string, nodes chatNodes, vault chatProviders) {
	const path = "/projects/chat-models"
	mux.HandleFunc("GET "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ctx := r.Context()
		models, err := data.ChatModels(ctx, func(project int64) (store.ChatModel, error) {
			p, err := vault.ChatProvider(ctx, project)
			if err != nil || !p.Valid() {
				return store.ChatModel{}, err
			}
			m := chatModelMetadata(p)
			return m, vault.SetModelConnection(ctx, m.Revision, p)
		})
		if err != nil {
			problem(w, 503, "无法加载模型列表，请检查凭据存储。")
			return
		}
		writeJSON(w, 200, models)
	}))
	mux.HandleFunc("POST "+path+"/test", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		in := readChatModelInput(w, r, data, vault, true)
		if in == nil {
			return
		}
		if err := chatkernel.TestConnection(r.Context(), in.Provider); err != nil {
			problem(w, 502, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	save := authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		in := readChatModelInput(w, r, data, vault, r.Method == "PUT")
		if in == nil {
			return
		}
		m := chatModelMetadata(in.Provider)
		m.ID = in.ID
		if r.Method == "POST" && in.ID == uuid.Nil && in.Revision == uuid.Nil {
			m.ID = uuid.New()
		} else if r.Method != "PUT" || in.ID == uuid.Nil || in.Revision == uuid.Nil {
			problem(w, 400, "无效的模型配置版本。")
			return
		}
		err := data.SaveChatModel(r.Context(), m, in.Revision, func() error { return vault.SetModelConnection(r.Context(), m.Revision, in.Provider) })
		if err != nil {
			if err == store.ErrConflict {
				problem(w, 409, "模型配置已更改，请重新读取。")
			} else {
				problem(w, 503, "未能保存模型，请检查凭据存储。")
			}
			return
		}
		writeJSON(w, 200, m)
	})
	mux.HandleFunc("POST "+path, save)
	mux.HandleFunc("PUT "+path, save)
	mux.HandleFunc("DELETE "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		var in struct{ ID, Revision uuid.UUID }
		if !decode(w, r, &in) {
			return
		}
		respond(w, 204, nil, data.RemoveChatModel(r.Context(), in.ID, in.Revision))
	}))
	mux.HandleFunc("POST /internal/management-chat/provider", nodes.authorize(func(project int64, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var in struct {
			ID    int64
			Lease string
		}
		if !decode(w, r, &in) {
			return
		}
		lease, err := uuid.Parse(in.Lease)
		if err != nil || in.ID <= 0 {
			problem(w, 400, "invalid chat lease")
			return
		}
		revision, err := data.LeasedChatModel(r.Context(), project, in.ID, lease.String())
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		var p chatkernel.Provider
		if revision.Valid {
			p, err = vault.ModelConnection(r.Context(), revision.UUID)
		} else {
			p, err = vault.ChatProvider(r.Context(), project)
		}
		if err != nil {
			problem(w, 503, "model credential store unavailable")
			return
		}
		if !p.Valid() {
			problem(w, 409, "model connection unavailable")
			return
		}
		writeJSON(w, 200, p)
	}))
}
