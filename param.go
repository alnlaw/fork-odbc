// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package odbc

import (
	"database/sql/driver"
	"fmt"
	"time"
	"unsafe"

	"github.com/alexbrainman/odbc/api"
)

type Parameter struct {
	SQLType     api.SQLSMALLINT
	Decimal     api.SQLSMALLINT
	Size        api.SQLULEN
	isDescribed bool
	// Following fields store data used later by SQLExecute.
	// The fields keep data alive and away from gc.
	Data             interface{}
	StrLen_or_IndPtr api.SQLLEN
}

// StoreStrLen_or_IndPtr stores v into StrLen_or_IndPtr field of p
// and returns address of that field.
func (p *Parameter) StoreStrLen_or_IndPtr(v api.SQLLEN) *api.SQLLEN {
	p.StrLen_or_IndPtr = v
	return &p.StrLen_or_IndPtr

}

func (p *Parameter) BindValue(h api.SQLHSTMT, idx int, v driver.Value, conn *Conn) error {
	// TODO(brainman): Reuse memory for previously bound values. If memory
	// is reused, we, probably, do not need to call SQLBindParameter either.
	var ctype, sqltype, decimal api.SQLSMALLINT
	var size api.SQLULEN
	var buflen api.SQLLEN
	var plen *api.SQLLEN
	var buf unsafe.Pointer
	switch d := v.(type) {
	case nil:
		ctype = api.SQL_C_WCHAR
		p.Data = nil
		buf = nil
		size = 1
		buflen = 0
		plen = p.StoreStrLen_or_IndPtr(api.SQL_NULL_DATA)
		sqltype = api.SQL_WCHAR
	case string:
		ctype = api.SQL_C_WCHAR
		b := api.StringToUTF16(d)
		p.Data = b
		buf = unsafe.Pointer(&b[0])
		l := len(b)
		l -= 1 // remove terminating 0
		size = api.SQLULEN(l)
		if size < 1 {
			// size cannot be less then 1 even for empty fields
			size = 1
		}
		l *= 2 // every char takes 2 bytes
		buflen = api.SQLLEN(l)
		plen = p.StoreStrLen_or_IndPtr(buflen)
		if !conn.isMSAccessDriver {
			sqltype, size, decimal = stringParamBinding(size, p)
		} else {
			// MS Acess requires SQL_WLONGVARCHAR for MEMO.
			// https://docs.microsoft.com/en-us/sql/odbc/microsoft/microsoft-access-data-types
			sqltype = api.SQL_WLONGVARCHAR
		}
	case int64:
		ctype, sqltype, size, decimal = intParamBinding(d, p)
		if ctype == api.SQL_C_LONG {
			d2 := int32(d)
			p.Data = &d2
			buf = unsafe.Pointer(&d2)
		} else {
			p.Data = &d
			buf = unsafe.Pointer(&d)
		}
	case bool:
		var b byte
		if d {
			b = 1
		}
		ctype = api.SQL_C_BIT
		p.Data = &b
		buf = unsafe.Pointer(&b)
		sqltype = api.SQL_BIT
		size = 1
	case float64:
		ctype = api.SQL_C_DOUBLE
		p.Data = &d
		buf = unsafe.Pointer(&d)
		sqltype = api.SQL_DOUBLE
		size = 8
	case time.Time:
		ctype = api.SQL_C_TYPE_TIMESTAMP
		y, m, day := d.Date()
		b := api.SQL_TIMESTAMP_STRUCT{
			Year:   api.SQLSMALLINT(y),
			Month:  api.SQLUSMALLINT(m),
			Day:    api.SQLUSMALLINT(day),
			Hour:   api.SQLUSMALLINT(d.Hour()),
			Minute: api.SQLUSMALLINT(d.Minute()),
			Second: api.SQLUSMALLINT(d.Second()),
			// Fraction is set below, once the scale (decimal) is known.
		}
		p.Data = &b
		buf = unsafe.Pointer(&b)
		sqltype = api.SQL_TYPE_TIMESTAMP
		if p.isDescribed && p.SQLType == api.SQL_TYPE_TIMESTAMP {
			decimal = p.Decimal
		}
		if decimal <= 0 {
			// represented as yyyy-mm-dd hh:mm:ss.fff format in ms sql server
			decimal = 3
		}
		// The SQL_TIMESTAMP_STRUCT.Fraction is in billionths of a second, but it
		// must carry no more precision than the scale (decimal) we declare to
		// SQLBindParameter. Some ODBC drivers reject an over-precise fraction
		// with "Datetime field overflow" (SQLSTATE 22008) — e.g. a microsecond
		// timestamp (Nanosecond()=123456000) bound at scale 3. Round the fraction
		// down to `decimal` significant digits so it stays a clean multiple, as
		// pyodbc does.
		b.Fraction = api.SQLUINTEGER(truncateFraction(d.Nanosecond(), int(decimal)))
		size = 20 + api.SQLULEN(decimal)
	case []byte:
		ctype = api.SQL_C_BINARY
		b := make([]byte, len(d))
		copy(b, d)
		p.Data = b
		if len(d) > 0 {
			buf = unsafe.Pointer(&b[0])
		} else {
			buf = nil
		}
		buflen = api.SQLLEN(len(b))
		plen = p.StoreStrLen_or_IndPtr(buflen)
		size = api.SQLULEN(len(b))
		switch {
		case p.isDescribed:
			sqltype = p.SQLType
		case size <= 0:
			sqltype = api.SQL_LONGVARBINARY
		case size >= 8000:
			sqltype = api.SQL_LONGVARBINARY
		default:
			sqltype = api.SQL_BINARY
		}
	default:
		return fmt.Errorf("unsupported type %T", v)
	}
	ret := api.SQLBindParameter(h, api.SQLUSMALLINT(idx+1),
		api.SQL_PARAM_INPUT, ctype, sqltype, size, decimal,
		api.SQLPOINTER(buf), buflen, plen)
	if IsError(ret) {
		return NewError("SQLBindParameter", h)
	}
	return nil
}

