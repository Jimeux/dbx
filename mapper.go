package dbx

import (
	"reflect"
	"slices"
	"sync"
)

// A FieldInfo is metadata for a struct field stored as a bidirectional tree.
type FieldInfo struct {
	// Traversal stores the index-based traversal path to the field
	// within the struct. For example, if the field is at
	// str.Field[3].Type.Field[0], then Traversal would be []int{3, 0}.
	Traversal []int
	Path      string
	Field     reflect.StructField
	Zero      reflect.Value
	Name      string
	Embedded  bool
	Children  []*FieldInfo
	Parent    *FieldInfo
}

func (f *FieldInfo) IsRecursive() bool {
	for p := f.Parent; p != nil; p = p.Parent {
		if f.Field.Type == p.Field.Type {
			return true
		}
	}
	return false
}

func (f *FieldInfo) IsStruct() bool {
	return f.Zero.Kind() == reflect.Struct ||
		(f.Zero.Kind() == reflect.Pointer && f.Zero.Type().Elem().Kind() == reflect.Struct)
}

// A StructMap is an index of field metadata for a struct.
type StructMap struct {
	Tree  *FieldInfo // tree of fields in the struct, including nested and embedded fields
	Index []*FieldInfo
	Names map[string]*FieldInfo // index of field name (extracted from tag or mapFunc) to FieldInfo
}

// Mapper is a general purpose mapper of names to struct fields.  A Mapper
// behaves like most marshallers in the standard library, obeying a field tag
// for name mapping but also providing a basic transform function.
type Mapper struct {
	cache      map[reflect.Type]*StructMap
	tagName    string
	tagMapFunc func(string) string // called on the whole tag (could be used to e.g. ignore omitempty -> return "" to ignore)
	mapFunc    func(string) string // maps field names to column names. Used when tag not available. Tag split from options (comma sep) by default
	mutex      sync.Mutex
}

// NewMapperFunc returns a new mapper which optionally obeys a field tag and
// a struct field name mapper func given by f.  Tags will take precedence, but
// for any other field, the mapped name will be f(field.Name)
func NewMapperFunc(tagName string, f func(string) string) *Mapper {
	return &Mapper{
		cache:   make(map[reflect.Type]*StructMap),
		tagName: tagName,
		mapFunc: f,
	}
}

// TypeMap returns a mapping of field strings to int slices representing
// the traversal down the struct to reach the field.
func (m *Mapper) TypeMap(t reflect.Type) *StructMap {
	m.mutex.Lock()
	mapping, ok := m.cache[t]
	if !ok {
		mapping = getMapping(t, m.tagName, m.mapFunc, m.tagMapFunc)
		m.cache[t] = mapping
	}
	m.mutex.Unlock()
	return mapping
}

// TraversalsByName returns a slice of int slices which represent the struct
// traversals for each mapped name.  Panics if t is not a struct or Indirectable
// to a struct.  Returns empty int slice for each name not found.
func (m *Mapper) TraversalsByName(t reflect.Type, names []string) [][]int {
	t = derefType(t)

	if k := t.Kind(); k != reflect.Struct {
		panic(&reflect.ValueError{Method: "TraversalsByName", Kind: k})
	}

	traversals := make([][]int, len(names))
	tm := m.TypeMap(t)
	for i, name := range names {
		// look up the FieldInfo for name and set the Traversal slice if it exists
		if fi, ok := tm.Names[name]; ok {
			traversals[i] = fi.Traversal
		}
	}
	return traversals
}

// typeQueue holds state for the BFS of the fields of a struct.
type typeQueue struct {
	typ        reflect.Type
	fieldInfo  *FieldInfo
	parentPath string
}

