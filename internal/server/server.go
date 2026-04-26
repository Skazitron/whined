package server

import (
	"context"
	"errors"
	"io"

	whinev1 "github.com/Skazitron/whined/gen/whine/v1"
	"github.com/Skazitron/whined/internal/engine"
)

type Server struct {
	whinev1.UnimplementedWhineControlServer
	eng *engine.Engine
}

func New(e *engine.Engine) *Server {
	return &Server{eng: e}
}

func (s *Server) applyMix(m *whinev1.Mix) error {
	if m == nil {
		return errors.New("mix is nil")
	}
	s.eng.SetMasterVolume(float64(m.MasterVolume))

	incoming := make(map[int]engine.VoiceParams, len(m.Voices))
	for _, v := range m.Voices {
		incoming[int(v.VoiceId)] = engine.VoiceParams{
			Enabled:   v.Enabled,
			Volume:    float64(v.Volume),
			CutoffHz:  float64(v.CutoffHz),
			Type:      toEngineNoiseType(v.Type),
			Frequency: float64(v.Frequency),
		}
	}

	for i := 0; i < engine.MaxVoices; i++ {
		if vp, ok := incoming[i]; ok {
			if err := s.eng.SetVoice(i, vp); err != nil {
				return err
			}
		} else {
			if err := s.eng.SetVoice(i, engine.VoiceParams{Enabled: false}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) SetMix(_ context.Context, m *whinev1.Mix) (*whinev1.Ack, error) {
	if err := s.applyMix(m); err != nil {
		return &whinev1.Ack{Ok: false, Message: err.Error()}, nil
	}
	return &whinev1.Ack{Ok: true}, nil
}

func (s *Server) StreamMix(stream whinev1.WhineControl_StreamMixServer) error {
	for {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.applyMix(m); err != nil {
			if sendErr := stream.Send(&whinev1.Ack{Ok: false, Message: err.Error()}); sendErr != nil {
				return sendErr
			}
			continue
		}
		if err := stream.Send(&whinev1.Ack{Ok: true}); err != nil {
			return err
		}
	}
}

func (s *Server) GetStatus(_ context.Context, _ *whinev1.Empty) (*whinev1.Status, error) {
	all := s.eng.AllVoices()
	voices := make([]*whinev1.VoiceParams, 0, len(all))
	for i, v := range all {
		voices = append(voices, voiceToProto(i, v))
	}
	return &whinev1.Status{
		Playing: s.eng.IsPlaying(),
		Mix: &whinev1.Mix{
			MasterVolume: float32(s.eng.GetMasterVolume()),
			Voices:       voices,
		},
	}, nil
}

func (s *Server) Play(_ context.Context, req *whinev1.FadeRequest) (*whinev1.Ack, error) {
	s.eng.Play(int(req.FadeMs))
	return &whinev1.Ack{Ok: true}, nil
}

func (s *Server) Pause(_ context.Context, req *whinev1.FadeRequest) (*whinev1.Ack, error) {
	s.eng.Pause(int(req.FadeMs))
	return &whinev1.Ack{Ok: true}, nil
}

// --- Translation helpers ---

func voiceToProto(id int, v engine.VoiceParams) *whinev1.VoiceParams {
	return &whinev1.VoiceParams{
		VoiceId:   int32(id),
		Enabled:   v.Enabled,
		Volume:    float32(v.Volume),
		CutoffHz:  float32(v.CutoffHz),
		Type:      toProtoNoiseType(v.Type),
		Frequency: float32(v.Frequency),
	}
}

func toEngineNoiseType(t whinev1.NoiseType) engine.NoiseType {
	switch t {
	case whinev1.NoiseType_NOISE_TYPE_PINK:
		return engine.NoisePink
	case whinev1.NoiseType_NOISE_TYPE_BROWN:
		return engine.NoiseBrown
	default:
		return engine.NoiseWhite
	}
}

func toProtoNoiseType(t engine.NoiseType) whinev1.NoiseType {
	switch t {
	case engine.NoisePink:
		return whinev1.NoiseType_NOISE_TYPE_PINK
	case engine.NoiseBrown:
		return whinev1.NoiseType_NOISE_TYPE_BROWN
	default:
		return whinev1.NoiseType_NOISE_TYPE_WHITE
	}
}
