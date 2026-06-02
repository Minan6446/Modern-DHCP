package server

import "sort"

// RouteInfo captures a single HTTP route registered on the Echo engine.
type RouteInfo struct {
	Method string
	Path   string
	Name   string
}

// RouteTable returns a snapshot of all registered routes (method + path pairs).
func (s *HTTPServer) RouteTable() []RouteInfo {
	if s == nil || s.echo == nil {
		return nil
	}
	routes := s.echo.Routes()
	result := make([]RouteInfo, 0, len(routes))
	for _, r := range routes {
		if r == nil {
			continue
		}
		result = append(result, RouteInfo{Method: r.Method, Path: r.Path, Name: r.Name})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path == result[j].Path {
			return result[i].Method < result[j].Method
		}
		return result[i].Path < result[j].Path
	})
	return result
}
