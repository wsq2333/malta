package workflow

import (
	"fmt"
	"github.com/wsq2333/malta/internal/domain"
)

func definitionFor(d Definition, kind string) (Definition, error) {
	if len(d.Implementations) > 0 {
		v, ok := d.Implementations[kind]
		if !ok {
			return d, domain.E("invalid", "case version has no implementation for "+kind)
		}
		d.Backend = kind
		d.Template = v.Template
		d.Bindings = v.Bindings
		d.Results = v.Results
		d.Implementations = nil
	}
	if d.Backend != kind {
		return d, domain.E("invalid", fmt.Sprintf("case backend %s differs from execution profile %s", d.Backend, kind))
	}
	return d, nil
}
