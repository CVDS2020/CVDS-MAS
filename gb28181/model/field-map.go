package model

import (
	"errors"
	"gitee.com/sy_183/common/uns"
	"reflect"
	"strings"
	"unicode"
	"unsafe"
)

var InvalidModelTypeError = errors.New("模型的类型必须为结构体或结构体指针")

type FieldMap map[string]reflect.StructField

func isValidTag(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c):
			// Backslash and quote chars are reserved, but
			// otherwise any punctuation chars are allowed
			// in a tag name.
		case !unicode.IsLetter(c) && !unicode.IsDigit(c):
			return false
		}
	}
	return true
}

func GetFieldMap(model any) FieldMap {
	typ := reflect.TypeOf(model)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		panic(InvalidModelTypeError)
	}
	fieldMap := make(FieldMap)
	for i, n := 0, typ.NumField(); i < n; i++ {
		field := typ.Field(i)
		if field.IsExported() {
			if jsonTag := field.Tag.Get("json"); jsonTag != "" {
				tag, _, _ := strings.Cut(jsonTag, ",")
				if tag != "-" && isValidTag(tag) {
					fieldMap[tag] = field
				}
			}
		}
	}
	return fieldMap
}

func MapField[M, F any](fieldMap FieldMap, model M, field string) (value F, ok bool) {
	switch typ := reflect.TypeOf(model); typ.Kind() {
	case reflect.Pointer:
		if field, has := fieldMap[field]; has {
			return *(*F)(unsafe.Add(uns.ToPointer(model), field.Offset)), true
		}
	case reflect.Struct:
		if field, has := fieldMap[field]; has {
			return *(*F)(unsafe.Add(unsafe.Pointer(&model), field.Offset)), true
		}
	default:
		panic(InvalidModelTypeError)
	}
	return
}

func MapFieldPtr[M, F any](fieldMap FieldMap, model M, field string) *F {
	switch typ := reflect.TypeOf(model); typ.Kind() {
	case reflect.Pointer:
		if field, has := fieldMap[field]; has {
			return (*F)(unsafe.Add(uns.ToPointer(model), field.Offset))
		}
	case reflect.Struct:
		if field, has := fieldMap[field]; has {
			return (*F)(unsafe.Add(unsafe.Pointer(&model), field.Offset))
		}
	default:
		panic(InvalidModelTypeError)
	}
	return nil
}
