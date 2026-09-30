package axlr

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// The library follows refs recursively during validation. Reject cyclic
// reference graphs, including nonproductive self refs, before resolving them.
// This intentionally excludes recursive schemas until validation has an
// explicit recursion budget; ordinary acyclic local references are supported.
func checkToolReferences(root *jsonschema.Schema) error {
	paths := map[string]*jsonschema.Schema{}
	anchors := map[string]*jsonschema.Schema{}
	edges := map[*jsonschema.Schema][]*jsonschema.Schema{}
	var walk func(*jsonschema.Schema, string) error
	walk = func(schema *jsonschema.Schema, path string) error {
		paths[path] = schema
		for _, anchor := range []string{schema.Anchor, schema.DynamicAnchor} {
			if anchor != "" {
				if previous, exists := anchors[anchor]; exists && previous != schema {
					return errors.New("ambiguous local schema anchor")
				}
				anchors[anchor] = schema
			}
		}
		visit := func(child *jsonschema.Schema, key string) error {
			if child == nil {
				return errors.New("null subschema")
			}
			edges[schema] = append(edges[schema], child)
			return walk(child, path+"/"+key)
		}
		value := reflect.ValueOf(schema).Elem()
		typ := value.Type()
		for i := 0; i < typ.NumField(); i++ {
			key := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			if key == "-" {
				switch typ.Field(i).Name {
				case "Items", "ItemsArray":
					key = "items"
				case "DependencySchemas":
					key = "dependencies"
				default:
					continue
				}
			}
			if key == "" {
				continue
			}
			switch children := value.Field(i).Interface().(type) {
			case *jsonschema.Schema:
				if children != nil {
					if err := visit(children, key); err != nil {
						return err
					}
				}
			case []*jsonschema.Schema:
				for j, child := range children {
					if err := visit(child, key+"/"+strconv.Itoa(j)); err != nil {
						return err
					}
				}
			case map[string]*jsonschema.Schema:
				for name, child := range children {
					escaped := strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
					if err := visit(child, key+"/"+escaped); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := walk(root, ""); err != nil {
		return err
	}
	for _, schema := range paths {
		for _, ref := range []string{schema.Ref, schema.DynamicRef} {
			if ref == "" {
				continue
			}
			fragment, err := url.PathUnescape(strings.TrimPrefix(ref, "#"))
			if err != nil {
				return err
			}
			target := paths[fragment]
			if fragment != "" && !strings.HasPrefix(fragment, "/") {
				target = anchors[fragment]
			}
			if target == nil {
				return fmt.Errorf("unresolved local schema reference %q", ref)
			}
			edges[schema] = append(edges[schema], target)
		}
	}
	colors := map[*jsonschema.Schema]uint8{}
	var check func(*jsonschema.Schema) error
	check = func(schema *jsonschema.Schema) error {
		if colors[schema] == 1 {
			return errors.New("cyclic schema references are unsupported")
		}
		if colors[schema] == 2 {
			return nil
		}
		colors[schema] = 1
		for _, target := range edges[schema] {
			if err := check(target); err != nil {
				return err
			}
		}
		colors[schema] = 2
		return nil
	}
	return check(root)
}
