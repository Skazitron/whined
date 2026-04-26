package engine

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sync/atomic"

	"github.com/gen2brain/malgo"
)

const SampleRate = 48000
const MaxVoices = 5

type NoiseType = int32

const (
	NoiseWhite NoiseType = 0
	NoisePink  NoiseType = 1
	NoiseBrown NoiseType = 2
)

type VoiceParams struct {
	Enabled   bool
	Volume    float64
	CutoffHz  float64
	Type      NoiseType
	Frequency float64
}

type voice struct {
	enabled   atomic.Bool
	volume    atomic.Uint64
	cutoff    atomic.Uint64
	noiseType atomic.Int32
	frequency atomic.Uint64
	lpPrev    float64
	brownPrev float64

	b0, b1, b2, b3, b4, b5, b6 float64
}

func (v *voice) setParams(p VoiceParams) {
	v.enabled.Store(p.Enabled)
	v.volume.Store(math.Float64bits(clamp(p.Volume, 0, 1)))
	v.cutoff.Store(math.Float64bits(clamp(p.CutoffHz, 20, 20000)))
	v.noiseType.Store(int32(p.Type))
	v.frequency.Store(math.Float64bits(clamp(p.Frequency, 0, 20000)))
}

func (v *voice) getParams() VoiceParams {
	return VoiceParams{
		Enabled:   v.enabled.Load(),
		Volume:    math.Float64frombits(v.volume.Load()),
		CutoffHz:  math.Float64frombits(v.cutoff.Load()),
		Type:      NoiseType(v.noiseType.Load()),
		Frequency: math.Float64frombits(v.frequency.Load()),
	}
}

func (v *voice) generate(lpCoef float64) float64 {
	if !v.enabled.Load() {
		return 0
	}
	vol := math.Float64frombits(v.volume.Load())
	nt := NoiseType(v.noiseType.Load())

	white := rand.Float64()*2 - 1

	var n float64
	switch nt {
	case NoiseWhite:
		n = white
	case NoisePink:
		v.b0 = 0.99886*v.b0 + white*0.0555179
		v.b1 = 0.99332*v.b1 + white*0.0750759
		v.b2 = 0.96900*v.b2 + white*0.1538520
		v.b3 = 0.86650*v.b3 + white*0.3104856
		v.b4 = 0.55000*v.b4 + white*0.5329522
		v.b5 = -0.7616*v.b5 - white*0.0168980
		n = (v.b0 + v.b1 + v.b2 + v.b3 + v.b4 + v.b5 + v.b6 + white*0.5362) * 0.11
		v.b6 = white * 0.115926
	case NoiseBrown:
		v.brownPrev = (v.brownPrev + 0.02*white) / 1.02
		n = v.brownPrev * 3.5
	}

	v.lpPrev += lpCoef * (n - v.lpPrev)
	return v.lpPrev * vol
}

func (v *voice) cutOffCoef() float64 {
	c := math.Float64frombits(v.cutoff.Load())
	return 1 - math.Exp(-2*math.Pi*c/SampleRate)
}

type Engine struct {
	voices [MaxVoices]voice

	masterVolume atomic.Uint64

	playTarget atomic.Uint64
	fadeCoef   atomic.Uint64

	currentGain float64

	mctx *malgo.AllocatedContext
	dev  *malgo.Device
}

func New() *Engine {
	e := &Engine{}
	e.SetMasterVolume(0.5)
	e.playTarget.Store(math.Float64bits(0))
	e.fadeCoef.Store(math.Float64bits(1.0))
	return e
}

func (e *Engine) SetMasterVolume(vol float64) {
	e.masterVolume.Store(math.Float64bits(clamp(vol, 0, 1)))
}

func (e *Engine) GetMasterVolume() float64 {
	return math.Float64frombits(e.masterVolume.Load())
}

func (e *Engine) SetVoice(voiceId int, p VoiceParams) error {
	if voiceId < 0 || voiceId >= MaxVoices {
		return fmt.Errorf("void id %d out of range [0, %d]", voiceId, MaxVoices)
	}
	e.voices[voiceId].setParams(p)
	return nil
}

func (e *Engine) GetVoice(voiceId int) (VoiceParams, error) {
	if voiceId < 0 || voiceId >= MaxVoices {
		return VoiceParams{}, fmt.Errorf("voiceId %d out of range [0, %d]", voiceId, MaxVoices)
	}
	return e.voices[voiceId].getParams(), nil
}

func (e *Engine) AllVoices() [MaxVoices]VoiceParams {
	var out [MaxVoices]VoiceParams
	for i := range e.voices {
		out[i] = e.voices[i].getParams()
	}
	return out
}

func (e *Engine) Play(fadeMs int) {
	e.setEnvelope(1.0, fadeMs)
}

func (e *Engine) Pause(fadeMs int) {
	e.setEnvelope(0.0, fadeMs)
}

func (e *Engine) IsPlaying() bool {
	return math.Float64frombits(e.playTarget.Load()) > 0.5
}

func (e *Engine) setEnvelope(target float64, fadeMs int) {
	var coef float64
	if fadeMs <= 0 {
		coef = 1.0
	} else {
		tauSamples := float64(fadeMs) * SampleRate / 1000.0 / 4.6
		coef = 1 - math.Exp(-1/tauSamples)
	}
	e.fadeCoef.Store(math.Float64bits(coef))
	e.playTarget.Store(math.Float64bits(target))
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
	master := math.Float64frombits(e.masterVolume.Load())
	target := math.Float64frombits(e.playTarget.Load())
	fadeCoef := math.Float64frombits(e.fadeCoef.Load())

	var coefs [MaxVoices]float64
	for i := range e.voices {
		coefs[i] = e.voices[i].cutOffCoef()
	}

	for i := uint32(0); i < frames; i++ {
		var mix float64 = 0
		for j := range e.voices {
			mix += e.voices[j].generate(coefs[j])
		}

		e.currentGain += fadeCoef * (target - e.currentGain)
		s := mix * master * e.currentGain

		if s > 1.0 {
			s = 1.0
		} else if s < -1.0 {
			s = -1.0
		}
		writeFloat32LE(out[i*4:], float32(s))
	}
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