// intParamBinding picks the C type, SQL type, column size and scale for binding
// an int64 parameter. When the driver described the parameter, honor the
// column's real SQL type/precision/scale — every other branch in BindValue
// already does this. Ignoring it under-declares the parameter (e.g.
// SQL_INTEGER with size 4 for a wide DECIMAL id column) and makes some ODBC
// drivers reject in-range values with SQLSTATE 22003 ("Numeric value
// out of range"). The C value always fits SQL_C_SBIGINT, so the driver converts
// it to the described type. Without a description, fall back to the value-magnitude
// heuristic: SQL_INTEGER when it fits int32 (some drivers don't support
// SQL_BIGINT, see issue #78), else SQL_BIGINT.
func intParamBinding(d int64, p *Parameter) (ctype, sqltype api.SQLSMALLINT, size api.SQLULEN, decimal api.SQLSMALLINT) {
	if p.isDescribed {
		return api.SQL_C_SBIGINT, p.SQLType, p.Size, p.Decimal
	}
	if -0x80000000 < d && d < 0x7fffffff {
		return api.SQL_C_LONG, api.SQL_INTEGER, 4, 0
	}
	return api.SQL_C_SBIGINT, api.SQL_BIGINT, 8, 0
}

// stringParamBinding decides the SQL type / column size / scale for a string
// parameter. Decimals travel as text (database/sql has no decimal driver.Value),
// so a described DECIMAL/NUMERIC param must declare the column's real
// precision/scale — not the text length with scale 0 — or some ODBC drivers
// truncate the fractional digits with SQLSTATE 22001 ("String data, right
// truncated"). Mirrors intParamBinding, which honors the described type too.
func stringParamBinding(strLen api.SQLULEN, p *Parameter) (sqltype api.SQLSMALLINT, size api.SQLULEN, decimal api.SQLSMALLINT) {
	switch {
	case strLen >= 4000:
		return api.SQL_WLONGVARCHAR, strLen, 0
	case p.isDescribed && isNumericSQLType(p.SQLType):
		return p.SQLType, p.Size, p.Decimal
	case p.isDescribed:
		return p.SQLType, strLen, 0
	case strLen <= 1:
		return api.SQL_WVARCHAR, strLen, 0
	default:
		return api.SQL_WCHAR, strLen, 0
	}
}

func isNumericSQLType(t api.SQLSMALLINT) bool {
	return t == api.SQL_DECIMAL || t == api.SQL_NUMERIC
}

// truncateFraction rounds a sub-second value (nanoseconds, 0..999999999) down to
// `scale` fractional digits, returned in nanoseconds. scale<=0 yields 0; scale>=9
// leaves it unchanged. So scale=3 keeps milliseconds (a multiple of 1e6), scale=6
// keeps microseconds (a multiple of 1e3) — matching what the bound DecimalDigits
// promises, so drivers don't see an over-precise fraction.
func truncateFraction(ns, scale int) int {
	if scale <= 0 {
		return 0
	}
	if scale >= 9 {
		return ns
	}
	div := 1
	for i := 0; i < 9-scale; i++ {
		div *= 10
	}
	return (ns / div) * div
}

func ExtractParameters(h api.SQLHSTMT) ([]Parameter, error) {
	// count parameters
	var n, nullable api.SQLSMALLINT
	ret := api.SQLNumParams(h, &n)
	if IsError(ret) {
		return nil, NewError("SQLNumParams", h)
	}
	if n <= 0 {
		// no parameters
		return nil, nil
	}
	ps := make([]Parameter, n)
	// fetch param descriptions
	for i := range ps {
		p := &ps[i]
		ret = api.SQLDescribeParam(h, api.SQLUSMALLINT(i+1),
			&p.SQLType, &p.Size, &p.Decimal, &nullable)
		if IsError(ret) {
			// SQLDescribeParam is not implemented by freedts,
			// it even fails for some statements on windows.
			// Will try request without these descriptions
			continue
		}
		p.isDescribed = true
		// SQL Server MAX types (varchar(max), nvarchar(max),
		// varbinary(max) are identified by size = 0
		if p.Size == 0 {
			switch p.SQLType {
			case api.SQL_VARBINARY:
				p.SQLType = api.SQL_LONGVARBINARY
			case api.SQL_VARCHAR:
				p.SQLType = api.SQL_LONGVARCHAR
			case api.SQL_WVARCHAR:
				p.SQLType = api.SQL_WLONGVARCHAR
			}
		}
	}
	return ps, nil
}
