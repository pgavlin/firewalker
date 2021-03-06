package nodejs

import (
	"bytes"
	"fmt"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/pkg/util/contract"

	"github.com/pgavlin/firewalker/il"
)

// computeHTTPInputs computes the arguments for a call to request-promise-native's single function from the bound input
// properties of the given http resource.
func (g *Generator) computeHTTPInputs(r *il.ResourceNode, indent bool, count string) (string, error) {
	urlProperty, ok := r.Properties.Elements["url"]
	if !ok {
		return "", errors.Errorf("missing required property \"url\" in resource %s", r.Config.Name)
	}
	url, _, err := g.computeProperty(urlProperty, indent, count)
	if err != nil {
		return "", err
	}

	requestHeadersProperty, hasRequestHeaders := r.Properties.Elements["request_headers"]
	if !hasRequestHeaders {
		return url, nil
	}

	requestHeaders, _, err := g.computeProperty(requestHeadersProperty, true, count)
	if err != nil {
		return "", err
	}

	buf := &bytes.Buffer{}
	buf.WriteString("{\n")
	fmt.Fprintf(buf, "%s    url: %s,\n", indent, url)
	fmt.Fprintf(buf, "%s    headers: %s,\n", indent, requestHeaders)
	fmt.Fprintf(buf, "%s}", indent)
	return buf.String(), nil
}

// generateHTTP generates the given http resource as a call to request-promise-native's single exported function.
func (g *Generator) generateHTTP(r *il.ResourceNode) error {
	contract.Require(r.Provider.Config.Name == "http", "r")

	name := resName(r.Config.Type, r.Config.Name)

	if r.Count == nil {
		inputs, err := g.computeHTTPInputs(r, false, "")
		if err != nil {
			return err
		}

		fmt.Printf("const %s = pulumi.output(rpn(%s).promise());\n", name, inputs)
	} else {
		count, _, err := g.computeProperty(r.Count, false, "")
		if err != nil {
			return err
		}
		inputs, err := g.computeHTTPInputs(r, true, "i")
		if err != nil {
			return err
		}

		fmt.Printf("const %s: pulumi.Output<string>[] = [];\n", name)
		fmt.Printf("for (let i = 0; i < %s; i++) {\n", count)
		fmt.Printf("    %s.push(pulumi.output(rpn(%s).promise()));\n", name, inputs)
		fmt.Printf("}\n")
	}

	return nil
}
