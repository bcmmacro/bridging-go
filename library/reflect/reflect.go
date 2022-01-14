package reflect

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

type ReflectValue struct {
	v reflect.Value
}

func NewReflectValue(o reflect.Value) *ReflectValue {
	return &ReflectValue{v: o}
}

// IsPointer returns true it is a pointer
func (r *ReflectValue) IsPointer() bool {
	return r.v.Kind() == reflect.Ptr
}

// Elem returns Elem of the input reflect.Value,
// it returns the reflect.Value itself, if it isn't reflect.Ptr nor reflect.Interface
func (r *ReflectValue) Elem() reflect.Value {
	ret := r.v
	for ret.Kind() == reflect.Ptr || ret.Kind() == reflect.Interface {
		ret = ret.Elem()
	}
	// if the value is not pointer or interface, we still can get fields
	// but can not set, so UnsetField or SetField may fail
	return ret
}

func ReflectValueOf(o interface{}) *ReflectValue {
	return NewReflectValue(reflect.ValueOf(o))
}

func (r *ReflectValue) GetInterface() interface{} {
	return r.v.Interface()
}

func (r *ReflectValue) GetField(name string) (reflect.Value, error) {
	return getField(r.Elem(), name, true)
}

func (r *ReflectValue) HasField(name string) bool {
	_, err := getField(r.Elem(), name, true)
	return err == nil
}

func (r *ReflectValue) UnsetField(name string) error {
	fv, err := getField(r.Elem(), name, false)
	if err != nil {
		return err
	}
	if fv.Kind() != reflect.Ptr {
		return fmt.Errorf("field %q not pointer", name)
	}
	fv.Set(reflect.Zero(fv.Type()))
	return nil
}

func (r *ReflectValue) SetField(name string, o interface{}) error {
	fv, err := getField(r.Elem(), name, false)
	if err != nil {
		return err
	}
	v := reflect.ValueOf(o)
	if v.Type() != fv.Type() {
		return fmt.Errorf("field %q type not match: %s", name, fv.Type())
	}
	fv.Set(v)
	return nil
}

// SetFieldByStr for non-slice param, use first element, for slice param, use all elements
func (r *ReflectValue) SetFieldByStr(name string, vv ...string) error {
	fv, err := getField(r.Elem(), name, false)
	if err != nil {
		return err
	}
	if err := setFieldValue(fv, vv...); err != nil {
		return fmt.Errorf("field %q set err: %s", name, err)
	}
	return nil
}

// can get nested field using dot separated name like "x.y"
// if x is a pointer and not readonly, new object will be created and set to x
func getField(v reflect.Value, name string, readonly bool) (_ reflect.Value, err error) {
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, errors.New("not struct")
	}
	child := ""
	if p := strings.SplitN(name, ".", 2); len(p) == 2 { // extract top level name and child name
		name, child = p[0], p[1]
	}
	fv := v.FieldByName(name)
	if !fv.IsValid() {
		return reflect.Value{}, fmt.Errorf("field %q not found", name)
	}
	if !fv.CanSet() && !readonly {
		return reflect.Value{}, fmt.Errorf("field %q cant set", name)
	}
	if child != "" {
		if fv.Type().Kind() == reflect.Ptr {
			if fv.IsNil() {
				if readonly {
					return reflect.Value{}, fmt.Errorf("field %q is nil", name)
				}
				fv.Set(reflect.New(fv.Type().Elem()))
				defer func(fv reflect.Value) { // undo new object if fails
					if err != nil {
						fv.Set(reflect.Zero(fv.Type()))
					}
				}(fv)
			}
			fv = fv.Elem()
		}
		return getField(fv, child, readonly)
	}
	return fv, nil
}

func setFieldValue(fv reflect.Value, vv ...string) error {
	if !canSetField(fv.Type()) {
		return errors.New("not supported")
	}
	if len(vv) == 0 {
		return nil
	}
	p := fv
	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			fv.Set(reflect.New(fv.Type().Elem()))
		}
		p = fv.Elem()
	}
	v := vv[0] // for convenience
	switch p.Kind() {
	case reflect.Bool:
		n, _ := strconv.ParseBool(v)
		p.SetBool(n)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(v, 0, 64)
		if err != nil {
			return err
		}
		p.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(v, 0, 64)
		if err != nil {
			return err
		}
		p.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return err
		}
		p.SetFloat(n)
	case reflect.String:
		p.SetString(v)
	case reflect.Slice:
		slice := reflect.MakeSlice(p.Type(), len(vv), len(vv))
		for i, v := range vv {
			e := slice.Index(i)
			if err := setFieldValue(e, v); err != nil {
				return err
			}
		}
		p.Set(slice)
	default:
		panic("can't set unknown type field")
	}
	return nil
}

func canSetField(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
	case reflect.Float32, reflect.Float64:
	case reflect.String:
	case reflect.Slice:
		return canSetField(t.Elem())
	default:
		return false
	}
	return true
}

// URLValues extracts fields from input and convert it to url.Values
// type A struct {
//       I int     `query:"i"`
//       J Int     `query:"j"`
//       K float32 `query:"k"`
//       X []int   `query:"x"`
//       Y *A      `query:"y"`
// }
// a := &A{I: 1, J: 2, K: 3.3, Z: true}
// a.Y = &A{I: 4, J: 5, K: 6.6, X: []int{7, 8, 9}, Z: false}
// fmt.Println(URLValues(a))
// OUT: map[i:[1] j:[2] k:[3.3] y.i:[4] y.j:[5] y.k:[6.6] y.x:[7 8 9] z:[true]]
func URLValues(i interface{}) url.Values {
	q := make(url.Values)
	v := reflect.ValueOf(i)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		panic("not struct")
	}
	populateURLValues("", q, v)
	return q
}

func populateURLValues(parent string, q url.Values, v reflect.Value) {
	keyPrefix := ""
	if len(parent) > 0 {
		keyPrefix = parent + "."
		if strings.Count(keyPrefix, ".") > 16 {
			panic("too many nested struct: " + parent)
		}
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		ft := t.Field(i)
		fv := v.Field(i)
		name, opts := ParseTag(ft.Tag.Get("query"))
		if name == "" || name == "-" {
			continue
		}
		if opts.Contains("omitempty") && isEmptyValue(fv) {
			continue
		}
		if fv.Kind() == reflect.Ptr {
			if fv.IsNil() {
				continue
			}
			fv = fv.Elem()
		}
		if fv.Kind() == reflect.Struct {
			populateURLValues(keyPrefix+name, q, fv)
		} else if fv.Kind() == reflect.Slice || fv.Kind() == reflect.Array {
			for i := 0; i < fv.Len(); i++ {
				e := fv.Index(i)
				if e.Kind() == reflect.Ptr {
					if e.IsNil() {
						continue
					}
					e = e.Elem()
				}
				if !canEncodeField2Query(e.Type()) {
					continue
				}
				q.Add(keyPrefix+name, fmt.Sprint(e.Interface()))
			}
		} else if canEncodeField2Query(fv.Type()) {
			q.Add(keyPrefix+name, fmt.Sprint(fv.Interface()))
		}
	}
}

func canEncodeField2Query(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
	case reflect.Float32, reflect.Float64:
	case reflect.String:
	default:
		return false
	}
	return true
}

func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}
