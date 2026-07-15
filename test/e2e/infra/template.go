package infra

import (
	"encoding/json"
	"fmt"
	"os"

	templatev1 "github.com/openshift/api/template/v1"
	"github.com/openshift/library-go/pkg/template/generator"
	"github.com/openshift/library-go/pkg/template/templateprocessing"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// ProcessTemplate reads an OpenShift Template file, sets parameter values,
// processes it using the openshift/library-go template processor, and
// returns the resulting Kubernetes objects as unstructured resources.
func ProcessTemplate(templatePath string, params map[string]string) ([]unstructured.Unstructured, error) {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("reading template %s: %w", templatePath, err)
	}

	scheme := runtime.NewScheme()
	if err := templatev1.Install(scheme); err != nil {
		return nil, fmt.Errorf("installing template scheme: %w", err)
	}
	codecs := serializer.NewCodecFactory(scheme)

	obj, _, err := codecs.UniversalDeserializer().Decode(data, nil, &templatev1.Template{})
	if err != nil {
		return nil, fmt.Errorf("decoding template %s: %w", templatePath, err)
	}
	tmpl := obj.(*templatev1.Template)

	for k, v := range params {
		if p := templateprocessing.GetParameterByName(tmpl, k); p != nil {
			p.Value = v
		}
	}

	processor := templateprocessing.NewProcessor(map[string]generator.Generator{})
	if errs := processor.Process(tmpl); len(errs) > 0 {
		return nil, fmt.Errorf("processing template %s: %v", templatePath, errs.ToAggregate())
	}

	var result []unstructured.Unstructured
	for i, raw := range tmpl.Objects {
		rawBytes := raw.Raw
		if len(rawBytes) == 0 && raw.Object != nil {
			rawBytes, err = json.Marshal(raw.Object)
			if err != nil {
				return nil, fmt.Errorf("template %s: marshaling object %d: %w", templatePath, i, err)
			}
		}
		if len(rawBytes) == 0 {
			return nil, fmt.Errorf("template %s: object %d has no data", templatePath, i)
		}

		var parsed map[string]any
		if err := json.Unmarshal(rawBytes, &parsed); err != nil {
			return nil, fmt.Errorf("template %s: unmarshaling object %d: %w", templatePath, i, err)
		}
		result = append(result, unstructured.Unstructured{Object: parsed})
	}

	return result, nil
}
