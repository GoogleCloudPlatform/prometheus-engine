package internal

import (
	"fmt"
	"math"
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export/internal/histogram"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// pickSchema returns the largest number n between -4 and 8 such that
// 2^(2^-n) is less or equal the provided bucketFactor.
//
// Special cases:
//   - bucketFactor <= 1: panics.
//   - bucketFactor < 2^(2^-8) (but > 1): still returns 8.
//
// copied from histogram.go
func pickSchema(bucketFactor float64) int32 {
	if bucketFactor <= 1 {
		panic(fmt.Errorf("bucketFactor %f is <=1", bucketFactor))
	}
	floor := math.Floor(math.Log2(math.Log2(bucketFactor)))
	switch {
	case floor <= -8:
		return 8
	case floor >= 4:
		return -4
	default:
		return -int32(floor)
	}
}

func dtoToIntHistogram(h *dto.Histogram) *histogram.Histogram {
	if len(h.PositiveCount) > 0 {
		panic("float histograms")
	}

	ret := &histogram.Histogram{
		Count:           h.GetSampleCount(),
		Sum:             h.GetSampleSum(),
		ZeroThreshold:   h.GetZeroThreshold(),
		ZeroCount:       h.GetZeroCount(),
		Schema:          h.GetSchema(),
		PositiveSpans:   make([]histogram.Span, len(h.GetPositiveSpan())),
		PositiveBuckets: h.GetPositiveDelta(),
		NegativeSpans:   make([]histogram.Span, len(h.GetNegativeSpan())),
		NegativeBuckets: h.GetNegativeDelta(),
	}
	for i, span := range h.GetPositiveSpan() {
		ret.PositiveSpans[i].Offset = span.GetOffset()
		ret.PositiveSpans[i].Length = span.GetLength()
	}
	for i, span := range h.GetNegativeSpan() {
		ret.NegativeSpans[i].Offset = span.GetOffset()
		ret.NegativeSpans[i].Length = span.GetLength()
	}
	ret.Compact(0)
	return ret
}

func TestNativeHistogramVersusGCM(t *testing.T) {
	// Native Prometheus.
	const factor = 1.1
	h := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "yolo",
		// NativeHistogramMaxBucketNumber is infinite by default.
		NativeHistogramBucketFactor:  factor,
		NativeHistogramZeroThreshold: prometheus.DefNativeHistogramZeroThreshold,
	})

	// For native histograms, schema defines how many buckets we should see
	// per power of 2 (e.g. from 1 to 2 and from 2 to 4, etc). In each power of
	// two we should see 2^n buckets.
	schema := pickSchema(factor)

	// schema 3 factor 1.1 number of buckets per 2^i 8
	fmt.Println("schema", schema, "factor", factor, "number of buckets per 2^i", math.Pow(2, float64(schema)))

	// Try to fill all buckets.
	for i := 0; i < 3; i++ {
		h.Observe(1.01 * (math.Pow(2, float64(i))))
		h.Observe(1.1 * (math.Pow(2, float64(i))))
		h.Observe(1.2 * (math.Pow(2, float64(i))))
		h.Observe(1.3 * (math.Pow(2, float64(i))))
		h.Observe(1.5 * (math.Pow(2, float64(i))))
		h.Observe(1.6 * (math.Pow(2, float64(i))))
		h.Observe(1.8 * (math.Pow(2, float64(i))))
		h.Observe(1.9 * (math.Pow(2, float64(i))))
	}
	h.Observe(77)

	m := dto.Metric{}
	if err := h.Write(&m); err != nil {
		t.Fatal(err)
	}

	intHist := dtoToIntHistogram(m.GetHistogram())
	iter := intHist.PositiveBucketIterator()
	for iter.Next() {
		b := iter.At()
		fmt.Printf("(%v,%v]:%v\n", b.Lower, b.Upper, b.Count)
	}

	fmt.Println("GCM----")
	// GCM distributions (https://cloud.google.com/monitoring/api/ref_v3/rest/v3/TypedValue#Exponential).
	// Assuming scale 1.0.
	numFiniteBuckets := intHist.Count - 1 // This only makes sense for non-sparse native histograms, obviously.
	// growthFactor is literally a real Prometheus Native histogram factor, based on 2^(2^-schema).
	var growthFactor = math.Pow(2, math.Pow(2, -1*float64(schema)))
	for i := 0; i < int(numFiniteBuckets); i++ {
		lower := math.Pow(growthFactor, float64(i))
		upper := math.Pow(growthFactor, float64(i+1))
		fmt.Printf("(%v,%v]:%v\n", lower, upper, 1)
	}

	lower := math.Pow(growthFactor, float64(50))
	upper := math.Pow(growthFactor, float64(51))
	fmt.Printf("(%v,%v]:%v\n", lower, upper, 1)

	// Output. Having some slight imprecision, but within error margin.
	//=== RUN   TestNativeHistogramVersusGCM
	//schema 3 factor 1.1 number of buckets per 2^i 8
	//(1,1.0905077326652577]:1
	//(1.0905077326652577,1.189207115002721]:1
	//(1.189207115002721,1.2968395546510096]:1
	//(1.2968395546510096,1.414213562373095]:1
	//(1.414213562373095,1.5422108254079407]:1
	//(1.5422108254079407,1.6817928305074288]:1
	//(1.6817928305074288,1.8340080864093422]:1
	//(1.8340080864093422,2]:1
	//(2,2.1810154653305154]:1
	//(2.1810154653305154,2.378414230005442]:1
	//(2.378414230005442,2.5936791093020193]:1
	//(2.5936791093020193,2.82842712474619]:1
	//(2.82842712474619,3.0844216508158815]:1
	//(3.0844216508158815,3.3635856610148576]:1
	//(3.3635856610148576,3.6680161728186844]:1
	//(3.6680161728186844,4]:1
	//(4,4.362030930661031]:1
	//(4.362030930661031,4.756828460010884]:1
	//(4.756828460010884,5.187358218604039]:1
	//(5.187358218604039,5.65685424949238]:1
	//(5.65685424949238,6.168843301631763]:1
	//(6.168843301631763,6.727171322029715]:1
	//(6.727171322029715,7.336032345637369]:1
	//(7.336032345637369,8]:1
	//(76.10925536017415,82.99773149766462]:1
	//GCM----
	//(1,1.0905077326652577]:1
	//(1.0905077326652577,1.189207115002721]:1
	//(1.189207115002721,1.2968395546510096]:1
	//(1.2968395546510096,1.414213562373095]:1
	//(1.414213562373095,1.5422108254079407]:1
	//(1.5422108254079407,1.6817928305074288]:1
	//(1.6817928305074288,1.8340080864093422]:1
	//(1.8340080864093422,1.9999999999999996]:1
	//(1.9999999999999996,2.181015465330515]:1
	//(2.181015465330515,2.3784142300054416]:1
	//(2.3784142300054416,2.593679109302019]:1
	//(2.593679109302019,2.8284271247461894]:1
	//(2.8284271247461894,3.0844216508158806]:1
	//(3.0844216508158806,3.3635856610148567]:1
	//(3.3635856610148567,3.6680161728186835]:1
	//(3.6680161728186835,3.9999999999999982]:1
	//(3.9999999999999982,4.362030930661029]:1
	//(4.362030930661029,4.756828460010882]:1
	//(4.756828460010882,5.187358218604036]:1
	//(5.187358218604036,5.656854249492377]:1
	//(5.656854249492377,6.16884330163176]:1
	//(6.16884330163176,6.7271713220297125]:1
	//(6.7271713220297125,7.336032345637365]:1
	//(7.336032345637365,7.999999999999995]:1
	//(76.10925536017405,82.9977314976645]:1
	//--- PASS: TestNativeHistogramVersusGCM (0.00s)
	//PASS
	//
	//Process finished with the exit code 0
}