// getMapping returns a mapping for the t type, using the tagName, mapFunc and
// tagMapFunc to determine the canonical names of fields.
// - mapFunc processes field names without tags f(field.Name)
// - tagMapFunc processes tag values. Useful for e.g. json because of "name,omitempty"
func getMapping(t reflect.Type, tagName string, mapFunc, tagMapFunc mapf) *StructMap {
	var fieldInfoList []*FieldInfo
	root := &FieldInfo{}
	queue := []typeQueue{
		{derefType(t), root, ""},
	}

	// @Jimeux notes: algorithm
	//   1. start a BFS from root (the original struct type)
	//   2. for each field, process children (if any)
	//     - parse the tag (db:"field")
	//     - if the field is a struct, add it to the queue
	// 		     - embedded structs are added to the queue without the parent path if they have no tag
	//     - update the field's Traversal array (integer-based path through the struct fields)
	//     - add field to the tree (register it in parent FieldInfo), and index it in m
	//   3. build and return the StructMap

	for len(queue) != 0 {
		// pop the first item off of the queue
		current := queue[0]
		queue = queue[1:]

		if current.fieldInfo.IsRecursive() {
			continue
		}

		// if current.typ is a struct, populate current.fieldInfo.Children with its fields
		nChildren := 0
		if current.typ.Kind() == reflect.Struct {
			nChildren = current.typ.NumField()
		}
		current.fieldInfo.Children = make([]*FieldInfo, nChildren)

		// iterate through all of its fields
		for fieldPos := range nChildren {
			field := current.typ.Field(fieldPos)

			// skip unexported non-embedded fields. Unexported fields embedded by
			// value are kept: their exported children are promoted and mappable.
			// Unexported fields embedded by pointer are skipped: the scan target
			// is always a fresh allocation so the pointer is always nil, and
			// reflect will not let us Set it.
			if !field.IsExported() && (!field.Anonymous || field.Type.Kind() == reflect.Pointer) {
				continue
			}

			// parse the tag and the target name using the mapping options for this field
			tag, name := parseName(field, tagName, mapFunc, tagMapFunc)

			// if the name is "-", disabled via a tag, skip it
			if name == "-" {
				continue
			}

			fieldInfo := FieldInfo{
				Field: field,
				Name:  name,
				Zero:  reflect.New(field.Type).Elem(),
			}

			// if the path is empty this path is just the name
			if current.parentPath == "" {
				fieldInfo.Path = fieldInfo.Name
			} else {
				fieldInfo.Path = current.parentPath + "." + fieldInfo.Name
			}

			// bfs search of anonymous embedded structs
			if field.Anonymous {
				// if embedded with no tag, the path is the parent path
				pp := current.parentPath
				if tag != "" {
					pp = fieldInfo.Path
				}

				fieldInfo.Embedded = true
				fieldInfo.Traversal = append(slices.Clone(current.fieldInfo.Traversal), fieldPos)
				nChildren := 0
				ft := derefType(field.Type)
				if ft.Kind() == reflect.Struct {
					nChildren = ft.NumField()
				}
				fieldInfo.Children = make([]*FieldInfo, nChildren)
				queue = append(queue, typeQueue{derefType(field.Type), &fieldInfo, pp})
			} else if fieldInfo.IsStruct() {
				fieldInfo.Traversal = append(slices.Clone(current.fieldInfo.Traversal), fieldPos)
				fieldInfo.Children = make([]*FieldInfo, derefType(field.Type).NumField())
				queue = append(queue, typeQueue{derefType(field.Type), &fieldInfo, fieldInfo.Path})
			}

			fieldInfo.Traversal = append(slices.Clone(current.fieldInfo.Traversal), fieldPos)
			fieldInfo.Parent = current.fieldInfo
			current.fieldInfo.Children[fieldPos] = &fieldInfo
			fieldInfoList = append(fieldInfoList, &fieldInfo)
		}
	}

	return buildStructMap(root, fieldInfoList)
}

func buildStructMap(root *FieldInfo, index []*FieldInfo) *StructMap {
	paths := map[string]*FieldInfo{}
	fields := &StructMap{Index: index, Tree: root, Names: map[string]*FieldInfo{}}
	for _, fi := range fields.Index {
		// check if nothing has already been pushed with the same path
		// sometimes you can choose to override a type using embedded struct
		fld, ok := paths[fi.Path]
		if !ok || fld.Embedded {
			paths[fi.Path] = fi
			if fi.Name != "" && !fi.Embedded {
				fields.Names[fi.Path] = fi
			}
		}
	}
	return fields
}
