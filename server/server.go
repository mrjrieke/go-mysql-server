// Copyright 2020-2021 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"crypto/tls"
	"time"

	"github.com/dolthub/vitess/go/mysql"
	"github.com/opentracing/opentracing-go"

	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/auth"
)

// Server is a MySQL server for SQLe engines.
type Server struct {
	Listener *mysql.Listener
	h        *Handler
}

// Config for the mysql server.
type Config struct {
	// Protocol for the connection.
	Protocol string
	// Address of the server.
	Address string
	// Auth of the server.
	Auth auth.Auth
	// Tracer to use in the server. By default, a noop tracer will be used if
	// no tracer is provided.
	Tracer opentracing.Tracer
	// Version string to advertise in running server
	Version string
	// ConnReadTimeout is the server's read timeout
	ConnReadTimeout time.Duration
	// ConnWriteTimeout is the server's write timeout
	ConnWriteTimeout time.Duration
	// MaxConnections is the maximum number of simultaneous connections that the server will allow.
	MaxConnections uint64
	// TLSConfig is the configuration for TLS on this server. If |nil|, TLS is not supported.
	TLSConfig *tls.Config
	// RequestSecureTransport will require incoming connections to be TLS. Requires non-|nil| TLSConfig.
	RequireSecureTransport bool
	// NoDefaults prevents using persisted configuration for new server sessions
	NoDefaults bool
}

// NewDefaultServer creates a Server with the default session builder.
func NewDefaultServer(cfg Config, e *sqle.Engine) (*Server, error) {
	return NewServer(cfg, e, DefaultSessionBuilder)
}

// NewServer creates a server with the given protocol, address, authentication
// details given a SQLe engine and a session builder.
func NewServer(cnf Config, e *sqle.Engine, sb SessionBuilder) (*Server, error) {
	var tracer opentracing.Tracer
	if cnf.Tracer != nil {
		tracer = cnf.Tracer
	} else {
		tracer = opentracing.NoopTracer{}
	}

	if cnf.ConnReadTimeout < 0 {
		cnf.ConnReadTimeout = 0
	}

	if cnf.ConnWriteTimeout < 0 {
		cnf.ConnWriteTimeout = 0
	}

	if cnf.MaxConnections < 0 {
		cnf.MaxConnections = 0
	}

	handler := NewHandler(e,
		NewSessionManager(
			sb,
			tracer,
			e.Analyzer.Catalog.HasDB,
			e.MemoryManager,
			e.ProcessList,
			cnf.Address),
		cnf.ConnReadTimeout)
	a := cnf.Auth.Mysql()
	l, err := NewListener(cnf.Protocol, cnf.Address, handler)
	if err != nil {
		return nil, err
	}

	listenerCfg := mysql.ListenerConfig{
		Listener:           l,
		AuthServer:         a,
		Handler:            handler,
		ConnReadTimeout:    cnf.ConnReadTimeout,
		ConnWriteTimeout:   cnf.ConnWriteTimeout,
		MaxConns:           cnf.MaxConnections,
		ConnReadBufferSize: mysql.DefaultConnBufferSize,
	}
	vtListnr, err := mysql.NewListenerWithConfig(listenerCfg)
	if err != nil {
		return nil, err
	}

	if cnf.Version != "" {
		vtListnr.ServerVersion = cnf.Version
	}
	vtListnr.TLSConfig = cnf.TLSConfig
	vtListnr.RequireSecureTransport = cnf.RequireSecureTransport

	return &Server{Listener: vtListnr, h: handler}, nil
}

// Start starts accepting connections on the server.
func (s *Server) Start() error {
	s.Listener.Accept()
	return nil
}

// Close closes the server connection.
func (s *Server) Close() error {
	s.Listener.Close()
	return nil
}
