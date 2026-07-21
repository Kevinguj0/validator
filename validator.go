package validator

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// FieldError represents a validation error on a struct field.
type FieldError struct {
	Field string
	Tag   string
	Param string
	Value interface{}
}

func (e FieldError) Error() string {
	if e.Param != "" {
		return fmt.Sprintf("Field validation for '%s' failed on the '%s' tag with param '%s'", e.Field, e.Tag, e.Param)
	}
	return fmt.Sprintf("Field validation for '%s' failed on the '%s' tag", e.Field, e.Tag)
}

// ValidationErrors is a slice of FieldError.
type ValidationErrors []FieldError

func (ve ValidationErrors) Error() string {
	if len(ve) == 0 {
		return ""
	}
	var errMsgs []string
	for _, err := range ve {
		errMsgs = append(errMsgs, err.Error())
	}
	return strings.Join(errMsgs, "; ")
}

// Validate is the core validation engine.
type Validate struct{}

// New creates a new Validate instance.
func New() *Validate {
	return &Validate{}
}

// Struct validates a struct's fields based on their tags.
func (v *Validate) Struct(s interface{}) error {
	return v.validateStructInternal(s, nil)
}

// StructPartial validates only the specified fields of a struct.
func (v *Validate) StructPartial(s interface{}, fields ...string) error {
	partialMap := make(map[string]bool)
	for _, f := range fields {
		partialMap[f] = true
	}
	return v.validateStructInternal(s, partialMap)
}

// Var validates a single variable against a tag.
func (v *Validate) Var(field interface{}, tag string) error {
	val := reflect.ValueOf(field)
	errs := validateFieldValue(val, tag, "")
	if len(errs) > 0 {
		return ValidationErrors(errs)
	}
	return nil
}

func (v *Validate) validateStructInternal(s interface{}, partialFields map[string]bool) error {
	val := reflect.ValueOf(s)
	for val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}
	if !val.IsValid() || val.Kind() != reflect.Struct {
		return nil
	}

	var errs ValidationErrors
	v.traverseStruct(val, "", false, partialFields, &errs)

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func getPromotedFieldNames(t reflect.Type) map[string]bool {
	names := make(map[string]bool)
	collectPromotedFieldNames(t, names)
	return names
}

func collectPromotedFieldNames(t reflect.Type, names map[string]bool) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" && !f.Anonymous {
			continue
		}
		names[f.Name] = true
		if f.Anonymous {
			ft := f.Type
			for ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				collectPromotedFieldNames(ft, names)
			}
		}
	}
}

func (v *Validate) traverseStruct(
	val reflect.Value,
	prefix string,
	parentSelected bool,
	partialFields map[string]bool,
	errs *ValidationErrors,
) {
	for val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return
	}

	typ := val.Type()
	isPartial := (partialFields != nil)

	for i := 0; i < val.NumField(); i++ {
		fieldVal := val.Field(i)
		fieldType := typ.Field(i)

		if fieldType.PkgPath != "" && !fieldType.Anonymous {
			continue
		}

		fieldName := fieldType.Name
		fieldPath := fieldName
		if prefix != "" {
			fieldPath = prefix + "." + fieldName
		}

		isSelected := !isPartial || parentSelected || partialFields[fieldPath] || partialFields[fieldName]
		tag := fieldType.Tag.Get("validate")

		actualVal := fieldVal
		for actualVal.Kind() == reflect.Ptr {
			if actualVal.IsNil() {
				break
			}
			actualVal = actualVal.Elem()
		}

		if actualVal.Kind() == reflect.Struct {
			shouldTraverse := !isPartial || parentSelected || isSelected

			if isPartial && !shouldTraverse {
				prefixPattern := fieldPath + "."
				for k := range partialFields {
					if strings.HasPrefix(k, prefixPattern) {
						shouldTraverse = true
						break
					}
				}
			}

			if isPartial && !shouldTraverse && fieldType.Anonymous {
				promoted := getPromotedFieldNames(fieldType.Type)
				for k := range partialFields {
					if promoted[k] {
						shouldTraverse = true
						break
					}
					if strings.HasPrefix(k, fieldPath+".") {
						subField := strings.TrimPrefix(k, fieldPath+".")
						if promoted[subField] {
							shouldTraverse = true
							break
						}
					}
				}
			}

			if shouldTraverse {
				if isSelected && tag != "" {
					fieldErrs := validateFieldValue(fieldVal, tag, fieldPath)
					*errs = append(*errs, fieldErrs...)
				}

				newPrefix := fieldPath
				nextParentSelected := parentSelected || (isPartial && (partialFields[fieldPath] || (prefix == "" && partialFields[fieldName])))

				v.traverseStruct(actualVal, newPrefix, nextParentSelected, partialFields, errs)
			}
		} else {
			if isSelected && tag != "" {
				fieldErrs := validateFieldValue(fieldVal, tag, fieldPath)
				*errs = append(*errs, fieldErrs...)
			}
		}
	}
}

