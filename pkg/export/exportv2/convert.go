package exportv2

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	monitoring_pb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	writev2 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/prompb/io/prometheus/write/v2"
	v2 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/prompb/write/v2"
	timestamp_pb "github.com/golang/protobuf/ptypes/timestamp"
	distribution_pb "google.golang.org/genproto/googleapis/api/distribution"
	metric_pb "google.golang.org/genproto/googleapis/api/metric"
	monitoredres_pb "google.golang.org/genproto/googleapis/api/monitoredres"
)

// The target label keys used for the Prometheus monitored resource.
const (
	KeyProjectID = "project_id"
	KeyLocation  = "location"
	KeyCluster   = "cluster"
	KeyNamespace = "namespace"
	KeyJob       = "job"
	KeyInstance  = "instance"

	// Maximum number of labels allowed on GCM series.
	maxLabelCount = 100

	metricTypePrefix = "prometheus.googleapis.com"
)

func isClassicHistogramSeries(ts *writev2.TimeSeries) bool {
	if ts.GetMetadata().GetType() == writev2.Metadata_METRIC_TYPE_HISTOGRAM {
		if len(ts.Samples) > 0 {
			//????
		}
		if getMetricSuffix(name) == metricSuffixBucket || getMetricSuffix(name) == metricSuffixSum || getMetricSuffix(name) == metricSuffixCount {
			// Classic histogram detected. This server requires "self-contained-histograms", return err for classic histograms.
			// See: https://docs.google.com/document/d/1mpcSWH1B82q-BtJza-eJ8xMLlKt6EJ9oFGH325vtY1Q/edit
			return fmt.Errorf("%v: self-contained-histogram feature is set; classic histogram metrics are not allowed (use native histograms with custom buckets instead)", errorSeriesRef(name, res.Labels, labels))
		}
	}
}

