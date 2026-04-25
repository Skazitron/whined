package engine

import (
	"fmt"
	"github.com/gen2brain/malgo"
	"math"
	"math/rand/v2"
	"sync/atomic"
)

const SampleRate = 48000

type NoiseType = int32

const (
	NoiseWhite NoiseType = 0
	NoisePink  NoiseType = 1
	NoiseBrown NoiseType = 2
)

type Params struct {
	Volume   float64
	CutoffHz float64
	Type     NoiseType
}

type Engine struct {
	volume    atomic.Uint64
	cutoff    atomic.Uint64
	noiseType atomic.Int32

	lpPrev    float64
	brownPrev float64

	b0, b1, b2, b3, b4, b5, b6 float64

	mctx *malgo.AllocatedContext
	dev  *malgo.Device
}

func New() *Engine {
	e := &Engine{}
	e.SetParams(Params{
		Volume:   0.02,
		CutoffHz: 8000,
		Type:     NoiseWhite,
	})
	return e
}

func (e *Engine) SetParams(p Params) {
	e.volume.Store(math.Float64bits(clamp(p.Volume, 0, 1)))
	e.cutoff.Store(math.Float64bits(clamp(p.CutoffHz, 20, 20000)))
	e.noiseType.Store(int32(p.Type))
}

func (e *Engine) Start() error {
	mctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return fmt.Errorf("malgo init context: %w", err)
	}
	e.mctx = mctx

	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format = malgo.FormatF32
	cfg.SampleRate = SampleRate
	cfg.Playback.Channels = 1
	cfg.Alsa.NoMMap = 1

	cb := malgo.DeviceCallbacks{
		Data: e.audioCallback,
	}

	dev, err := malgo.InitDevice(mctx.Context, cfg, cb)
	if err != nil {
		return fmt.Errorf("malgo init device: %w", err)
	}
	e.dev = dev
	if err := dev.Start(); err != nil {
		return fmt.Errorf("malgo start: %w", err)
	}
	return nil
}

func (e *Engine) Stop() {
	if e.dev != nil {
		e.dev.Uninit()
		e.dev = nil
	}
	if e.mctx != nil {
		_ = e.mctx.Uninit()
		e.mctx.Free()
		e.mctx = nil
	}
}

func (e *Engine) audioCallback(out, _ []byte, frames uint32) {
	vol := math.Float64frombits(e.volume.Load())
	cutoff := math.Float64frombits(e.cutoff.Load())
	nt := NoiseType(e.noiseType.Load())

	// Lowpass coefficient
	a := 1 - math.Exp(-2*math.Pi*cutoff/SampleRate)
	for i := range frames {
		s := e.generateSample(nt, a, vol)
		writeFloat32LE(out[i*4:], float32(s))
	}
}

func (e *Engine) generateSample(nt NoiseType, lpCoef, vol float64) float64 {
	white := rand.Float64()*2 - 1

	var n float64
	switch nt {
	case NoiseWhite:
		n = white
	case NoisePink:
		e.b0 = 0.99886*e.b0 + white*0.0555179
		e.b1 = 0.99332*e.b1 + white*0.0750759
		e.b2 = 0.96900*e.b2 + white*0.1538520
		e.b3 = 0.86650*e.b3 + white*0.3104856
		e.b4 = 0.55000*e.b4 + white*0.5329522
		e.b5 = -0.7616*e.b5 - white*0.0168980
		n = (e.b0 + e.b1 + e.b2 + e.b3 + e.b4 + e.b5 + e.b6 + white*0.5362) * 0.11
		e.b6 = white * 0.115926
	case NoiseBrown:
		e.brownPrev = (e.brownPrev + 0.02*white) / 1.02
		n = e.brownPrev * 3.5
	}
	e.lpPrev += lpCoef * (n - e.lpPrev)

	return e.lpPrev * vol
}

func writeFloat32LE(buf []byte, f float32) {
	bits := math.Float32bits(f)
	buf[0] = byte(bits)
	buf[1] = byte(bits >> 8)
	buf[2] = byte(bits >> 16)
	buf[3] = byte(bits >> 24)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
