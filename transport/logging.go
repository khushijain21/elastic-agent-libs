// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package transport

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"time"

	"github.com/elastic/elastic-agent-libs/logp"
)

type loggingConn struct {
	net.Conn
	logger      *logp.Logger
	idleTimeout time.Duration
}

func LoggingDialer(d Dialer, logger *logp.Logger, idleTimeout time.Duration) Dialer {
	return DialerFunc(func(ctx context.Context, network, addr string) (net.Conn, error) {
		logger := logger.With("network.transport", network, "server.address", addr)
		c, err := d.DialContext(ctx, network, addr)
		if err != nil {
			logger.Errorf("Error dialing %v", err)
			return nil, err
		}

		logger.Debugf("Completed dialing successfully")
		return &loggingConn{c, logger, idleTimeout}, nil
	})
}

func (l *loggingConn) Read(b []byte) (int, error) {

	// Update the deadline for the connection to expire after idleTimeout duration
	err := l.Conn.SetDeadline(time.Now().Add(l.idleTimeout))
	if err != nil {
		l.logger.Errorf("Error setting deadline: %v", err)
		return 0, err
	}

	// Attempt to read from the connection
	n, err := l.Conn.Read(b)

	// Handle errors, including timeout
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			l.logger.Debugf("Idle connection timeout reached: %v", err)
			return n, err
		}
		if err != io.EOF {
			l.logger.Debugf("Error reading from connection: %v", err)
		}
		return n, err
	}

	// Reset the deadline again after a successful read
	err = l.Conn.SetDeadline(time.Now().Add(l.idleTimeout))
	if err != nil {
		l.logger.Errorf("Error resetting deadline: %v", err)
		return n, err
	}

	return n, nil
}

func (l *loggingConn) Write(b []byte) (int, error) {
	n, err := l.Conn.Write(b)
	if err != nil && !errors.Is(err, io.EOF) {
		l.logger.Debugf("Error writing to connection: %v", err)
	}
	return n, err
}
