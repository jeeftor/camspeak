package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestInlineRequestFieldsDocumented(t *testing.T) {
	type operation struct {
		RequestBody struct {
			Content map[string]struct {
				Schema struct {
					Ref        string         `json:"$ref"`
					Properties map[string]any `json:"properties"`
				} `json:"schema"`
			} `json:"content"`
		} `json:"requestBody"`
	}
	var spec struct {
		Paths      map[string]map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Properties map[string]any `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(openAPISpec), &spec); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string][]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				value, ok := node.(*ast.ValueSpec)
				if !ok || len(value.Names) != 1 || value.Names[0].Name != "req" {
					return true
				}
				typ, ok := value.Type.(*ast.StructType)
				if !ok {
					return true
				}
				for _, field := range typ.Fields.List {
					if field.Tag != nil {
						tag, _ := strconv.Unquote(field.Tag.Value)
						name := strings.Split(reflect.StructTag(tag).Get("json"), ",")[0]
						if name != "" && name != "-" {
							fields[fn.Name.Name] = append(fields[fn.Name.Name], name)
						}
					}
				}
				return true
			})
		}
	}
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	params := regexp.MustCompile(`:([A-Za-z_][A-Za-z_0-9]*)`)
	for _, match := range regexp.MustCompile(`api\.(POST|PUT|PATCH)\("([^"]+)", h\.([A-Za-z]+)\)`).FindAllStringSubmatch(string(source), -1) {
		method, path, handler := strings.ToLower(
			match[1],
		), params.ReplaceAllString(
			match[2],
			"{$1}",
		), match[3]
		var op operation
		if err := json.Unmarshal(spec.Paths[path][method], &op); err != nil {
			t.Fatal(err)
		}
		schema := op.RequestBody.Content["application/json"].Schema
		properties := schema.Properties
		if schema.Ref != "" {
			properties = spec.Components.Schemas[strings.TrimPrefix(schema.Ref, "#/components/schemas/")].Properties
		}
		for _, field := range fields[handler] {
			if _, ok := properties[field]; !ok {
				t.Errorf("%s %s request field %s missing", method, path, field)
			}
		}
	}
}
