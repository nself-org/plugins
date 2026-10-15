package render

import (
	"path"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/traefik/internal/contract"
)

// docsFile limits aliases to the two files mounted read-only in the helper.
func docsFile(r contract.Route, l contract.Location) bool {
	return r.ID == apiDocsRouteID && l.StaticFile != nil &&
		(*l.StaticFile == "/opt/nself/.dist/openapi.json" || *l.StaticFile == "/opt/nself/.dist/scalar.html")
}

// docsMiddleware preserves the alias suffix: /api-docs/x must remain a missing
// file, rather than serving openapi.json. Location add_header overrides inherited
// server headers, as in nginx. Invalid header values fail closed.
func (b *builder) docsMiddleware(r contract.Route, l contract.Location, field string) ([]string, bool) {
	for k, v := range l.HeadersAdd {
		if !headerOK(k, v) || strings.Contains(v, "$") {
			b.refuse(r.ID, field+".headers_add."+k, "header must have a literal value and valid name")
			return nil, false
		}
	}
	mws := []string{}
	if len(l.HeadersAdd) > 0 {
		mws = append(mws, b.mw("headers", map[string]any{"customResponseHeaders": l.HeadersAdd}))
	}
	mws = append(mws, b.mw("replacePathRegex", map[string]any{
		"regex":       "^" + regexp.QuoteMeta(l.Path) + "(.*)$",
		"replacement": "/__nself/docs/" + path.Base(*l.StaticFile) + "${1}",
	}))
	return mws, true
}
