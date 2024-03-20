package dbx

import (
	"reflect"
	"slices"
	"sync"
)

// A FieldInfo is metadata for a struct field.
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
		(f.Zero.Kind() == reflect.Ptr && f.Zero.Type().Elem().Kind() == reflect.Struct)
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
	t          reflect.Type
	fi         *FieldInfo
	parentPath string
}

// getMapping returns a mapping for the t type, using the tagName, mapFunc and
// tagMapFunc to determine the canonical names of fields.
// - mapFunc processes field names without tags f(field.Name)
// - tagMapFunc processes tag values. Useful for e.g. json because of "name,omitempty"
func getMapping(t reflect.Type, tagName string, mapFunc, tagMapFunc mapf) *StructMap {
	var m []*FieldInfo
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
		tq := queue[0]
		queue = queue[1:]

		if tq.fi.IsRecursive() {
			continue
		}

		// if tq.t is a struct, populate tq.fi.Children with its fields
		nChildren := 0
		if tq.t.Kind() == reflect.Struct {
			nChildren = tq.t.NumField()
		}
		tq.fi.Children = make([]*FieldInfo, nChildren)

		// iterate through all of its fields
		for fieldPos := 0; fieldPos < nChildren; fieldPos++ {
			f := tq.t.Field(fieldPos)

			// skip unexported fields that aren't embedded structs
			if !f.IsExported() && !f.Anonymous {
				continue
			}

			// parse the tag and the target name using the mapping options for this field
			tag, name := parseName(f, tagName, mapFunc, tagMapFunc)

			// if the name is "-", disabled via a tag, skip it
			if name == "-" {
				continue
			}

			fi := FieldInfo{
				Field: f,
				Name:  name,
				Zero:  reflect.New(f.Type).Elem(),
			}

			// if the path is empty this path is just the name
			if tq.parentPath == "" {
				fi.Path = fi.Name
			} else {
				fi.Path = tq.parentPath + "." + fi.Name
			}

			// bfs search of anonymous embedded structs
			if f.Anonymous {
				// if embedded with no tag, the path is the parent path
				pp := tq.parentPath
				if tag != "" {
					pp = fi.Path
				}

				fi.Embedded = true

				fi.Traversal = append(slices.Clone(tq.fi.Traversal), fieldPos)
				nChildren := 0
				ft := derefType(f.Type)
				if ft.Kind() == reflect.Struct {
					nChildren = ft.NumField()
				}
				fi.Children = make([]*FieldInfo, nChildren)
				queue = append(queue, typeQueue{derefType(f.Type), &fi, pp})
			} else if fi.IsStruct() {
				fi.Traversal = append(slices.Clone(tq.fi.Traversal), fieldPos)
				fi.Children = make([]*FieldInfo, derefType(f.Type).NumField())
				queue = append(queue, typeQueue{derefType(f.Type), &fi, fi.Path})
			}

			fi.Traversal = append(slices.Clone(tq.fi.Traversal), fieldPos)
			fi.Parent = tq.fi
			tq.fi.Children[fieldPos] = &fi
			m = append(m, &fi)
		}
	}

	return buildStructMap(root, m)
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