// exportTimeSeries converts and enqueues self-contained series.
func exportTimeSeries(ts *writev2.TimeSeries, sym []string, exportGCMTimeSeriesPointFn func(*monitoring_pb.TimeSeries)) error {
	name, res, labels, err := p.extractNameResourceAndLabels(ts.LabelsRefs)
	if err != nil {
		return fmt.Errorf("%v: %w", errorSeriesRef(name, res.Labels, labels), err)
	}
	if ts.GetMetadata() == nil {
		return fmt.Errorf("%v: metadata is required", errorSeriesRef(name, res.Labels, labels))
	}

	// Drop series with too many labels.
	// TODO: Remove once field limit is lifted in the GCM API.
	if len(labels) > maxLabelCount {
		return fmt.Errorf("%v: metric labels exceed the limit of %d", errorSeriesRef(name, res.Labels, labels), maxLabelCount)
	}

	if ts.GetMetadata().GetType() == writev2.Metadata_METRIC_TYPE_HISTOGRAM {
		if len(ts.Samples) > 0 {
			//????
		}
		if getMetricSuffix(name) == metricSuffixBucket || getMetricSuffix(name) == metricSuffixSum || getMetricSuffix(name) == metricSuffixCount {
			// Classic histogram detected. This server requires "self-contained-histograms", return err for classic histograms.
			// See: https://docs.google.com/document/d/1mpcSWH1B82q-BtJza-eJ8xMLlKt6EJ9oFGH325vtY1Q/edit
			return fmt.Errorf("%v: self-contained-histogram feature is set; classic histogram metrics are not allowed (use native histograms with custom buckets instead)", errorSeriesRef(name, res.Labels, labels))
		}
	}

	descriptor, kind, err := describeMetric(name, ts.GetMetadata().GetType())
	if err != nil {
		return fmt.Errorf("%v: %w", errorSeriesRef(name, res.Labels, labels), err)
	}

	if kind == metric_pb.MetricDescriptor_CUMULATIVE && ts.CreatedTimestamp == 0 {
		return fmt.Errorf("%v: created timestamp is required for every cumulative metric", errorSeriesRef(name, res.Labels, labels))
	}

	// As per https://cloud.google.com/monitoring/api/ref_v3/rpc/google.monitoring.v3
	// GCM API allows at most 1 point per timeseries, so make we will copy gts below.
	gts := &monitoring_pb.TimeSeries{
		Resource:   res,
		Metric:     &metric_pb.Metric{Type: descriptor, Labels: labels},
		MetricKind: kind,
	}

	// TODO(bwplotka): Exemplars.

	var errs []error

	// Histogram samples.
	if ts.GetMetadata().GetType() == v2.Metadata_METRIC_TYPE_HISTOGRAM {
		if len(ts.GetHistograms()) > 0 {
			// Process native histogram samples.
			gts.ValueType = metric_pb.MetricDescriptor_DISTRIBUTION
			for _, s := range ts.GetHistograms() {
				gtsCopy := *gts // TODO(bwplotka): Pool this potentially.

				var startTime *timestamp_pb.Timestamp
				if kind == metric_pb.MetricDescriptor_CUMULATIVE {
					startTime = getTimestamp(ts.GetCreatedTimestamp())
				}

				distributionSample, err := histogramSampleToDistribution(s)
				if err != nil {
					errs = append(errs, fmt.Errorf("%v: created timestamp is required for every cumulative metric", errorSeriesRef(name, res.Labels, labels)))
					continue
				}

				gtsCopy.Points = []*monitoring_pb.Point{{
					Interval: &monitoring_pb.TimeInterval{
						StartTime: startTime,
						EndTime:   getTimestamp(s.Timestamp),
					},
					Value: &monitoring_pb.TypedValue{
						Value: &monitoring_pb.TypedValue_DistributionValue{DistributionValue: distributionSample},
					}},
				}
				exportGCMTimeSeriesPointFn(&gtsCopy)
			}
			return errors.Join(errs...)
		}
		// Process classic histogram samples.
		gts.ValueType = metric_pb.MetricDescriptor_DOUBLE
		for _, s := range ts.GetSamples() {
			gtsCopy := *gts // TODO(bwplotka): Pool this potentially.

			var startTime *timestamp_pb.Timestamp
			if kind == metric_pb.MetricDescriptor_CUMULATIVE {
				startTime = getTimestamp(ts.GetCreatedTimestamp())
			}

			gtsCopy.Points = []*monitoring_pb.Point{{
				Interval: &monitoring_pb.TimeInterval{
					StartTime: startTime,
					EndTime:   getTimestamp(s.Timestamp),
				},
				Value: &monitoring_pb.TypedValue{
					Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: s.Value},
				}},
			}
			exportGCMTimeSeriesPointFn(&gtsCopy)
		}
		return nil
	}

	// Float sample.
	gts.ValueType = metric_pb.MetricDescriptor_DOUBLE
	for _, s := range ts.GetSamples() {
		gtsCopy := *gts // TODO(bwplotka): Pool this potentially.

		var startTime *timestamp_pb.Timestamp
		if kind == metric_pb.MetricDescriptor_CUMULATIVE {
			startTime = getTimestamp(ts.GetCreatedTimestamp())
		}

		gtsCopy.Points = []*monitoring_pb.Point{{
			Interval: &monitoring_pb.TimeInterval{
				StartTime: startTime,
				EndTime:   getTimestamp(s.Timestamp),
			},
			Value: &monitoring_pb.TypedValue{
				Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: s.Value},
			}},
		}
		exportGCMTimeSeriesPointFn(&gtsCopy)
	}
	return nil
}

// getTimestamp converts a millisecond timestamp into a protobuf timestamp.
func getTimestamp(t int64) *timestamp_pb.Timestamp {
	return &timestamp_pb.Timestamp{
		Seconds: t / 1000,
		Nanos:   int32((t % 1000) * int64(time.Millisecond)),
	}
}

func histogramSampleToDistribution(s *v2.Histogram) (*distribution_pb.Distribution, error) {
	var (
		count int64
		dev   float64
	)

	countInt, ok := s.Count.(*v2.Histogram_CountInt)
	if !ok {
		countFloat, ok := s.Count.(*v2.Histogram_CountFloat)
		if !ok {
			return nil, errors.New("unknown histogram.count type")
		}
		count = int64(countFloat.CountFloat) // Bad, but no other way, should we error instead?
	} else {
		count = int64(countInt.CountInt)
	}

	// TODO(bwplotka): Calculate dev.

	// TODO(bwplotka): Consider pooling distributions.
	d := &distribution_pb.Distribution{
		Count:                 count,
		Mean:                  s.Sum / float64(count),
		SumOfSquaredDeviation: dev,
		//	Exemplars:    buildExemplars(d.exemplars),
	}

	if len(s.CustomBounds) > 0 { // TODO(bwplotka): Use schema for this.
		// Classic histograms encoded in custom histograms or just custom histograms.
		d.BucketOptions = &distribution_pb.Distribution_BucketOptions{
			Options: &distribution_pb.Distribution_BucketOptions_ExplicitBuckets{
				ExplicitBuckets: &distribution_pb.Distribution_BucketOptions_Explicit{
					Bounds: s.CustomBounds,
				},
			},
		}
		if len(s.PositiveCounts) > 0 {
			d.BucketCounts = make([]int64, len(s.PositiveCounts))
			for i := range d.BucketCounts {
				d.BucketCounts[i] = int64(s.PositiveCounts[i]) // Bad, but no other way, should we error instead?
			}
		} else if len(s.PositiveDeltas) > 0 {
			d.BucketCounts = make([]int64, len(s.PositiveDeltas))
			prev := int64(0)
			for i := range d.BucketCounts {
				d.BucketCounts[i] = s.PositiveDeltas[i] - prev
			}
		}
		return d, nil
	}
	return d, errors.New("exponential histogram not implemented yet")

}

