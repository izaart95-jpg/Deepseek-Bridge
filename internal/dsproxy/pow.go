package dsproxy

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed wasm/sha3_wasm_bg.7b9ca65ddd.wasm
var powWasm []byte

// Challenge is the proof-of-work challenge payload returned by
// /chat/create_pow_challenge (data.biz_data.challenge).
type Challenge struct {
	Algorithm  string `json:"algorithm"`
	Challenge  string `json:"challenge"`
	Salt       string `json:"salt"`
	Difficulty int    `json:"difficulty"`
	ExpireAt   int    `json:"expire_at"`
	Signature  string `json:"signature"`
	TargetPath string `json:"target_path"`
}

// DeepSeekHash wraps the sha3 WASM module (wasmtime -> wazero port).
type DeepSeekHash struct {
	mod    api.Module
	memory api.Memory
	malloc api.Function
	stack  api.Function
	solve  api.Function
}

func NewDeepSeekHash() (*DeepSeekHash, error) {
	ctx := context.Background()
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCloseOnContextDone(true))
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		return nil, fmt.Errorf("wasi instantiate: %w", err)
	}
	compiled, err := rt.CompileModule(ctx, powWasm)
	if err != nil {
		return nil, fmt.Errorf("compile wasm: %w", err)
	}
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	if err != nil {
		return nil, fmt.Errorf("instantiate wasm: %w", err)
	}
	h := &DeepSeekHash{mod: mod, memory: mod.Memory()}
	if h.malloc = mod.ExportedFunction("__wbindgen_export_0"); h.malloc == nil {
		return nil, fmt.Errorf("wasm export __wbindgen_export_0 missing")
	}
	if h.stack = mod.ExportedFunction("__wbindgen_add_to_stack_pointer"); h.stack == nil {
		return nil, fmt.Errorf("wasm export __wbindgen_add_to_stack_pointer missing")
	}
	if h.solve = mod.ExportedFunction("wasm_solve"); h.solve == nil {
		return nil, fmt.Errorf("wasm export wasm_solve missing")
	}
	return h, nil
}

func (h *DeepSeekHash) writeMemory(text string) (uint32, uint32, error) {
	b := []byte(text)
	length := uint32(len(b))
	ctx := context.Background()
	res, err := h.malloc.Call(ctx, uint64(length), 1)
	if err != nil {
		return 0, 0, fmt.Errorf("wasm malloc: %w", err)
	}
	ptr := uint32(res[0])
	if !h.memory.Write(ptr, b) {
		return 0, 0, fmt.Errorf("wasm memory write failed")
	}
	return ptr, length, nil
}

// CalculateHash solves the challenge inside the WASM module. The second
// return value is false when the solver reports status 0 (no answer found).
func (h *DeepSeekHash) CalculateHash(algorithm, challenge, salt string, difficulty int, expireAt int) (int64, bool, error) {
	ctx := context.Background()
	prefix := fmt.Sprintf("%s_%d_", salt, expireAt)

	var sixteen uint64 = 16
	res, err := h.stack.Call(ctx, 0-sixteen)
	if err != nil {
		return 0, false, fmt.Errorf("wasm stack adjust: %w", err)
	}
	retptr := uint32(res[0])
	defer h.stack.Call(ctx, 16)

	chPtr, chLen, err := h.writeMemory(challenge)
	if err != nil {
		return 0, false, err
	}
	prePtr, preLen, err := h.writeMemory(prefix)
	if err != nil {
		return 0, false, err
	}

	if _, err := h.solve.Call(ctx,
		uint64(retptr), uint64(chPtr), uint64(chLen),
		uint64(prePtr), uint64(preLen),
		math.Float64bits(float64(difficulty)),
	); err != nil {
		return 0, false, fmt.Errorf("wasm solve: %w", err)
	}

	statusBytes, ok := h.memory.Read(retptr, 4)
	if !ok {
		return 0, false, fmt.Errorf("wasm memory read (status) failed")
	}
	status := int32(binary.LittleEndian.Uint32(statusBytes))
	if status == 0 {
		return 0, false, nil
	}

	valueBytes, ok := h.memory.Read(retptr+8, 8)
	if !ok {
		return 0, false, fmt.Errorf("wasm memory read (answer) failed")
	}
	value := math.Float64frombits(binary.LittleEndian.Uint64(valueBytes))
	return int64(value), true, nil
}

// DeepSeekPOW solves challenges and encodes the base64 response header value.
type DeepSeekPOW struct {
	mu     sync.Mutex
	hasher *DeepSeekHash
}

func NewDeepSeekPOW() (*DeepSeekPOW, error) {
	hasher, err := NewDeepSeekHash()
	if err != nil {
		return nil, err
	}
	return &DeepSeekPOW{hasher: hasher}, nil
}

// SolveChallenge returns the x-ds-pow-response header value for a challenge.
func (p *DeepSeekPOW) SolveChallenge(config Challenge) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	answer, ok, err := p.hasher.CalculateHash(
		config.Algorithm,
		config.Challenge,
		config.Salt,
		config.Difficulty,
		config.ExpireAt,
	)
	if err != nil {
		return "", err
	}

	var answerJSON any
	if ok {
		answerJSON = answer
	}

	result := map[string]any{
		"algorithm":   config.Algorithm,
		"challenge":   config.Challenge,
		"salt":        config.Salt,
		"answer":      answerJSON,
		"signature":   config.Signature,
		"target_path": config.TargetPath,
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("pow result marshal: %w", err)
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}
