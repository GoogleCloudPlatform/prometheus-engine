package operator

import (
	"reflect"
	"testing"

	monitoringv1alpha1 "github.com/google/gpe-collector/pkg/operator/apis/monitoring/v1alpha1"
	"github.com/prometheus/common/model"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/relabel"
)

func TestLabelMappingRelabelConfigs(t *testing.T) {
	cases := []struct {
		doc      string
		mappings []monitoringv1alpha1.LabelMapping
		prefix   model.LabelName
		expected []*relabel.Config
		expErr   bool
	}{
		{
			doc:      "good podmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{{From: "from", To: "to"}},
			prefix:   podLabelPrefix,
			expected: []*relabel.Config{{
				Action:       relabel.Replace,
				SourceLabels: prommodel.LabelNames{podLabelPrefix + "from"},
				TargetLabel:  "to",
			}},
			expErr: false,
		},
		{
			doc:      "colliding podmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{{From: "from-instance", To: "instance"}},
			prefix:   podLabelPrefix,
			expected: nil,
			expErr:   true,
		},
		{
			doc: "both good and colliding podmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{
				{From: "from", To: "to"},
				{From: "from-instance", To: "instance"}},
			prefix:   podLabelPrefix,
			expected: nil,
			expErr:   true,
		},
		{
			doc:      "good svcmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{{From: "from", To: "to"}},
			prefix:   serviceLabelPrefix,
			expected: []*relabel.Config{{
				Action:       relabel.Replace,
				SourceLabels: prommodel.LabelNames{serviceLabelPrefix + "from"},
				TargetLabel:  "to",
			}},
			expErr: false,
		},
		{
			doc:      "colliding svcmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{{From: "from-instance", To: "instance"}},
			prefix:   serviceLabelPrefix,
			expected: nil,
			expErr:   true,
		},
		{
			doc: "both good and colliding svcmonitoring relabel",
			mappings: []monitoringv1alpha1.LabelMapping{
				{From: "from", To: "to"},
				{From: "from-instance", To: "instance"}},
			prefix:   serviceLabelPrefix,
			expected: nil,
			expErr:   true,
		},
	}

	for _, c := range cases {
		t.Run(c.doc, func(t *testing.T) {
			// If we get an error when we don't expect, fail test.
			actual, err := labelMappingRelabelConfigs(c.mappings, c.prefix)
			if err != nil && !c.expErr {
				t.Errorf("returned unexpected error: %s", err)
			}
			if err == nil && c.expErr {
				t.Errorf("should have returned an error")
			}
			if !reflect.DeepEqual(c.expected, actual) {
				t.Errorf("returned unexpected config")
			}
		})
	}
}
