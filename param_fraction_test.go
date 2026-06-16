package odbc

import "testing"

// Guards the fix for SQLSTATE 22008 ("Datetime field overflow"): the bound
// SQL_TIMESTAMP_STRUCT.Fraction must carry no more precision than the scale we
// declare to SQLBindParameter, or some ODBC drivers reject it.
func TestTruncateFraction(t *testing.T) {
	const microNow = 123456000 // .123456s in ns, e.g. a now() with µs precision
	cases := []struct {
		ns, scale, want int
	}{
		{microNow, 3, 123000000}, // ms: clean multiple of 1e6
		{microNow, 6, 123456000}, // µs: unchanged (already a multiple of 1e3)
		{microNow, 9, 123456000}, // ns: unchanged
		{microNow, 0, 0},         // no fractional seconds
		{microNow, 2, 120000000}, // centiseconds
		{999999999, 3, 999000000},
		{0, 3, 0},
	}
	for _, c := range cases {
		if got := truncateFraction(c.ns, c.scale); got != c.want {
			t.Errorf("truncateFraction(%d, %d) = %d, want %d", c.ns, c.scale, got, c.want)
		}
		// Whatever the scale, the result must be a multiple of 10^(9-scale) so the
		// fraction is consistent with the declared DecimalDigits.
		if c.scale > 0 && c.scale < 9 {
			div := 1
			for i := 0; i < 9-c.scale; i++ {
				div *= 10
			}
			if got := truncateFraction(c.ns, c.scale); got%div != 0 {
				t.Errorf("truncateFraction(%d, %d) = %d not a multiple of %d", c.ns, c.scale, got, div)
			}
		}
	}
}
