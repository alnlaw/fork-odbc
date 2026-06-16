// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package odbc

import (
	"context"
	"database/sql/driver"
	"errors"
	"sync"

	"github.com/alexbrainman/odbc/api"
)

type Stmt struct {
	c     *Conn
	query string
	os    *ODBCStmt
	mu    sync.Mutex
}

func (c *Conn) Prepare(query string) (driver.Stmt, error) {
	if c.bad {
		return nil, driver.ErrBadConn
	}
	os, err := c.PrepareODBCStmt(query)
	if err != nil {
		return nil, err
	}
	return &Stmt{c: c, os: os, query: query}, nil
}

func (s *Stmt) NumInput() int {
	if s.os == nil {
		return -1
	}
	return len(s.os.Parameters)
}

func (s *Stmt) Close() error {
	if s.os == nil {
		return errors.New("Stmt is already closed")
	}
	ret := s.os.closeByStmt()
	s.os = nil
	return ret
}

func (s *Stmt) Exec(args []driver.Value) (driver.Result, error) {
	if s.os == nil {
		return nil, errors.New("Stmt is closed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.os.usedByRows {
		s.os.closeByStmt()
		s.os = nil
		os, err := s.c.PrepareODBCStmt(s.query)
		if err != nil {
			return nil, err
		}
		s.os = os
	}
	err := s.os.Exec(args, s.c)
	if err != nil {
		return nil, err
	}
	var sumRowCount int64
	for {
		var c api.SQLLEN
		ret := api.SQLRowCount(s.os.h, &c)
		if IsError(ret) {
			return nil, NewError("SQLRowCount", s.os.h)
		}
		sumRowCount += int64(c)
		if ret = api.SQLMoreResults(s.os.h); ret == api.SQL_NO_DATA {
			break
		}
	}
	return &Result{rowCount: sumRowCount}, nil
}

// ExecContext implements driver.StmtExecContext so a prepared statement's
// Exec honours the context. Without it, database/sql falls back to the
// uncancellable Stmt.Exec above — and an executemany-style path
// (PrepareContext + looped stmt.ExecContext) would then leave its
// INSERT/UPDATE/DELETE statements uncancellable.
// It reuses Conn.wrapExec/waitExec (see conn.go): run in a goroutine, wait on
// ctx, and SQLCancel the in-flight statement on cancellation.
func (s *Stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	dargs, err := namedValueToValue(args)
	if err != nil {
		return nil, err
	}
	if s.os == nil {
		return nil, errors.New("Stmt is closed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.os.usedByRows {
		s.os.closeByStmt()
		s.os = nil
		os, err := s.c.PrepareODBCStmt(s.query)
		if err != nil {
			return nil, err
		}
		s.os = os
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	resChan := make(chan driver.Result)
	errorChan := make(chan error)
	go func() {
		res, err := s.c.wrapExec(s.os, dargs)
		if err != nil {
			errorChan <- err
			return
		}
		resChan <- res
	}()
	return s.c.waitExec(ctx, s.os, resChan, errorChan)
}

func (s *Stmt) Query(args []driver.Value) (driver.Rows, error) {
	if s.os == nil {
		return nil, errors.New("Stmt is closed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.os.usedByRows {
		s.os.closeByStmt()
		s.os = nil
		os, err := s.c.PrepareODBCStmt(s.query)
		if err != nil {
			return nil, err
		}
		s.os = os
	}
	err := s.os.Exec(args, s.c)
	if err != nil {
		return nil, err
	}
	err = s.os.BindColumns()
	if err != nil {
		return nil, err
	}
	s.os.usedByRows = true // now both Stmt and Rows refer to it
	return &Rows{os: s.os}, nil
}
