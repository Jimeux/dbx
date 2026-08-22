package dbx

import (
	"reflect"
	"strings"
)

// derefType is Indirect for reflect.Types
func derefType(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

type mapf func(string) string

// parseName parses the tag and the target name for the given field using
// the tagName (eg 'json' for `json:"foo"` tags), mapFunc for mapping the
// field's name to a target name, and tagMapFunc for mapping the tag to
// a target name.
func parseName(field reflect.StructField, tagName string, mapFunc, tagMapFunc mapf) (tag, fieldName string) {
	// first, set the fieldName to the field's name
	fieldName = field.Name
	// if a mapFunc is set, use that to override the fieldName
	if mapFunc != nil {
		fieldName = mapFunc(fieldName)
	}

	// if there's no tag to look for, return the field name
	if tagName == "" {
		return "", fieldName
	}

	// if this tag is not set using the normal convention in the tag,
	// then return the fieldname.
	tag, ok := field.Tag.Lookup(tagName)
	if !ok {
		return "", fieldName
	}

	// if we have a mapper function, call it on the whole tag
	// XXX: this is a change from the old version, which pulled out the name
	// before the tagMapFunc could be run, but I think this is the right way
	if tagMapFunc != nil {
		tag = tagMapFunc(tag)
	}

	// finally, split the options from the name
	fieldName, _, _ = strings.Cut(tag, ",")
	return tag, fieldName
}
