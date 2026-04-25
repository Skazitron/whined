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