func initialGoogleTimeSeriesFromLabels(seriesLabelsRefs []uint32, symbols []string) (*monitoring_pb.TimeSeries, error) {
	metricName := ""
	resLabels := map[string]string{}
	metricLabels := map[string]string{}

	// Remote Write contains all labels in one sorted, interned array.
	// Validate if we have all labels required for the resource.
	// TODO(bwplotka): Check len(labelRefs) mod 2
	for i := 0; i < len(seriesLabelsRefs); i += 2 {
		lname := symbols[seriesLabelsRefs[i]] // TODO(bwplotka): Recover panics causes by this, or validate.
		lvalue := symbols[seriesLabelsRefs[i+1]]

		switch lname {
		case "__name__":
			if lvalue == "" {
				return nil, newHTTPError(errors.New("got metric name (__name__) label, but it has empty value"), http.StatusBadRequest)
			}
			metricName = lvalue

		case KeyProjectID, KeyLocation, KeyCluster, KeyNamespace, KeyJob, KeyInstance:
			resLabels[lname] = lvalue // TODO(bwplotka): What if lvalue is empty?
		default:
			metricLabels[lname] = lvalue
		}
	}

	if metricName == "" {
		return nil, errors.New("got empty metric name (__name__)")
	}

	// TODO(bwplotka): Do we always need all of them? We used to validate only ProjectID and Location.
	if len(resLabels) != 6 {
		return "", nil, nil, fmt.Errorf("GCM requires [%v] labels for prometheus_target monitored resource, got %v", []string{KeyProjectID, KeyLocation, KeyCluster, KeyNamespace, KeyJob, KeyInstance}, len(resLabels))
	}

	descriptor, kind, err := describeMetric(name, ts.GetMetadata().GetType())
	if err != nil {
		return fmt.Errorf("%v: %w", errorSeriesRef(name, res.Labels, labels), err)
	}

	// Transfer resource fields from label set onto the resource. If they are not set,
	// the respective field is set to an empty string. This explicitly is a valid value
	// in Cloud Monitoring and not the same as being unset.
	res := &monitoredres_pb.MonitoredResource{
		Type:   "prometheus_target",
		Labels: resLabels,
	}
	return &monitoring_pb.TimeSeries{
		Resource:   res,
		Metric:     &metric_pb.Metric{Type: descriptor, Labels: labels},
		MetricKind: kind,
	}

	return name, res, metricLabels, nil
}

// extractNameResourceAndLabels returns the metric name, monitored resource, the series labels, and whether the operation succeeded.
// This methods validates if expected resource labels are set, otherwise error is returned.
// All strings for labels share memory, assume immutability and read only use.

