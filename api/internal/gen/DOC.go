// Package gen hosts types and the route interface generated from
// contracts/openapi.yaml. Per Constitution III this directory is generated,
// never hand-edited. To regenerate, run `make generate`.
//
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=oapi-codegen.yaml ../../contracts/openapi.yaml
package gen
