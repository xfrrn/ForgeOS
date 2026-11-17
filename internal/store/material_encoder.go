package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type materialEncoder struct {
	endpoint, model string
	client          *http.Client
}

// ConfigureMaterialSearch runs before the HTTP server starts. Plain PostgreSQL remains sufficient for keyword search.
func (s *Store) ConfigureMaterialSearch(ctx context.Context, endpoint, model string) error {
	if endpoint == "" && model == "" {
		return nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(model) == "" || len(model) > 256 {
		return errors.New("configure FORGEOS_EMBEDDINGS_URL and FORGEOS_EMBEDDINGS_MODEL with an embedding endpoint and model revision")
	}
	var distance float64
	if err := s.db.QueryRowContext(ctx, `SELECT '[1,0]'::vector <=> '[0,1]'::vector`).Scan(&distance); err != nil {
		return fmt.Errorf("material search requires the pgvector extension on the database search_path: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	s.embeddings = &materialEncoder{endpoint: endpoint, model: model, client: &http.Client{
		Timeout: 90 * time.Second, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
	return nil
}

func (e *materialEncoder) encode(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, errors.New("cannot encode an empty batch")
	}
	body, err := json.Marshal(struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}{e.model, inputs})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("material encoder request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("material encoder returned HTTP %d", resp.StatusCode)
	}
	var reply struct {
		Data []struct {
			Index  *int      `json:"index"`
			Vector []float32 `json:"embedding"`
		} `json:"data"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 8<<20 || json.Unmarshal(raw, &reply) != nil || len(reply.Data) != len(inputs) {
		return nil, errors.New("invalid material encoder response")
	}
	vectors := make([][]float32, len(inputs))
	dim := len(reply.Data[0].Vector)
	for _, row := range reply.Data {
		if row.Index == nil || *row.Index < 0 || *row.Index >= len(inputs) || vectors[*row.Index] != nil || dim == 0 || dim > 16000 || len(row.Vector) != dim {
			return nil, errors.New("invalid material embedding index or dimensions")
		}
		var norm float64
		for _, x := range row.Vector {
			norm += float64(x) * float64(x)
		}
		if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
			return nil, errors.New("invalid material embedding norm")
		}
		scale := math.Sqrt(norm)
		for i, x := range row.Vector {
			row.Vector[i] = float32(float64(x) / scale)
		}
		vectors[*row.Index] = row.Vector
	}
	return vectors, nil
}

func (e *materialEncoder) queryVector(ctx context.Context, query string) ([]float32, error) {
	text := []rune(query)
	passages := materialPassages(text)
	inputs := make([]string, len(passages))
	for i, p := range passages {
		inputs[i] = string(text[p.Start:p.End])
	}
	vectors, err := e.encode(ctx, inputs)
	if err != nil {
		return nil, err
	}
	if len(vectors) == 1 {
		return vectors[0], nil
	}
	mean := make([]float32, len(vectors[0]))
	for _, v := range vectors {
		for i, x := range v {
			mean[i] += x / float32(len(vectors))
		}
	}
	for _, x := range mean {
		if x != 0 {
			return mean, nil
		}
	}
	return nil, errors.New("material encoder returned an empty query direction")
}
