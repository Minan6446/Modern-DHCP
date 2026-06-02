package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"go.uber.org/zap"

	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/policy"
	"modern-dhcp/internal/pool"
	securitypolicy "modern-dhcp/internal/security/policy"
	"modern-dhcp/internal/server"
)

type routeKey struct {
	method string
	path   string
}

type comparisonRow struct {
	route  routeKey
	inSpec bool
	inImpl bool
}

func main() {
	specPath := flag.String("spec", "docs/api/openapi_v2.yaml", "Path to the OpenAPI spec file")
	format := flag.String("format", "markdown", "Output format: markdown or text")
	flag.Parse()

	specRoutes, err := loadSpecRoutes(*specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load spec: %v\n", err)
		os.Exit(1)
	}

	implRoutes, err := loadImplementationRoutes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to inspect server routes: %v\n", err)
		os.Exit(1)
	}

	rows := buildComparison(specRoutes, implRoutes)
	switch strings.ToLower(*format) {
	case "text":
		printText(rows)
	default:
		printMarkdown(rows)
	}

	for _, row := range rows {
		if row.inSpec != row.inImpl {
			os.Exit(2)
		}
	}
}

func loadSpecRoutes(specFile string) (map[routeKey]struct{}, error) {
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	abs, err := filepath.Abs(specFile)
	if err != nil {
		return nil, err
	}
	doc, err := loader.LoadFromFile(abs)
	if err != nil {
		return nil, err
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, err
	}

	bases := extractServerBases(doc)
	if len(bases) == 0 {
		bases = []string{""}
	}

	routes := make(map[routeKey]struct{})
	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, base := range bases {
			addOperation := func(op *openapi3.Operation, method string) {
				if op == nil {
					return
				}
				full := joinURLPath(base, path)
				routes[routeKey{method: strings.ToUpper(method), path: full}] = struct{}{}
			}
			addOperation(item.Get, httpGet)
			addOperation(item.Post, httpPost)
			addOperation(item.Put, httpPut)
			addOperation(item.Patch, httpPatch)
			addOperation(item.Delete, httpDelete)
			addOperation(item.Options, httpOptions)
			addOperation(item.Head, httpHead)
		}
	}
	return routes, nil
}

func loadImplementationRoutes() (map[routeKey]struct{}, error) {
	opts := server.Options{}
	opts.API.Versions = []string{"v1"}
	opts.RequireAuth = false

	leaseSvc := &lease.Service{}
	policySvc := &policy.Service{}
	securityPolicySvc := &securitypolicy.Service{}
	poolSvc := &pool.Service{}

	srv, err := server.NewHTTPServer(zap.NewNop(), opts, leaseSvc, nil, policySvc, securityPolicySvc, poolSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		return nil, err
	}

	routes := make(map[routeKey]struct{})
	for _, r := range srv.RouteTable() {
		if !strings.HasPrefix(r.Path, "/api/") {
			continue
		}
		key := routeKey{method: strings.ToUpper(r.Method), path: r.Path}
		routes[key] = struct{}{}
	}
	return routes, nil
}

func buildComparison(spec, impl map[routeKey]struct{}) []comparisonRow {
	union := make(map[routeKey]struct{})
	for k := range spec {
		union[k] = struct{}{}
	}
	for k := range impl {
		union[k] = struct{}{}
	}
	rows := make([]comparisonRow, 0, len(union))
	for k := range union {
		_, inSpec := spec[k]
		_, inImpl := impl[k]
		rows = append(rows, comparisonRow{route: k, inSpec: inSpec, inImpl: inImpl})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].route.path == rows[j].route.path {
			return rows[i].route.method < rows[j].route.method
		}
		return rows[i].route.path < rows[j].route.path
	})
	return rows
}

func printMarkdown(rows []comparisonRow) {
	fmt.Println("| Method | Path | OpenAPI | Server |")
	fmt.Println("| --- | --- | --- | --- |")
	for _, row := range rows {
		fmt.Printf("| %s | %s | %s | %s |\n", row.route.method, row.route.path, boolIcon(row.inSpec), boolIcon(row.inImpl))
	}
}

func printText(rows []comparisonRow) {
	for _, row := range rows {
		fmt.Printf("%-6s %-40s spec:%s impl:%s\n", row.route.method, row.route.path, boolText(row.inSpec), boolText(row.inImpl))
	}
}

func boolIcon(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}

func boolText(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}

const (
	httpGet     = "GET"
	httpPost    = "POST"
	httpPut     = "PUT"
	httpPatch   = "PATCH"
	httpDelete  = "DELETE"
	httpOptions = "OPTIONS"
	httpHead    = "HEAD"
)

func extractServerBases(doc *openapi3.T) []string {
	bases := make([]string, 0, len(doc.Servers))
	for _, srv := range doc.Servers {
		if srv == nil {
			continue
		}
		bases = append(bases, srv.URL)
	}
	return bases
}

func joinURLPath(base, path string) string {
	combined := singleSlash(extractBasePath(base), path)
	if combined == "" {
		return "/"
	}
	if !strings.HasPrefix(combined, "/") {
		return "/" + combined
	}
	return combined
}

func singleSlash(a, b string) string {
	ap := strings.TrimRight(a, "/")
	bp := strings.TrimLeft(b, "/")
	if ap == "" && bp == "" {
		return "/"
	}
	if ap == "" {
		return "/" + bp
	}
	if bp == "" {
		return ap
	}
	return ap + "/" + bp
}

func extractBasePath(base string) string {
	if base == "" {
		return ""
	}
	if parsed, err := url.Parse(base); err == nil {
		return parsed.Path
	}
	if idx := strings.Index(base, "//"); idx >= 0 {
		rest := base[idx+2:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			return rest[slash:]
		}
		return ""
	}
	if strings.HasPrefix(base, "/") {
		return base
	}
	if slash := strings.Index(base, "/"); slash >= 0 {
		return base[slash:]
	}
	return ""
}
