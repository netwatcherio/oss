package probe_data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"netwatcher-controller/internal/probe"
	"sync"
	"time"
)

// ---- Top-level meta you want at the main level ----

type ProbeData struct {
	ID                uint            `json:"id"`
	ProbeID           uint            `json:"probeID"`
	OriginalAgentID   uint            `json:"originalAgentID"`
	SubmittingAgentID uint            `json:"submittingAgentID"`
	Triggered         bool            `json:"triggered"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	Type              probe.Type      `json:"type"`
	Payload           json.RawMessage `json:"payload"`
	// Optional: carry target string if you still resolve AGENT types dynamically
	Target      string `json:"target,omitempty"`
	TargetAgent uint   `json:"targetAgent,omitempty"`
}

// ---- Non-generic handler interface the registry stores ----

type Handler interface {
	Kind() probe.Type
	DecodeAndValidate(raw json.RawMessage) (any, error)
	Process(ctx context.Context, data ProbeData, payload any) error
}

// ---- Generic helper to build type-safe handlers ----

type TypedHandler[T any] struct {
	kind     probe.Type
	validate func(T) error
	process  func(context.Context, ProbeData, T) error
}

func NewHandler[T any](kind probe.Type,
	validate func(T) error,
	process func(context.Context, ProbeData, T) error,
) Handler {
	return &TypedHandler[T]{kind: kind, validate: validate, process: process}
}

func (h *TypedHandler[T]) Kind() probe.Type { return h.kind }

func (h *TypedHandler[T]) DecodeAndValidate(raw json.RawMessage) (any, error) {
	var p T
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode %q: %w", h.kind, err)
		}
	}
	if h.validate != nil {
		if err := h.validate(p); err != nil {
			return nil, fmt.Errorf("validate %q: %w", h.kind, err)
		}
	}
	return p, nil
}

func (h *TypedHandler[T]) Process(ctx context.Context, meta ProbeData, payload any) error {
	p, ok := payload.(T)
	if !ok {
		return fmt.Errorf("internal type assertion failed for kind %q", h.kind)
	}
	return h.process(ctx, meta, p)
}

// ---- Registry ----

type Registry struct {
	mu       sync.RWMutex
	handlers map[probe.Type]Handler
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[probe.Type]Handler)}
}

func (r *Registry) Register(h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[h.Kind()] = h
}

func (r *Registry) Has(kind probe.Type) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.handlers[kind]
	return ok
}

func (r *Registry) Dispatch(ctx context.Context, pp ProbeData) error {
	kind := pp.Type

	// Optional: if kind == AGENT, derive the real type from Target prefix "TYPE%%%..."
	if kind == probe.TypeAgent && pp.Target != "" {
		if i := indexOf(pp.Target, "%%%"); i >= 0 {
			kind = probe.Type(pp.Target[:i])
		}
	}

	r.mu.RLock()
	h, ok := r.handlers[kind]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no handler registered for kind %q", kind)
	}

	payload, err := h.DecodeAndValidate(pp.Payload)
	if err != nil {
		return err
	}
	return h.Process(ctx, pp, payload)
}

// small helper (no strings package split alloc if you only need first)
func indexOf(s, sep string) int {
	n := len(sep)
	if n == 0 {
		return 0
	}
	for i := 0; i+n <= len(s); i++ {
		if s[i:i+n] == sep {
			return i
		}
	}
	return -1
}

// Optional convenience
var Default = NewRegistry()

func Register(h Handler) { Default.Register(h) }

func Dispatch(ctx context.Context, pp ProbeData) error {
	if !Default.Has(pp.Type) && pp.Type != probe.TypeAgent {
		return errors.New("no handler registered and not an agent-derived kind")
	}
	return Default.Dispatch(ctx, pp)
}

func InitWorkers() {
	initNetInfo()
}
