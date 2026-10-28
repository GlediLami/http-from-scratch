package http1

import (
	"net/http"
	"sort"
	"strings"
)

type Router struct {
	routes []route
}

type route struct {
	method  string
	segs    []seg
	handler http.Handler
	pattern string
}

type seg struct {
	lit   string // literal text when wild
	name  string // capture name when wild
	wild  bool
	multi bool // {name ...}
}

func NewRouter() *Router { return &Router{} }

func (rt *Router) Handle(pattern string, h http.Handler) {
	method, path := splitPattern(pattern)
	rt.routes = append(rt.routes, route{
		method:  method,
		segs:    parsePath(path),
		handler: h,
		pattern: pattern,
	})
}

func (rt *Router) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	rt.Handle(pattern, http.HandlerFunc(h))
}

func splitPattern(pattern string) (method, path string) {
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		return pattern[:i], strings.TrimSpace(pattern[i+1:])
	}
	return "", pattern
}

func parsePath(path string) []seg {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil
	}

	parts := strings.Split(path, "/")
	segs := make([]seg, 0, len(parts))
	for _, part := range parts {
		if len(part) >= 2 && part[0] == '{' && part[len(part)-1] == '}' {
			name := part[1 : len(part)-1]
			multi := strings.HasSuffix(name, "...")
			if multi {
				name = strings.TrimSuffix(name, "...")
			}
			segs = append(segs, seg{
				name:  name,
				wild:  true,
				multi: multi,
			})
			continue
		}
		segs = append(segs, seg{lit: part})
	}
	return segs
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqSegs := splitSegs(r.URL.Path)
	var best *route
	var bestParams map[string]string
	pathMatched := false
	allowed := map[string]bool{}

	for i := range rt.routes {
		rr := &rt.routes[i]
		params, ok := matchSegs(rr.segs, reqSegs)
		if !ok {
			continue
		}
		pathMatched = true
		if rr.method != "" {
			allowed[rr.method] = true
		}

		if !methodMatch(rr.method, r.Method) {
			continue
		}
		if best == nil || score(rr.segs) > score(best.segs) {
			best = rr
			bestParams = params
		}
	}

	if best == nil {
		if pathMatched {
			writeAllow(w, allowed) // 405
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.NotFound(w, r) // 404
		return
	}
	for k, v := range bestParams {
		r.SetPathValue(k, v)
	}

	r.Pattern = best.pattern
	best.handler.ServeHTTP(w, r)
}

func splitSegs(path string) []string {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func matchSegs(pat []seg, req []string) (map[string]string, bool) {
	var params map[string]string
	for i, s := range pat {
		// {name...} swallows the rest of the path and ends the match.
		if s.wild && s.multi {
			if params == nil {
				params = map[string]string{}
			}
			params[s.name] = strings.Join(req[i:], "/")
			return params, true
		}
		if i >= len(req) {
			return nil, false // pattern longer than path
		}
		if s.wild {
			if req[i] == "" {
				return nil, false // {id} cannot capture an empty segment
			}
			if params == nil {
				params = map[string]string{}
			}
			params[s.name] = req[i]
			continue
		}
		if s.lit != req[i] {
			return nil, false // literal mismatch
		}
	}
	if len(req) != len(pat) {
		return nil, false // path longer than pattern
	}
	return params, true
}

func methodMatch(routeMethod, reqMethod string) bool {
	switch {
	case routeMethod == "":
		return true // pattern with no method matches anything
	case routeMethod == reqMethod:
		return true
	case routeMethod == http.MethodGet && reqMethod == http.MethodHead:
		return true // HEAD is served by the GET handler
	}
	return false
}

// score prefers the pattern with more literal (non-wildcard) segments,
// so /users/me beats /users/{id}.
func score(segs []seg) int {
	n := 0
	for _, s := range segs {
		if !s.wild {
			n += 2
		}
	}
	return n
}

func writeAllow(w http.ResponseWriter, allowed map[string]bool) {
	methods := make([]string, 0, len(allowed))
	for m := range allowed {
		methods = append(methods, m)
	}
	sort.Strings(methods)
	w.Header().Set("Allow", strings.Join(methods, ", "))
}