func validateFieldValue(val reflect.Value, tagStr string, fieldName string) []FieldError {
	if tagStr == "" || tagStr == "-" {
		return nil
	}
	var errs []FieldError

	isNil := false
	for val.Kind() == reflect.Ptr || val.Kind() == reflect.Interface {
		if val.IsNil() {
			isNil = true
			break
		}
		val = val.Elem()
	}

	rules := strings.Split(tagStr, ",")
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		parts := strings.SplitN(rule, "=", 2)
		ruleName := parts[0]
		param := ""
		if len(parts) > 1 {
			param = parts[1]
		}

		switch ruleName {
		case "required":
			if isNil {
				errs = append(errs, FieldError{Field: fieldName, Tag: "required", Param: param, Value: nil})
			} else if isZeroValue(val) {
				var rawVal interface{}
				if val.IsValid() {
					rawVal = val.Interface()
				}
				errs = append(errs, FieldError{Field: fieldName, Tag: "required", Param: param, Value: rawVal})
			}
		case "min":
			if !isNil && val.IsValid() {
				n, err := strconv.ParseFloat(param, 64)
				if err == nil {
					switch val.Kind() {
					case reflect.String:
						if float64(len(val.String())) < n {
							errs = append(errs, FieldError{Field: fieldName, Tag: "min", Param: param, Value: val.String()})
						}
					case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
						if float64(val.Int()) < n {
							errs = append(errs, FieldError{Field: fieldName, Tag: "min", Param: param, Value: val.Int()})
						}
					case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
						if float64(val.Uint()) < n {
							errs = append(errs, FieldError{Field: fieldName, Tag: "min", Param: param, Value: val.Uint()})
						}
					case reflect.Float32, reflect.Float64:
						if val.Float() < n {
							errs = append(errs, FieldError{Field: fieldName, Tag: "min", Param: param, Value: val.Float()})
						}
					case reflect.Slice, reflect.Map, reflect.Array:
						if float64(val.Len()) < n {
							errs = append(errs, FieldError{Field: fieldName, Tag: "min", Param: param, Value: val.Len()})
						}
					}
				}
			}
		case "max":
			if !isNil && val.IsValid() {
				n, err := strconv.ParseFloat(param, 64)
				if err == nil {
					switch val.Kind() {
					case reflect.String:
						if float64(len(val.String())) > n {
							errs = append(errs, FieldError{Field: fieldName, Tag: "max", Param: param, Value: val.String()})
						}
					case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
						if float64(val.Int()) > n {
							errs = append(errs, FieldError{Field: fieldName, Tag: "max", Param: param, Value: val.Int()})
						}
					case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
						if float64(val.Uint()) > n {
							errs = append(errs, FieldError{Field: fieldName, Tag: "max", Param: param, Value: val.Uint()})
						}
					case reflect.Float32, reflect.Float64:
						if val.Float() > n {
							errs = append(errs, FieldError{Field: fieldName, Tag: "max", Param: param, Value: val.Float()})
						}
					case reflect.Slice, reflect.Map, reflect.Array:
						if float64(val.Len()) > n {
							errs = append(errs, FieldError{Field: fieldName, Tag: "max", Param: param, Value: val.Len()})
						}
					}
				}
			}
		case "email":
			if !isNil && val.Kind() == reflect.String {
				str := val.String()
				if str != "" && (!strings.Contains(str, "@") || !strings.Contains(str, ".")) {
					errs = append(errs, FieldError{Field: fieldName, Tag: "email", Param: param, Value: str})
				}
			}
		}
	}
	return errs
}

func isZeroValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
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
	case reflect.Slice, reflect.Map:
		return v.IsNil() || v.Len() == 0
	case reflect.Struct:
		return v.IsZero()
	default:
		return false
	}
}
