// Exposes each result column's ODBC SQL type as a database/sql
// DatabaseTypeName. The upstream driver maps both SQL_TYPE_DATE and
// SQL_TYPE_TIMESTAMP (and others) onto a Go time.Time and never surfaces the
// SQL type, so a DATE and a midnight DATETIME are indistinguishable downstream.
// With this, database/sql's ColumnType.DatabaseTypeName() returns a usable name
// and callers can map types faithfully (e.g. DATE -> date, TIMESTAMP ->
// datetime).

package odbc

import "github.com/alexbrainman/odbc/api"

// ColumnTypeDatabaseTypeName implements driver.RowsColumnTypeDatabaseTypeName.
func (r *Rows) ColumnTypeDatabaseTypeName(index int) string {
	return sqlTypeName(r.os.Cols[index].SQLTypeCode())
}

// sqlTypeName maps the standard ODBC SQL type codes to their canonical names.
func sqlTypeName(t api.SQLSMALLINT) string {
	switch t {
	case api.SQL_BIT:
		return "BIT"
	case api.SQL_TINYINT:
		return "TINYINT"
	case api.SQL_SMALLINT:
		return "SMALLINT"
	case api.SQL_INTEGER:
		return "INTEGER"
	case api.SQL_BIGINT:
		return "BIGINT"
	case api.SQL_DECIMAL:
		return "DECIMAL"
	case api.SQL_NUMERIC:
		return "NUMERIC"
	case api.SQL_REAL:
		return "REAL"
	case api.SQL_FLOAT:
		return "FLOAT"
	case api.SQL_DOUBLE:
		return "DOUBLE"
	case api.SQL_TYPE_DATE:
		return "DATE"
	case api.SQL_TYPE_TIME, api.SQL_SS_TIME2:
		return "TIME"
	case api.SQL_TYPE_TIMESTAMP:
		return "TIMESTAMP"
	case api.SQL_CHAR:
		return "CHAR"
	case api.SQL_VARCHAR:
		return "VARCHAR"
	case api.SQL_LONGVARCHAR:
		return "LONGVARCHAR"
	case api.SQL_WCHAR:
		return "WCHAR"
	case api.SQL_WVARCHAR:
		return "WVARCHAR"
	case api.SQL_WLONGVARCHAR:
		return "WLONGVARCHAR"
	case api.SQL_BINARY:
		return "BINARY"
	case api.SQL_VARBINARY:
		return "VARBINARY"
	case api.SQL_LONGVARBINARY:
		return "LONGVARBINARY"
	case api.SQL_GUID:
		return "GUID"
	default:
		return ""
	}
}