// describeMetric creates a GCM metric type from the Prometheus metric name and a type suffix.
// Optionally, a secondary type suffix may be provided for series for which a Prometheus type
// may be written as different GCM series.
// The general rule is that if the primary suffix is ambigious about whether the specific series
// is to be treated as a counter or gauge at query time, the secondarySuffix is set to "counter"
// for the counter variant, and left empty for the gauge variant.
func describeMetric(name string, typ writev2.Metadata_MetricType) (
	descriptor string,
	kind metric_pb.MetricDescriptor_MetricKind,
	_ error,
) {
	suffix := gcmMetricSuffixNone
	extraSuffix := gcmMetricSuffixNone

	switch typ {
	case writev2.Metadata_METRIC_TYPE_COUNTER:
		suffix = gcmMetricSuffixCounter
		kind = metric_pb.MetricDescriptor_CUMULATIVE
	case writev2.Metadata_METRIC_TYPE_GAUGE:
		suffix = gcmMetricSuffixGauge
		kind = metric_pb.MetricDescriptor_GAUGE
	case writev2.Metadata_METRIC_TYPE_HISTOGRAM:
		// We assume native (exponential or custom) histogram.
		suffix = gcmMetricSuffixHistogram
		kind = metric_pb.MetricDescriptor_CUMULATIVE
	case writev2.Metadata_METRIC_TYPE_GAUGEHISTOGRAM:
		// TODO(bwplotka): Test the new type.
		suffix = gcmMetricSuffixHistogram
		kind = metric_pb.MetricDescriptor_GAUGE
	case writev2.Metadata_METRIC_TYPE_SUMMARY:
		switch ms := getMetricSuffix(name); ms {
		case metricSuffixSum:
			suffix = gcmMetricSuffixSummary
			extraSuffix = gcmMetricSuffixCounter
			kind = metric_pb.MetricDescriptor_CUMULATIVE

		case metricSuffixCount:
			suffix = gcmMetricSuffixSummary
			extraSuffix = gcmMetricSuffixNone
			kind = metric_pb.MetricDescriptor_CUMULATIVE

		case metricSuffixNone: // Actual quantiles.
			suffix = gcmMetricSuffixSummary
			extraSuffix = gcmMetricSuffixNone
			kind = metric_pb.MetricDescriptor_GAUGE
		default:
			return "", kind, fmt.Errorf("unknown summary series suffix %v", ms)
		}
	case writev2.Metadata_METRIC_TYPE_INFO:
		// TODO(bwplotka): Test the new type.
		suffix = gcmMetricSuffixGauge
		kind = metric_pb.MetricDescriptor_GAUGE
	case writev2.Metadata_METRIC_TYPE_STATESET:
		// TODO(bwplotka): Test the new type.
		suffix = gcmMetricSuffixGauge
		kind = metric_pb.MetricDescriptor_GAUGE
	case writev2.Metadata_METRIC_TYPE_UNSPECIFIED:
		fallthrough
	default:
		return "", kind, fmt.Errorf("unknown metric type %v", typ)
	}
	if extraSuffix == gcmMetricSuffixNone {
		return fmt.Sprintf("%s/%s/%s", metricTypePrefix, name, suffix), kind, nil
	}
	return fmt.Sprintf("%s/%s/%s:%s", metricTypePrefix, name, suffix, extraSuffix), kind, nil
}

func getMetricSuffix(name string) metricSuffix {
	if strings.HasSuffix(name, string(metricSuffixTotal)) {
		return metricSuffixTotal
	}
	if strings.HasSuffix(name, string(metricSuffixBucket)) {
		return metricSuffixBucket
	}
	if strings.HasSuffix(name, string(metricSuffixCount)) {
		return metricSuffixCount
	}
	if strings.HasSuffix(name, string(metricSuffixSum)) {
		return metricSuffixSum
	}
	return metricSuffixNone
}

// Metric name suffixes used by various Prometheus metric types.
type metricSuffix string

const (
	metricSuffixNone   metricSuffix = ""
	metricSuffixTotal  metricSuffix = "_total"
	metricSuffixBucket metricSuffix = "_bucket"
	metricSuffixSum    metricSuffix = "_sum"
	metricSuffixCount  metricSuffix = "_count"
)

// Suffixes appended to GCM metric types. They are equivalent to the respective
// Prometheus types but we redfine them here to ensure they don't unexpectedly change
// by updating a Prometheus library.
type gcmMetricSuffix string

const (
	gcmMetricSuffixNone      gcmMetricSuffix = ""
	gcmMetricSuffixUnknown   gcmMetricSuffix = "unknown"
	gcmMetricSuffixGauge     gcmMetricSuffix = "gauge"
	gcmMetricSuffixCounter   gcmMetricSuffix = "counter"
	gcmMetricSuffixHistogram gcmMetricSuffix = "histogram"
	gcmMetricSuffixSummary   gcmMetricSuffix = "summary"
)

func errorSeriesRef(name string, resLabels map[string]string, metricLabels map[string]string) string {
	b := strings.Builder{}
	b.WriteString(name)
	b.WriteString("{")
	i := 0
	for n, v := range resLabels {
		b.WriteString(fmt.Sprintf("%q=%s", n, v))
		if i < len(resLabels) {
			b.WriteString(", ")
			i++
		}
	}
	i = 0
	for n, v := range metricLabels {
		b.WriteString(fmt.Sprintf("%q=%s", n, v))
		if i < len(metricLabels) {
			b.WriteString(", ")
			i++
		}
	}
	b.WriteString("}")
	return b.String()
}
