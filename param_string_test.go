package odbc

import (
	"testing"

	"github.com/alexbrainman/odbc/api"
)

// Guards the fix for SQLSTATE 22001 ("String data, right truncated"): decimals
// travel as text, so a described DECIMAL/NUMERIC param must declare the column's
// real precision/scale instead of the text length with scale 0. Otherwise
// some ODBC drivers see the param as DECIMAL(len,0) and truncate the
// fractional digits (e.g. binding "3174.69" as DECIMAL(7,0)).
func TestStringParamBinding(t *testing.T) {
	describedDecimal := &Parameter{isDescribed: true, SQLType: api.SQL_DECIMAL, Size: 12, Decimal: 2}
	if sqltype, size, decimal := stringParamBinding(7, describedDecimal); sqltype != api.SQL_DECIMAL ||
		size != 12 || decimal != 2 {
		t.Errorf("described DECIMAL(12,2): got sqltype=%d size=%d decimal=%d", sqltype, size, decimal)
	}

	describedNumeric := &Parameter{isDescribed: true, SQLType: api.SQL_NUMERIC, Size: 18, Decimal: 4}
	if sqltype, size, decimal := stringParamBinding(9, describedNumeric); sqltype != api.SQL_NUMERIC ||
		size != 18 || decimal != 4 {
		t.Errorf("described NUMERIC(18,4): got sqltype=%d size=%d decimal=%d", sqltype, size, decimal)
	}

	// Described non-numeric (text) keeps the text length and scale 0.
	describedText := &Parameter{isDescribed: true, SQLType: api.SQL_WVARCHAR, Size: 50, Decimal: 0}
	if sqltype, size, decimal := stringParamBinding(7, describedText); sqltype != api.SQL_WVARCHAR ||
		size != 7 || decimal != 0 {
		t.Errorf("described WVARCHAR: got sqltype=%d size=%d decimal=%d", sqltype, size, decimal)
	}

	undescribed := &Parameter{}
	if sqltype, size, _ := stringParamBinding(7, undescribed); sqltype != api.SQL_WCHAR || size != 7 {
		t.Errorf("undescribed WCHAR: got sqltype=%d size=%d", sqltype, size)
	}
	if sqltype, size, _ := stringParamBinding(1, undescribed); sqltype != api.SQL_WVARCHAR || size != 1 {
		t.Errorf("undescribed empty: got sqltype=%d size=%d", sqltype, size)
	}
	if sqltype, size, _ := stringParamBinding(4000, undescribed); sqltype != api.SQL_WLONGVARCHAR || size != 4000 {
		t.Errorf("undescribed long: got sqltype=%d size=%d", sqltype, size)
	}
}
