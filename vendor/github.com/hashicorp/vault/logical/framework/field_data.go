package framework

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/hashicorp/vault/helper/parseutil"
	"github.com/hashicorp/vault/helper/strutil"
	"github.com/mitchellh/mapstructure"
)

// FieldData is the structure passed to the callback to handle a path
// containing the populated parameters for fields. This should be used
// instead of the raw (*vault.Request).Data to access data in a type-safe
// way.
type FieldData struct {
	Raw    map[string]interface{}
	Schema map[string]*FieldSchema
}

// Validate cycles through raw data and validate conversions in
// the schema, so we don't get an error/panic later when
// trying to get data out.  Data not in the schema is not
// an error at this point, so we don't worry about it.
func (d *FieldData) Validate() error {
	for field, value := range d.Raw {

		schema, ok := d.Schema[field]
		if !ok {
			continue
		}

		switch schema.Type {
		case TypeBool, TypeInt, TypeMap, TypeDurationSecond, TypeString,
			TypeNameString, TypeSlice, TypeStringSlice, TypeCommaStringSlice:
			_, _, err := d.getPrimitive(field, schema)
			if err != nil {
				return fmt.Errorf("Error converting input %v for field %s: %s", value, field, err)
			}
		default:
			return fmt.Errorf("unknown field type %s for field %s",
				schema.Type, field)
		}
	}

	return nil
}

// Get gets the value for the given field. If the key is an invalid field,
// FieldData will panic. If you want a safer version of this method, use
// GetOk. If the field k is not set, the default value (if set) will be
// returned, otherwise the zero value will be returned.
func (d *FieldData) Get(k string) interface{} {
	schema, ok := d.Schema[k]
	if !ok {
		panic(fmt.Sprintf("field %s not in the schema", k))
	}

	value, ok := d.GetOk(k)
	if !ok {
		value = schema.DefaultOrZero()
	}

	return value
}

// GetDefaultOrZero gets the default value set on the schema for the given
// field. If there is no default value set, the zero value of the type
// will be returned.
func (d *FieldData) GetDefaultOrZero(k string) interface{} {
	schema, ok := d.Schema[k]
	if !ok {
		panic(fmt.Sprintf("field %s not in the schema", k))
	}

	return schema.DefaultOrZero()
}

// GetOk gets the value for the given field. The second return value
// will be false if the key is invalid or the key is not set at all.
func (d *FieldData) GetOk(k string) (interface{}, bool) {
	schema, ok := d.Schema[k]
	if !ok {
		return nil, false
	}

	result, ok, err := d.GetOkErr(k)
	if err != nil {
		panic(fmt.Sprintf("error reading %s: %s", k, err))
	}

	if ok && result == nil {
		result = schema.DefaultOrZero()
	}

	return result, ok
}

// GetOkErr is the most conservative of all the Get methods. It returns
// whether key is set or not, but also an error value. The error value is
// non-nil if the field doesn't exist or there was an error parsing the
// field value.
func (d *FieldData) GetOkErr(k string) (interface{}, bool, error) {
	schema, ok := d.Schema[k]
	if !ok {
		return nil, false, fmt.Errorf("unknown field: %s", k)
	}

	switch schema.Type {
	case TypeBool, TypeInt, TypeMap, TypeDurationSecond, TypeString,
		TypeNameString, TypeSlice, TypeStringSlice, TypeCommaStringSlice:
		return d.getPrimitive(k, schema)
	default:
		return nil, false,
			fmt.Errorf("unknown field type %s for field %s", schema.Type, k)
	}
}

func (d *FieldData) getPrimitive(
	k string, schema *FieldSchema) (interface{}, bool, error) {
	raw, ok := d.Raw[k]
	if !ok {
		return nil, false, nil
	}

	switch schema.Type {
	case TypeBool:
		var result bool
		if err := mapstructure.WeakDecode(raw, &result); err != nil {
			return nil, true, err
		}
		return result, true, nil

	case TypeInt:
		var result int
		if err := mapstructure.WeakDecode(raw, &result); err != nil {
			return nil, true, err
		}
		return result, true, nil

	case TypeString:
		var result string
		if err := mapstructure.WeakDecode(raw, &result); err != nil {
			return nil, true, err
		}
		return result, true, nil

	case TypeNameString:
		var result string
		if err := mapstructure.WeakDecode(raw, &result); err != nil {
			return nil, true, err
		}
		matched, err := regexp.MatchString("^\\w(([\\w-.]+)?\\w)?$", result)
		if err != nil {
			return nil, true, err
		}
		if !matched {
			return nil, true, errors.New("field does not match the formatting rules")
		}
		return result, true, nil

	case TypeMap:
		var result map[string]interface{}
		if err := mapstructure.WeakDecode(raw, &result); err != nil {
			return nil, true, err
		}
		return result, true, nil

	case TypeDurationSecond:
		var result int
		switch inp := raw.(type) {
		case nil:
			return nil, false, nil
		case int:
			result = inp
		case int32:
			result = int(inp)
		case int64:
			result = int(inp)
		case uint:
			result = int(inp)
		case uint32:
			result = int(inp)
		case uint64:
			result = int(inp)
		case float32:
			result = int(inp)
		case float64:
			result = int(inp)
		case string:
			dur, err := parseutil.ParseDurationSecond(inp)
			if err != nil {
				return nil, true, err
			}
			result = int(dur.Seconds())
		case json.Number:
			valInt64, err := inp.Int64()
			if err != nil {
				return nil, true, err
			}
			result = int(valInt64)
		default:
			return nil, false, fmt.Errorf("invalid input '%v'", raw)
		}
		return result, true, nil

	case TypeSlice:
		var result []interface{}
		if err := mapstructure.WeakDecode(raw, &result); err != nil {
			return nil, true, err
		}
		return result, true, nil

	case TypeStringSlice:
		var result []string
		if err := mapstructure.WeakDecode(raw, &result); err != nil {
			return nil, true, err
		}
		return strutil.TrimStrings(result), true, nil

	case TypeCommaStringSlice:
		var result []string
		config := &mapstructure.DecoderConfig{
			Result:           &result,
			WeaklyTypedInput: true,
			DecodeHook:       mapstructure.StringToSliceHookFunc(","),
		}
		decoder, err := mapstructure.NewDecoder(config)
		if err != nil {
			return nil, false, err
		}
		if err := decoder.Decode(raw); err != nil {
			return nil, false, err
		}
		return strutil.TrimStrings(result), true, nil

	default:
		panic(fmt.Sprintf("Unknown type: %s", schema.Type))
	}
}
