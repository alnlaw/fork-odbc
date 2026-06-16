package odbc

import (
	"testing"

	"github.com/alexbrainman/odbc/api"
)

// Guards the fix for SQLSTATE 22003 ("Numeric value out of range"): when the
// driver described the parameter, the int64 binding must honor the column's real
// SQL type/precision/scale instead of picking SQL_INTEGER/SQL_BIGINT from the
// value's magnitude. Some ODBC drivers reject an in-range id otherwise (e.g. a
// wide DECIMAL id column described, but bound as SQL_INTEGER with size 4).
func TestIntParamBinding(t *testing.T) {
	described := &Parameter{isDescribed: true, SQLType: api.SQL_DECIMAL, Size: 18, Decimal: 0}
	if ctype, sqltype, size, decimal := intParamBinding(305328908, described); ctype != api.SQL_C_SBIGINT ||
		sqltype != api.SQL_DECIMAL || size != 18 || decimal != 0 {
		t.Errorf("described DECIMAL(18): got ctype=%d sqltype=%d size=%d decimal=%d",
			ctype, sqltype, size, decimal)
	}

	undescribed := &Parameter{}
	if ctype, sqltype, size, _ := intParamBinding(42, undescribed); ctype != api.SQL_C_LONG ||
		sqltype != api.SQL_INTEGER || size != 4 {
		t.Errorf("undescribed int32: got ctype=%d sqltype=%d size=%d", ctype, sqltype, size)
	}
	if ctype, sqltype, size, _ := intParamBinding(1<<40, undescribed); ctype != api.SQL_C_SBIGINT ||
		sqltype != api.SQL_BIGINT || size != 8 {
		t.Errorf("undescribed bigint: got ctype=%d sqltype=%d size=%d", ctype, sqltype, size)
	}
}
