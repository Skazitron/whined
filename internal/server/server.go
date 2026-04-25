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
