package server

import (
	"context"
	"errors"
	"io"
	whinev1 "whine/gen/whine/v1"
	"whine/internal/engine"
)

type Server struct {
	whinev1.UnimplementedWhineControlServer
	eng *engine.Engine
}

func New(e *engine.Engine) *Server {
	return &Server{eng: e}
}

func (s *Server) SetParams(_ context.Context, p *whinev1.Params) (*whinev1.Ack, error) {
	s.eng.SetParams(toEngineParams(p))
	return &whinev1.Ack{Ok: true}, nil
}

func (s *Server) GetParams(_ context.Context, _ *whinev1.Empty) (*whinev1.Params, error) {
	return toProtoParams(s.eng.GetParams()), nil
}

func (s *Server) StreamParams(stream whinev1.WhineControl_StreamParamsServer) error {
	for {
		p, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		s.eng.SetParams(toEngineParams(p))
		if err := stream.Send(&whinev1.Ack{Ok: true}); err != nil {
			return err
		}
	}
}

func (s *Server) Play(_ context.Context, req *whinev1.PlayRequest) (*whinev1.Ack, error) {
	s.eng.Play(int(req.FadeMs))
	return &whinev1.Ack{Ok: true}, nil
}

func (s *Server) Pause(_ context.Context, req *whinev1.PauseRequest) (*whinev1.Ack, error) {
	s.eng.Pause(int(req.FadeMs))
	return &whinev1.Ack{Ok: true}, nil
}

func (s *Server) GetStatus(_ context.Context, _ *whinev1.Empty) (*whinev1.Status, error) {
	return &whinev1.Status{
		Playing: s.eng.IsPlaying(),
		Params:  toProtoParams(s.eng.GetParams()),
	}, nil
}

func toEngineParams(p *whinev1.Params) engine.Params {
	return engine.Params{
		Volume:   float64(p.Volume),
		CutoffHz: float64(p.CutoffHz),
		Type:     toEngineNoiseType(p.Type),
		// TODO: update this when building the frequency modulator
		Frequency: float64(20000),
	}
}

func toProtoParams(p engine.Params) *whinev1.Params {
	return &whinev1.Params{
		Volume:   float32(p.Volume),
		CutoffHz: float32(p.CutoffHz),
		Type:     toProtoNoiseType(p.Type),
		// TODO: update this when building the frequency modulator
		Frequency: float32(20000),
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
