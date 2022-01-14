package common

import (
	"bytes"
	"strconv"
	"time"
)

// Int64 implements json.Unmarshaler & json.Marshaler for
//  the case that we send/recv 64bit interger to FE
//  because JavaScript doesn't currently include standard support for 64-bit integer values
//  we use string in json instead
type Int64 int64

// UnmarshalJSON implements json.Unmarshaler.
// it unmarshals str form of number like `"123"`
func (n *Int64) UnmarshalJSON(b []byte) error {
	b = bytes.Trim(b, `"`) // compat str or number
	i, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return err
	}
	*n = Int64(i)
	return nil
}

// MarshalJSON implements json.Marshaler.
// it marshals int64 to str like `"123"` instead of `123`
func (n Int64) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, 30)
	b = append(b, '"')
	b = strconv.AppendInt(b, int64(n), 10)
	b = append(b, '"')
	return b, nil
}

// String converts Int64 to string
func (n Int64) String() string {
	return strconv.FormatInt(int64(n), 10)
}

// Int converts Int64 to int64
func (n Int64) Int() int64 {
	return int64(n)
}

// Int converts Int64 to int64 pointer
func (n Int64) IntPointer() *int64 {
	r := int64(n)
	return &r
}

// Int64Slice converts []Int64 to []int64
func Int64Slice(ii []Int64) []int64 {
	if ii == nil {
		return nil
	}
	ret := make([]int64, 0, len(ii))
	for _, i := range ii {
		ret = append(ret, i.Int())
	}
	return ret
}

func RawInt64Slice(ii []int64) []Int64 {
	if ii == nil {
		return nil
	}

	ret := make([]Int64, 0, len(ii))
	for _, i := range ii {
		ret = append(ret, Int64(i))
	}
	return ret
}

type TimeMS int64

func MakeTimeMS(t time.Time) TimeMS {
	return TimeMS(t.UnixNano()/1000000)
}

// Int converts Int64 to int64
func (n TimeMS) Time() time.Time {
	return time.Unix(0, int64(n) * 1000000)
}